package schemas

import "time"

// CreateLoyaltyProgramRequest defines the expected payload to create a new loyalty program
type CreateLoyaltyProgramRequest struct {
	Name             string  `json:"name" validate:"required"`
	Code             string  `json:"code" validate:"required"`
	Color            *string `json:"color" validate:"omitempty"`
	DefaultAirlineID string  `json:"default_airline_id" validate:"required,uuid"`
}

// UpdateLoyaltyProgramRequest defines the payload for updating an existing loyalty program.
// All fields are optional pointers: nil means "don't change this field".
type UpdateLoyaltyProgramRequest struct {
	Name             *string `json:"name" validate:"omitempty"`
	Code             *string `json:"code" validate:"omitempty"`
	Color            *string `json:"color" validate:"omitempty"`
	DefaultAirlineID *string `json:"default_airline_id" validate:"omitempty,uuid"`
}

// LoyaltyProgramResponse defines how a loyalty program should be returned in the API.
type LoyaltyProgramResponse struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Code             string           `json:"code"`
	Color            string           `json:"color"`
	DefaultAirlineID string           `json:"default_airline_id"`
	DefaultAirline   *AirlineResponse `json:"default_airline,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}
