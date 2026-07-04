package data

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type Account struct {
	ID               string            `gorm:"primaryKey;type:uuid" json:"id"`
	LoyaltyProgramID string            `gorm:"column:loyalty_program_id;type:uuid;not null" json:"loyalty_program_id"`
	LoyaltyProgram   *LoyaltyProgram   `gorm:"foreignKey:LoyaltyProgramID;references:ID" json:"loyalty_program,omitempty"`
	Login            *string           `json:"login"`
	Password         *string           `json:"password"`
	HolderID         *string           `gorm:"column:holder_id;type:uuid" json:"holder_id"`
	Holder           *Person           `gorm:"foreignKey:HolderID;references:ID" json:"holder,omitempty"`
	AdministratorID  *string           `gorm:"column:administrator_id;type:uuid" json:"administrator_id"`
	Administrator    *Person           `gorm:"foreignKey:AdministratorID;references:ID" json:"administrator,omitempty"`
	Nicknames        []AccountNickname `gorm:"foreignKey:AccountID;references:ID" json:"nicknames,omitempty"`
	Complete         bool              `gorm:"not null;default:false" json:"complete"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

func (a *Account) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

func (a *Account) BeforeSave(tx *gorm.DB) error {
	if a.Password != nil && !strings.HasPrefix(*a.Password, "enc:") {
		encrypted, err := utils.Encrypt(*a.Password)
		if err != nil {
			return err
		}
		a.Password = &encrypted
	}
	return nil
}

func (a *Account) AfterFind(tx *gorm.DB) error {
	if a.Password != nil {
		decrypted, err := utils.Decrypt(*a.Password)
		if err != nil {
			return err
		}
		a.Password = &decrypted
	}
	return nil
}

// ComputeComplete reports whether all essential fields of an account are filled in.
func (a *Account) ComputeComplete() bool {
	return a.LoyaltyProgramID != "" &&
		a.Login != nil &&
		a.Password != nil &&
		a.HolderID != nil &&
		a.AdministratorID != nil
}
