package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Airline struct {
	ID                     string `gorm:"primaryKey;type:uuid" json:"id"`
	Name                   string `gorm:"not null" json:"name"`
	Code                   string `gorm:"uniqueIndex;not null" json:"code"`
	TraveosSupported       bool   `gorm:"column:traveos_supported;not null;default:false" json:"traveos_supported"`
	RequiresLastName       bool   `gorm:"column:requires_last_name;not null;default:false" json:"requires_last_name"`
	RequiresPurchaseNumber bool   `gorm:"column:requires_purchase_number;not null;default:false" json:"requires_purchase_number"`
	// TraveosAirlineID is the carrier's id in Traveos' catalog, required by every
	// emission request. Nil means nobody mapped it yet - a different state from
	// TraveosSupported being false, which is a deliberate choice not to use Traveos.
	TraveosAirlineID *int `gorm:"column:traveos_airline_id" json:"traveos_airline_id"`
	// BrandColor is the carrier's primary colour ("#RRGGBB"), used by the ticket
	// layout. Stored rather than fetched per render, since the ticket must draw for
	// an issuance whose Traveos data was fetched days ago.
	BrandColor *string   `gorm:"column:brand_color" json:"brand_color"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (a *Airline) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
