package azulsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/azul"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// windowDays is how far back a sync reads: at most a month, the bound the plan set. The window
// is [now-windowDays, now] and is what ReplaceAzulStatementWindow swaps wholesale.
const windowDays = 31

// failureThreshold is how many rounds in a row may fail before the account is raised for
// attention on the panel. Below it a failure is transient (a blip at the edge, a token that
// died early) and just retried on the next tick.
const failureThreshold = 5

// Reader is the slice of the azul client this package needs. An interface so the round can be
// tested against a fake without a browser or the live edge; internal/azul.Client satisfies it.
type Reader interface {
	Balance(ctx context.Context, customerKey string) (azul.Balance, error)
	LegacyStatement(ctx context.Context, customerKey string, from, to time.Time) ([]azul.Transaction, error)
}

// SyncAccount runs one round for one account and records the outcome in azul_sync_state. It
// returns the terminal error (if any) for the caller to log; the panel-visible state is always
// written regardless of the return.
//
// The shape of a round:
//  1. read the balance (the absolute truth); fill real_balance_points from it
//  2. if the balance has not moved since last time, there is nothing new to pull - done
//  3. otherwise pull the last month's statement, then read the balance AGAIN: if it changed
//     mid-pull the statement we hold is already stale, so postpone rather than store a torn read
//  4. validate the statement's last-day balance against points/resume; only if they agree is
//     the window replaced. A mismatch leaves the stored window untouched and marks the account
//     inconsistent - the balance is trusted, the statement is not
func SyncAccount(ctx context.Context, db *gorm.DB, reader Reader, account data.Account, now time.Time) error {
	state, err := loadOrInitState(db, account.ID)
	if err != nil {
		return fmt.Errorf("azulsync: loading state: %w", err)
	}

	customerKey := deref(account.AzulCustomerKey)
	if customerKey == "" {
		// Not connected yet: nothing to read. Left pending for the login step (C7) to fill.
		return recordPending(db, state)
	}

	balance, err := reader.Balance(ctx, customerKey)
	if err != nil {
		return recordFailure(db, state, err)
	}

	// A good balance read is worth keeping on its own: it is the number the panel compares
	// against, and it is trustworthy even on a round where the statement is not pulled.
	if err := setRealBalance(db, account.ID, balance.Points, now); err != nil {
		return fmt.Errorf("azulsync: writing real balance: %w", err)
	}

	if state.LastBalancePoints != nil && *state.LastBalancePoints == balance.Points {
		return recordUnchanged(db, state, balance.Points, now)
	}

	from := windowStart(now)
	txs, err := reader.LegacyStatement(ctx, customerKey, from, now)
	if err != nil {
		return recordFailure(db, state, err)
	}

	// Read the balance again: if points moved while we were paging the statement, what we hold
	// is a torn read. Postpone without storing - the next tick starts clean.
	after, err := reader.Balance(ctx, customerKey)
	if err != nil {
		return recordFailure(db, state, err)
	}
	if after.Points != balance.Points {
		return recordPostponed(db, state)
	}

	lastDay, ok := lastDayBalance(txs)
	if !ok || lastDay != balance.Points {
		// The statement does not add up to the balance the account actually holds. Trust the
		// balance: leave the stored window alone and raise it for a human.
		detail := fmt.Sprintf("saldo %d não bate com o último dia do extrato (%d)", balance.Points, lastDay)
		if !ok {
			detail = fmt.Sprintf("extrato sem movimentações para validar contra o saldo %d", balance.Points)
		}
		return recordInconsistent(db, state, detail)
	}

	lines := toStatementLines(account.ID, txs)
	if err := data.ReplaceAzulStatementWindow(db, account.ID, from, lines); err != nil {
		return recordFailure(db, state, fmt.Errorf("replacing the window: %w", err))
	}
	return recordSynced(db, state, balance.Points, now)
}

// windowStart is the earliest day a round reads, truncated to a whole day in UTC.
func windowStart(now time.Time) time.Time {
	d := now.AddDate(0, 0, -windowDays)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

// lastDayBalance returns the balance of the most recent day in the statement, which is the
// figure that must equal points/resume. Every line of a day carries that day's balance, and the
// latest day's is the one figure the airline reports as an absolute (earlier days are relative
// to their own response), so the latest-dated line's DayBalance is the whole account balance.
func lastDayBalance(txs []azul.Transaction) (int, bool) {
	if len(txs) == 0 {
		return 0, false
	}
	latest := txs[0]
	for _, t := range txs[1:] {
		if t.Date.After(latest.Date) {
			latest = t
		}
	}
	return latest.DayBalance, true
}

// toStatementLines maps the read transactions onto storable rows, deriving the panel-terms kind
// from the airline's type code.
//
// It also numbers each line within its day. The airline gives no clock, but it returns every day
// newest first (checked against the issuances' own creation times), so a line's position is the
// only real order there is: the day's last line is its oldest and gets 0. A day always arrives
// whole in one response, because LegacyStatement splits a window on day boundaries.
func toStatementLines(accountID string, txs []azul.Transaction) []data.AzulStatementLine {
	perDay := make(map[time.Time]int, len(txs))
	for _, t := range txs {
		perDay[t.Date]++
	}
	seenInDay := make(map[time.Time]int, len(perDay))

	lines := make([]data.AzulStatementLine, 0, len(txs))
	for _, t := range txs {
		position := perDay[t.Date] - 1 - seenInDay[t.Date]
		seenInDay[t.Date]++
		lines = append(lines, data.AzulStatementLine{
			AccountID:       accountID,
			OccurredOn:      t.Date,
			Kind:            data.AzulKindFromTypeCode(t.TypeCode, t.PartnerName+" "+t.TypeDescription, t.Comments),
			TypeCode:        ptrOrNil(t.TypeCode),
			TypeDescription: ptrOrNil(t.TypeDescription),
			Points:          t.Points,
			Locator:         ptrOrNil(t.Locator),
			Origin:          ptrOrNil(t.Origin),
			Destination:     ptrOrNil(t.Destination),
			PartnerName:     ptrOrNil(t.PartnerName),
			Description:     ptrOrNil(firstNonEmpty(t.PartnerName, t.TypeDescription)),
			Comment:         ptrOrNil(t.Comments),
			DayBalance:      intPtr(t.DayBalance),
			PositionInDay:   intPtr(position),
		})
	}
	return lines
}

// --- state transitions, each writing exactly one row ---

func recordSynced(db *gorm.DB, state *data.AzulSyncState, balance int, now time.Time) error {
	state.State = data.AzulSyncOK
	state.LastBalancePoints = &balance
	state.LastSyncedAt = &now
	state.ConsecutiveFailures = 0
	state.Detail = nil
	return upsertState(db, state)
}

func recordUnchanged(db *gorm.DB, state *data.AzulSyncState, balance int, now time.Time) error {
	state.State = data.AzulSyncOK
	state.LastBalancePoints = &balance
	state.LastSyncedAt = &now
	state.ConsecutiveFailures = 0
	state.Detail = nil
	return upsertState(db, state)
}

func recordInconsistent(db *gorm.DB, state *data.AzulSyncState, detail string) error {
	state.State = data.AzulSyncInconsistent
	state.ConsecutiveFailures = 0 // it is not a failure to reach - the data disagrees
	state.Detail = &detail
	return upsertState(db, state)
}

// recordPostponed leaves the state as it was but notes the torn read, without touching
// LastBalancePoints so the next tick re-pulls.
func recordPostponed(db *gorm.DB, state *data.AzulSyncState) error {
	detail := "saldo mudou durante a leitura do extrato; adiado para a próxima rodada"
	state.Detail = &detail
	return upsertState(db, state)
}

func recordPending(db *gorm.DB, state *data.AzulSyncState) error {
	state.State = data.AzulSyncPending
	detail := "conta sem customerKey; aguardando login"
	state.Detail = &detail
	return upsertState(db, state)
}

// recordFailure increments the failure counter and raises the account for attention once it has
// failed failureThreshold rounds in a row. An edge block is spelled out because its fix is a
// transport one, not a credential one.
func recordFailure(db *gorm.DB, state *data.AzulSyncState, cause error) error {
	state.ConsecutiveFailures++
	detail := cause.Error()
	if errors.Is(cause, azul.ErrEdgeBlocked) {
		detail = "bloqueado no edge (edge) - verificar User-Agent: " + detail
	}
	state.Detail = &detail
	if state.ConsecutiveFailures >= failureThreshold {
		state.State = data.AzulSyncNeedsAttention
	} else {
		state.State = data.AzulSyncError
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

func intPtr(i int) *int { return &i }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
