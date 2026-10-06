package latamsync

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/latam"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDBOnce.Do(func() {
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"), os.Getenv("DB_PORT"))
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
		data.Connect() // owns the schema via golang-migrate; sets data.DB
		testDB = data.DB
	})
	if testDBErr != nil {
		t.Skipf("skipping latamsync database tests: no test database available: %v", testDBErr)
	}
	return testDB
}

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

// seedAccount creates the chain an account needs and returns it with a plaintext refresh token,
// the way SyncAccount receives it (AfterFind decrypts the stored one).
func seedAccount(t *testing.T, tx *gorm.DB, refreshToken string) data.Account {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	mustCreate(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Programa " + suffix, Code: "P" + suffix, DefaultAirlineID: airline.ID}
	mustCreate(t, tx, &program)
	holder := data.Person{Name: "Titular " + suffix}
	mustCreate(t, tx, &holder)

	account := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID}
	if refreshToken != "" {
		account.LatamRefreshToken = &refreshToken
	}
	mustCreate(t, tx, &account)
	if refreshToken == "" {
		account.LatamRefreshToken = nil
	} else {
		account.LatamRefreshToken = &refreshToken
	}
	return account
}

func mustCreate(t *testing.T, tx *gorm.DB, value any) {
	t.Helper()
	if err := tx.Create(value).Error; err != nil {
		t.Fatalf("seeding %T: %v", value, err)
	}
}

func stateOf(t *testing.T, tx *gorm.DB, accountID string) data.LatamSyncState {
	t.Helper()
	var state data.LatamSyncState
	if err := tx.First(&state, "account_id = ?", accountID).Error; err != nil {
		t.Fatalf("reading sync state: %v", err)
	}
	return state
}

func reload(t *testing.T, tx *gorm.DB, accountID string) data.Account {
	t.Helper()
	var account data.Account
	if err := tx.First(&account, "id = ?", accountID).Error; err != nil {
		t.Fatalf("reloading account: %v", err)
	}
	return account
}

// fakeReader answers with configured values and records what it was asked.
type fakeReader struct {
	mu sync.Mutex

	refreshErr   error
	rotatedTo    string
	miles        int
	balanceErr   error
	refreshCalls int
	balanceCalls int
}

func (f *fakeReader) Refresh(_ context.Context, refreshToken string) (latam.AccountSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshCalls++
	if f.refreshErr != nil {
		return latam.AccountSession{}, f.refreshErr
	}
	refresh := refreshToken
	if f.rotatedTo != "" {
		refresh = f.rotatedTo
	}
	return latam.AccountSession{AccessToken: "access", IDToken: "id", RefreshToken: refresh, UserID: "user"}, nil
}

func (f *fakeReader) Balance(context.Context, latam.AccountSession) (latam.AccountBalance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.balanceCalls++
	if f.balanceErr != nil {
		return latam.AccountBalance{}, f.balanceErr
	}
	return latam.AccountBalance{Miles: f.miles}, nil
}
