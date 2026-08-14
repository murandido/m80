package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SalePayment is one movement of money against a sale. A sale is settled when its payments
// add up to what it is worth, which is why sales.paid stopped being something an operator
// sets and became something the server derives - see internal/api/v1/handlers/sale_payment.go.
//
// No soft delete on purpose: a payment that was never received is a typo, not history. It is
// removed, and the balance goes back to what it was. The facts worth keeping about a real
// payment - when the money moved, who recorded it - are on the row itself.
type SalePayment struct {
	ID     string `gorm:"primaryKey;type:uuid" json:"id"`
	SaleID string `gorm:"column:sale_id;type:uuid;not null" json:"sale_id"`
	Sale   *Sale  `gorm:"foreignKey:SaleID;references:ID" json:"sale,omitempty"`
	// AmountCents is signed: negative is money going back to the client. That is how a sale
	// whose ticket was cancelled after being paid returns to a zero balance, without a
	// second concept for refunds.
	AmountCents int `gorm:"column:amount_cents;not null" json:"amount_cents"`
	// PaidOn is the day the money moved - what the operator knows and what the statement is
	// read by. CreatedAt is when the line was typed, which can be another day entirely.
	PaidOn      time.Time `gorm:"column:paid_on;type:date;not null" json:"paid_on"`
	Note        *string   `gorm:"type:text" json:"note"`
	CreatedByID string    `gorm:"column:created_by_id;type:uuid;not null" json:"created_by_id"`
	CreatedBy   *User     `gorm:"foreignKey:CreatedByID;references:ID" json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (sp *SalePayment) BeforeCreate(tx *gorm.DB) error {
	if sp.ID == "" {
		sp.ID = uuid.New().String()
	}
	return nil
}
