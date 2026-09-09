package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InventoryPeriod is a stretch of an account's costing that has been settled between the
// agencies and may no longer be recomputed. See migration 51.
type InventoryPeriod struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	AccountID string    `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	Account   *Account  `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`
	StartsOn  time.Time `gorm:"column:starts_on;type:date;not null" json:"starts_on"`
	EndsOn    time.Time `gorm:"column:ends_on;type:date;not null" json:"ends_on"`
	// ClosedAt nil is a period somebody opened but has not settled yet. Only a closed
	// period freezes the allocations inside it.
	ClosedAt   *time.Time `gorm:"column:closed_at" json:"closed_at"`
	ClosedByID *string    `gorm:"column:closed_by_id;type:uuid" json:"closed_by_id"`
	ClosedBy   *User      `gorm:"foreignKey:ClosedByID;references:ID" json:"closed_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (p *InventoryPeriod) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}
