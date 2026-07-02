package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Issuance struct {
	ID                  string          `gorm:"primaryKey;type:uuid" json:"id"`
	LoyaltyProgramID    string          `gorm:"column:loyalty_program_id;type:uuid;not null" json:"loyalty_program_id"`
	LoyaltyProgram      *LoyaltyProgram `gorm:"foreignKey:LoyaltyProgramID;references:ID" json:"loyalty_program,omitempty"`
	AirlineID           string          `gorm:"column:airline_id;type:uuid;not null" json:"airline_id"`
	Airline             *Airline        `gorm:"foreignKey:AirlineID;references:ID" json:"airline,omitempty"`
	ProgramLocator      string          `gorm:"column:program_locator;not null" json:"program_locator"`
	AirlineLocator      *string         `gorm:"column:airline_locator" json:"airline_locator"`
	LatamPurchaseNumber *string         `gorm:"column:latam_purchase_number" json:"latam_purchase_number"`
	MainPassengerName   *string         `gorm:"column:main_passenger_name" json:"main_passenger_name"`
	TripType            *string         `gorm:"column:trip_type;type:trip_type" json:"trip_type"`
	OriginIATACode      *string         `gorm:"column:origin_iata_code" json:"origin_iata_code"`
	OriginAirport       *Airport        `gorm:"foreignKey:OriginIATACode;references:IATACode" json:"origin_airport,omitempty"`
	DestinationIATACode *string         `gorm:"column:destination_iata_code" json:"destination_iata_code"`
	DestinationAirport  *Airport        `gorm:"foreignKey:DestinationIATACode;references:IATACode" json:"destination_airport,omitempty"`
	Points              int             `gorm:"not null" json:"points"`
	ValueOfTheThousand  int             `gorm:"column:value_of_the_thousand;not null" json:"value_of_the_thousand"`
	Taxes               *int            `json:"taxes"`
	AccountID           string          `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	Account             *Account        `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`
	IssuerID            string          `gorm:"column:issuer_id;type:uuid;not null" json:"issuer_id"`
	Issuer              *User           `gorm:"foreignKey:IssuerID;references:ID" json:"issuer,omitempty"`
	AgencyID            string          `gorm:"column:agency_id;type:uuid;not null" json:"agency_id"`
	Agency              *Agency         `gorm:"foreignKey:AgencyID;references:ID" json:"agency,omitempty"`
	Complete            bool            `gorm:"not null;default:false" json:"complete"`
	AdditionalServices  datatypes.JSON  `gorm:"column:additional_services;type:jsonb" json:"additional_services"`
	Observation         *string         `gorm:"type:text" json:"observation"`
	Active              bool            `gorm:"not null;default:true" json:"active"`
	CancelledAt         *time.Time      `gorm:"column:cancelled_at" json:"cancelled_at"`
	RefundedPoints      *int            `gorm:"column:refunded_points" json:"refunded_points"`
	RefundedTaxes       *int            `gorm:"column:refunded_taxes" json:"refunded_taxes"`
	CancellationFee     *int            `gorm:"column:cancellation_fee" json:"cancellation_fee"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	DeletedAt           gorm.DeletedAt  `gorm:"index" json:"-"`
}

func (i *Issuance) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return nil
}
