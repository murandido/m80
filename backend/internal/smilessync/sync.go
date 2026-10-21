package smilessync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/carriers/smiles"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// windowDays is how far back a round reads: at most a month, the same bound the Azul sync
// uses. The window is [now-windowDays, now] and is what ReplaceSmilesStatementWindow swaps
// wholesale.
const windowDays = 31

// failureThreshold is how many rounds in a row may fail before the account is raised for
// attention on the panel. Below it a failure is transient (a blip, a host mid-swap) and just
// retried on the next tick.
const failureThreshold = 5

// Reader is the slice of the smiles client this package needs. An interface so a round can be
// tested against a fake without the live tenant; smiles.Client satisfies it.
type Reader interface {
	Refresh(ctx context.Context, refreshToken string) (smiles.Session, error)
	Balance(ctx context.Context, session smiles.Session) (smiles.Balance, error)
	Statement(ctx context.Context, session smiles.Session, from, to time.Time) ([]smiles.Transaction, error)
}

// SyncAccount runs one round for one account and records the outcome in smiles_sync_state. It
// returns the terminal error (if any) for the caller to log; the panel-visible state is always
// written regardless of the return.
//
// The shape of a round:
//  1. renew the session and store the rotated token at once - the one just used is dead
//  2. read the balance; fill real_balance_points from it. An account that does not track its
//     statement stops here
//  3. if the balance has not moved since last time, there is nothing new to pull - done
//  4. otherwise pull the last month's statement, then read the balance AGAIN: if it changed
//     mid-pull the statement we hold is already stale, so postpone rather than store a torn
//     read
//  5. replace the window
//
// There is no step validating the statement against the balance, which is the one thing the
// Azul round does and this one cannot: Smiles publishes no end-of-day balance to check against.
func SyncAccount(ctx context.Context, db *gorm.DB, reader Reader, guard *SessionGuard, account data.Account, now time.Time) error {
	unlock := guard.Lock(account.ID)
	defer unlock()

	state, err := loadOrInitState(db, account.ID)
	if err != nil {
		return fmt.Errorf("smilessync: loading state: %w", err)
	}

	// The caller may have loaded this account before waiting for the guard. Another round
	// can rotate its refresh token while we wait, so read the current value under the lock.
	var current data.Account
	if err := db.Select("id", "smiles_refresh_token").First(&current, "id = ?", account.ID).Error; err != nil {
		return fmt.Errorf("smilessync: reloading session: %w", err)
	}
	refreshToken := deref(current.SmilesRefreshToken)
	if refreshToken == "" {
		// Not connected: nothing to read. Left pending for the connect flow to fill.
		return recordPending(db, state)
	}

	session, err := reader.Refresh(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, smiles.ErrSessionExpired) {
			// The only exit is a human typing a new code. Retrying cannot help, and falling
			// back to the password would be how an account gets locked.
			detail := "a sessão da Smiles não é mais aceita; conecte a conta de novo para receber um código"
			if markErr := MarkNeedsMFA(db, account.ID, detail); markErr != nil {
				return markErr
			}
			return err
		}
		return recordFailure(db, state, err)
	}

	// Store before using: the token we just spent is already refused, so a round that reads
	// first and crashes leaves the account locked out of the next one.
	if err := storeSession(db, account.ID, session.RefreshToken); err != nil {
		return fmt.Errorf("smilessync: storing the rotated session: %w", err)
	}

	balance, err := reader.Balance(ctx, session)
	if err != nil {
		return recordFailure(db, state, err)
	}

	// A good balance read is worth keeping on its own: it is the number the panel compares
	// against, and it is trustworthy even on a round where the statement is not pulled.
	if err := setRealBalance(db, account.ID, balance.Miles, now); err != nil {
		return fmt.Errorf("smilessync: writing real balance: %w", err)
	}

	// An account that does not track its statement stops here: its balance is all a round is for.
	if !account.TracksStatement {
		return recordBalanceOnly(db, state, now)
	}

	if state.LastBalanceMiles != nil && *state.LastBalanceMiles == balance.Miles {
		return recordUnchanged(db, state, balance.Miles, now)
	}

	from := windowStart(now)
	movements, err := reader.Statement(ctx, session, from, now)
	if err != nil {
		return recordFailure(db, state, err)
	}

	// Read the balance again: if miles moved while the statement was being pulled, what we
	// hold is a torn read. Postpone without storing - the next tick starts clean.
	after, err := reader.Balance(ctx, session)
	if err != nil {
		return recordFailure(db, state, err)
	}
	if after.Miles != balance.Miles {
		return recordPostponed(db, state)
	}

	lines := toStatementLines(account.ID, movements)
	fillDayBalances(lines, balance.Miles)
	if err := data.ReplaceSmilesStatementWindow(db, account.ID, from, lines); err != nil {
		return recordFailure(db, state, fmt.Errorf("replacing the window: %w", err))
	}
	return recordSynced(db, state, balance.Miles, now)
}

// Disconnect forgets an account's session and its stored window, leaving it as it was before
// it was ever connected. The order matters only in that nothing should be readable after the
// session is gone.
func Disconnect(db *gorm.DB, guard *SessionGuard, accountID string) error {
	unlock := guard.Lock(accountID)
	defer unlock()

	if err := clearSession(db, accountID); err != nil {
		return fmt.Errorf("smilessync: clearing the session: %w", err)
	}
	if err := db.Where("account_id = ?", accountID).
		Delete(&data.SmilesStatementLine{}).Error; err != nil {
		return fmt.Errorf("smilessync: clearing the window: %w", err)
	}
	return MarkDisconnected(db, accountID)
}

// StoreSession persists a session obtained by the connect flow. Exported because the handler
// that finishes a verification code has the same duty as a round: store the rotated token
// before anything else can spend it.
func StoreSession(db *gorm.DB, guard *SessionGuard, accountID, refreshToken string) error {
	unlock := guard.Lock(accountID)
	defer unlock()
	return storeSession(db, accountID, refreshToken)
}

// windowStart is the earliest day a round reads, truncated to a whole day in UTC.
func windowStart(now time.Time) time.Time {
	d := now.AddDate(0, 0, -windowDays)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

// toStatementLines maps the read movements onto storable rows, deriving the panel-terms kind
// from the airline's own classification.
func toStatementLines(accountID string, movements []smiles.Transaction) []data.SmilesStatementLine {
	lines := make([]data.SmilesStatementLine, 0, len(movements))
	for _, movement := range movements {
		lines = append(lines, data.SmilesStatementLine{
			AccountID:  accountID,
			OccurredOn: movement.Date,
			Kind: data.SmilesKindFromType(movement.TypeCode, movement.SubType,
				movement.ProductName, movement.PartnerName),
			TypeCode:        ptrOrNil(movement.TypeCode),
			SubType:         ptrOrNil(movement.SubType),
			TypeDescription: ptrOrNil(movement.ProductName),
			Points:          movement.Miles,
			Locator:         ptrOrNil(movement.Locator),
			Origin:          ptrOrNil(movement.Origin),
			Destination:     ptrOrNil(movement.Destination),
			PartnerName:     ptrOrNil(movement.PartnerName),
			// Description is what the page shows next to the movement, and what tells two
			// identical credits apart. The product name is the operator's own vocabulary
			// ("Pontos Livelo", "Compra de Milhas PaGol"); the partner is the fallback for a
			// movement that has no product name of its own.
			Description: ptrOrNil(firstNonEmpty(movement.ProductName, movement.PartnerName)),
			Channel:     ptrOrNil(movement.Channel),
			ExternalID:  ptrOrNil(movement.ExternalID),
		})
	}
	return lines
}

// fillDayBalances works out what the account held at the end of each day of the window, and
// writes it onto every line of that day.
//
// Smiles reports no closing balance, so this is arithmetic rather than a reading: the balance
// that was just read is where the window ends, and subtracting the window's own net gives where
// it began. Walking the days forward from there fills the rest.
//
// The consequence is worth stating because it is the opposite of Azul's: a figure derived from
// the movements can never disagree with them. It cannot detect a missing movement - it would
// simply be wrong by that amount from the gap backwards, silently. So it is shown, never
// checked. The sync's protection against a torn window remains the second balance read.
//
// balance is the figure read AFTER the statement was pulled and confirmed unmoved, which is what
// makes "the window ends here" true.
func fillDayBalances(lines []data.SmilesStatementLine, balance int) {
	if len(lines) == 0 {
		return
	}

	net := 0
	for _, line := range lines {
		net += line.Points
	}
	// Where the window began, before any of its movements.
	running := balance - net

	// Days in order, and every line of a day carries that day's closing figure - the same shape
	// the Azul rows have, so the page reads both the same way.
	byDay := make(map[string][]int, len(lines))
	order := make([]string, 0, len(lines))
	for i, line := range lines {
		day := line.OccurredOn.Format("2006-01-02")
		if _, seen := byDay[day]; !seen {
			order = append(order, day)
		}
		byDay[day] = append(byDay[day], i)
	}
	sort.Strings(order)

	for _, day := range order {
		for _, i := range byDay[day] {
			running += lines[i].Points
		}
		closing := running
		for _, i := range byDay[day] {
			lines[i].DayBalance = &closing
		}
	}
}

// --- state transitions, each writing exactly one row ---

func recordSynced(db *gorm.DB, state *data.SmilesSyncState, balance int, now time.Time) error {
	state.State = data.SmilesSyncOK
	state.LastBalanceMiles = &balance
	state.LastSyncedAt = &now
	state.ConsecutiveFailures = 0
	state.Detail = nil
	return upsertState(db, state)
}

// recordBalanceOnly closes a round on an account that does not track its statement: the balance was
// read and written to real_balance_points, and nothing else was. LastBalanceMiles is deliberately left
// alone - it is the statement's bookmark, the balance the stored window was last reconciled to.
// Moving it here would make the first round after "Acompanhar extrato" is switched on see an
// "unchanged" balance and skip the statement it was switched on to read.
func recordBalanceOnly(db *gorm.DB, state *data.SmilesSyncState, now time.Time) error {
	state.State = data.SmilesSyncOK
	state.LastSyncedAt = &now
	state.ConsecutiveFailures = 0
	state.Detail = nil
	return upsertState(db, state)
}

func recordUnchanged(db *gorm.DB, state *data.SmilesSyncState, balance int, now time.Time) error {
	state.State = data.SmilesSyncOK
	state.LastBalanceMiles = &balance
	state.LastSyncedAt = &now
	state.ConsecutiveFailures = 0
	state.Detail = nil
	return upsertState(db, state)
}

// recordPostponed leaves the state as it was but notes the torn read, without touching
// LastBalanceMiles so the next tick re-pulls.
func recordPostponed(db *gorm.DB, state *data.SmilesSyncState) error {
	detail := "o saldo mudou durante a leitura do extrato; adiado para a próxima rodada"
	state.Detail = &detail
	return upsertState(db, state)
}

func recordPending(db *gorm.DB, state *data.SmilesSyncState) error {
	state.State = data.SmilesSyncPending
	detail := "conta sem sessão da Smiles; aguardando conexão"
	state.Detail = &detail
	return upsertState(db, state)
}

// recordFailure increments the failure counter and raises the account for attention once it
// has failed failureThreshold rounds in a row. A rate limit is spelled out because its fix is
// waiting, not touching anything.
func recordFailure(db *gorm.DB, state *data.SmilesSyncState, cause error) error {
	state.ConsecutiveFailures++
	detail := cause.Error()
	if errors.Is(cause, smiles.ErrRateLimited) {
		detail = "a Smiles está limitando as consultas; a próxima rodada tenta de novo mais devagar: " + detail
	}
	state.Detail = &detail
	if state.ConsecutiveFailures >= failureThreshold {
		state.State = data.SmilesSyncNeedsAttention
	} else {
		state.State = data.SmilesSyncError
	}
	if err := upsertState(db, state); err != nil {
		return err
	}
	return cause
}

// --- small helpers ---

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
