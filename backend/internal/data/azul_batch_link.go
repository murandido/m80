package data

import "time"

// AzulBatchLink is a manual override pinning one account's statement line (a purchase or an
// accrual) to a real point batch in this database. It exists only for the pins the operator
// makes by hand; the ordinary match against the batch's recorded credits is computed, not
// stored. See migration 59. The sibling of AzulBookingLink (issuances).
//
// Keyed by (account_id, line_key): a line is pinned by the same key azul_booking_links uses -
// its locator, or a content signature when it has none.
type AzulBatchLink struct {
	AccountID string `gorm:"column:account_id;type:uuid;primaryKey" json:"account_id"`
	LineKey   string `gorm:"column:line_key;primaryKey" json:"line_key"`
	BatchID   string `gorm:"column:batch_id;type:uuid;not null" json:"batch_id"`

	CreatedByID *string `gorm:"column:created_by;type:uuid" json:"created_by"`

	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (AzulBatchLink) TableName() string {
	return "azul_batch_links"
}
