// Package azulsync runs one reconciliation round per Azul account: read the balance, and when
// it has moved, pull the last month's statement, validate it against the balance, and store the
// window. It is the orchestration that ties internal/azul (the read client) to the
// azul_statement_lines and azul_sync_state tables.
//
// The balance is the absolute truth. A statement window is only stored when its last day's
// balance matches points/resume; when it does not, the window is left alone and the account is
// marked inconsistent for a human to look at. Point expiration is carried through to the stored
// lines but kept OUT of the FIFO costing (see internal/statement).
package azulsync

import (
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// loadOrInitState reads the account's sync state, returning a fresh pending one if the account
// has never been synced. The row is not created here - it is written once, with the round's
// outcome, by upsertState.
func loadOrInitState(db *gorm.DB, accountID string) (*data.AzulSyncState, error) {
	var state data.AzulSyncState
	err := db.First(&state, "account_id = ?", accountID).Error
	if err == nil {
		return &state, nil
	}
	if err == gorm.ErrRecordNotFound {
		return &data.AzulSyncState{AccountID: accountID, State: data.AzulSyncPending}, nil
	}
	return nil, err
}

// LoadState returns the account's sync state, or a fresh pending one if it has never been
// synced. It is the read the panel uses to draw the badge; like loadOrInitState it never writes
// a row.
func LoadState(db *gorm.DB, accountID string) (*data.AzulSyncState, error) {
	return loadOrInitState(db, accountID)
}

// upsertState writes the state, inserting it the first time and overwriting it thereafter. The
// account id is the primary key, so there is exactly one row per account to keep current.
func upsertState(db *gorm.DB, state *data.AzulSyncState) error {
	state.UpdatedAt = time.Now()
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}},
		UpdateAll: true,
	}).Create(state).Error
}

// MarkConnected records that an account was just connected (its customerKey stored). It leaves
// the state pending with the failure counter cleared, so the next ticker pass runs the first
// real round. Used by the connect endpoint after a successful login.
func MarkConnected(db *gorm.DB, accountID string) error {
	state, err := loadOrInitState(db, accountID)
	if err != nil {
		return err
	}
	state.State = data.AzulSyncPending
	state.ConsecutiveFailures = 0
	detail := "conta conectada; aguardando a primeira sincronização"
	state.Detail = &detail
	return upsertState(db, state)
}

// MarkLoginFailure records a failed connect attempt. Login failures count toward the same
// threshold as read failures: enough of them in a row raises the account for attention, which
// is the only moment the panel asks a human to step in (re-enter credentials).
func MarkLoginFailure(db *gorm.DB, accountID string, cause error) error {
	state, err := loadOrInitState(db, accountID)
	if err != nil {
		return err
	}
	if state.State == data.AzulSyncNeedsReconnect {
		// Still parked: the old key is dead, so flipping to error would put the account back in
		// the ticker to fail on every round. It stays put until a login succeeds, and the reason
		// is kept for the panel.
		detail := "não reconectou sozinha (" + cause.Error() + "): confira a senha cadastrada e clique em Reconectar"
		state.Detail = &detail
		return upsertState(db, state)
	}
	state.ConsecutiveFailures++
	detail := cause.Error()
	state.Detail = &detail
	if state.ConsecutiveFailures >= failureThreshold {
		state.State = data.AzulSyncNeedsAttention
	} else {
		state.State = data.AzulSyncError
	}
	return upsertState(db, state)
}

// setRealBalance fills the account's real_balance_points and the day it was read. It updates
// just those two columns on the account row rather than saving the loaded struct, so the
// encrypted fields (password, customerKey) are never re-encrypted as a side effect.
//
// This is the field #160 created for a hand-read balance; the sync now keeps it current from
// the airline's own points/resume, which is the same "absolute truth" a human was copying by
// hand.
func setRealBalance(db *gorm.DB, accountID string, points int, on time.Time) error {
	return db.Model(&data.Account{}).
		Where("id = ?", accountID).
		Updates(map[string]any{
			"real_balance_points":     points,
			"real_balance_checked_on": on,
		}).Error
}
