package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LoyaltyProgram struct {
	ID               string    `gorm:"primaryKey;type:uuid" json:"id"`
	Name             string    `gorm:"not null" json:"name"`
	Code             string    `gorm:"uniqueIndex;not null" json:"code"`
	Color            string    `gorm:"not null;default:'#64748B'" json:"color"`
	DefaultAirlineID string    `gorm:"column:default_airline_id;type:uuid;not null" json:"default_airline_id"`
	DefaultAirline   *Airline  `gorm:"foreignKey:DefaultAirlineID;references:ID" json:"default_airline,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Same reasoning as Account: accounts and issuances reference a program with
	// ON DELETE RESTRICT, so a program that was ever used could never be removed.
	// Exposed for the same reason too - the grid gets archived programs embedded in
	// old issuances and has to label them without offering them in the picker.
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (lp *LoyaltyProgram) BeforeCreate(tx *gorm.DB) error {
	if lp.ID == "" {
		lp.ID = uuid.New().String()
	}
	return nil
}
