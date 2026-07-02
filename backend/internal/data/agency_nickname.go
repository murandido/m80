package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AgencyNickname struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	AgencyID  string    `gorm:"column:agency_id;type:uuid;not null" json:"agency_id"`
	Agency    *Agency   `gorm:"foreignKey:AgencyID;references:ID" json:"agency,omitempty"`
	Nickname  string    `gorm:"uniqueIndex;not null" json:"nickname"`
	CreatedAt time.Time `json:"created_at"`
}

func (an *AgencyNickname) BeforeCreate(tx *gorm.DB) error {
	if an.ID == "" {
		an.ID = uuid.New().String()
	}
	return nil
}
