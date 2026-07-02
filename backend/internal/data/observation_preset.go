package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ObservationPreset struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	Code      string    `gorm:"uniqueIndex;not null" json:"code"`
	Text      string    `gorm:"not null" json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

func (op *ObservationPreset) BeforeCreate(tx *gorm.DB) error {
	if op.ID == "" {
		op.ID = uuid.New().String()
	}
	return nil
}
