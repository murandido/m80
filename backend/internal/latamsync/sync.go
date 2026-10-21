package latamsync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/carriers/latam"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// failureThreshold is how many rounds in a row may fail before the account is raised for
// attention on the panel. Below it a failure is transient and just retried on the next tick.
const failureThreshold = 5

// Reader is the slice of latam.AccountClient a round needs. An interface so a round can be tested
// without the live tenant.
type Reader interface {
	Refresh(ctx context.Context, refreshToken string) (latam.AccountSession, error)
	Balance(ctx context.Context, session latam.AccountSession) (latam.AccountBalance, error)
}

// SyncAccount runs one round for one account and records the outcome in latam_sync_state:
//  1. renew the session and store what comes back at once - a rotated token makes the old one
//     useless, so a round that reads first and crashes would lock the account out
//  2. read the balance and write it to real_balance_points
//
// It returns the terminal error (if any) for the caller to log; the panel-visible state is always
// written regardless of the return.
func SyncAccount(ctx context.Context, db *gorm.DB, reader Reader, guard *SessionGuard, account data.Account, now time.Time) error {
	unlock := guard.Lock(account.ID)
	defer unlock()
	return syncLocked(ctx, db, reader, account.ID, deref(account.LatamRefreshToken), now)
}

// syncLocked is the round itself, for a caller already holding the account's lock.
func syncLocked(ctx context.Context, db *gorm.DB, reader Reader, accountID, refreshToken string, now time.Time) error {
	state, err := loadOrInitState(db, accountID)
	if err != nil {
		return fmt.Errorf("latamsync: loading state: %w", err)
	}
	if refreshToken == "" {
		// Not linked: nothing to read.
		return recordPending(db, state)
	}

	session, err := reader.Refresh(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, latam.ErrSessionExpired) {
			detail := "a sessão da LATAM não é mais aceita; conecte a conta de novo"
			if markErr := markNeedsLogin(db, accountID, detail); markErr != nil {
				return markErr
			}
			return err
		}
		return recordFailure(db, state, err)
	}
	if session.RefreshToken != refreshToken {
		if err := storeSession(db, accountID, session.RefreshToken); err != nil {
			return fmt.Errorf("latamsync: storing the rotated session: %w", err)
		}
	}

	balance, err := reader.Balance(ctx, session)
	if err != nil {
		return recordFailure(db, state, err)
	}
	if err := setRealBalance(db, accountID, balance.Miles, now); err != nil {
		return fmt.Errorf("latamsync: writing real balance: %w", err)
	}
	return recordSynced(db, state, now)
}

// Connect stores a freshly linked session and reads the balance once, so the panel shows it
// without waiting for the next tick. A failed first read leaves the account linked - the ticker
// retries it - and is returned only for the caller to log.
func Connect(ctx context.Context, db *gorm.DB, reader Reader, guard *SessionGuard, accountID string, session latam.AccountSession, now time.Time) error {
	unlock := guard.Lock(accountID)
	defer unlock()

	if err := storeSession(db, accountID, session.RefreshToken); err != nil {
		return fmt.Errorf("latamsync: storing the session: %w", err)
	}
	if err := markConnected(db, accountID); err != nil {
		return fmt.Errorf("latamsync: recording the connection: %w", err)
	}
	return syncLocked(ctx, db, reader, accountID, session.RefreshToken, now)
}

// Disconnect forgets an account's session and its state. The balance already read stays in
// real_balance_points: it was true on the day it says.
func Disconnect(db *gorm.DB, guard *SessionGuard, accountID string) error {
	unlock := guard.Lock(accountID)
	defer unlock()

	if err := db.Model(&data.Account{}).
		Where("id = ?", accountID).
		Update("latam_refresh_token", nil).Error; err != nil {
		return fmt.Errorf("latamsync: clearing the session: %w", err)
	}
	return db.Where("account_id = ?", accountID).Delete(&data.LatamSyncState{}).Error
}

// --- state transitions, each writing exactly one row ---

func recordSynced(db *gorm.DB, state *data.LatamSyncState, now time.Time) error {
	state.State = data.LatamSyncOK
	state.LastSyncedAt = &now
	state.ConsecutiveFailures = 0
	state.Detail = nil
	return upsertState(db, state)
}

func recordPending(db *gorm.DB, state *data.LatamSyncState) error {
	state.State = data.LatamSyncPending
	detail := "conta sem sessão da LATAM; aguardando conexão"
	state.Detail = &detail
	return upsertState(db, state)
}

// recordFailure increments the failure counter and raises the account for attention once it has
// failed failureThreshold rounds in a row.
func recordFailure(db *gorm.DB, state *data.LatamSyncState, cause error) error {
	state.ConsecutiveFailures++
	detail := cause.Error()
	if errors.Is(cause, latam.ErrRateLimited) {
		detail = "a LATAM está limitando as consultas; a próxima rodada tenta de novo mais devagar: " + detail
	}
	state.Detail = &detail
	if state.ConsecutiveFailures >= failureThreshold {
		state.State = data.LatamSyncNeedsAttention
	} else {
		state.State = data.LatamSyncError
	}
	if err := upsertState(db, state); err != nil {
		return err
	}
	return cause
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
