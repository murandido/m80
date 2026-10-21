package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// What a vault fetch was for. The table constrains action to exactly these.
const (
	CredentialAccessCopy = "copy"
	CredentialAccessFill = "fill"
)

// AccountCredentialAccess is one password read through the browser extension's vault. See
// migration 000082 for why each read is kept.
type AccountCredentialAccess struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	AccountID *string   `gorm:"column:account_id;type:uuid" json:"account_id"`
	UserID    string    `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	Action    string    `gorm:"not null" json:"action"`
	ClientIP  *string   `gorm:"column:client_ip" json:"client_ip"`
	CreatedAt time.Time `json:"created_at"`
}

func (a *AccountCredentialAccess) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
