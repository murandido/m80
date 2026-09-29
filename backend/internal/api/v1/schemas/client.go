package schemas

import (
	"time"
)

// CreateClientRequest defines the expected payload to create a new client
type CreateClientRequest struct {
	Name  string  `json:"name" validate:"required"`
	Phone *string `json:"phone" validate:"omitempty"`
	// AgencyID is the agency the client buys through. Absent means the caller's default
	// agency, so a panel tab opened before this field existed still creates clients.
	AgencyID *string `json:"agency_id" validate:"omitempty,uuid"`
}

// ClientResponse defines how a client should be returned in the API
type ClientResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Phone     *string   `json:"phone"`
	AgencyID  *string   `json:"agency_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateClientRequest defines the payload for updating an existing client
type UpdateClientRequest struct {
	Name     string  `json:"name" validate:"omitempty"`
	Phone    *string `json:"phone" validate:"omitempty"`
	AgencyID *string `json:"agency_id" validate:"omitempty,uuid"`
}
