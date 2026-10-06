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
		{Date: day("2026-09-08"), OccurredAt: at("2026-09-08T15:00:00"), DayBalance: 165000},
		{Date: day("2026-09-10"), OccurredAt: at("2026-09-10T15:00:00"), DayBalance: 150000}, // latest
		{Date: day("2026-09-01"), OccurredAt: at("2026-09-01T15:00:00"), DayBalance: 170000},
	}
	got, ok := lastDayBalance(txs)
	if !ok || got != 150000 {
		t.Errorf("lastDayBalance = %d,%v, want 150000,true", got, ok)
	}
	if _, ok := lastDayBalance(nil); ok {
		t.Error("lastDayBalance of an empty statement reported ok")
	}
}

func TestToStatementLinesKeepsTheInstantAndNumbersEachDayOldestFirst(t *testing.T) {
	t.Parallel()
	// The legacy endpoint's own order: days newest first, and each day's movements newest first.
	txs := []azul.Transaction{
		{Date: day("2026-09-26"), OccurredAt: at("2026-09-26T15:17:07"), TypeCode: "AN", Points: -8000},
		{Date: day("2026-09-26"), OccurredAt: at("2026-09-26T13:52:07"), TypeCode: "AN", Points: -32000},
		{Date: day("2026-09-26"), OccurredAt: at("2026-09-26T13:52:07"), TypeCode: "AN", Points: -28000}, // same second, the day's oldest
		{Date: day("2026-09-25"), OccurredAt: at("2026-09-25T14:07:06"), TypeCode: "AN", Points: -22000},
		{Date: day("2026-09-25"), OccurredAt: at("2026-09-25T03:00:00"), TypeCode: "ER", Points: 675000}, // the day's oldest
	}
	lines := toStatementLines("acc", txs)

	want := []int{2, 1, 0, 1, 0}
	for i, line := range lines {
		if line.PositionInDay == nil || *line.PositionInDay != want[i] {
			t.Errorf("line %d (%d pts) position = %v, want %d", i, line.Points, line.PositionInDay, want[i])
		}
		if line.OccurredAt == nil || !line.OccurredAt.Equal(txs[i].OccurredAt) {
			t.Errorf("line %d instant = %v, want %v", i, line.OccurredAt, txs[i].OccurredAt)
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

// at parses a movement instant the way the airline stamps it (to the second, UTC).
func at(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		panic(err)
	}
	return t
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

	// txsByCall, when set, answers the n-th statement call with its n-th entry (last repeats),
	// and froms records the first day each call asked for - how a test tells an incremental read
	// (yesterday) from a whole-month one.
	txsByCall [][]azul.Transaction
	froms     []time.Time
}

func (f *fakeReader) Balance(_ context.Context, _ string) (azul.Balance, error) {
	if f.balanceErr != nil {
		return azul.Balance{}, f.balanceErr
	}
	b := f.balances[minInt(f.balanceAt, len(f.balances)-1)]
	f.balanceAt++
	return b, nil
}

func (f *fakeReader) LegacyStatement(_ context.Context, _ string, from, _ time.Time) ([]azul.Transaction, error) {
	f.txCalls++
	f.froms = append(f.froms, from)
	if len(f.txsByCall) > 0 {
		return f.txsByCall[minInt(f.txCalls-1, len(f.txsByCall)-1)], f.txErr
	}
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
				{Date: day("2026-09-10"), OccurredAt: at("2026-09-10T15:00:00"), TypeCode: "AN", Points: -20000, DayBalance: 150000, Locator: "ABC123"},
				{Date: day("2026-09-05"), OccurredAt: at("2026-09-05T15:00:00"), TypeCode: "AI", Points: 5000, DayBalance: 170000, PartnerName: "Livelo"},
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
			txs: []azul.Transaction{{Date: day("2026-09-10"), OccurredAt: at("2026-09-10T15:00:00"), TypeCode: "AN", Points: -20000, DayBalance: 999999}},
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

// An account that does not track its statement gets its balance and nothing else, and leaves the
// statement's bookmark alone so switching tracking on later pulls the whole month.
func TestSyncAccount_BalanceOnlyWhenNotTracked(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		account.TracksStatement = false
		now := day("2026-09-13")
		reader := &fakeReader{
			balances: []azul.Balance{{Points: 150000}},
			txs: []azul.Transaction{
				{Date: day("2026-09-10"), OccurredAt: at("2026-09-10T15:00:00"), TypeCode: "AN", Points: -20000, DayBalance: 150000},
			},
		}

		if err := SyncAccount(context.Background(), tx, reader, account, now); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		if reader.txCalls != 0 {
			t.Errorf("statement pulled %d times, want 0 (not tracked)", reader.txCalls)
		}
		if len(linesOf(t, tx, account.ID)) != 0 {
			t.Error("a window was stored on an account that does not track its statement")
		}
		state := stateOf(t, tx, account.ID)
		if state.State != data.AzulSyncOK || state.LastSyncedAt == nil {
			t.Errorf("state = %q synced=%v, want ok with a sync time", state.State, state.LastSyncedAt)
		}
		if state.LastBalancePoints != nil {
			t.Errorf("last balance = %v, want it untouched (nil) so tracking later pulls the month", *state.LastBalancePoints)
		}
		var acc data.Account
		if err := tx.First(&acc, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading account: %v", err)
		}
		if acc.RealBalancePoints == nil || *acc.RealBalancePoints != 150000 {
			t.Errorf("real balance = %v, want 150000", acc.RealBalancePoints)
		}

		// Tracking switched on: the next round pulls and stores the month.
		account.TracksStatement = true
		if err := SyncAccount(context.Background(), tx, reader, account, now); err != nil {
			t.Fatalf("SyncAccount after tracking: %v", err)
		}
		if reader.txCalls == 0 {
			t.Error("the statement was not pulled after tracking was switched on")
		}
		if len(linesOf(t, tx, account.ID)) != 1 {
			t.Errorf("stored %d lines after tracking, want 1", len(linesOf(t, tx, account.ID)))
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
			txs:      []azul.Transaction{{Date: day("2026-09-10"), OccurredAt: at("2026-09-10T15:00:00"), TypeCode: "AN", Points: -20000, DayBalance: 150000}},
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

// --- incremental rounds ---

// movement is one airline movement at an instant, filed under the Brazilian day of that instant.
func movement(instant, typeCode string, points, dayBalance int, comment string) azul.Transaction {
	occurredAt := at(instant)
	return azul.Transaction{
		OccurredAt: occurredAt, Date: azul.BrazilDay(occurredAt),
		TypeCode: typeCode, Points: points, DayBalance: dayBalance, PartnerName: "Azul", Comments: comment,
	}
}

// seedSynced stores the given movements and a sync state that validated them at balance, as a
// previous round would have left them.
func seedSynced(t *testing.T, tx *gorm.DB, accountID string, balance int, txs ...azul.Transaction) {
	t.Helper()
	if err := data.InsertAzulStatementLines(tx, accountID, toStatementLines(accountID, txs)); err != nil {
		t.Fatalf("seeding lines: %v", err)
	}
	state := &data.AzulSyncState{AccountID: accountID, State: data.AzulSyncOK, LastBalancePoints: &balance}
	if err := upsertState(tx, state); err != nil {
		t.Fatalf("seeding state: %v", err)
	}
}

func TestNewMovementsCountsIdenticalOnes(t *testing.T) {
	t.Parallel()
	leg := movement("2026-09-26T13:52:07", "AN", -32000, 0, "Booking: RQR7HK | From Confins to Salvador")
	other := movement("2026-09-26T15:17:07", "AN", -8000, 0, "Booking: MTJFFB | From X to Y")
	stored := toStatementLines("acc", []azul.Transaction{leg, leg})
	fetched := toStatementLines("acc", []azul.Transaction{leg, leg, leg, other})

	fresh := newMovements(stored, fetched)
	if len(fresh) != 2 {
		t.Fatalf("found %d new movements, want 2 (the third identical leg and the other one)", len(fresh))
	}

	// A line stored earlier that the response no longer carries is not a new one, nor removed.
	if got := newMovements(toStatementLines("acc", []azul.Transaction{leg, other}), toStatementLines("acc", []azul.Transaction{other})); len(got) != 0 {
		t.Errorf("a missing stored line produced %d new movements, want 0", len(got))
	}
}

func TestMovementSignatureIgnoresWhatThePanelDerives(t *testing.T) {
	t.Parallel()
	line := toStatementLines("acc", []azul.Transaction{movement("2026-09-26T13:52:07", "AN", -32000, 0, "Booking: RQR7HK")})[0]
	reparsed := line
	locator := "SOMETHING-ELSE"
	reparsed.Locator = &locator
	reparsed.Kind = data.AzulLineOther
	if movementSignature(line) != movementSignature(reparsed) {
		t.Error("a different derived locator/kind changed the signature")
	}
}

func TestSyncAccount_IncrementalAddsOnlyNewMovements(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		leg := movement("2026-09-26T13:52:07", "AN", -32000, 36000, "Booking: RQR7HK | From Confins to Salvador")
		seedSynced(t, tx, account.ID, 100000, leg, leg)

		// 20:30 in São Paulo on the 26th: today is the 26th, so the read starts on the 25th.
		now := at("2026-09-26T23:30:00")
		newLeg := leg
		refund := movement("2026-09-26T22:10:00", "RF", 10000, 0, "PNR MTJFFB")
		reader := &fakeReader{
			balances: []azul.Balance{{Points: 78000}}, // 100000 - 32000 + 10000
			txs:      []azul.Transaction{refund, newLeg, leg, leg},
		}

		if err := SyncAccount(context.Background(), tx, reader, account, now); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		if reader.txCalls != 1 || !reader.froms[0].Equal(day("2026-09-25")) {
			t.Fatalf("statement calls = %d from %v, want one read from 2026-09-25", reader.txCalls, reader.froms)
		}
		if got := linesOf(t, tx, account.ID); len(got) != 4 {
			t.Errorf("stored %d lines, want 4 (2 kept + 1 identical leg + 1 refund)", len(got))
		}
		state := stateOf(t, tx, account.ID)
		if state.State != data.AzulSyncOK || state.LastBalancePoints == nil || *state.LastBalancePoints != 78000 {
			t.Errorf("state = %q balance %v, want ok at 78000 (detail=%v)", state.State, state.LastBalancePoints, state.Detail)
		}
	})
}

func TestSyncAccount_IncrementalThatDoesNotAddUpPullsTheMonth(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		kept := movement("2026-09-20T12:00:00", "AN", -20000, 80000, "Booking: AAAAAA")
		seedSynced(t, tx, account.ID, 80000, kept)

		now := at("2026-09-26T15:00:00")
		late := movement("2026-09-22T10:00:00", "ER", 5000, 85000, "filed days back")
		today := movement("2026-09-26T14:00:00", "AN", -1000, 84000, "Booking: BBBBBB")
		reader := &fakeReader{
			balances: []azul.Balance{{Points: 84000}}, // 80000 + 5000 - 1000
			txsByCall: [][]azul.Transaction{
				{today},             // incremental: -1000 alone does not reach 84000
				{today, late, kept}, // the whole month does
			},
		}

		if err := SyncAccount(context.Background(), tx, reader, account, now); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		if reader.txCalls != 2 || !reader.froms[1].Equal(windowStart(now)) {
			t.Fatalf("statement calls = %d from %v, want a second whole-month read", reader.txCalls, reader.froms)
		}
		if got := linesOf(t, tx, account.ID); len(got) != 3 {
			t.Errorf("stored %d lines, want the month's 3", len(got))
		}
		if state := stateOf(t, tx, account.ID); state.State != data.AzulSyncOK {
			t.Errorf("state = %q, want ok (detail=%v)", state.State, state.Detail)
		}
	})
}

func TestSyncAccount_ClocklessRowsInTheWindowForceTheMonth(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		seedSynced(t, tx, account.ID, 80000)
		// A row stored before the clock was kept: its signature cannot be compared.
		old := data.AzulStatementLine{AccountID: account.ID, OccurredOn: day("2026-09-20"), Kind: data.AzulLineRedemption, Points: -20000}
		if err := tx.Create(&old).Error; err != nil {
			t.Fatalf("seeding a clockless row: %v", err)
		}

		now := at("2026-09-26T15:00:00")
		reader := &fakeReader{
			balances: []azul.Balance{{Points: 60000}},
			txs:      []azul.Transaction{movement("2026-09-20T12:00:00", "AN", -20000, 60000, "Booking: AAAAAA")},
		}
		if err := SyncAccount(context.Background(), tx, reader, account, now); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		if reader.txCalls != 1 || !reader.froms[0].Equal(windowStart(now)) {
			t.Fatalf("statement calls = %d from %v, want one whole-month read", reader.txCalls, reader.froms)
		}
		lines := linesOf(t, tx, account.ID)
		if len(lines) != 1 || lines[0].OccurredAt == nil {
			t.Errorf("stored %d lines (instant %v), want the clockless row replaced by one with its instant", len(lines), lines)
		}
	})
}

func TestSyncAccount_WholeMonthLeavesTheEveningBeforeTheWindow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "cust-key-1")
		now := at("2026-09-26T15:00:00")
		from := windowStart(now) // 2026-08-26
		// 01:00 UTC on the window's first UTC date is 22:00 of the day before in São Paulo.
		evening := movement(from.Format("2006-01-02")+"T01:00:00", "AN", -5000, 95000, "Booking: EVENIN")
		inside := movement("2026-09-20T12:00:00", "AN", -1000, 94000, "Booking: INSIDE")
		reader := &fakeReader{balances: []azul.Balance{{Points: 94000}}, txs: []azul.Transaction{inside, evening}}

		if err := SyncAccount(context.Background(), tx, reader, account, now); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		lines := linesOf(t, tx, account.ID)
		if len(lines) != 1 || lines[0].Points != -1000 {
			t.Errorf("stored %d lines, want only the one inside the window", len(lines))
		}
	})
}
