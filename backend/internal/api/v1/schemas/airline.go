package schemas

import "time"

// CreateAirlineRequest defines the expected payload to create a new airline
type CreateAirlineRequest struct {
	Name                   string `json:"name" validate:"required"`
	Code                   string `json:"code" validate:"required"`
	TraveosSupported       *bool  `json:"traveos_supported" validate:"omitempty"`
	RequiresLastName       *bool  `json:"requires_last_name" validate:"omitempty"`
	RequiresPurchaseNumber *bool  `json:"requires_purchase_number" validate:"omitempty"`
	TraveosAirlineID       *int   `json:"traveos_airline_id" validate:"omitempty,gt=0"`
	// BrandColor is validated as a 6-digit hex colour because it is interpolated
	// straight into the ticket's CSS - a free-form string there would be a way to
	// inject style rules into every ticket the airline appears on.
	BrandColor *string `json:"brand_color" validate:"omitempty,hexcolor"`
}

// UpdateAirlineRequest defines the payload for updating an existing airline.
// Fields are optional pointers: nil means "don't change this field", except for
// the two nullable columns (TraveosAirlineID and BrandColor) where nil means
// "clear it" - otherwise a value set by mistake could never be taken back.
type UpdateAirlineRequest struct {
	Name                   *string `json:"name" validate:"omitempty"`
	Code                   *string `json:"code" validate:"omitempty"`
	TraveosSupported       *bool   `json:"traveos_supported" validate:"omitempty"`
	RequiresLastName       *bool   `json:"requires_last_name" validate:"omitempty"`
	RequiresPurchaseNumber *bool   `json:"requires_purchase_number" validate:"omitempty"`
	TraveosAirlineID       *int    `json:"traveos_airline_id" validate:"omitempty,gt=0"`
	BrandColor             *string `json:"brand_color" validate:"omitempty,hexcolor"`
}

// AirlineResponse defines how an airline should be returned in the API.
type AirlineResponse struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Code                   string    `json:"code"`
	TraveosSupported       bool      `json:"traveos_supported"`
	RequiresLastName       bool      `json:"requires_last_name"`
	RequiresPurchaseNumber bool      `json:"requires_purchase_number"`
	TraveosAirlineID       *int      `json:"traveos_airline_id"`
	BrandColor             *string   `json:"brand_color"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}
