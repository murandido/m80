package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AuditLog struct {
	ID          string         `gorm:"primaryKey;type:uuid" json:"id"`
	IssuanceID  string         `gorm:"column:issuance_id;type:uuid;not null" json:"issuance_id"`
	Issuance    *Issuance      `gorm:"foreignKey:IssuanceID;references:ID" json:"issuance,omitempty"`
	ChangedByID string         `gorm:"column:changed_by_id;type:uuid;not null" json:"changed_by_id"`
	ChangedBy   *User          `gorm:"foreignKey:ChangedByID;references:ID" json:"changed_by,omitempty"`
	Action      string         `gorm:"type:audit_action;not null" json:"action"`
	Details     datatypes.JSON `gorm:"type:jsonb" json:"details"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (al *AuditLog) BeforeCreate(tx *gorm.DB) error {
	if al.ID == "" {
		al.ID = uuid.New().String()
	}
	return nil
}
