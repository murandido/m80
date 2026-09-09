package inventory

import (
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

// Recompute is the half of the costing that is mostly database work - which batch is
// frozen, which slices belong to an open period - so these tests need the live test
// database. Without one the file skips rather than fails, the same arrangement the handler
// and legacyimport tests use, so `go test ./...` stays green on a bare machine.
//
// The pure costing in allocate_test.go and reverse_test.go needs none of this, which is
// why it is separate: the figures that have to agree with the spreadsheet are checked
// with nothing running.

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error
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

		// Schema is owned by golang-migrate; data.Connect runs it and sets data.DB.
		data.Connect()
		testDB = data.DB
	})

	if testDBErr != nil {
		t.Skipf("skipping inventory database tests: no test database available: %v", testDBErr)
	}
	return testDB
}

// runInTx runs the body in a transaction that is always rolled back, so seeded rows never
// leak between tests. It marks the test parallel, which the rollback makes safe: under
// READ COMMITTED an uncommitted transaction is invisible to every other one.
//
// A test that calls this may not also call t.Setenv - the runtime rejects it after
// t.Parallel.
func runInTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)
	t.Parallel()

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("beginning the test transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })

	fn(tx)
}

// fixture is the cast an issuance needs before it can exist, plus the account under test.
type fixture struct {
	account data.Account
	agency  data.Agency
	origin  data.PointBatchOrigin
	program data.LoyaltyProgram
	airline data.Airline
	user    data.User
}

// seedFixture creates that cast. Unique-constrained columns carry a per-call suffix, so
// parallel tests against the same database never collide.
func seedFixture(t *testing.T, tx *gorm.DB) fixture {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	mustCreate(t, tx, &airline)

	program := data.LoyaltyProgram{
		Name:             "Programa " + suffix,
		Code:             "PRG" + suffix,
		DefaultAirlineID: airline.ID,
	}
	mustCreate(t, tx, &program)

	agency := data.Agency{Name: "Agência " + suffix}
	mustCreate(t, tx, &agency)

	user := data.User{Name: "Operador " + suffix, Phone: nextPhone(), Password: "secret123"}
	mustCreate(t, tx, &user)

	holder := data.Person{Name: "Titular " + suffix}
	mustCreate(t, tx, &holder)

	account := data.Account{
		LoyaltyProgramID:   program.ID,
		HolderID:           &holder.ID,
		OwnsPointInventory: true,
	}
	mustCreate(t, tx, &account)

	origin := data.PointBatchOrigin{Name: "Livelo " + suffix, Active: true}
	mustCreate(t, tx, &origin)

	return fixture{account: account, agency: agency, origin: origin, program: program, airline: airline, user: user}
}

// seedAgency creates a second agency, for the settlement cases.
func seedAgency(t *testing.T, tx *gorm.DB) data.Agency {
	t.Helper()

	agency := data.Agency{Name: "Agência " + uuid.New().String()[:8]}
	mustCreate(t, tx, &agency)
	return agency
}

// seedBatch records one acquisition on the fixture's account.
func seedBatch(t *testing.T, tx *gorm.DB, f fixture, acquiredOn string, points, costCents int) data.PointBatch {
	t.Helper()

	batch := data.PointBatch{
		AccountID:       f.account.ID,
		OriginID:        &f.origin.ID,
		AcquiredOn:      day(acquiredOn),
		Points:          points,
		CostCents:       costCents,
		FundingAgencyID: f.agency.ID,
	}
	mustCreate(t, tx, &batch)
	return batch
}

// seedIssuance writes a booking dated on the given day. created_at is set explicitly
// because it is what orders the queue, and a row created "now" would always sort last.
func seedIssuance(t *testing.T, tx *gorm.DB, f fixture, issuedOn string, points int) data.Issuance {
	t.Helper()
	suffix := uuid.New().String()[:6]

	issuance := data.Issuance{
		LoyaltyProgramID:   f.program.ID,
		AirlineID:          f.airline.ID,
		ProgramLocator:     suffix,
		Points:             points,
		ValueOfTheThousand: 1500,
		AccountID:          f.account.ID,
		IssuerID:           f.user.ID,
		AgencyID:           f.agency.ID,
		CreatedAt:          day(issuedOn),
	}
	mustCreate(t, tx, &issuance)
	return issuance
}

func mustCreate(t *testing.T, tx *gorm.DB, value any) {
	t.Helper()
	if err := tx.Create(value).Error; err != nil {
		t.Fatalf("seeding %T: %v", value, err)
	}
}

// allocationsOf reads back what Recompute wrote for one account, in a stable order.
func allocationsOf(t *testing.T, tx *gorm.DB, accountID string) []data.PointBatchAllocation {
	t.Helper()

	var rows []data.PointBatchAllocation
	err := tx.Joins("JOIN issuances ON issuances.id = point_batch_allocations.issuance_id").
		Where("issuances.account_id = ?", accountID).
		Order("point_batch_allocations.occurred_on ASC").
		Order("point_batch_allocations.created_at ASC").
		Find(&rows).Error
	if err != nil {
		t.Fatalf("reading allocations: %v", err)
	}
	return rows
}

func sumPoints(rows []data.PointBatchAllocation) int {
	total := 0
	for _, row := range rows {
		total += row.Points
	}
	return total
}

func sumCost(rows []data.PointBatchAllocation) int {
	total := 0
	for _, row := range rows {
		total += row.CostCents
	}
	return total
}

var phoneCounter int64

// nextPhone hands out distinct, valid Brazilian mobile numbers - users.phone is unique.
// Atomic because runInTx marks its tests parallel: a plain increment would hand the same
// number to two goroutines and the second insert would die on the unique index rather
// than on anything the test is about.
func nextPhone() string {
	return fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
}
