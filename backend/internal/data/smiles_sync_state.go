package data

import "time"

// The states one account's Smiles statement sync can be in. Stored in English like every
// other enum in the schema; the Portuguese labels live only on the panel. Mirrors the
// smiles_sync_state_kind enum in migration 63.
const (
	// SmilesSyncPending is a connected account that has never been synced.
	SmilesSyncPending = "pending"
	// SmilesSyncOK is the last round reading the balance and storing the window.
	SmilesSyncOK = "ok"
	// SmilesSyncError is a transport failure; it is retried on the next tick.
	SmilesSyncError = "error"
	// SmilesSyncNeedsMFA is the stored session having been refused. It is not an error to
	// retry: only a human typing a new verification code fixes it, and the sync must never
	// answer it by replaying the password - repeated password attempts are how an account
	// gets locked. The panel raises this one for the operator.
	SmilesSyncNeedsMFA = "needs_mfa"
	// SmilesSyncNeedsAttention is enough failed rounds in a row that the account is worth a
	// look.
	SmilesSyncNeedsAttention = "needs_attention"
)

// SmilesSyncState is one account's statement-sync lifecycle: which balance it last saw (so
// the ticker knows when to pull the statement) and how the last round went. One row per
// account, keyed by the account itself. See migration 63.
//
// It is the twin of AzulSyncState, minus the "inconsistent" state: Smiles publishes no
// end-of-day balance, so there is no figure to validate a window against.
type SmilesSyncState struct {
	AccountID string   `gorm:"column:account_id;type:uuid;primaryKey" json:"account_id"`
	Account   *Account `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`

	// LastSyncedAt and LastBalanceMiles record the last successful read. LastBalanceMiles is
	// what the ticker compares against to decide whether the statement moved and needs
	// pulling. Nil on both until the first success.
	LastSyncedAt     *time.Time `gorm:"column:last_synced_at" json:"last_synced_at"`
	LastBalanceMiles *int       `gorm:"column:last_balance_miles" json:"last_balance_miles"`

	State string `gorm:"column:state;type:smiles_sync_state_kind;not null;default:'pending'" json:"state"`

	// ConsecutiveFailures drives the threshold that flips State to needs_attention. Reset to
	// zero on any success.
	ConsecutiveFailures int `gorm:"column:consecutive_failures;not null;default:0" json:"consecutive_failures"`

	// Detail is a short human-readable note for the panel (the last error, or why a code is
	// needed). Nothing parses it.
	Detail *string `gorm:"column:detail" json:"detail"`

	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (SmilesSyncState) TableName() string {
	return "smiles_sync_state"
}
