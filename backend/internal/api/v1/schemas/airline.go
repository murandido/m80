package schemas

import "time"

// CreateAirlineRequest defines the expected payload to create a new airline
type CreateAirlineRequest struct {
	Name                   string `json:"name" validate:"required"`
	Code                   string `json:"code" validate:"required"`
	TraveosSupported       *bool  `json:"traveos_supported" validate:"omitempty"`
	RequiresLastName       *bool  `json:"requires_last_name" validate:"omitempty"`
	RequiresPurchaseNumber *bool  `json:"requires_purchase_number" validate:"omitempty"`
}

// UpdateAirlineRequest defines the payload for updating an existing airline.
// All fields are optional pointers: nil means "don't change this field".
type UpdateAirlineRequest struct {
	Name                   *string `json:"name" validate:"omitempty"`
	Code                   *string `json:"code" validate:"omitempty"`
	TraveosSupported       *bool   `json:"traveos_supported" validate:"omitempty"`
	RequiresLastName       *bool   `json:"requires_last_name" validate:"omitempty"`
	RequiresPurchaseNumber *bool   `json:"requires_purchase_number" validate:"omitempty"`
}

// AirlineResponse defines how an airline should be returned in the API.
type AirlineResponse struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Code                   string    `json:"code"`
	TraveosSupported       bool      `json:"traveos_supported"`
	RequiresLastName       bool      `json:"requires_last_name"`
	RequiresPurchaseNumber bool      `json:"requires_purchase_number"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}
