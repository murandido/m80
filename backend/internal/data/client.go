package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Client struct {
	ID string `gorm:"primaryKey;type:uuid" json:"id"`
	// Name is unique within an agency (migration 000072), not across the table.
	Name  string  `gorm:"not null" json:"name"`
	Phone *string `json:"phone"`
	// AgencyID is the agency the client buys through. NULL only for the clients migration
	// 000072 could not place - they sold through more than one agency - until a dev does.
	AgencyID  *string   `gorm:"type:uuid" json:"agency_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *Client) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return nil
}
