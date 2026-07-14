package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Agency struct {
	ID        string           `gorm:"primaryKey;type:uuid" json:"id"`
	Name      string           `gorm:"not null" json:"name"`
	IsDefault bool             `gorm:"column:is_default;not null;default:false" json:"is_default"`
	Nicknames []AgencyNickname `gorm:"foreignKey:AgencyID;references:ID" json:"nicknames,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

func (a *Agency) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
