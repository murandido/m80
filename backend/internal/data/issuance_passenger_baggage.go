package data

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// IssuancePassengerBaggage is one baggage entitlement a passenger has for one
// direction - not for one leg. The vendor reports baggage as a list of classes
// (carry-on, checked) covering a whole direction, and Position is that list's
// own index: it is what decides which icon the ticket draws, because the order
// is information the vendor gives us and the weight alone is not.
type IssuancePassengerBaggage struct {
	ID          string             `gorm:"primaryKey;type:uuid" json:"id"`
	PassengerID string             `gorm:"column:passenger_id;type:uuid;not null" json:"passenger_id"`
	Passenger   *IssuancePassenger `gorm:"foreignKey:PassengerID;references:ID" json:"passenger,omitempty"`
	Direction   string             `gorm:"type:flight_direction;not null" json:"direction"`
	Position    int                `gorm:"not null" json:"position"`
	// Weight is nil when the airline sends a bare count instead of a class with
	// a weight - GOL's `[1, 0]`. That is a real shape, not missing data.
	Weight   *string `json:"weight"`
	Quantity int     `gorm:"not null;default:0" json:"quantity"`
}

func (ipb *IssuancePassengerBaggage) TableName() string {
	return "issuance_passenger_baggage"
}

func (ipb *IssuancePassengerBaggage) BeforeCreate(tx *gorm.DB) error {
	if ipb.ID == "" {
		ipb.ID = uuid.New().String()
	}
	return nil
}
