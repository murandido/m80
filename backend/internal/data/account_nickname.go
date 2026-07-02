package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AccountNickname struct {
	ID               string          `gorm:"primaryKey;type:uuid" json:"id"`
	AccountID        string          `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	Account          *Account        `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`
	LoyaltyProgramID string          `gorm:"column:loyalty_program_id;type:uuid;not null" json:"loyalty_program_id"`
	LoyaltyProgram   *LoyaltyProgram `gorm:"foreignKey:LoyaltyProgramID;references:ID" json:"loyalty_program,omitempty"`
	Nickname         string          `gorm:"not null" json:"nickname"`
	CreatedAt        time.Time       `json:"created_at"`
}

func (an *AccountNickname) BeforeCreate(tx *gorm.DB) error {
	if an.ID == "" {
		an.ID = uuid.New().String()
	}
	return nil
}
