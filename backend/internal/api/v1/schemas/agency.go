package schemas

import "time"

// CreateAgencyRequest defines the expected payload to create a new agency
type CreateAgencyRequest struct {
	Name      string `json:"name" validate:"required"`
	IsDefault *bool  `json:"is_default" validate:"omitempty"`
}

// UpdateAgencyRequest defines the payload for updating an existing agency.
// All fields are optional pointers: nil means "don't change this field".
type UpdateAgencyRequest struct {
	Name      *string `json:"name" validate:"omitempty"`
	IsDefault *bool   `json:"is_default" validate:"omitempty"`
}

// AgencyResponse defines how an agency should be returned in the API.
type AgencyResponse struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	IsDefault bool                     `json:"is_default"`
	Nicknames []AgencyNicknameResponse `json:"nicknames"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}
