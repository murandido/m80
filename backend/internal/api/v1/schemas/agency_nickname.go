package schemas

import "time"

// CreateAgencyNicknameRequest defines the expected payload to create a new agency nickname
type CreateAgencyNicknameRequest struct {
	Nickname string `json:"nickname" validate:"required"`
}

// UpdateAgencyNicknameRequest defines the payload for updating an existing agency nickname.
// The field is an optional pointer: nil means "don't change this field".
type UpdateAgencyNicknameRequest struct {
	Nickname *string `json:"nickname" validate:"omitempty"`
}

// AgencyNicknameResponse defines how an agency nickname should be returned in the API
type AgencyNicknameResponse struct {
	ID        string    `json:"id"`
	AgencyID  string    `json:"agency_id"`
	Nickname  string    `json:"nickname"`
	CreatedAt time.Time `json:"created_at"`
}
