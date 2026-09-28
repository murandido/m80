package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ClientPayment is a lump sum a client handed over, spread over their open sales oldest
// first. The sale_payments it produced carry its id and are rebuilt from the active rows
// whenever needed - see internal/sale/client_payment.go, the only place that writes these.
//
// Soft-deleted by hand rather than through gorm.DeletedAt: the history screen lists the
// undone rows too, and every other read filters deleted_at explicitly.
type ClientPayment struct {
	ID          string     `gorm:"primaryKey;type:uuid" json:"id"`
	ClientID    string     `gorm:"column:client_id;type:uuid;not null" json:"client_id"`
	AmountCents int        `gorm:"column:amount_cents;not null" json:"amount_cents"`
	PaidOn      time.Time  `gorm:"column:paid_on;type:date;not null" json:"paid_on"`
	CreatedByID string     `gorm:"column:created_by_id;type:uuid;not null" json:"created_by_id"`
	CreatedBy   *User      `gorm:"foreignKey:CreatedByID;references:ID" json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at" json:"deleted_at"`
	DeletedByID *string    `gorm:"column:deleted_by_id;type:uuid" json:"deleted_by_id"`
	DeletedBy   *User      `gorm:"foreignKey:DeletedByID;references:ID" json:"deleted_by,omitempty"`
}

func (cp *ClientPayment) BeforeCreate(tx *gorm.DB) error {
	if cp.ID == "" {
		cp.ID = uuid.New().String()
	}
	return nil
}
