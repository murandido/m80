package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PersonNickname struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	PersonID  string    `gorm:"column:person_id;type:uuid;not null" json:"person_id"`
	Person    *Person   `gorm:"foreignKey:PersonID;references:ID" json:"person,omitempty"`
	Nickname  string    `gorm:"uniqueIndex;not null" json:"nickname"`
	CreatedAt time.Time `json:"created_at"`
}

func (pn *PersonNickname) BeforeCreate(tx *gorm.DB) error {
	if pn.ID == "" {
		pn.ID = uuid.New().String()
	}
	return nil
}
