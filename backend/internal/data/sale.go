package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Sale struct {
	ID         string    `gorm:"primaryKey;type:uuid" json:"id"`
	IssuanceID string    `gorm:"column:issuance_id;type:uuid;not null;uniqueIndex" json:"issuance_id"`
	Issuance   *Issuance `gorm:"foreignKey:IssuanceID;references:ID" json:"issuance,omitempty"`
	ClientID   *string   `gorm:"column:client_id;type:uuid" json:"client_id"`
	Client     *Client   `gorm:"foreignKey:ClientID;references:ID" json:"client,omitempty"`
	Paid       bool      `gorm:"not null;default:false" json:"paid"`
	// Note is the payments screen's free-text line about this sale's money. Nil is
	// "nobody wrote anything"; the handler stores an emptied note as nil too.
	Note *string `gorm:"type:text" json:"note"`
	// PriceAdjustmentCents is what the client is charged above (positive) or below
	// (negative) the computed price. See migration 83 for why it is a difference.
	PriceAdjustmentCents int       `gorm:"column:price_adjustment_cents;not null;default:0" json:"price_adjustment_cents"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (s *Sale) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}
