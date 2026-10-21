package azulsync

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/carriers/azul"
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

// ErrCustomerKeyRefused is the balance read answering 404: Azul no longer recognises the stored
// customerKey. Distinct from a 404 anywhere else (the statement) because only this one parks the
// account and is worth a login.
var ErrCustomerKeyRefused = errors.New("azulsync: Azul no longer recognises the customerKey")

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
//  1. read the balance (the absolute truth); fill real_balance_points from it. An account that
//     does not track its statement stops here
//  2. if the balance has not moved since last time, there is nothing new to pull - done
//  3. otherwise pull only today and yesterday (Brazilian days), read the balance AGAIN (a change
//     mid-pull means a torn read: postpone), and add the movements not stored yet. If the stored
//     balance plus the new movements equals the balance, that is the round
//  4. when there is no stored balance to add to (a first sync, or rows stored before the clock
//     was kept), or the new movements do not add up (a movement filed days back), pull the whole
//     month instead and validate its last-day balance against points/resume; only if they agree
//     is the window replaced. A mismatch leaves the stored window untouched and marks the account
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
	if errors.Is(err, azul.ErrNotFound) {
		// Azul no longer knows this customerKey - in practice the holder changed the password and
		// a new key was issued. No read round can fix that, so it is not counted as a failure.
		if markErr := recordNeedsReconnect(db, state); markErr != nil {
			return markErr
		}
		return fmt.Errorf("%w: %w", ErrCustomerKeyRefused, err)
	}
	if err != nil {
		return recordFailure(db, state, err)
	}

	// A good balance read is worth keeping on its own: it is the number the panel compares
	// against, and it is trustworthy even on a round where the statement is not pulled.
	if err := setRealBalance(db, account.ID, balance.Points, now); err != nil {
		return fmt.Errorf("azulsync: writing real balance: %w", err)
	}

	// An account that does not track its statement stops here: its balance is all a round is for.
	if !account.TracksStatement {
		return recordBalanceOnly(db, state, now)
	}

	if state.LastBalancePoints != nil && *state.LastBalancePoints == balance.Points {
		return recordUnchanged(db, state, balance.Points, now)
	}

	full, err := needsFullPull(db, account.ID, state, windowStart(now))
	if err != nil {
		return fmt.Errorf("azulsync: checking the stored window: %w", err)
	}
	if !full {
		settled, err := syncIncrementally(ctx, db, reader, account.ID, customerKey, state, balance.Points, now)
		if err != nil || settled {
			return err
		}
		// The new movements do not add up to the balance: something was filed outside the last
		// two days. Fall through to the whole month, which is the truth for its period.
	}
	return syncWholeWindow(ctx, db, reader, account.ID, customerKey, state, balance.Points, now)
}

// needsFullPull reports whether a round must read the whole month rather than today and
// yesterday: when there is no stored balance to add new movements to, or when the window still
// holds rows stored before the movement's clock was kept (their signature cannot be compared).
func needsFullPull(db *gorm.DB, accountID string, state *data.AzulSyncState, from time.Time) (bool, error) {
	if state.LastBalancePoints == nil {
		return true, nil
	}
	var clockless int64
	if err := db.Model(&data.AzulStatementLine{}).
		Where("account_id = ? AND occurred_on >= ? AND occurred_at IS NULL", accountID, from).
		Count(&clockless).Error; err != nil {
		return false, err
	}
	return clockless > 0, nil
}

// syncIncrementally reads today and yesterday and stores the movements it has not seen. It
// reports settled=false, having stored nothing, when the new movements do not take the stored
// balance to the current one - the caller then pulls the whole month.
//
// The airline answers by UTC day, so asking from yesterday's date also returns the evening
// before it in Brazil. The comparison covers exactly the span that came back - every stored
// movement from that UTC midnight on - so those evening rows match themselves instead of
// looking new.
func syncIncrementally(ctx context.Context, db *gorm.DB, reader Reader, accountID, customerKey string,
	state *data.AzulSyncState, balance int, now time.Time) (settled bool, err error) {
	from := azul.BrazilDay(now).AddDate(0, 0, -1)
	txs, postponed, err := pullStatement(ctx, reader, customerKey, from, now, balance)
	if err != nil {
		return true, recordFailure(db, state, err)
	}
	if postponed {
		return true, recordPostponed(db, state)
	}

	var stored []data.AzulStatementLine
	if err := db.Where("account_id = ? AND occurred_at >= ?", accountID, from).
		Find(&stored).Error; err != nil {
		return true, fmt.Errorf("azulsync: reading the stored movements: %w", err)
	}
	fresh := newMovements(stored, toStatementLines(accountID, txs))

	net := 0
	for _, line := range fresh {
		net += line.Points
	}
	if *state.LastBalancePoints+net != balance {
		return false, nil
	}
	if err := data.InsertAzulStatementLines(db, accountID, fresh); err != nil {
		return true, recordFailure(db, state, fmt.Errorf("adding the new movements: %w", err))
	}
	return true, recordSynced(db, state, balance, now)
}

// syncWholeWindow reads the whole month and replaces the stored window with it, after checking
// its last day against the balance.
func syncWholeWindow(ctx context.Context, db *gorm.DB, reader Reader, accountID, customerKey string,
	state *data.AzulSyncState, balance int, now time.Time) error {
	from := windowStart(now)
	txs, postponed, err := pullStatement(ctx, reader, customerKey, from, now, balance)
	if err != nil {
		return recordFailure(db, state, err)
	}
	if postponed {
		return recordPostponed(db, state)
	}

	lastDay, ok := lastDayBalance(txs)
	if !ok || lastDay != balance {
		// The statement does not add up to the balance the account actually holds. Trust the
		// balance: leave the stored window alone and raise it for a human.
		detail := fmt.Sprintf("saldo %d não bate com o último dia do extrato (%d)", balance, lastDay)
		if !ok {
			detail = fmt.Sprintf("extrato sem movimentações para validar contra o saldo %d", balance)
		}
		return recordInconsistent(db, state, detail)
	}

	// Asking from the window's first UTC date also returns the Brazilian evening before it. That
	// day is outside the window, so it is neither deleted nor rewritten: keeping its evening rows
	// here would store them twice.
	lines := make([]data.AzulStatementLine, 0, len(txs))
	for _, line := range toStatementLines(accountID, txs) {
		// A movement whose date did not parse has no day to judge by; keep it, so the layout
		// surprise shows up on the page instead of vanishing.
		if line.OccurredAt == nil || !line.OccurredOn.Before(from) {
			lines = append(lines, line)
		}
	}
	if err := data.ReplaceAzulStatementWindow(db, accountID, from, lines); err != nil {
		return recordFailure(db, state, fmt.Errorf("replacing the window: %w", err))
	}
	return recordSynced(db, state, balance, now)
}

// pullStatement reads the statement from the given day to now, then reads the balance AGAIN: if
// points moved while it was paging, what it holds is a torn read, reported as postponed so the
// next tick starts clean.
func pullStatement(ctx context.Context, reader Reader, customerKey string, from, now time.Time,
	balance int) (txs []azul.Transaction, postponed bool, err error) {
	txs, err = reader.LegacyStatement(ctx, customerKey, from, now)
	if err != nil {
		return nil, false, err
	}
	after, err := reader.Balance(ctx, customerKey)
	if err != nil {
		return nil, false, err
	}
	return txs, after.Points != balance, nil
}

// newMovements returns the fetched lines that are not stored yet. The airline has no movement id
// (transactionId is always 0) and identical movements on one day are legitimate - four
// passengers at the same fare - so this compares counts, not presence: two stored and three
// fetched with one signature means one is new. A stored line the response no longer carries is
// never treated as removed; the airline caps a response at 100 movements, and a busy day's oldest
// ones, stored by earlier ticks, can fall off it.
func newMovements(stored, fetched []data.AzulStatementLine) []data.AzulStatementLine {
	seen := make(map[string]int, len(stored))
	for _, line := range stored {
		seen[movementSignature(line)]++
	}
	var fresh []data.AzulStatementLine
	for _, line := range fetched {
		signature := movementSignature(line)
		if seen[signature] > 0 {
			seen[signature]--
			continue
		}
		fresh = append(fresh, line)
	}
	return fresh
}

// movementSignature identifies a movement by the airline's own fields only. Nothing the panel
// derives (locator, route, kind) goes in: that parsing has changed before, and a changed
// derivation would make every stored line look new.
func movementSignature(line data.AzulStatementLine) string {
	instant := ""
	if line.OccurredAt != nil {
		instant = line.OccurredAt.UTC().Format(time.RFC3339)
	}
	return strings.Join([]string{
		instant, deref(line.TypeCode), strconv.Itoa(line.Points), deref(line.PartnerName), deref(line.Comment),
	}, "\x1f")
}

// windowStart is the first Brazilian day a whole-month round covers.
func windowStart(now time.Time) time.Time {
	return azul.BrazilDay(now).AddDate(0, 0, -windowDays)
}

// lastDayBalance returns the balance of the most recent day in the statement, which is the
// figure that must equal points/resume. Every line of a day carries that day's balance, and the
// latest day's is the one figure the airline reports as an absolute (earlier days are relative
// to their own response), so the latest movement's DayBalance is the whole account balance.
func lastDayBalance(txs []azul.Transaction) (int, bool) {
	if len(txs) == 0 {
		return 0, false
	}
	latest := txs[0]
	for _, t := range txs[1:] {
		if t.OccurredAt.After(latest.OccurredAt) {
			latest = t
		}
	}
	return latest.DayBalance, true
}

// toStatementLines maps the read transactions onto storable rows, deriving the panel-terms kind
// from the airline's type code.
//
// Lines are ordered by their instant; it also numbers each line within the airline's own (UTC)
// day, in the order the response gave them - newest first - so the day's last line is its oldest
// and gets 0. That position only breaks ties between movements stamped with the same second (a
// round trip's two legs often are). A UTC day always arrives whole in one response, because
// LegacyStatement splits a window on day boundaries.
func toStatementLines(accountID string, txs []azul.Transaction) []data.AzulStatementLine {
	airlineDay := func(t azul.Transaction) time.Time { return t.OccurredAt.Truncate(24 * time.Hour) }
	perDay := make(map[time.Time]int, len(txs))
	for _, t := range txs {
		perDay[airlineDay(t)]++
	}
	seenInDay := make(map[time.Time]int, len(perDay))

	lines := make([]data.AzulStatementLine, 0, len(txs))
	for _, t := range txs {
		day := airlineDay(t)
		position := perDay[day] - 1 - seenInDay[day]
		seenInDay[day]++
		lines = append(lines, data.AzulStatementLine{
			AccountID:       accountID,
			OccurredOn:      t.Date,
			OccurredAt:      timePtr(t.OccurredAt),
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

// recordBalanceOnly closes a round on an account that does not track its statement: the balance was
// read and written to real_balance_points, and nothing else was. LastBalancePoints is deliberately left
// alone - it is the statement's bookmark, the balance the stored window was last reconciled to.
// Moving it here would make the first round after "Acompanhar extrato" is switched on see an
// "unchanged" balance and skip the statement it was switched on to read.
func recordBalanceOnly(db *gorm.DB, state *data.AzulSyncState, now time.Time) error {
	state.State = data.AzulSyncOK
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

// recordNeedsReconnect parks the account until a login stores a new customerKey: the ticker skips
// it from then on (see eligibleAccounts). It clears the failure counter on purpose, like
// smilessync.MarkNeedsMFA: this is a terminal state with exactly one exit, and letting it pile up
// toward needs_attention would trade a precise message for a vague one.
func recordNeedsReconnect(db *gorm.DB, state *data.AzulSyncState) error {
	state.State = data.AzulSyncNeedsReconnect
	state.ConsecutiveFailures = 0
	detail := "a Azul não reconhece mais esta conta (a senha provavelmente mudou)"
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
		detail = "bloqueado no edge - verificar User-Agent: " + detail
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

// timePtr is nil for the zero time, which is what an unparseable movement date comes back as.
func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
