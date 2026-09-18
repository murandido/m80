package azulsync

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/murandido/m-80-backend/internal/azul"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

const (
	// DefaultInterval is how often the balance is polled when AZUL_SYNC_INTERVAL is unset.
	DefaultInterval = 10 * time.Minute
	// maxBackoff caps how far the loop backs off after the edge pushes back. An hour is long
	// enough to stop hammering edge and short enough that a transient block clears on its own
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
	db       *gorm.DB
	reader   Reader
	interval time.Duration
	log      *slog.Logger
	now      func() time.Time
	// backoff is the extra wait after a pass hit the edge (403/429); it grows each time and
	// resets to zero on a clean pass.
	backoff time.Duration
}

// NewLoop builds the loop. A non-positive interval falls back to DefaultInterval.
func NewLoop(db *gorm.DB, reader Reader, interval time.Duration) *Loop {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Loop{
		db:       db,
		reader:   reader,
		interval: interval,
		log:      slog.Default(),
		now:      time.Now,
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

// eligibleAccounts are the connected inventory accounts: they own their point inventory and
// have a customerKey to read. Accounts without a key stay pending until login (C7) fills it,
// and are not worth a round.
func eligibleAccounts(db *gorm.DB) ([]data.Account, error) {
	var accounts []data.Account
	err := db.
		Where("owns_point_inventory = ? AND azul_customer_key IS NOT NULL", true).
		Find(&accounts).Error
	return accounts, err
}
