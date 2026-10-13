package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PointBatchAllocation is one slice of one batch paying for one issuance.
//
// Derived data: rebuilt by internal/inventory from the batches and the issuances, never
// typed. See migration 50 for why an issuance needs several of these rather than one
// averaged rate of its own.
type PointBatchAllocation struct {
	ID         string    `gorm:"primaryKey;type:uuid" json:"id"`
	IssuanceID string    `gorm:"column:issuance_id;type:uuid;not null" json:"issuance_id"`
	Issuance   *Issuance `gorm:"foreignKey:IssuanceID;references:ID" json:"issuance,omitempty"`
	// BatchID is nil while no batch covers these points yet - the pending state. The
	// costing reports the shortfall instead of refusing the issuance, because the bot
	// writes bookings in real time and the purchase is entered whenever it is entered.
	BatchID *string     `gorm:"column:batch_id;type:uuid" json:"batch_id"`
	Batch   *PointBatch `gorm:"foreignKey:BatchID;references:ID" json:"batch,omitempty"`
	// OccurredOn is the issuance's date when points are consumed and the refund's date
	// when they are given back, which is how a reversal lands in an open period while the
	// booking it reverses stays in the closed one.
	OccurredOn time.Time `gorm:"column:occurred_on;type:date;not null" json:"occurred_on"`
	// Points and CostCents are negative on a reversal, so a batch's remaining balance and
	// a period's profit are both plain sums.
	Points    int `gorm:"not null" json:"points"`
	CostCents int `gorm:"column:cost_cents;not null" json:"cost_cents"`
	// AdjustmentID is the alteration this slice belongs to, nil for the booking itself
	// and for its cancellation. The closing prices the slice at that movement's rate.
	AdjustmentID *string   `gorm:"column:adjustment_id;type:uuid" json:"adjustment_id"`
	CreatedAt    time.Time `json:"created_at"`
}

func (a *PointBatchAllocation) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
