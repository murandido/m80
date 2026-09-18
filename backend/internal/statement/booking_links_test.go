package statement

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

// ResolveBookingLinks and the manual-link store are mostly database work - match an issuance
// by locator, save an override, read it back - so these need the live test database. Without
// one the file skips rather than fails, the same arrangement the rest of the suite uses, so
// `go test ./...` stays green on a bare machine. (The `day` helper lives in reconcile_test.go.)

var (
	testDBOnce   sync.Once
	testDB       *gorm.DB
	testDBErr    error
	phoneCounter int64
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	testDBOnce.Do(func() {
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"), os.Getenv("DB_PORT"),
		)
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
		t.Skipf("skipping statement database tests: no test database available: %v", testDBErr)
	}
	return testDB
}

// runInTx runs the body in a transaction that is always rolled back, so seeded rows never
// leak between tests. It marks the test parallel, which the rollback makes safe under READ
// COMMITTED. A test that calls this may not also call t.Setenv.
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

// linkFixture is the cast an issuance needs before it can exist, plus the account under test.
type linkFixture struct {
	account data.Account
	agency  data.Agency
	program data.LoyaltyProgram
	airline data.Airline
	user    data.User
}

func seedLinkFixture(t *testing.T, tx *gorm.DB) linkFixture {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	mustCreate(t, tx, &airline)

	program := data.LoyaltyProgram{Name: "Programa " + suffix, Code: "PRG" + suffix, DefaultAirlineID: airline.ID}
	mustCreate(t, tx, &program)

	agency := data.Agency{Name: "Agência " + suffix}
	mustCreate(t, tx, &agency)

	user := data.User{Name: "Operador " + suffix, Phone: nextPhone(), Password: "secret123"}
	mustCreate(t, tx, &user)

	holder := data.Person{Name: "Titular " + suffix}
	mustCreate(t, tx, &holder)

	account := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID, OwnsPointInventory: true}
	mustCreate(t, tx, &account)

	return linkFixture{account: account, agency: agency, program: program, airline: airline, user: user}
}

// seedLinkIssuance writes a booking with the given locator, dated on the given day. created_at
// is set explicitly so the date is deliberately far from any statement day - the match must
// not depend on it.
func seedLinkIssuance(t *testing.T, tx *gorm.DB, f linkFixture, locator, issuedOn string, points int) data.Issuance {
	t.Helper()

	issuance := data.Issuance{
		LoyaltyProgramID:   f.program.ID,
		AirlineID:          f.airline.ID,
		ProgramLocator:     locator,
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

// nextPhone hands out distinct, valid Brazilian mobile numbers - users.phone is unique.
// Atomic because runInTx marks its tests parallel.
func nextPhone() string {
	return fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
}

func TestResolveBookingLinks_AutoMatchesByLocatorIgnoringDate(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		// The issuance is dated months before the statement day it should still match.
		issuance := seedLinkIssuance(t, tx, f, "ABC123", "2026-01-15", 25000)

		resolved, err := ResolveBookingLinks(tx, f.account.ID, []string{"ABC123"})
		if err != nil {
			t.Fatalf("ResolveBookingLinks: %v", err)
		}
		booking, ok := resolved["ABC123"]
		if !ok {
			t.Fatal("locator ABC123 was not resolved")
		}
		if booking.Issuance.ID != issuance.ID {
			t.Errorf("resolved issuance = %q, want %q", booking.Issuance.ID, issuance.ID)
		}
		if booking.Manual {
			t.Error("auto match reported Manual = true")
		}
	})
}

func TestResolveBookingLinks_UnmatchedLocatorIsAbsent(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		seedLinkIssuance(t, tx, f, "HAVE", "2026-03-01", 10000)

		resolved, err := ResolveBookingLinks(tx, f.account.ID, []string{"MISSING"})
		if err != nil {
			t.Fatalf("ResolveBookingLinks: %v", err)
		}
		if _, ok := resolved["MISSING"]; ok {
			t.Error("an unmatched locator was resolved; it should be absent")
		}
	})
}

func TestResolveBookingLinks_MatchIsScopedToTheAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		mine := seedLinkFixture(t, tx)
		other := seedLinkFixture(t, tx)
		// The same locator on another account must not leak into this account's resolution.
		seedLinkIssuance(t, tx, other, "SHARED", "2026-02-01", 5000)

		resolved, err := ResolveBookingLinks(tx, mine.account.ID, []string{"SHARED"})
		if err != nil {
			t.Fatalf("ResolveBookingLinks: %v", err)
		}
		if _, ok := resolved["SHARED"]; ok {
			t.Error("resolved a locator that belongs to another account")
		}
	})
}

func TestResolveBookingLinks_ManualPinWinsOverAutoMatch(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		// The auto match for locator "PIN" would be autoHit; the operator pins it to another.
		autoHit := seedLinkIssuance(t, tx, f, "PIN", "2026-04-01", 8000)
		pinned := seedLinkIssuance(t, tx, f, "OTHERLOC", "2026-05-01", 9000)

		if err := UpsertBookingLink(tx, f.account.ID, "PIN", pinned.ID, &f.user.ID); err != nil {
			t.Fatalf("UpsertBookingLink: %v", err)
		}

		resolved, err := ResolveBookingLinks(tx, f.account.ID, []string{"PIN"})
		if err != nil {
			t.Fatalf("ResolveBookingLinks: %v", err)
		}
		booking := resolved["PIN"]
		if booking.Issuance.ID != pinned.ID {
			t.Errorf("resolved issuance = %q, want the pinned %q (auto would be %q)",
				booking.Issuance.ID, pinned.ID, autoHit.ID)
		}
		if !booking.Manual {
			t.Error("a pinned link reported Manual = false")
		}
	})
}

func TestResolveBookingLinks_UpsertReplacesAndDeleteFallsBackToAuto(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		auto := seedLinkIssuance(t, tx, f, "LOC", "2026-06-01", 3000)
		first := seedLinkIssuance(t, tx, f, "L1", "2026-06-02", 3100)
		second := seedLinkIssuance(t, tx, f, "L2", "2026-06-03", 3200)

		if err := UpsertBookingLink(tx, f.account.ID, "LOC", first.ID, &f.user.ID); err != nil {
			t.Fatalf("first upsert: %v", err)
		}
		// Re-pinning the same locator replaces the target rather than erroring on the PK.
		if err := UpsertBookingLink(tx, f.account.ID, "LOC", second.ID, &f.user.ID); err != nil {
			t.Fatalf("replacing upsert: %v", err)
		}
		resolved, err := ResolveBookingLinks(tx, f.account.ID, []string{"LOC"})
		if err != nil {
			t.Fatalf("ResolveBookingLinks after replace: %v", err)
		}
		if got := resolved["LOC"]; got.Issuance.ID != second.ID || !got.Manual {
			t.Errorf("after replace = %q manual=%v, want %q manual=true", got.Issuance.ID, got.Manual, second.ID)
		}

		// Removing the pin hands the locator back to the auto match.
		if err := DeleteBookingLink(tx, f.account.ID, "LOC"); err != nil {
			t.Fatalf("DeleteBookingLink: %v", err)
		}
		resolved, err = ResolveBookingLinks(tx, f.account.ID, []string{"LOC"})
		if err != nil {
			t.Fatalf("ResolveBookingLinks after delete: %v", err)
		}
		if got := resolved["LOC"]; got.Issuance.ID != auto.ID || got.Manual {
			t.Errorf("after delete = %q manual=%v, want the auto %q manual=false", got.Issuance.ID, got.Manual, auto.ID)
		}
	})
}

func TestResolveBookingLinks_SkipsBlankAndDuplicateLocators(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		issuance := seedLinkIssuance(t, tx, f, "DUP", "2026-07-01", 4000)

		resolved, err := ResolveBookingLinks(tx, f.account.ID, []string{"", "DUP", "DUP", ""})
		if err != nil {
			t.Fatalf("ResolveBookingLinks: %v", err)
		}
		if len(resolved) != 1 {
			t.Fatalf("resolved %d locators, want 1 (blanks/dupes dropped)", len(resolved))
		}
		if resolved["DUP"].Issuance.ID != issuance.ID {
			t.Errorf("resolved issuance = %q, want %q", resolved["DUP"].Issuance.ID, issuance.ID)
		}
	})
}

func TestResolveBookingLinks_PinToSoftDeletedIssuanceFallsBackToAuto(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		auto := seedLinkIssuance(t, tx, f, "FALL", "2026-08-01", 6000)
		gone := seedLinkIssuance(t, tx, f, "GONELOC", "2026-08-02", 6100)

		if err := UpsertBookingLink(tx, f.account.ID, "FALL", gone.ID, &f.user.ID); err != nil {
			t.Fatalf("UpsertBookingLink: %v", err)
		}
		// Soft-delete the pinned issuance: the pin still exists but points at nothing the
		// default scope returns, so the resolver must fall through to the auto match.
		if err := tx.Delete(&data.Issuance{}, "id = ?", gone.ID).Error; err != nil {
			t.Fatalf("soft-deleting the pinned issuance: %v", err)
		}

		resolved, err := ResolveBookingLinks(tx, f.account.ID, []string{"FALL"})
		if err != nil {
			t.Fatalf("ResolveBookingLinks: %v", err)
		}
		if got := resolved["FALL"]; got.Issuance.ID != auto.ID || got.Manual {
			t.Errorf("after pinned issuance gone = %q manual=%v, want auto %q manual=false", got.Issuance.ID, got.Manual, auto.ID)
		}
	})
}
