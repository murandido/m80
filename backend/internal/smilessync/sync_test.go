package smilessync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/carriers/smiles"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func movement(day string, miles int, typeCode, subType, product, externalID string) smiles.Transaction {
	return smiles.Transaction{
		ExternalID:  externalID,
		Date:        parseDay(day),
		TypeCode:    typeCode,
		SubType:     subType,
		ProductName: product,
		Miles:       miles,
	}
}

func parseDay(s string) time.Time {
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return parsed
}

// now is inside the window the round reads, so the seeded movements land in it.
func syncNow() time.Time { return time.Now().UTC() }

func recentDay(offset int) string {
	return syncNow().AddDate(0, 0, offset).Format("2006-01-02")
}

func TestSyncAccountStoresTheWindow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		reader := &fakeReader{
			balances: []int{150000, 150000},
			movements: []smiles.Transaction{
				movement(recentDay(-3), -27400, smiles.CodeRedemption, "Product", "VOO", "IOTFQF"),
				movement(recentDay(-2), 300000, smiles.CodeAccrual, "Product", "Pontos Livelo", "TX-1"),
				movement(recentDay(-1), 5000, smiles.CodeAccrual, smiles.SubTypeCancellation, "VOO", "CANCEL"),
			},
		}

		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, syncNow()); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}

		lines := linesOf(t, tx, account.ID)
		if len(lines) != 3 {
			t.Fatalf("stored %d lines, want 3", len(lines))
		}
		kinds := map[string]int{}
		for _, line := range lines {
			kinds[line.Kind]++
		}
		if kinds[data.SmilesLineRedemption] != 1 || kinds[data.SmilesLinePurchase] != 1 || kinds[data.SmilesLineRefund] != 1 {
			t.Errorf("kinds = %v", kinds)
		}

		state := stateOf(t, tx, account.ID)
		if state.State != data.SmilesSyncOK {
			t.Errorf("state = %q, want ok", state.State)
		}
		if state.LastBalanceMiles == nil || *state.LastBalanceMiles != 150000 {
			t.Errorf("last balance = %v", state.LastBalanceMiles)
		}
	})
}

func TestSyncAccountStoresTheRotatedSessionBeforeReading(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		// The balance read fails, so the round never gets as far as the statement. The
		// rotated token must already be stored: the one it replaced is dead, and losing the
		// new one locks the account out of every future round.
		reader := &fakeReader{
			rotatedTo:  "refresh-2",
			balances:   []int{0},
			balanceErr: errors.New("a rede caiu"),
		}

		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, syncNow()); err == nil {
			t.Fatal("expected the round to fail")
		}

		stored := storedSession(t, tx, account.ID)
		if stored == nil || *stored != "refresh-2" {
			t.Fatalf("stored session = %v, want the rotated token", stored)
		}
	})
}

func TestSyncAccountReloadsTheTokenAfterAcquiringTheGuard(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		reader := &fakeReader{balances: []int{150000}}
		guard := NewSessionGuard()

		// Both callers hold the same pre-rotation account snapshot. The second call must
		// use the token persisted by the first, or Auth0 revokes the token family.
		if err := SyncAccount(context.Background(), tx, reader, guard, account, syncNow()); err != nil {
			t.Fatalf("first round: %v", err)
		}
		if err := SyncAccount(context.Background(), tx, reader, guard, account, syncNow()); err != nil {
			t.Fatalf("second round: %v", err)
		}
		if len(reader.refreshTokens) != 2 || reader.refreshTokens[0] != "refresh-1" ||
			reader.refreshTokens[1] != "refresh-1-rotacionado" {
			t.Fatalf("refresh inputs = %v, want successive tokens", reader.refreshTokens)
		}
	})
}

func TestSyncAccountSkipsTheStatementWhenTheBalanceHasNotMoved(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		reader := &fakeReader{balances: []int{150000}}

		// First round establishes the balance...
		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, syncNow()); err != nil {
			t.Fatalf("first round: %v", err)
		}
		before := reader.statementCall

		// ...and the second has nothing new to pull. Reading the statement anyway is two
		// wasted calls per account per tick against a service that rate limits.
		reloaded := seedReload(t, tx, account.ID)
		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), reloaded, syncNow()); err != nil {
			t.Fatalf("second round: %v", err)
		}
		if reader.statementCall != before {
			t.Errorf("the statement was pulled again: %d calls, want %d", reader.statementCall, before)
		}
		if state := stateOf(t, tx, account.ID); state.State != data.SmilesSyncOK {
			t.Errorf("state = %q, want ok", state.State)
		}
	})
}

// An account that does not track its statement gets its balance (and its rotated session) and
// nothing else, and leaves the statement's bookmark alone so switching tracking on later pulls the
// window instead of skipping it as "unchanged".
func TestSyncAccountReadsOnlyTheBalanceWhenNotTracked(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		account.TracksStatement = false
		reader := &fakeReader{
			rotatedTo: "refresh-2",
			balances:  []int{150000},
			movements: []smiles.Transaction{
				movement(recentDay(-3), -27400, smiles.CodeRedemption, "Product", "VOO", "IOTFQF"),
			},
		}

		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, syncNow()); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		if reader.statementCall != 0 {
			t.Errorf("statement pulled %d times, want 0 (not tracked)", reader.statementCall)
		}
		if stored := storedSession(t, tx, account.ID); stored == nil || *stored != "refresh-2" {
			t.Errorf("stored session = %v, want the rotated token", stored)
		}
		state := stateOf(t, tx, account.ID)
		if state.State != data.SmilesSyncOK || state.LastSyncedAt == nil {
			t.Errorf("state = %q synced=%v, want ok with a sync time", state.State, state.LastSyncedAt)
		}
		if state.LastBalanceMiles != nil {
			t.Errorf("last balance = %v, want it untouched (nil)", *state.LastBalanceMiles)
		}
		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading account: %v", err)
		}
		if reloaded.RealBalancePoints == nil || *reloaded.RealBalancePoints != 150000 {
			t.Errorf("real balance = %v, want 150000", reloaded.RealBalancePoints)
		}

		// Tracking switched on with the balance unchanged: the window is still pulled.
		reloaded.TracksStatement = true
		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), reloaded, syncNow()); err != nil {
			t.Fatalf("SyncAccount after tracking: %v", err)
		}
		if reader.statementCall == 0 {
			t.Error("the statement was not pulled after tracking was switched on")
		}
		if len(linesOf(t, tx, account.ID)) != 1 {
			t.Errorf("stored %d lines after tracking, want 1", len(linesOf(t, tx, account.ID)))
		}
	})
}

func TestSyncAccountPostponesOnATornRead(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		// The balance moves between the two reads, so the statement in hand is already stale.
		reader := &fakeReader{
			balances: []int{150000, 149000},
			movements: []smiles.Transaction{
				movement(recentDay(-1), -27400, smiles.CodeRedemption, "Product", "VOO", "IOTFQF"),
			},
		}

		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, syncNow()); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}

		if lines := linesOf(t, tx, account.ID); len(lines) != 0 {
			t.Fatalf("stored %d lines, want 0 - a torn read must not be stored", len(lines))
		}
		state := stateOf(t, tx, account.ID)
		// LastBalanceMiles must stay unset, or the next tick would think nothing moved.
		if state.LastBalanceMiles != nil {
			t.Errorf("last balance = %v, want nil so the next round re-pulls", *state.LastBalanceMiles)
		}
	})
}

func TestSyncAccountAsksForANewCodeWhenTheSessionIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "revogado")
		reader := &fakeReader{refreshErr: smiles.ErrSessionExpired, balances: []int{0}}

		err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, syncNow())
		if !errors.Is(err, smiles.ErrSessionExpired) {
			t.Fatalf("err = %v, want ErrSessionExpired", err)
		}

		state := stateOf(t, tx, account.ID)
		if state.State != data.SmilesSyncNeedsMFA {
			t.Fatalf("state = %q, want needs_mfa", state.State)
		}
		// Not a failure to accumulate: there is exactly one exit, and letting it drift toward
		// needs_attention would replace a precise message with a vague one.
		if state.ConsecutiveFailures != 0 {
			t.Errorf("consecutive failures = %d, want 0", state.ConsecutiveFailures)
		}
		if state.Detail == nil || *state.Detail == "" {
			t.Error("needs_mfa must say what the operator has to do")
		}
	})
}

func TestSyncAccountRaisesForAttentionAfterEnoughFailures(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		reader := &fakeReader{balances: []int{0}, balanceErr: errors.New("gateway fora do ar")}
		guard := NewSessionGuard()

		for round := 1; round <= failureThreshold; round++ {
			if err := SyncAccount(context.Background(), tx, reader, guard, seedReload(t, tx, account.ID), syncNow()); err == nil {
				t.Fatalf("round %d: expected a failure", round)
			}
			state := stateOf(t, tx, account.ID)
			wantState := data.SmilesSyncError
			if round >= failureThreshold {
				wantState = data.SmilesSyncNeedsAttention
			}
			if state.State != wantState {
				t.Fatalf("round %d: state = %q, want %q", round, state.State, wantState)
			}
		}
	})
}

func TestSyncAccountWithoutASessionStaysPending(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "")
		reader := &fakeReader{balances: []int{150000}}

		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, syncNow()); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}
		if reader.refreshCalls != 0 {
			t.Error("an unconnected account must not reach the tenant at all")
		}
		if state := stateOf(t, tx, account.ID); state.State != data.SmilesSyncPending {
			t.Errorf("state = %q, want pending", state.State)
		}
	})
}

func TestDisconnectForgetsTheSessionAndTheWindow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		reader := &fakeReader{
			balances:  []int{150000, 150000},
			movements: []smiles.Transaction{movement(recentDay(-1), -27400, smiles.CodeRedemption, "Product", "VOO", "IOTFQF")},
		}
		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, syncNow()); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}

		if err := Disconnect(tx, NewSessionGuard(), account.ID); err != nil {
			t.Fatalf("Disconnect: %v", err)
		}

		if stored := storedSession(t, tx, account.ID); stored != nil {
			t.Error("the session survived a disconnect")
		}
		if lines := linesOf(t, tx, account.ID); len(lines) != 0 {
			t.Errorf("%d statement lines survived a disconnect", len(lines))
		}
		var state data.SmilesSyncState
		if err := tx.First(&state, "account_id = ?", account.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Errorf("the sync badge survived a disconnect: %v", err)
		}
	})
}

// seedReload re-reads the account the way the loop does, so the AfterFind hook hands back a
// decrypted session rather than the ciphertext the previous round wrote.
func seedReload(t *testing.T, tx *gorm.DB, accountID string) data.Account {
	t.Helper()
	var account data.Account
	if err := tx.First(&account, "id = ?", accountID).Error; err != nil {
		t.Fatalf("reloading account: %v", err)
	}
	return account
}

// The airline reports no closing balance, so the sync works one out per day. The test pins the
// arithmetic AND its direction: the balance that was read is the window's END, so the earliest
// day must come out lower, not higher.
func TestFillDayBalancesWalksTheWindowForwardFromItsOpening(t *testing.T) {
	t.Parallel()

	lines := []data.SmilesStatementLine{
		{OccurredOn: parseDay("2026-09-01"), Points: 10000},
		{OccurredOn: parseDay("2026-09-03"), Points: -4000},
		// Two movements on one day: both carry that day's closing figure, not a running one.
		{OccurredOn: parseDay("2026-09-03"), Points: -1000},
		{OccurredOn: parseDay("2026-09-05"), Points: 500},
	}
	// The account holds 55.500 now, and the window moved it by +5.500, so it opened at 50.000.
	fillDayBalances(lines, 55500)

	want := []int{60000, 55000, 55000, 55500}
	for i, line := range lines {
		if line.DayBalance == nil {
			t.Fatalf("line %d has no day balance", i)
		}
		if *line.DayBalance != want[i] {
			t.Errorf("line %d day balance = %d, want %d", i, *line.DayBalance, want[i])
		}
	}
	// The last day must land exactly on the balance that was read, or the walk has drifted.
	if *lines[len(lines)-1].DayBalance != 55500 {
		t.Errorf("the window does not close on the balance that was read")
	}
}

// The lines arrive in whatever order the airline sent them, and the two transaction types are
// concatenated - so the walk has to order the days itself.
func TestFillDayBalancesOrdersTheDaysItself(t *testing.T) {
	t.Parallel()

	lines := []data.SmilesStatementLine{
		{OccurredOn: parseDay("2026-09-05"), Points: 500},
		{OccurredOn: parseDay("2026-09-01"), Points: 10000},
		{OccurredOn: parseDay("2026-09-03"), Points: -5000},
	}
	fillDayBalances(lines, 55500)

	byDay := map[string]int{}
	for _, line := range lines {
		byDay[line.OccurredOn.Format("2006-01-02")] = *line.DayBalance
	}
	for day, want := range map[string]int{"2026-09-01": 60000, "2026-09-03": 55000, "2026-09-05": 55500} {
		if byDay[day] != want {
			t.Errorf("%s closed at %d, want %d", day, byDay[day], want)
		}
	}
}

func TestFillDayBalancesToleratesAnEmptyWindow(t *testing.T) {
	t.Parallel()
	fillDayBalances(nil, 55500)
}

func TestSyncAccountStoresADayBalanceOnEveryLine(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		reader := &fakeReader{
			balances: []int{150000, 150000},
			movements: []smiles.Transaction{
				movement(recentDay(-3), -27400, smiles.CodeRedemption, "Product", "VOO", "IOTFQF"),
				movement(recentDay(-2), 300000, smiles.CodeAccrual, "Product", "Pontos Livelo", "TX-1"),
			},
		}

		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, syncNow()); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}

		lines := linesOf(t, tx, account.ID)
		if len(lines) != 2 {
			t.Fatalf("stored %d lines, want 2", len(lines))
		}
		for _, line := range lines {
			if line.DayBalance == nil {
				t.Fatalf("a stored line has no day balance: %+v", line)
			}
		}
		// linesOf orders by day, so the last line is the window's end and must equal the balance.
		if *lines[len(lines)-1].DayBalance != 150000 {
			t.Errorf("the window closes at %d, want the balance 150000", *lines[len(lines)-1].DayBalance)
		}
	})
}
