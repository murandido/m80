package azulsync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/azul"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// --- pure tests ---

func TestLastDayBalanceTakesTheLatestDay(t *testing.T) {
	t.Parallel()
	txs := []azul.Transaction{
		{Date: day("2026-09-08"), DayBalance: 165000},
		{Date: day("2026-09-10"), DayBalance: 150000}, // latest
		{Date: day("2026-09-01"), DayBalance: 170000},
	}
	got, ok := lastDayBalance(txs)
	if !ok || got != 150000 {
		t.Errorf("lastDayBalance = %d,%v, want 150000,true", got, ok)
	}
	if _, ok := lastDayBalance(nil); ok {
		t.Error("lastDayBalance of an empty statement reported ok")
	}
}

func TestToStatementLinesNumbersEachDayOldestFirst(t *testing.T) {
	t.Parallel()
	// The legacy endpoint's own order: days newest first, and each day's movements newest first.
	txs := []azul.Transaction{
		{Date: day("2026-09-26"), TypeCode: "AN", Points: -8000},  // 12:17
		{Date: day("2026-09-26"), TypeCode: "AN", Points: -32000}, // 10:52
		{Date: day("2026-09-26"), TypeCode: "AN", Points: -60000}, // 10:07, the day's oldest
		{Date: day("2026-09-25"), TypeCode: "AN", Points: -22000},
		{Date: day("2026-09-25"), TypeCode: "ER", Points: 675000}, // the day's oldest
	}
	lines := toStatementLines("acc", txs)

	want := []int{2, 1, 0, 1, 0}
	for i, line := range lines {
		if line.PositionInDay == nil || *line.PositionInDay != want[i] {
			t.Errorf("line %d (%d pts) position = %v, want %d", i, line.Points, line.PositionInDay, want[i])
		}
	}
}

// --- DB integration ---

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
		t.Skipf("skipping azulsync database tests: no test database available: %v", testDBErr)
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

func day(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

// seedAccount creates the chain an account needs and returns it with a plaintext customerKey,
// the way SyncAccount receives it (AfterFind decrypts the stored one).
func seedAccount(t *testing.T, tx *gorm.DB, customerKey string) data.Account {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	mustCreate(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Programa " + suffix, Code: "P" + suffix, DefaultAirlineID: airline.ID}
	mustCreate(t, tx, &program)
	holder := data.Person{Name: "Titular " + suffix}
	mustCreate(t, tx, &holder)

	account := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID, OwnsPointInventory: true, TracksStatement: true}
	if customerKey != "" {
		account.AzulCustomerKey = &customerKey
	}
	mustCreate(t, tx, &account)
	// Hand back the plaintext key: the caller works with a decrypted account, as the loop does.
	account.AzulCustomerKey = nilIfEmpty(customerKey)
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

func stateOf(t *testing.T, tx *gorm.DB, accountID string) data.AzulSyncState {
	t.Helper()
	var s data.AzulSyncState
	if err := tx.First(&s, "account_id = ?", accountID).Error; err != nil {
		t.Fatalf("reading sync state: %v", err)
	}
	return s
}

func linesOf(t *testing.T, tx *gorm.DB, accountID string) []data.AzulStatementLine {
	t.Helper()
	var out []data.AzulStatementLine
	if err := tx.Where("account_id = ?", accountID).Order("occurred_on").Find(&out).Error; err != nil {
		t.Fatalf("reading lines: %v", err)
	}
	return out
}

// fakeReader answers with configured values, and records how many times each method was called.
type fakeReader struct {
	balances   []azul.Balance // returned in order, last repeats
	balanceAt  int
	balanceErr error

	txs     []azul.Transaction
	txErr   error
	txCalls int
}

func (f *fakeReader) Balance(_ context.Context, _ string) (azul.Balance, error) {
	if f.balanceErr != nil {
		return azul.Balance{}, f.balanceErr
	}
	b := f.balances[minInt(f.balanceAt, len(f.balances)-1)]
	f.balanceAt++
	return b, nil
}

func (f *fakeReader) LegacyStatement(_ context.Context, _ string, _, _ time.Time) ([]azul.Transaction, error) {
	f.txCalls++
	return f.txs, f.txErr
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSyncAccount_StoresWindowWhenBalanceMatches(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		now := day("2026-09-13")
		reader := &fakeReader{
			balances: []azul.Balance{{Points: 150000}},
			txs: []azul.Transaction{
				{Date: day("2026-09-10"), TypeCode: "AN", Points: -20000, DayBalance: 150000, Locator: "ABC123"},
				{Date: day("2026-09-05"), TypeCode: "AI", Points: 5000, DayBalance: 170000, PartnerName: "Livelo"},
			},
		}

		if err := SyncAccount(context.Background(), tx, reader, account, now); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}

		state := stateOf(t, tx, account.ID)
		if state.State != data.AzulSyncOK {
			t.Errorf("state = %q, want ok (detail=%v)", state.State, state.Detail)
		}
		if state.LastBalancePoints == nil || *state.LastBalancePoints != 150000 {
			t.Errorf("last balance = %v, want 150000", state.LastBalancePoints)
		}
		lines := linesOf(t, tx, account.ID)
		if len(lines) != 2 {
			t.Fatalf("stored %d lines, want 2", len(lines))
		}

		// real_balance_points was filled from the balance.
		var acc data.Account
		if err := tx.First(&acc, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading account: %v", err)
		}
		if acc.RealBalancePoints == nil || *acc.RealBalancePoints != 150000 {
			t.Errorf("real balance = %v, want 150000", acc.RealBalancePoints)
		}
	})
}

func TestSyncAccount_InconsistentWhenBalanceDoesNotMatch(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		now := day("2026-09-13")
		reader := &fakeReader{
			balances: []azul.Balance{{Points: 150000}},
			// last-day balance 999999 disagrees with the balance 150000
			txs: []azul.Transaction{{Date: day("2026-09-10"), TypeCode: "AN", Points: -20000, DayBalance: 999999}},
		}

		err := SyncAccount(context.Background(), tx, reader, account, now)
		if err != nil {
			t.Fatalf("SyncAccount returned error: %v", err)
		}

		state := stateOf(t, tx, account.ID)
		if state.State != data.AzulSyncInconsistent {
			t.Errorf("state = %q, want inconsistent", state.State)
		}
		if len(linesOf(t, tx, account.ID)) != 0 {
			t.Error("a window was stored despite the balance mismatch")
		}
		// The balance is still trusted and written even when the statement is not stored.
		var acc data.Account
		_ = tx.First(&acc, "id = ?", account.ID)
		if acc.RealBalancePoints == nil || *acc.RealBalancePoints != 150000 {
			t.Errorf("real balance = %v, want 150000 (balance is trusted)", acc.RealBalancePoints)
		}
	})
}

func TestSyncAccount_UnchangedSkipsTheStatement(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		now := day("2026-09-13")
		last := 150000
		mustCreate(t, tx, &data.AzulSyncState{
			AccountID: account.ID, State: data.AzulSyncOK, LastBalancePoints: &last,
		})
		reader := &fakeReader{balances: []azul.Balance{{Points: 150000}}}

		if err := SyncAccount(context.Background(), tx, reader, account, now); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		if reader.txCalls != 0 {
			t.Errorf("statement pulled %d times, want 0 (balance unchanged)", reader.txCalls)
		}
		if stateOf(t, tx, account.ID).State != data.AzulSyncOK {
			t.Error("state should stay ok on an unchanged balance")
		}
	})
}

func TestSyncAccount_PostponesOnTornRead(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		now := day("2026-09-13")
		// balance moves between the first read and the re-read after pulling the statement.
		reader := &fakeReader{
			balances: []azul.Balance{{Points: 150000}, {Points: 151000}},
			txs:      []azul.Transaction{{Date: day("2026-09-10"), TypeCode: "AN", Points: -20000, DayBalance: 150000}},
		}

		if err := SyncAccount(context.Background(), tx, reader, account, now); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		if len(linesOf(t, tx, account.ID)) != 0 {
			t.Error("a torn read was stored")
		}
		state := stateOf(t, tx, account.ID)
		if state.LastBalancePoints != nil {
			t.Error("last balance was advanced on a postponed round")
		}
	})
}

func TestSyncAccount_FailureThenNeedsAttention(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		now := day("2026-09-13")
		reader := &fakeReader{balanceErr: azul.ErrEdgeBlocked}

		// One failure is transient (error); reaching the threshold raises for attention.
		for i := 1; i <= failureThreshold; i++ {
			err := SyncAccount(context.Background(), tx, reader, account, now)
			if !errors.Is(err, azul.ErrEdgeBlocked) {
				t.Fatalf("round %d error = %v, want ErrEdgeBlocked", i, err)
			}
			state := stateOf(t, tx, account.ID)
			if i < failureThreshold && state.State != data.AzulSyncError {
				t.Errorf("round %d state = %q, want error", i, state.State)
			}
			if i == failureThreshold && state.State != data.AzulSyncNeedsAttention {
				t.Errorf("round %d state = %q, want needs_attention", i, state.State)
			}
		}
	})
}

func TestMarkConnectedLeavesPending(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		// A prior failure that connecting should clear.
		mustCreate(t, tx, &data.AzulSyncState{
			AccountID: account.ID, State: data.AzulSyncError, ConsecutiveFailures: 3,
		})

		if err := MarkConnected(tx, account.ID); err != nil {
			t.Fatalf("MarkConnected: %v", err)
		}
		state := stateOf(t, tx, account.ID)
		if state.State != data.AzulSyncPending {
			t.Errorf("state = %q, want pending", state.State)
		}
		if state.ConsecutiveFailures != 0 {
			t.Errorf("failures = %d, want 0 (connecting clears them)", state.ConsecutiveFailures)
		}
	})
}

func TestMarkLoginFailureRaisesAttentionAtThreshold(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")

		for i := 1; i <= failureThreshold; i++ {
			if err := MarkLoginFailure(tx, account.ID, errors.New("bad credentials")); err != nil {
				t.Fatalf("MarkLoginFailure %d: %v", i, err)
			}
			state := stateOf(t, tx, account.ID)
			if i < failureThreshold && state.State != data.AzulSyncError {
				t.Errorf("after %d failures state = %q, want error", i, state.State)
			}
			if i == failureThreshold && state.State != data.AzulSyncNeedsAttention {
				t.Errorf("after %d failures state = %q, want needs_attention", i, state.State)
			}
		}
	})
}

func TestSyncAccount_PendingWithoutCustomerKey(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "") // no customerKey
		reader := &fakeReader{}

		if err := SyncAccount(context.Background(), tx, reader, account, day("2026-09-13")); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		if reader.txCalls != 0 {
			t.Error("statement pulled for an account with no customerKey")
		}
		if stateOf(t, tx, account.ID).State != data.AzulSyncPending {
			t.Error("state should be pending without a customerKey")
		}
	})
}
