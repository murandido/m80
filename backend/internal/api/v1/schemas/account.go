package schemas

import "time"

// CreateAccountRequest defines the expected payload to create a new account
type CreateAccountRequest struct {
	LoyaltyProgramID string  `json:"loyalty_program_id" validate:"required,uuid"`
	Login            *string `json:"login" validate:"omitempty"`
	Password         *string `json:"password" validate:"omitempty"`
	HolderID         *string `json:"holder_id" validate:"omitempty,uuid"`
	AdministratorID  *string `json:"administrator_id" validate:"omitempty,uuid"`
	// Complete overrides the automatically computed value when provided
	Complete *bool `json:"complete" validate:"omitempty"`
}

// UpdateAccountRequest defines the payload for updating an existing account.
// All fields are optional pointers: nil means "don't change this field".
type UpdateAccountRequest struct {
	LoyaltyProgramID *string `json:"loyalty_program_id" validate:"omitempty,uuid"`
	Login            *string `json:"login" validate:"omitempty"`
	Password         *string `json:"password" validate:"omitempty"`
	HolderID         *string `json:"holder_id" validate:"omitempty,uuid"`
	AdministratorID  *string `json:"administrator_id" validate:"omitempty,uuid"`
	// Complete overrides the automatically recomputed value when provided
	Complete *bool `json:"complete" validate:"omitempty"`
}

// AccountResponse defines how an account should be returned in the API.
// Password is returned decrypted (unlike User's password, which is write-only)
// since staff need to see it to log into the loyalty program on the customer's behalf.
type AccountResponse struct {
	ID               string                    `json:"id"`
	LoyaltyProgramID string                    `json:"loyalty_program_id"`
	LoyaltyProgram   LoyaltyProgramResponse    `json:"loyalty_program"`
	Login            *string                   `json:"login"`
	Password         *string                   `json:"password"`
	HolderID         *string                   `json:"holder_id"`
	AdministratorID  *string                   `json:"administrator_id"`
	Nicknames        []AccountNicknameResponse `json:"nicknames"`
	Complete         bool                      `json:"complete"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
	// Only ever non-null on an account reached through a past issuance - the account
	// list itself never returns archived ones. It is what lets the grid label an old
	// row with an archived account while keeping it out of the picker.
	DeletedAt *time.Time `json:"deleted_at"`
}
