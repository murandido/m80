package data

import "time"

// The states one account's Azul statement sync can be in. Stored in English like every other
// enum in the schema; the Portuguese labels live only on the panel. Mirrors the
// azul_sync_state_kind enum in migrations 54 and 80.
const (
	// AzulSyncPending is a connected account that has never been synced.
	AzulSyncPending = "pending"
	// AzulSyncOK is the last round matching the balance and storing the window.
	AzulSyncOK = "ok"
	// AzulSyncError is a transport or credential failure; it is retried on the next tick.
	AzulSyncError = "error"
	// AzulSyncInconsistent is the statement's last-day balance not matching points/resume. The
	// window is NOT stored in this state - the balance is the absolute truth - and a human is
	// expected to look. See the sync round in internal/azulsync.
	AzulSyncInconsistent = "inconsistent"
	// AzulSyncNeedsAttention is login having failed enough times that the stored credentials
	// are suspect. It is the only state the panel raises for manual intervention.
	AzulSyncNeedsAttention = "needs_attention"
	// AzulSyncNeedsReconnect is the Azul API no longer recognising the stored customerKey
	// (points/resume answers 404 - in practice a password change). Read rounds cannot fix it, so
	// the ticker skips the account until a login stores a new key; the panel asks the operator to
	// check the password. Added in migration 80.
	AzulSyncNeedsReconnect = "needs_reconnect"
)

// AzulSyncState is one account's statement-sync lifecycle: which balance it last saw (so the
// ticker knows when to pull the statement) and how the last round went. One row per account,
// keyed by the account itself. See migration 54.
type AzulSyncState struct {
	AccountID string   `gorm:"column:account_id;type:uuid;primaryKey" json:"account_id"`
	Account   *Account `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`

	// LastSyncedAt and LastBalancePoints record the last successful read. LastBalancePoints is
	// what the ticker compares against to decide whether the statement moved and needs pulling.
	// Nil on both until the first success.
	LastSyncedAt      *time.Time `gorm:"column:last_synced_at" json:"last_synced_at"`
	LastBalancePoints *int       `gorm:"column:last_balance_points" json:"last_balance_points"`

	State string `gorm:"column:state;type:azul_sync_state_kind;not null;default:'pending'" json:"state"`

	// ConsecutiveFailures drives the backoff and the threshold that flips State to
	// needs_attention. Reset to zero on any success.
	ConsecutiveFailures int `gorm:"column:consecutive_failures;not null;default:0" json:"consecutive_failures"`

	// Detail is a short human-readable note for the panel (the last error, or the day a
	// balance mismatch was seen). Nothing parses it.
	Detail *string `gorm:"column:detail" json:"detail"`

	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (AzulSyncState) TableName() string {
	return "azul_sync_state"
}
