package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ClientCredit is one movement of a client's credit with the agency: a surplus received
// (positive) or credit spent on one of their sales (negative, with SaleID set). The balance
// is the sum - see internal/sale/client_payment.go, the only place that writes these.
type ClientCredit struct {
	ID          string    `gorm:"primaryKey;type:uuid" json:"id"`
	ClientID    string    `gorm:"column:client_id;type:uuid;not null" json:"client_id"`
	AmountCents int       `gorm:"column:amount_cents;not null" json:"amount_cents"`
	SaleID      *string   `gorm:"column:sale_id;type:uuid" json:"sale_id"`
	Note        *string   `gorm:"type:text" json:"note"`
	CreatedByID string    `gorm:"column:created_by_id;type:uuid;not null" json:"created_by_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func (cc *ClientCredit) BeforeCreate(tx *gorm.DB) error {
	if cc.ID == "" {
		cc.ID = uuid.New().String()
	}
	return nil
}
