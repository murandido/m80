package issuance

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Create writes to four tables in one transaction, so there is nothing
// meaningful left to assert once the database is mocked away. These tests need
// a live Postgres - the same skip-if-unreachable convention the handler tests
// use (internal/api/v1/handlers/issuances_cancel_test.go) keeps
// `go test ./...` green on a machine with no database up.

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error

	phoneCounter int64
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	testDBOnce.Do(func() {
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			os.Getenv("DB_PORT"),
		)

		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			testDBErr = err
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			testDBErr = err
			return
		}
		if err := sqlDB.Ping(); err != nil {
			testDBErr = err
			return
		}

		data.Connect()
		testDB = data.DB
	})

	if testDBErr != nil {
		t.Skipf("skipping issuance service tests: no test database available: %v", testDBErr)
	}
	return testDB
}

// runInTx runs the test body inside a transaction that is always rolled back.
// Create opens its own transaction, which GORM maps to a SAVEPOINT under this
// one - the very nesting the bot relies on.
//
// It also marks the test parallel. These tests spend almost all of their time waiting on
// Postgres, and the rollback is what makes that safe: under READ COMMITTED a transaction
// nobody ever commits is invisible to every other one, so concurrent tests cannot see each
// other's rows. Seeds that touch a unique column already carry a per-call random suffix.
//
// The consequence to know about: a test that calls this may not also call t.Setenv, which
// the runtime rejects after t.Parallel. Process-wide environment belongs in TestMain.
func runInTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)
	t.Parallel()

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("failed to begin test transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })

	fn(tx)
}

// fixture is the minimal FK chain a valid issuance needs.
type fixture struct {
	Airline data.Airline
	Program data.LoyaltyProgram
	Account data.Account
	Agency  data.Agency
	User    data.User
	Suffix  string
}

func seedFixture(t *testing.T, tx *gorm.DB, airline data.Airline) fixture {
	t.Helper()
	suffix := uuid.New().String()[:8]

	if airline.Name == "" {
		airline.Name = "Test Air"
	}
	airline.Code = "T" + suffix
	create(t, tx, &airline)

	program := data.LoyaltyProgram{Name: "Test Program", Code: "L" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)

	account := data.Account{LoyaltyProgramID: program.ID}
	create(t, tx, &account)

	// is_default is left false on purpose: a shared database may already have a
	// default agency, and two would make resolveAgencyID's pick arbitrary.
	agency := data.Agency{Name: "Test Agency " + suffix}
	create(t, tx, &agency)

	phone := fmt.Sprintf("1199%07d", atomic.AddInt64(&phoneCounter, 1)%10000000)
	user := data.User{Name: "Tester", Phone: phone, Password: "irrelevant"}
	create(t, tx, &user)

	return fixture{Airline: airline, Program: program, Account: account, Agency: agency, User: user, Suffix: suffix}
}

func create(t *testing.T, tx *gorm.DB, model any) {
	t.Helper()
	if err := tx.Create(model).Error; err != nil {
		t.Fatalf("failed to seed %T: %v", model, err)
	}
}

func (f fixture) input() CreateInput {
	return CreateInput{
		LoyaltyProgramID:   f.Program.ID,
		ProgramLocator:     "PL" + f.Suffix,
		Points:             31000,
		ValueOfTheThousand: 1650,
		AccountID:          f.Account.ID,
		AgencyID:           &f.Agency.ID,
	}
}

func TestCreate_PersistsIssuanceSaleAndAuditLog(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})

		created, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if created.ID == "" {
			t.Fatal("expected the created issuance to carry an id")
		}
		// Defaulted from the loyalty program, since the input named no airline.
		if created.AirlineID != f.Airline.ID {
			t.Fatalf("airline_id = %q, want the program default %q", created.AirlineID, f.Airline.ID)
		}
		if created.Complete {
			t.Fatal("expected the issuance to be born incomplete")
		}

		assertCount(t, tx, "sales", "issuance_id = ?", created.ID, 1)
		assertCount(t, tx, "audit_logs", "issuance_id = ? AND action = 'created'", created.ID, 1)
		assertCount(t, tx, "issuance_flights", "issuance_id = ?", created.ID, 0)
		assertCount(t, tx, "issuance_passengers", "issuance_id = ?", created.ID, 0)
	})
}

// The autosave regression this whole completion-boundary design exists to
// prevent: the grid POSTs a draft long before a passenger name is typed, so an
// incomplete issuance must be creatable even for an airline that requires one.
func TestCreate_IncompleteSkipsAirlineRequirements(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{RequiresLastName: true, RequiresPurchaseNumber: true})

		if _, err := Create(tx, f.input(), f.User.ID); err != nil {
			t.Fatalf("expected an incomplete issuance to be created despite the airline flags, got %v", err)
		}
	})
}

func TestCreate_CompleteEnforcesAirlineRequirements(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{RequiresLastName: true})

		in := f.input()
		in.Complete = true

		_, err := Create(tx, in, f.User.ID)
		if !errors.Is(err, ErrLastNameRequired) {
			t.Fatalf("err = %v, want ErrLastNameRequired", err)
		}

		lastName := "MACHADO"
		in.MainPassengerName = &lastName
		if _, err := Create(tx, in, f.User.ID); err != nil {
			t.Fatalf("expected the same issuance to be accepted once the last name is present, got %v", err)
		}
	})
}

func TestCreate_CompleteEnforcesPurchaseNumber(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{RequiresPurchaseNumber: true})

		in := f.input()
		in.Complete = true

		_, err := Create(tx, in, f.User.ID)
		if !errors.Is(err, ErrPurchaseNumberRequired) {
			t.Fatalf("err = %v, want ErrPurchaseNumberRequired", err)
		}

		purchase := "LA9574243YWPJ"
		in.LatamPurchaseNumber = &purchase
		if _, err := Create(tx, in, f.User.ID); err != nil {
			t.Fatalf("expected the same issuance to be accepted once the purchase number is present, got %v", err)
		}
	})
}

func TestCreate_UnknownAirportCodeIsNamed(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})

		in := f.input()
		unknown := "ZZZ"
		in.OriginIATACode = &unknown

		_, err := Create(tx, in, f.User.ID)
		if !errors.Is(err, ErrAirportNotFound) {
			t.Fatalf("err = %v, want ErrAirportNotFound", err)
		}
		// The offending code must survive into the message - it is the only
		// actionable part of the error for whoever mistyped it.
		if got := err.Error(); got != "airport not found: ZZZ" {
			t.Fatalf("err.Error() = %q, want it to name the code", got)
		}
	})
}

func TestCreate_DuplicateProgramLocatorIsTyped(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})

		if _, err := Create(tx, f.input(), f.User.ID); err != nil {
			t.Fatalf("first Create failed: %v", err)
		}

		_, err := Create(tx, f.input(), f.User.ID)
		if !errors.Is(err, ErrProgramLocatorTaken) {
			t.Fatalf("err = %v, want ErrProgramLocatorTaken", err)
		}
	})
}

func TestCreate_RollsBackEverythingOnFailure(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})

		in := f.input()
		unknown := "ZZZ"
		in.DestinationIATACode = &unknown

		if _, err := Create(tx, in, f.User.ID); err == nil {
			t.Fatal("expected Create to fail on an unknown destination")
		}

		// Nothing may survive a failed create - not even the sale, which is
		// written before the failing validation would be reached on a naive
		// ordering.
		assertCount(t, tx, "issuances", "program_locator = ?", in.ProgramLocator, 0)
	})
}

func assertCount(t *testing.T, tx *gorm.DB, table, where string, arg any, want int64) {
	t.Helper()

	var got int64
	if err := tx.Table(table).Where(where, arg).Count(&got).Error; err != nil {
		t.Fatalf("failed to count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
