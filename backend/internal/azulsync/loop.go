package azulsync

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/murandido/m-80-backend/carriers/azul"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

const (
	// DefaultInterval is how often the balance is polled when AZUL_SYNC_INTERVAL is unset.
	DefaultInterval = 10 * time.Minute
	// maxBackoff caps how far the loop backs off after the edge pushes back. An hour is long
	// enough to stop hammering the edge and short enough that a transient block clears on its own
	// within a working day.
	maxBackoff = time.Hour
	// betweenAccounts is a small pause between accounts in a pass, to be gentle on the edge the
	// collector shares with the promo service rather than firing every account back to back.
	betweenAccounts = 3 * time.Second
)

// Loop polls the connected Azul accounts on a ticker, one pass at a time, and reconciles each.
// It is created once in cmd/api and started behind AZUL_SYNC_ENABLED; with the flag off it is
// never constructed, so the whole feature ships inert.
type Loop struct {
	db     *gorm.DB
	reader Reader
	// connector starts the one automatic login an account gets when Azul stops recognising its
	// key. Nil leaves the account parked for the operator (the tests that do not exercise it).
	connector ConnectStarter
	interval  time.Duration
	log       *slog.Logger
	now       func() time.Time
	// backoff is the extra wait after a pass hit the edge (403/429); it grows each time and
	// resets to zero on a clean pass.
	backoff time.Duration
}

// ConnectStarter is the slice of Connector the loop needs; an interface so the loop tests can count
// the starts without a login service.
type ConnectStarter interface {
	Start(ctx context.Context, accountID string) error
}

// NewLoop builds the loop. A non-positive interval falls back to DefaultInterval. connector may be
// nil.
func NewLoop(db *gorm.DB, reader Reader, connector ConnectStarter, interval time.Duration) *Loop {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Loop{
		db:        db,
		reader:    reader,
		connector: connector,
		interval:  interval,
		log:       slog.Default(),
		now:       time.Now,
	}
}

// Run drives the loop until ctx is cancelled. It runs a first pass almost immediately, then
// waits interval between passes - longer when the edge is pushing back. Passes never overlap:
// the next timer is set only after a pass returns.
func (l *Loop) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			timer.Reset(l.runPass(ctx))
		}
	}
}

// runPass reconciles every eligible account once and returns how long to wait before the next
// pass. A pass that the edge blocks stops early and returns a growing backoff; a clean pass
// returns the plain interval.
func (l *Loop) runPass(ctx context.Context) time.Duration {
	accounts, err := eligibleAccounts(l.db)
	if err != nil {
		l.log.Error("azulsync: listing accounts", "err", err)
		return l.interval
	}

	for _, account := range accounts {
		if ctx.Err() != nil {
			return l.interval
		}
		if err := SyncAccount(ctx, l.db, l.reader, account, l.now()); err != nil {
			l.log.Warn("azulsync: account round failed", "account_id", account.ID, "err", err)
			if errors.Is(err, ErrCustomerKeyRefused) {
				// The key was refused and the account is now parked (needs_reconnect), so the next
				// pass skips it. Try a login once with the stored password - the usual cause is a
				// password change already saved on the account. If it fails it stays parked, and
				// only the operator or a new password tries again: repeated logins with a wrong
				// password are how an account gets locked.
				l.reconnect(ctx, account.ID)
			}
			if errors.Is(err, azul.ErrEdgeBlocked) || errors.Is(err, azul.ErrRateLimited) {
				// The edge is pushing back on the shared IP. Stop this pass and back off
				// rather than march through the rest of the accounts making it worse.
				l.backoff = growBackoff(l.backoff, l.interval)
				l.log.Warn("azulsync: backing off after an edge block", "wait", l.backoff)
				return l.backoff
			}
		}
		l.sleep(ctx, betweenAccounts)
	}

	l.backoff = 0
	return l.interval
}

// reconnect starts the automatic login for an account whose key was refused.
func (l *Loop) reconnect(ctx context.Context, accountID string) {
	if l.connector == nil {
		return
	}
	if err := l.connector.Start(ctx, accountID); err != nil && !errors.Is(err, ErrAlreadyConnecting) {
		l.log.Warn("azulsync: could not start the automatic reconnect", "account_id", accountID, "err", err)
		return
	}
	l.log.Info("azulsync: customerKey refused; reconnecting once with the stored password", "account_id", accountID)
}

// sleep waits d, or returns early if the context is cancelled.
func (l *Loop) sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// growBackoff doubles the current backoff, starting from the interval, capped at maxBackoff.
func growBackoff(current, interval time.Duration) time.Duration {
	next := current * 2
	if next <= 0 {
		next = interval
	}
	if next > maxBackoff {
		next = maxBackoff
	}
	return next
}

// eligibleAccounts are the connected accounts: every account with a customerKey to read, whether
// or not it tracks its statement - the balance is read for all of them, and SyncAccount decides
// per account whether the statement is pulled too. Accounts without a key stay pending until a
// connect fills it, and are not worth a round; neither are accounts parked in needs_reconnect.
func eligibleAccounts(db *gorm.DB) ([]data.Account, error) {
	var accounts []data.Account
	err := db.
		Where("azul_customer_key IS NOT NULL").
		// A key Azul refused is not read again: no round can fix it, only a login (which puts
		// the state back to pending). This is what stops the ticker hammering a dead key.
		Where("NOT EXISTS (SELECT 1 FROM azul_sync_state s WHERE s.account_id = accounts.id AND s.state = ?)",
			data.AzulSyncNeedsReconnect).
		Find(&accounts).Error
	return accounts, err
}
