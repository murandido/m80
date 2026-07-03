package data

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type IssuancePassenger struct {
	ID         string    `gorm:"primaryKey;type:uuid" json:"id"`
	IssuanceID string    `gorm:"column:issuance_id;type:uuid;not null" json:"issuance_id"`
	Issuance   *Issuance `gorm:"foreignKey:IssuanceID;references:ID" json:"issuance,omitempty"`
	FirstName  string    `gorm:"column:first_name;not null" json:"first_name"`
	LastName   string    `gorm:"column:last_name;not null" json:"last_name"`
	Type       string    `gorm:"type:passenger_type;not null;default:'ADT'" json:"type"`
	Document   *string   `json:"document"`
	Position   int       `gorm:"not null" json:"position"`
	CreatedAt  time.Time `json:"created_at"`
}

func (ip *IssuancePassenger) BeforeCreate(tx *gorm.DB) error {
	if ip.ID == "" {
		ip.ID = uuid.New().String()
	}
	return nil
}

func (ip *IssuancePassenger) BeforeSave(tx *gorm.DB) error {
	if ip.Document != nil && !strings.HasPrefix(*ip.Document, "enc:") {
		encrypted, err := utils.Encrypt(*ip.Document)
		if err != nil {
			return err
		}
		ip.Document = &encrypted
	}
	return nil
}

func (ip *IssuancePassenger) AfterFind(tx *gorm.DB) error {
	if ip.Document != nil {
		decrypted, err := utils.Decrypt(*ip.Document)
		if err != nil {
			return err
		}
		ip.Document = &decrypted
	}
	return nil
}
