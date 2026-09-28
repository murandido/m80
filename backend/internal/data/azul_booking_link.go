package data

import "time"

// AzulBookingLink is a manual override pinning one account's statement locator to a real
// issuance in this database. It exists only for the pins the operator makes by hand on the
// statement page; the ordinary locator match is computed, not stored. See migration 58.
//
// Keyed by (account_id, locator): a locator names a single booking on an account's statement,
// so there is exactly one manual link per pair. No BeforeCreate hook - the primary key is the
// composite, not a generated uuid.
type AzulBookingLink struct {
	AccountID  string `gorm:"column:account_id;type:uuid;primaryKey" json:"account_id"`
	Locator    string `gorm:"column:locator;primaryKey" json:"locator"`
	IssuanceID string `gorm:"column:issuance_id;type:uuid;not null" json:"issuance_id"`

	// CreatedByID is who made the pin, for audit. Nil when the user has since been removed
	// (the FK is ON DELETE SET NULL: losing the user drops the attribution, not the link).
	CreatedByID *string `gorm:"column:created_by;type:uuid" json:"created_by"`

	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (AzulBookingLink) TableName() string {
	return "azul_booking_links"
}
