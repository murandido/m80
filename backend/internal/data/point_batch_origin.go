package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PointBatchOrigin is where a batch of points came from: a transfer partner, a bank, a
// direct purchase from the airline, a club subscription, flight accrual.
//
// It carries no "kind" column on purpose. What separates a club from a purchase is that
// its batch costs nothing, and that already lives on the batch - a type here would be a
// second, redundant way to say it, free to drift out of agreement with the first.
type PointBatchOrigin struct {
	ID   string `gorm:"primaryKey;type:uuid" json:"id"`
	Name string `gorm:"not null" json:"name"`
	// Active is how an origin stops being offered without disturbing the batches that
	// already reference it. It exists instead of a soft delete: gorm.DeletedAt would do
	// the same hiding while also freeing the unique name, and two origins called "Livelo"
	// is exactly what the unique index is there to prevent.
	Active    bool      `gorm:"not null;default:true" json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (o *PointBatchOrigin) BeforeCreate(tx *gorm.DB) error {
	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	return nil
}
