package legacyimport

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Resolving and committing are both mostly database work - unaccent() in particular is a
// Postgres extension with no Go equivalent - so these tests need the live test database CI
// provisions. Without one the file skips rather than fails, the same arrangement the
// handler tests use, so `go test ./...` stays green on a machine with nothing running.

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
		t.Skipf("skipping legacy import tests: no test database available: %v", testDBErr)
	}
	return testDB
}

// runInTx runs the body inside a transaction that is always rolled back, so seeded rows
// never leak between tests. Commit opens its own transaction, which GORM turns into a
// SAVEPOINT under this one.
func runInTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("beginning the test transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })

	fn(tx)
}

// world is the cast a test needs before an issuance can exist: an airline, a loyalty
// program pointing at it, an agency, and a user to be the issuer.
type world struct {
	suffix  string
	airline data.Airline
	program data.LoyaltyProgram
	agency  data.Agency
	user    data.User
}

// seedWorld creates that cast. Unique-constrained columns carry a per-call suffix, so
// tests running against the same database never collide.
func seedWorld(t *testing.T, tx *gorm.DB, programCode string) world {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	mustCreate(t, tx, &airline)

	program := data.LoyaltyProgram{
		Name:             programCode + " " + suffix,
		Code:             programCode + suffix,
		DefaultAirlineID: airline.ID,
	}
	mustCreate(t, tx, &program)

	agency := data.Agency{Name: "Agência " + suffix}
	mustCreate(t, tx, &agency)

	user := data.User{Name: "Marcus " + suffix, Phone: nextPhone(), Password: "secret123"}
	mustCreate(t, tx, &user)

	return world{suffix: suffix, airline: airline, program: program, agency: agency, user: user}
}

// seedAccount creates an account with a holder, and optionally the alias the spreadsheet
// would name it by.
func seedAccount(t *testing.T, tx *gorm.DB, w world, holderName, nickname string) data.Account {
	t.Helper()

	holder := data.Person{Name: holderName}
	mustCreate(t, tx, &holder)

	account := data.Account{LoyaltyProgramID: w.program.ID, HolderID: &holder.ID}
	mustCreate(t, tx, &account)

	if nickname != "" {
		alias := data.AccountNickname{
			AccountID:        account.ID,
			LoyaltyProgramID: w.program.ID,
			Nickname:         nickname,
		}
		mustCreate(t, tx, &alias)
	}

	return account
}

func mustCreate(t *testing.T, tx *gorm.DB, value any) {
	t.Helper()
	if err := tx.Create(value).Error; err != nil {
		t.Fatalf("seeding %T: %v", value, err)
	}
}

var phoneCounter int

// nextPhone hands out distinct, valid Brazilian mobile numbers - users.phone is unique.
func nextPhone() string {
	phoneCounter++
	return fmt.Sprintf("119%08d", phoneCounter)
}
