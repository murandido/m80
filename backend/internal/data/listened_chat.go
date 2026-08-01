package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ListenedChat struct {
	ID            string  `gorm:"primaryKey;type:uuid" json:"id"`
	ChatJID       string  `gorm:"column:chat_jid;uniqueIndex;not null" json:"chat_jid"`
	FriendlyName  *string `gorm:"column:friendly_name" json:"friendly_name"`
	RequestedByID string  `gorm:"column:requested_by_id;type:uuid;not null" json:"requested_by_id"`
	RequestedBy   *User   `gorm:"foreignKey:RequestedByID;references:ID" json:"requested_by,omitempty"`
	ApprovedByID  *string `gorm:"column:approved_by_id;type:uuid" json:"approved_by_id"`
	ApprovedBy    *User   `gorm:"foreignKey:ApprovedByID;references:ID" json:"approved_by,omitempty"`
	Status        string  `gorm:"type:chat_status;not null;default:'pending'" json:"status"`
	// No `default` tag on purpose: GORM treats a Go zero value as "unset" for
	// columns with a default and lets the database fill it in, which silently
	// turned an explicit Active:false on create into active = true - exactly
	// what a pending, not-yet-approved chat must never be. The column keeps its
	// DEFAULT TRUE in the migration for inserts that don't go through GORM.
	Active bool `gorm:"not null" json:"active"`
	// PDFMode makes the bot send the ticket as a PDF after an EMISSAO it managed
	// to complete from Traveos. Same reasoning as Active above for the missing
	// `default` tag: an explicit false has to survive the write.
	PDFMode   bool      `gorm:"column:pdf_mode;not null" json:"pdf_mode"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (lc *ListenedChat) BeforeCreate(tx *gorm.DB) error {
	if lc.ID == "" {
		lc.ID = uuid.New().String()
	}
	return nil
}
