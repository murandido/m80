package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Outcomes of a BookingLookupAttempt, matching the CHECK in migration 66.
const (
	LookupFound       = "found"
	LookupNotFound    = "not_found"
	LookupUnavailable = "unavailable"
	LookupRefused     = "refused"
)

// BookingLookupAttempt is one booking provider asked during an itinerary sync. It is
// temporary scaffolding for the move off Traveos - see migration 66.
type BookingLookupAttempt struct {
	ID string `gorm:"primaryKey;type:uuid" json:"id"`
	// SyncID groups the attempts of one sync, so a fallback can be told from two syncs.
	SyncID     string    `gorm:"column:sync_id;type:uuid;not null" json:"sync_id"`
	IssuanceID string    `gorm:"column:issuance_id;type:uuid;not null" json:"issuance_id"`
	AirlineID  string    `gorm:"column:airline_id;type:uuid;not null" json:"airline_id"`
	Provider   string    `gorm:"column:provider;not null" json:"provider"`
	Outcome    string    `gorm:"column:outcome;not null" json:"outcome"`
	Error      *string   `gorm:"column:error" json:"error"`
	DurationMS int       `gorm:"column:duration_ms;not null" json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

func (a *BookingLookupAttempt) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
