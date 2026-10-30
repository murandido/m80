// Package smilessync runs one reconciliation round per Smiles account: renew the session,
// read the balance, and when it has moved, pull the last month's statement and store the
// window. It is the orchestration that ties internal/smiles (the API client) to the
// smiles_statement_lines and smiles_sync_state tables, and it is the twin of internal/azulsync.
//
// Two things make it its own package rather than a parameter of that one.
//
// The session. Azul stores an identifier and reads with a technical token anyone can mint;
// Smiles stores a live credential that the tenant rotates on every renewal. So a round starts
// by renewing and persisting, serialised per account, and a refused renewal is not an error to
// retry - it is an account waiting on a human to type a verification code.
//
// The absence of a balance to check against. Azul validates a window before storing it,
// because every statement day carries its closing balance. Smiles publishes no such figure
// anywhere, so there is nothing to validate and no "inconsistent" state; what protects a round
// is re-reading the balance afterwards and postponing on a torn read.
package smilessync

import (
	"errors"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// loadOrInitState reads the account's sync state, returning a fresh pending one if the account
// has never been synced. The row is not created here - it is written once, with the round's
// outcome, by upsertState.
func loadOrInitState(db *gorm.DB, accountID string) (*data.SmilesSyncState, error) {
	var state data.SmilesSyncState
	err := db.First(&state, "account_id = ?", accountID).Error
	if err == nil {
		return &state, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &data.SmilesSyncState{AccountID: accountID, State: data.SmilesSyncPending}, nil
	}
	return nil, err
}

// LoadState returns the account's sync state, or a fresh pending one if it has never been
// synced. It is the read the panel uses to draw the badge; like loadOrInitState it never writes
// a row.
func LoadState(db *gorm.DB, accountID string) (*data.SmilesSyncState, error) {
	return loadOrInitState(db, accountID)
}

// upsertState writes the state, inserting it the first time and overwriting it thereafter. The
// account id is the primary key, so there is exactly one row per account to keep current.
func upsertState(db *gorm.DB, state *data.SmilesSyncState) error {
	state.UpdatedAt = time.Now()
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}},
		UpdateAll: true,
	}).Create(state).Error
}

// MarkConnected records that an account was just connected (its session stored). It leaves the
// state pending with the failure counter cleared, so the first real round runs next. Used by
// the connect endpoint after a verification code is accepted.
func MarkConnected(db *gorm.DB, accountID string) error {
	state, err := loadOrInitState(db, accountID)
	if err != nil {
		return err
	}
	state.State = data.SmilesSyncPending
	state.ConsecutiveFailures = 0
	detail := "conta conectada; aguardando a primeira sincronização"
	state.Detail = &detail
	return upsertState(db, state)
}

// MarkNeedsMFA records that the stored session is no longer accepted, so the account is waiting
// on a human to type a new verification code.
//
// It clears the failure counter on purpose: this is not a run of bad luck that might pass, it
// is a terminal state with exactly one exit, and letting it accumulate toward needs_attention
// would replace a precise message ("the code expired, connect again") with a vague one.
func MarkNeedsMFA(db *gorm.DB, accountID, detail string) error {
	state, err := loadOrInitState(db, accountID)
	if err != nil {
		return err
	}
	state.State = data.SmilesSyncNeedsMFA
	state.ConsecutiveFailures = 0
	state.Detail = &detail
	return upsertState(db, state)
}

// MarkDisconnected drops the account's sync bookkeeping, for when an operator disconnects it.
// The stored session and the statement window are removed by the caller; without this the panel
// would keep showing the badge of a sync that no longer exists.
func MarkDisconnected(db *gorm.DB, accountID string) error {
	return db.Where("account_id = ?", accountID).Delete(&data.SmilesSyncState{}).Error
}

// storeSession persists the rotated refresh token, and nothing else.
//
// It updates the one column rather than saving the loaded account struct, for the same reason
// setRealBalance does: saving the struct would re-encrypt every encrypted field as a side
// effect. Here it matters more than usual - the account also carries the password.
//
// The write is what makes rotation safe. The tenant refuses the token it just replaced, so a
// round that renews and fails to store has locked the account out of the next one.
func storeSession(db *gorm.DB, accountID, refreshToken string) error {
	// A session with no refresh token cannot renew itself. Storing it would claim a connection
	// that the next round has to take back with "conta sem sessão" - Smiles answers this way
	// for a blocked account and for a holder with no second factor (measured 2026-10-30), so
	// the connect flow reports it instead of saving a credential that never existed.
	if strings.TrimSpace(refreshToken) == "" {
		return errors.New("smilessync: refusing to store an empty refresh token")
	}
	// Encrypt explicitly: a column update does not run the model's BeforeSave hook, so the
	// hook that keeps accounts.password out of the clear would not fire for this path.
	encrypted, err := utils.Encrypt(refreshToken)
	if err != nil {
		return err
	}
	return db.Model(&data.Account{}).
		Where("id = ?", accountID).
		Update("smiles_refresh_token", encrypted).Error
}

// clearSession forgets the stored session, leaving the account disconnected.
func clearSession(db *gorm.DB, accountID string) error {
	return db.Model(&data.Account{}).
		Where("id = ?", accountID).
		Update("smiles_refresh_token", nil).Error
}

// setRealBalance fills the account's real_balance_points and the day it was read - the same
// field the Azul sync keeps current, and the same field a human used to copy by hand. Only
// those two columns are touched, so the encrypted fields are never re-encrypted as a side
// effect.
func setRealBalance(db *gorm.DB, accountID string, miles int, on time.Time) error {
	return db.Model(&data.Account{}).
		Where("id = ?", accountID).
		Updates(map[string]any{
			"real_balance_points":     miles,
			"real_balance_checked_on": on,
		}).Error
}
