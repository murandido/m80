package schemas

import (
	"time"

	"github.com/murandido/m-80-backend/internal/account"
)

// CreateAccountRequest defines the expected payload to create a new account
type CreateAccountRequest struct {
	LoyaltyProgramID string  `json:"loyalty_program_id" validate:"required,uuid"`
	Login            *string `json:"login" validate:"omitempty"`
	Password         *string `json:"password" validate:"omitempty"`
	HolderID         *string `json:"holder_id" validate:"omitempty,uuid"`
	AdministratorID  *string `json:"administrator_id" validate:"omitempty,uuid"`
	// Category decides whether the program's beneficiary cap applies. Omitted means
	// "unidentified", the honest default for an account nobody has classified.
	Category *string `json:"category" validate:"omitempty,oneof=unidentified standard injunction"`
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
	Category         *string `json:"category" validate:"omitempty,oneof=unidentified standard injunction"`
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
	// Category is "unidentified", "standard" or "injunction" - the Portuguese labels
	// live on the screen. See data.AccountCategory*.
	Category string `json:"category"`
	// CPFUsage is how much of the program's beneficiary cap this account has consumed,
	// and is null whenever there is no cap to report: an injunction suspends it, an
	// unidentified account gives no basis to claim one, and a program nobody has
	// researched has no limit. Null means "render no counter", not "zero".
	CPFUsage  *account.Usage `json:"cpf_usage"`
	Complete  bool           `json:"complete"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	// Only ever non-null on an account reached through a past issuance - the account
	// list itself never returns archived ones. It is what lets the grid label an old
	// row with an archived account while keeping it out of the picker.
	DeletedAt *time.Time `json:"deleted_at"`
}

// MergeAccountRequest names the account to absorb into the one in the URL.
type MergeAccountRequest struct {
	SourceID string `json:"source_id" validate:"required,uuid"`
}

// MergeAccountPreviewResponse is what a merge would move, counted before it runs.
type MergeAccountPreviewResponse struct {
	// Issuances includes the soft-deleted ones: they hold account_id like any other.
	Issuances    int64    `json:"issuances"`
	Nicknames    int64    `json:"nicknames"`
	FieldsToFill []string `json:"fields_to_fill"`
	// CollidingNicknames are the absorbed aliases the surviving account already answers
	// to. They are archived instead of moved, so the panel warns about them by name.
	CollidingNicknames []string `json:"colliding_nicknames"`
}
