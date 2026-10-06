package data

import "time"

// The states one account's LATAM balance read can be in. Stored in English like every other
// enum in the schema; the Portuguese labels live only on the panel. Mirrors the
// latam_sync_state_kind enum in migration 81.
const (
	// LatamSyncPending is a linked account that has never been read.
	LatamSyncPending = "pending"
	// LatamSyncOK is the last round reading the balance.
	LatamSyncOK = "ok"
	// LatamSyncError is a transport failure; it is retried on the next tick.
	LatamSyncError = "error"
	// LatamSyncNeedsLogin is the stored session having been refused. Only a new login on
	// LATAM's own page fixes it, so the panel raises it for the operator.
	LatamSyncNeedsLogin = "needs_login"
	// LatamSyncNeedsAttention is enough failed rounds in a row that the account is worth a look.
	LatamSyncNeedsAttention = "needs_attention"
)

// LatamSyncState is one account's balance-read lifecycle. One row per account, keyed by the
// account itself. See migration 81.
//
// It is SmilesSyncState without the statement: no last balance (the figure lives in
// accounts.real_balance_points and decides nothing here) and no window.
type LatamSyncState struct {
	AccountID string   `gorm:"column:account_id;type:uuid;primaryKey" json:"account_id"`
	Account   *Account `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`

	// LastSyncedAt is the last successful read. Nil until the first one.
	LastSyncedAt *time.Time `gorm:"column:last_synced_at" json:"last_synced_at"`

	State string `gorm:"column:state;type:latam_sync_state_kind;not null;default:'pending'" json:"state"`

	// ConsecutiveFailures drives the threshold that flips State to needs_attention. Reset to
	// zero on any success.
	ConsecutiveFailures int `gorm:"column:consecutive_failures;not null;default:0" json:"consecutive_failures"`

	// Detail is a short human-readable note for the panel (the last error, or why a login is
	// needed). Nothing parses it.
	Detail *string `gorm:"column:detail" json:"detail"`

	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (LatamSyncState) TableName() string {
	return "latam_sync_state"
}
