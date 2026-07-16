package schemas

import "time"

// CreateIssuancePassengerRequest defines the expected payload to create a new issuance passenger.
type CreateIssuancePassengerRequest struct {
	FirstName string  `json:"first_name" validate:"required"`
	LastName  string  `json:"last_name" validate:"required"`
	Type      *string `json:"type" validate:"omitempty,oneof=ADT CHD INF"`
	Document  *string `json:"document" validate:"omitempty"`
	Position  int     `json:"position" validate:"required"`
}

// UpdateIssuancePassengerRequest defines the payload for updating an existing issuance passenger.
// All fields are optional pointers: nil means "don't change this field".
type UpdateIssuancePassengerRequest struct {
	FirstName *string `json:"first_name" validate:"omitempty"`
	LastName  *string `json:"last_name" validate:"omitempty"`
	Type      *string `json:"type" validate:"omitempty,oneof=ADT CHD INF"`
	Document  *string `json:"document" validate:"omitempty"`
	Position  *int    `json:"position" validate:"omitempty"`
}

// IssuancePassengerResponse defines how an issuance passenger should be returned in the API.
type IssuancePassengerResponse struct {
	ID         string    `json:"id"`
	IssuanceID string    `json:"issuance_id"`
	FirstName  string    `json:"first_name"`
	LastName   string    `json:"last_name"`
	Type       string    `json:"type"`
	Document   *string   `json:"document"`
	Position   int       `json:"position"`
	CreatedAt  time.Time `json:"created_at"`
}
