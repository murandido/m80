package schemas

import (
	"time"

	"gorm.io/datatypes"
)

// CreateIssuanceRequest defines the expected payload to create a new issuance.
// complete is never accepted - it always starts as false.
type CreateIssuanceRequest struct {
	ProgramLocator     string `json:"program_locator" validate:"required"`
	LoyaltyProgramID   string `json:"loyalty_program_id" validate:"required,uuid"`
	AccountID          string `json:"account_id" validate:"required,uuid"`
	Points             int    `json:"points" validate:"required"`
	ValueOfTheThousand int    `json:"value_of_the_thousand" validate:"required"`
	// AirlineID defaults to the loyalty program's default_airline_id when omitted.
	AirlineID *string `json:"airline_id" validate:"omitempty,uuid"`
	// AgencyID defaults to the agency where is_default=true when omitted.
	AgencyID *string `json:"agency_id" validate:"omitempty,uuid"`
	// IssuerID defaults to the authenticated (JWT) user when omitted.
	IssuerID            *string        `json:"issuer_id" validate:"omitempty,uuid"`
	AirlineLocator      *string        `json:"airline_locator" validate:"omitempty"`
	LatamPurchaseNumber *string        `json:"latam_purchase_number" validate:"omitempty"`
	MainPassengerName   *string        `json:"main_passenger_name" validate:"omitempty"`
	TripType            *string        `json:"trip_type" validate:"omitempty,oneof=one_way round_trip multi_leg"`
	OriginIATACode      *string        `json:"origin_iata_code" validate:"omitempty"`
	DestinationIATACode *string        `json:"destination_iata_code" validate:"omitempty"`
	Taxes               *int           `json:"taxes" validate:"omitempty"`
	Observation         *string        `json:"observation" validate:"omitempty"`
	AdditionalServices  datatypes.JSON `json:"additional_services" validate:"omitempty" swaggertype:"object"`
	// FlightDate (YYYY-MM-DD) creates the primary outbound flight leg.
	FlightDate *string `json:"flight_date" validate:"omitempty,datetime=2006-01-02"`
}

// UpdateIssuanceRequest defines the payload for a partial (autosave) update of an
// existing issuance. All fields are optional pointers: nil means "don't change this
// field". complete is never accepted directly - use Status instead, which the handler
// translates into complete/cancelled_at.
type UpdateIssuanceRequest struct {
	LoyaltyProgramID    *string        `json:"loyalty_program_id" validate:"omitempty,uuid"`
	AirlineID           *string        `json:"airline_id" validate:"omitempty,uuid"`
	AccountID           *string        `json:"account_id" validate:"omitempty,uuid"`
	AgencyID            *string        `json:"agency_id" validate:"omitempty,uuid"`
	IssuerID            *string        `json:"issuer_id" validate:"omitempty,uuid"`
	ProgramLocator      *string        `json:"program_locator" validate:"omitempty"`
	AirlineLocator      *string        `json:"airline_locator" validate:"omitempty"`
	LatamPurchaseNumber *string        `json:"latam_purchase_number" validate:"omitempty"`
	MainPassengerName   *string        `json:"main_passenger_name" validate:"omitempty"`
	TripType            *string        `json:"trip_type" validate:"omitempty,oneof=one_way round_trip multi_leg"`
	OriginIATACode      *string        `json:"origin_iata_code" validate:"omitempty"`
	DestinationIATACode *string        `json:"destination_iata_code" validate:"omitempty"`
	Points              *int           `json:"points" validate:"omitempty"`
	ValueOfTheThousand  *int           `json:"value_of_the_thousand" validate:"omitempty"`
	Taxes               *int           `json:"taxes" validate:"omitempty"`
	Observation         *string        `json:"observation" validate:"omitempty"`
	AdditionalServices  datatypes.JSON `json:"additional_services" validate:"omitempty" swaggertype:"object"`
	// Status transitions the issuance between incomplete/complete/cancelled. "deleted"
	// is not a valid value here - it's only reachable via DELETE /issuances/{id}.
	Status *string `json:"status" validate:"omitempty,oneof=incomplete complete cancelled"`
	// FlightDate (YYYY-MM-DD) upserts the primary outbound flight leg's departure date.
	FlightDate *string `json:"flight_date" validate:"omitempty,datetime=2006-01-02"`
}

// IssuanceFlightResponse defines how a flight leg should be returned nested in an issuance response.
type IssuanceFlightResponse struct {
	ID                  string     `json:"id"`
	Direction           string     `json:"direction"`
	LegOrder            int        `json:"leg_order"`
	FlightNumber        *string    `json:"flight_number"`
	OriginIATACode      *string    `json:"origin_iata_code"`
	DestinationIATACode *string    `json:"destination_iata_code"`
	DepartureAt         time.Time  `json:"departure_at"`
	ArrivalAt           *time.Time `json:"arrival_at"`
	DurationMin         *int       `json:"duration_min"`
	Cabin               *string    `json:"cabin"`
	CreatedAt           time.Time  `json:"created_at"`
}

// IssuanceResponse defines how an issuance should be returned in the API.
type IssuanceResponse struct {
	ID                  string                  `json:"id"`
	LoyaltyProgramID    string                  `json:"loyalty_program_id"`
	LoyaltyProgram      *LoyaltyProgramResponse `json:"loyalty_program,omitempty"`
	AirlineID           string                  `json:"airline_id"`
	Airline             *AirlineResponse        `json:"airline,omitempty"`
	ProgramLocator      string                  `json:"program_locator"`
	AirlineLocator      *string                 `json:"airline_locator"`
	LatamPurchaseNumber *string                 `json:"latam_purchase_number"`
	MainPassengerName   *string                 `json:"main_passenger_name"`
	TripType            *string                 `json:"trip_type"`
	OriginIATACode      *string                 `json:"origin_iata_code"`
	OriginAirport       *AirportResponse        `json:"origin_airport,omitempty"`
	DestinationIATACode *string                 `json:"destination_iata_code"`
	DestinationAirport  *AirportResponse        `json:"destination_airport,omitempty"`
	Points              int                     `json:"points"`
	ValueOfTheThousand  int                     `json:"value_of_the_thousand"`
	Taxes               *int                    `json:"taxes"`
	AccountID           string                  `json:"account_id"`
	Account             *AccountResponse        `json:"account,omitempty"`
	IssuerID            string                  `json:"issuer_id"`
	Issuer              *UserResponse           `json:"issuer,omitempty"`
	AgencyID            string                  `json:"agency_id"`
	Agency              *AgencyResponse         `json:"agency,omitempty"`
	Complete            bool                    `json:"complete"`
	// Status is derived: "incomplete" | "complete" | "cancelled".
	Status             string                      `json:"status" example:"incomplete"`
	Flights            []IssuanceFlightResponse    `json:"flights,omitempty"`
	Passengers         []IssuancePassengerResponse `json:"passengers,omitempty"`
	AdditionalServices datatypes.JSON              `json:"additional_services" swaggertype:"object"`
	Observation        *string                     `json:"observation"`
	Active             bool                        `json:"active"`
	CancelledAt        *time.Time                  `json:"cancelled_at"`
	RefundedPoints     *int                        `json:"refunded_points"`
	RefundedTaxes      *int                        `json:"refunded_taxes"`
	CancellationFee    *int                        `json:"cancellation_fee"`
	CreatedAt          time.Time                   `json:"created_at"`
	UpdatedAt          time.Time                   `json:"updated_at"`
}

// IssuanceSummaryResponse defines a lightweight issuance representation for listing
// screens - only the fields needed to render a table row, no nested relations.
type IssuanceSummaryResponse struct {
	ID                  string     `json:"id"`
	LoyaltyProgramName  string     `json:"loyalty_program_name"`
	AirlineName         string     `json:"airline_name"`
	ProgramLocator      string     `json:"program_locator"`
	AirlineLocator      *string    `json:"airline_locator"`
	IssuedAt            time.Time  `json:"issued_at"`
	FlightDate          *time.Time `json:"flight_date"`
	MainPassengerName   *string    `json:"main_passenger_name"`
	OriginIATACode      *string    `json:"origin_iata_code"`
	DestinationIATACode *string    `json:"destination_iata_code"`
	Points              int        `json:"points"`
	ValueOfTheThousand  int        `json:"value_of_the_thousand"`
	Taxes               *int       `json:"taxes"`
	AccountLogin        *string    `json:"account_login"`
	IssuerName          string     `json:"issuer_name"`
	AgencyName          string     `json:"agency_name"`
	// Status is derived: "incomplete" | "complete" | "cancelled".
	Status string `json:"status" example:"incomplete"`
}
