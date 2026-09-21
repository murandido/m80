package smilessync

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/smiles"
	"gorm.io/gorm"
)

const (
	// DefaultInterval is how often the balance is read when SMILES_SYNC_INTERVAL is unset.
	DefaultInterval = 10 * time.Minute
	// maxBackoff caps how far the loop backs off after the service pushes back. An hour is
	// long enough to stop asking and short enough that a transient limit clears within a
	// working day.
	maxBackoff = time.Hour
	// betweenAccounts is a small pause between accounts in a pass. Each account costs a
	// renewal plus three reads, and firing them back to back is how a rate limit is found.
	betweenAccounts = 3 * time.Second
)

// Loop reads the connected Smiles accounts on a ticker, one pass at a time. It is created
// once in cmd/api and started behind SMILES_SYNC_ENABLED; with the flag off it is never
// constructed, so the whole feature ships inert.
type Loop struct {
	db       *gorm.DB
	reader   Reader
	guard    *SessionGuard
	interval time.Duration
	log      *slog.Logger
	now      func() time.Time
	// backoff is the extra wait after a pass was rate limited; it grows each time and resets
	// to zero on a clean pass.
	backoff time.Duration
}

// NewLoop builds the loop. A non-positive interval falls back to DefaultInterval. The guard is
// the one the connect handlers share, so a pass and an operator connecting cannot renew the
// same session at once.
func NewLoop(db *gorm.DB, reader Reader, guard *SessionGuard, interval time.Duration) *Loop {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Loop{
		db:       db,
		reader:   reader,
		guard:    guard,
		interval: interval,
		log:      slog.Default(),
		now:      time.Now,
	}
}

// Run drives the loop until ctx is cancelled. It runs a first pass almost immediately, then
// waits interval between passes - longer when the service is pushing back. Passes never
// overlap: the next timer is set only after a pass returns.
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

// runPass reads every eligible account once and returns how long to wait before the next pass.
// A pass that gets rate limited stops early and returns a growing backoff; a clean pass returns
// the plain interval.
func (l *Loop) runPass(ctx context.Context) time.Duration {
	accounts, err := eligibleAccounts(l.db)
	if err != nil {
		l.log.Error("smilessync: listing accounts", "err", err)
		return l.interval
	}

	for _, account := range accounts {
		if ctx.Err() != nil {
			return l.interval
		}
		if err := SyncAccount(ctx, l.db, l.reader, l.guard, account, l.now()); err != nil {
			// The account id is enough to find the account; nothing about the session or the
			// credentials is ever logged.
			l.log.Warn("smilessync: account round failed", "account_id", account.ID, "err", err)

			if errors.Is(err, smiles.ErrSessionExpired) {
				// Already recorded as needs_mfa, and nothing about it is transient: carry on
				// with the other accounts rather than treating it as a bad pass.
				l.sleep(ctx, betweenAccounts)
				continue
			}
			if errors.Is(err, smiles.ErrRateLimited) {
				// Stop this pass rather than march through the rest of the accounts making
				// it worse.
				l.backoff = growBackoff(l.backoff, l.interval)
				l.log.Warn("smilessync: backing off after a rate limit", "wait", l.backoff)
				return l.backoff
			}
		}
		l.sleep(ctx, betweenAccounts)
	}

	l.backoff = 0
	return l.interval
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

// eligibleAccounts are the connected inventory accounts: they own their miles inventory and
// have a session to renew. An account without one stays pending until it is connected, and is
// not worth a round.
func eligibleAccounts(db *gorm.DB) ([]data.Account, error) {
	var accounts []data.Account
	err := db.
		Where("owns_point_inventory = ? AND smiles_refresh_token IS NOT NULL", true).
		Find(&accounts).Error
	return accounts, err
}
