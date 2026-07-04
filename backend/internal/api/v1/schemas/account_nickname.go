package schemas

import "time"

// CreateAccountNicknameRequest defines the expected payload to create a new account nickname
type CreateAccountNicknameRequest struct {
	LoyaltyProgramID string `json:"loyalty_program_id" validate:"required,uuid"`
	Nickname         string `json:"nickname" validate:"required"`
}

// UpdateAccountNicknameRequest defines the payload for updating an existing account nickname.
// All fields are optional pointers: nil means "don't change this field".
type UpdateAccountNicknameRequest struct {
	LoyaltyProgramID *string `json:"loyalty_program_id" validate:"omitempty,uuid"`
	Nickname         *string `json:"nickname" validate:"omitempty"`
}

// AccountNicknameResponse defines how an account nickname should be returned in the API
type AccountNicknameResponse struct {
	ID               string    `json:"id"`
	AccountID        string    `json:"account_id"`
	LoyaltyProgramID string    `json:"loyalty_program_id"`
	Nickname         string    `json:"nickname"`
	CreatedAt        time.Time `json:"created_at"`
}
