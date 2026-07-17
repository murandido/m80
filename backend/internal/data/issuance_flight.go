package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type IssuanceFlight struct {
	ID                  string     `gorm:"primaryKey;type:uuid" json:"id"`
	IssuanceID          string     `gorm:"column:issuance_id;type:uuid;not null" json:"issuance_id"`
	Issuance            *Issuance  `gorm:"foreignKey:IssuanceID;references:ID" json:"issuance,omitempty"`
	Direction           string     `gorm:"type:flight_direction;not null" json:"direction"`
	LegOrder            int        `gorm:"column:leg_order;not null" json:"leg_order"`
	FlightNumber        *string    `gorm:"column:flight_number" json:"flight_number"`
	OriginIATACode      *string    `gorm:"column:origin_iata_code" json:"origin_iata_code"`
	OriginAirport       *Airport   `gorm:"foreignKey:OriginIATACode;references:IATACode" json:"origin_airport,omitempty"`
	DestinationIATACode *string    `gorm:"column:destination_iata_code" json:"destination_iata_code"`
	DestinationAirport  *Airport   `gorm:"foreignKey:DestinationIATACode;references:IATACode" json:"destination_airport,omitempty"`
	DepartureAt         time.Time  `gorm:"column:departure_at;not null" json:"departure_at"`
	ArrivalAt           *time.Time `gorm:"column:arrival_at" json:"arrival_at"`
	DurationMin         *int       `gorm:"column:duration_min" json:"duration_min"`
	Cabin               *string    `json:"cabin"`
	CreatedAt           time.Time  `json:"created_at"`
}

func (f *IssuanceFlight) BeforeCreate(tx *gorm.DB) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	return nil
}
