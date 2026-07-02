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
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (s *Sale) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}
