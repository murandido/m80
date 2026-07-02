package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Airline struct {
	ID                     string    `gorm:"primaryKey;type:uuid" json:"id"`
	Name                   string    `gorm:"not null" json:"name"`
	Code                   string    `gorm:"uniqueIndex;not null" json:"code"`
	TraveosSupported       bool      `gorm:"column:traveos_supported;not null;default:false" json:"traveos_supported"`
	RequiresLastName       bool      `gorm:"column:requires_last_name;not null;default:false" json:"requires_last_name"`
	RequiresPurchaseNumber bool      `gorm:"column:requires_purchase_number;not null;default:false" json:"requires_purchase_number"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

func (a *Airline) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
