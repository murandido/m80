package schemas

import "time"

// CreatePersonNicknameRequest defines the expected payload to create a new person nickname
type CreatePersonNicknameRequest struct {
	Nickname string `json:"nickname" validate:"required"`
}

// UpdatePersonNicknameRequest defines the payload for updating an existing person nickname.
// The field is an optional pointer: nil means "don't change this field".
type UpdatePersonNicknameRequest struct {
	Nickname *string `json:"nickname" validate:"omitempty"`
}

// PersonNicknameResponse defines how a person nickname should be returned in the API
type PersonNicknameResponse struct {
	ID        string    `json:"id"`
	PersonID  string    `json:"person_id"`
	Nickname  string    `json:"nickname"`
	CreatedAt time.Time `json:"created_at"`
}

// NicknameAvailabilityResponse reports whether a nickname is free to use
type NicknameAvailabilityResponse struct {
	Available bool `json:"available"`
}
