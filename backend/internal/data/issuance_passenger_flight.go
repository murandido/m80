package data

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type IssuancePassengerFlight struct {
	ID          string             `gorm:"primaryKey;type:uuid" json:"id"`
	PassengerID string             `gorm:"column:passenger_id;type:uuid;not null" json:"passenger_id"`
	Passenger   *IssuancePassenger `gorm:"foreignKey:PassengerID;references:ID" json:"passenger,omitempty"`
	FlightID    string             `gorm:"column:flight_id;type:uuid;not null" json:"flight_id"`
	Flight      *IssuanceFlight    `gorm:"foreignKey:FlightID;references:ID" json:"flight,omitempty"`
	Seat        *string            `json:"seat"`
	BaggageQty  *int               `gorm:"column:baggage_qty;default:0" json:"baggage_qty"`
}

func (ipf *IssuancePassengerFlight) BeforeCreate(tx *gorm.DB) error {
	if ipf.ID == "" {
		ipf.ID = uuid.New().String()
	}
	return nil
}
