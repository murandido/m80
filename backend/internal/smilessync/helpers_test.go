package smilessync

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/smiles"
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
		t.Skipf("skipping smilessync database tests: no test database available: %v", testDBErr)
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

// seedAccount creates the chain an account needs and returns it with a plaintext refresh
// token, the way SyncAccount receives it (AfterFind decrypts the stored one).
func seedAccount(t *testing.T, tx *gorm.DB, refreshToken string) data.Account {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	mustCreate(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Programa " + suffix, Code: "P" + suffix, DefaultAirlineID: airline.ID}
	mustCreate(t, tx, &program)
	holder := data.Person{Name: "Titular " + suffix}
	mustCreate(t, tx, &holder)

	account := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID, OwnsPointInventory: true, TracksStatement: true}
	if refreshToken != "" {
		account.SmilesRefreshToken = &refreshToken
	}
	mustCreate(t, tx, &account)
	// Hand back the plaintext token: the caller works with a decrypted account, as the loop does.
	account.SmilesRefreshToken = nilIfEmpty(refreshToken)
	return account
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mustCreate(t *testing.T, tx *gorm.DB, value any) {
	t.Helper()
	if err := tx.Create(value).Error; err != nil {
		t.Fatalf("seeding %T: %v", value, err)
	}
}

func stateOf(t *testing.T, tx *gorm.DB, accountID string) data.SmilesSyncState {
	t.Helper()
	var state data.SmilesSyncState
	if err := tx.First(&state, "account_id = ?", accountID).Error; err != nil {
		t.Fatalf("reading sync state: %v", err)
	}
	return state
}

func linesOf(t *testing.T, tx *gorm.DB, accountID string) []data.SmilesStatementLine {
	t.Helper()
	var out []data.SmilesStatementLine
	if err := tx.Where("account_id = ?", accountID).Order("occurred_on").Find(&out).Error; err != nil {
		t.Fatalf("reading lines: %v", err)
	}
	return out
}

func storedSession(t *testing.T, tx *gorm.DB, accountID string) *string {
	t.Helper()
	var account data.Account
	if err := tx.First(&account, "id = ?", accountID).Error; err != nil {
		t.Fatalf("reloading account: %v", err)
	}
	return account.SmilesRefreshToken
}

// fakeReader answers with configured values and records what it was asked. Balances are served
// in order, so a round that reads the balance twice can be made to see it move.
type fakeReader struct {
	mu sync.Mutex

	refreshErr    error
	rotatedTo     string
	balances      []int
	balanceErr    error
	movements     []smiles.Transaction
	statementErr  error
	refreshCalls  int
	balanceCalls  int
	statementCall int
}

func (f *fakeReader) Refresh(_ context.Context, refreshToken string) (smiles.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshCalls++
	if f.refreshErr != nil {
		return smiles.Session{}, f.refreshErr
	}
	rotated := f.rotatedTo
	if rotated == "" {
		rotated = refreshToken + "-rotacionado"
	}
	return smiles.Session{
		AccessToken:  "access",
		RefreshToken: rotated,
		OAGToken:     "oag",
		MemberNumber: "112233",
		ExpiresAt:    time.Now().Add(time.Hour),
	}, nil
}

func (f *fakeReader) Balance(context.Context, smiles.Session) (smiles.Balance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.balanceErr != nil {
		return smiles.Balance{}, f.balanceErr
	}
	index := f.balanceCalls
	f.balanceCalls++
	if index >= len(f.balances) {
		index = len(f.balances) - 1
	}
	return smiles.Balance{Miles: f.balances[index]}, nil
}

func (f *fakeReader) Statement(context.Context, smiles.Session, time.Time, time.Time) ([]smiles.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statementCall++
	if f.statementErr != nil {
		return nil, f.statementErr
	}
	return f.movements, nil
}
