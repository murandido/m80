package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Airport struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	IATACode  string    `gorm:"column:iata_code;uniqueIndex;not null" json:"iata_code"`
	Name      string    `gorm:"not null" json:"name"`
	City      *string   `json:"city"`
	State     *string   `json:"state"`
	Country   string    `gorm:"not null;default:'BR'" json:"country"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (a *Airport) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
