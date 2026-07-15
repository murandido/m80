package schemas

import (
	"time"
)

// CreateClientRequest defines the expected payload to create a new client
type CreateClientRequest struct {
	Name  string  `json:"name" validate:"required"`
	Phone *string `json:"phone" validate:"omitempty"`
}

// ClientResponse defines how a client should be returned in the API
type ClientResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Phone     *string   `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateClientRequest defines the payload for updating an existing client
type UpdateClientRequest struct {
	Name  string  `json:"name" validate:"omitempty"`
	Phone *string `json:"phone" validate:"omitempty"`
}
