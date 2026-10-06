package latamsync

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/latam"
	"gorm.io/gorm"
)

const (
	// DefaultInterval is how often the balances are read. A balance is all there is to read, so
	// there is no reason to ask as often as the statement syncs do - and no env to tune it: the
	// read has no switch of its own either.
	DefaultInterval = 30 * time.Minute
	// maxBackoff caps how far the loop backs off after the service pushes back.
	maxBackoff = time.Hour
	// betweenAccounts is a small pause between accounts in a pass. Each account costs a renewal
	// and one read; firing them back to back is how a rate limit is found.
	betweenAccounts = 3 * time.Second
)

// Loop reads the linked LATAM accounts on a ticker, one pass at a time. Unlike the Azul and
// Smiles loops it has no switch: with no account linked a pass is one empty query.
type Loop struct {
	db       *gorm.DB
	reader   Reader
	guard    *SessionGuard
	interval time.Duration
	log      *slog.Logger
	now      func() time.Time
	// backoff is the extra wait after a pass was rate limited; it grows each time and resets to
	// zero on a clean pass.
	backoff time.Duration
}

// NewLoop builds the loop. A non-positive interval falls back to DefaultInterval. The guard is
// the one the connect handler shares.
func NewLoop(db *gorm.DB, reader Reader, guard *SessionGuard, interval time.Duration) *Loop {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Loop{db: db, reader: reader, guard: guard, interval: interval, log: slog.Default(), now: time.Now}
}

// Run blocks until ctx is cancelled. Passes never overlap: the next timer is set only after a
// pass returns.
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

// runPass reads every linked account once and returns how long to wait before the next pass.
func (l *Loop) runPass(ctx context.Context) time.Duration {
	accounts, err := eligibleAccounts(l.db)
	if err != nil {
		l.log.Error("latamsync: listing accounts", "err", err)
		return l.interval
	}

	for _, account := range accounts {
		if ctx.Err() != nil {
			return l.interval
		}
		if err := SyncAccount(ctx, l.db, l.reader, l.guard, account, l.now()); err != nil {
			// The account id is enough to find the account; nothing about the session is logged.
			l.log.Warn("latamsync: account round failed", "account_id", account.ID, "err", err)

			if errors.Is(err, latam.ErrRateLimited) {
				l.backoff = growBackoff(l.backoff, l.interval)
				l.log.Warn("latamsync: backing off after a rate limit", "wait", l.backoff)
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

// eligibleAccounts are the linked accounts whose session is still worth renewing: every account
// with a stored session, except the ones already waiting on a new login - renewing a refused
// token again cannot succeed.
func eligibleAccounts(db *gorm.DB) ([]data.Account, error) {
	var accounts []data.Account
	err := db.
		Where("latam_refresh_token IS NOT NULL").
		Where("NOT EXISTS (SELECT 1 FROM latam_sync_state s WHERE s.account_id = accounts.id AND s.state = ?)",
			data.LatamSyncNeedsLogin).
		Find(&accounts).Error
	return accounts, err
}
