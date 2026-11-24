package data

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// The values microsoft_connections.state takes.
const (
	MicrosoftConnectionOK         = "ok"
	MicrosoftConnectionNeedsLogin = "needs_login"
	MicrosoftConnectionError      = "error"
)

// MicrosoftConnection is the Microsoft login the API reads the Tropas' workbooks with
// (migration 000095). One per installation.
type MicrosoftConnection struct {
	ID            string     `gorm:"primaryKey;type:uuid" json:"id"`
	ConnectedByID string     `gorm:"column:connected_by_id;type:uuid;not null" json:"connected_by_id"`
	AccountName   *string    `gorm:"column:account_name" json:"account_name"`
	AccountEmail  *string    `gorm:"column:account_email" json:"account_email"`
	RefreshToken  string     `gorm:"column:refresh_token;not null" json:"-"`
	State         string     `gorm:"not null;default:ok" json:"state"`
	Detail        *string    `json:"detail"`
	LastOKAt      *time.Time `gorm:"column:last_ok_at" json:"last_ok_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (c *MicrosoftConnection) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return nil
}

// BeforeSave encrypts the refresh token, the same pair of hooks the other encrypted columns
// use. A value already carrying the enc: prefix is left alone, so a row read and saved again
// is not encrypted twice.
func (c *MicrosoftConnection) BeforeSave(tx *gorm.DB) error {
	if c.RefreshToken == "" || strings.HasPrefix(c.RefreshToken, "enc:") {
		return nil
	}
	encrypted, err := utils.Encrypt(c.RefreshToken)
	if err != nil {
		return err
	}
	c.RefreshToken = encrypted
	return nil
}

func (c *MicrosoftConnection) AfterFind(tx *gorm.DB) error {
	plain, err := utils.Decrypt(c.RefreshToken)
	if err != nil {
		return err
	}
	c.RefreshToken = plain
	return nil
}
