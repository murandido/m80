package data

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// InjunctionGroup is one entry of the Controle de Liminares (migration 000095): the
// injunction accounts one OneDrive workbook covers, the WhatsApp group the agent talks to
// about them, and how the workbook's labels map onto the panel. It has no agency of its own
// (migration 000096): each issuer mapping carries the agency their issuances belong to.
type InjunctionGroup struct {
	ID               string  `gorm:"primaryKey;type:uuid" json:"id"`
	Name             string  `gorm:"not null" json:"name"`
	WhatsAppChatName *string `gorm:"column:whatsapp_chat_name" json:"whatsapp_chat_name"`
	// SpreadsheetURL is encrypted at rest: it opens a workbook full of passenger names to
	// anyone signed in with access. Never sent back in full.
	SpreadsheetURL *string                 `gorm:"column:spreadsheet_url" json:"-"`
	Mappings       InjunctionGroupMappings `gorm:"type:jsonb;serializer:json;not null" json:"mappings"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

// InjunctionGroupMappings mirrors legacyimport.Mappings for the part a Tropa keeps (data
// cannot import legacyimport). Only rows whose account label is listed are read: the same
// workbook also carries accounts that are not injunctions.
type InjunctionGroupMappings struct {
	Accounts []InjunctionAccountMapping `json:"accounts"`
	Issuers  []InjunctionIssuerMapping  `json:"issuers"`
}

// InjunctionAccountMapping is one (program, label as typed in the Conta column) pair.
type InjunctionAccountMapping struct {
	ProgramCode  string `json:"program_code"`
	AccountLabel string `json:"account_label"`
	AccountID    string `json:"account_id"`
}

// InjunctionIssuerMapping is one name as typed in the Emissor column, and the agency the
// issuances of that issuer belong to. AgencyID is empty only on mappings saved before
// migration 000096; their rows stay "needs mapping" until one is picked.
type InjunctionIssuerMapping struct {
	IssuerName string `json:"issuer_name"`
	UserID     string `json:"user_id"`
	AgencyID   string `json:"agency_id"`
}

func (g *InjunctionGroup) BeforeCreate(tx *gorm.DB) error {
	if g.ID == "" {
		g.ID = uuid.New().String()
	}
	return nil
}

func (g *InjunctionGroup) BeforeSave(tx *gorm.DB) error {
	if g.SpreadsheetURL == nil || *g.SpreadsheetURL == "" || strings.HasPrefix(*g.SpreadsheetURL, "enc:") {
		return nil
	}
	encrypted, err := utils.Encrypt(*g.SpreadsheetURL)
	if err != nil {
		return err
	}
	g.SpreadsheetURL = &encrypted
	return nil
}

func (g *InjunctionGroup) AfterFind(tx *gorm.DB) error {
	if g.SpreadsheetURL == nil {
		return nil
	}
	plain, err := utils.Decrypt(*g.SpreadsheetURL)
	if err != nil {
		return err
	}
	g.SpreadsheetURL = &plain
	return nil
}
