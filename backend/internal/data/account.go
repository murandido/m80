package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Account struct {
	ID               string          `gorm:"primaryKey;type:uuid" json:"id"`
	LoyaltyProgramID string          `gorm:"column:loyalty_program_id;type:uuid;not null" json:"loyalty_program_id"`
	LoyaltyProgram   *LoyaltyProgram `gorm:"foreignKey:LoyaltyProgramID;references:ID" json:"loyalty_program,omitempty"`
	Login            *string         `json:"login"`
	Password         *string         `json:"password"`
	HolderID         *string         `gorm:"column:holder_id;type:uuid" json:"holder_id"`
	Holder           *Person         `gorm:"foreignKey:HolderID;references:ID" json:"holder,omitempty"`
	AdministratorID  *string         `gorm:"column:administrator_id;type:uuid" json:"administrator_id"`
	Administrator    *Person         `gorm:"foreignKey:AdministratorID;references:ID" json:"administrator,omitempty"`
	Complete         bool            `gorm:"not null;default:false" json:"complete"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func (a *Account) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
