// Package latamsync keeps the LATAM Pass balance of every linked account current: renew the
// session, read the balance, write it to accounts.real_balance_points. It ties internal/latam's
// AccountClient to the latam_sync_state table.
//
// It is internal/smilessync without the statement. LATAM is read for its balance only - the
// statement stays an Azul and Smiles feature - so a round is a renewal and one read, and there is
// no window, no torn read and no "Acompanhar extrato" to consult: every linked account is read.
//
// What it keeps from smilessync is the session discipline. The stored credential is a refresh
// token that Auth0 may rotate on a renewal, so a round renews and persists before anything else,
// serialised per account, and a refused renewal is not an error to retry - it is an account
// waiting on a human to log in on LATAM's page again.
package latamsync

import (
	"errors"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// loadOrInitState reads the account's state, returning a fresh pending one if the account has
// never been read. The row is not created here - it is written once, with the round's outcome,
// by upsertState.
func loadOrInitState(db *gorm.DB, accountID string) (*data.LatamSyncState, error) {
	var state data.LatamSyncState
	err := db.First(&state, "account_id = ?", accountID).Error
	if err == nil {
		return &state, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &data.LatamSyncState{AccountID: accountID, State: data.LatamSyncPending}, nil
	}
	return nil, err
}

// LoadState returns the account's state, or a fresh pending one if it has never been read. It is
// the read the panel uses to draw the badge, and never writes a row.
func LoadState(db *gorm.DB, accountID string) (*data.LatamSyncState, error) {
	return loadOrInitState(db, accountID)
}

// upsertState writes the state, inserting it the first time and overwriting it thereafter.
func upsertState(db *gorm.DB, state *data.LatamSyncState) error {
	state.UpdatedAt = time.Now()
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}},
		UpdateAll: true,
	}).Create(state).Error
}

// markConnected records that an account was just linked (its session stored), leaving it pending
// with the failure counter cleared so the first read runs next.
func markConnected(db *gorm.DB, accountID string) error {
	state, err := loadOrInitState(db, accountID)
	if err != nil {
		return err
	}
	state.State = data.LatamSyncPending
	state.ConsecutiveFailures = 0
	detail := "conta conectada; aguardando a primeira leitura do saldo"
	state.Detail = &detail
	return upsertState(db, state)
}

// markNeedsLogin records that the stored session is no longer accepted. It clears the failure
// counter on purpose: this is a terminal state with exactly one exit (a new login), and letting
// it drift into needs_attention would replace a precise message with a vague one.
func markNeedsLogin(db *gorm.DB, accountID, detail string) error {
	state, err := loadOrInitState(db, accountID)
	if err != nil {
		return err
	}
	state.State = data.LatamSyncNeedsLogin
	state.ConsecutiveFailures = 0
	state.Detail = &detail
	return upsertState(db, state)
}

// storeSession persists the refresh token, and nothing else. It updates the one column rather
// than saving the loaded account struct, which would re-encrypt every encrypted field as a side
// effect - and the account also carries the password.
func storeSession(db *gorm.DB, accountID, refreshToken string) error {
	// Encrypt explicitly: a column update does not run the model's BeforeSave hook.
	encrypted, err := utils.Encrypt(refreshToken)
	if err != nil {
		return err
	}
	return db.Model(&data.Account{}).
		Where("id = ?", accountID).
		Update("latam_refresh_token", encrypted).Error
}

// setRealBalance fills the account's real_balance_points and the day it was read - the same field
// the Azul and Smiles syncs keep current. Only those two columns are touched.
func setRealBalance(db *gorm.DB, accountID string, miles int, on time.Time) error {
	return db.Model(&data.Account{}).
		Where("id = ?", accountID).
		Updates(map[string]any{
			"real_balance_points":     miles,
			"real_balance_checked_on": on,
		}).Error
}
