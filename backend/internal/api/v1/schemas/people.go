package schemas

import "time"

// CreatePersonRequest defines the expected payload to create a new person
type CreatePersonRequest struct {
	Name          string  `json:"name" validate:"required"`
	CPF           *string `json:"cpf" validate:"omitempty"`
	BirthDate     *string `json:"birth_date" validate:"omitempty"`
	Email         *string `json:"email" validate:"omitempty,email"`
	Phone         *string `json:"phone" validate:"omitempty"`
	ZipCode       *string `json:"zip_code" validate:"omitempty"`
	State         *string `json:"state" validate:"omitempty"`
	City          *string `json:"city" validate:"omitempty"`
	Address       *string `json:"address" validate:"omitempty"`
	AddressNumber *string `json:"address_number" validate:"omitempty"`
	Complement    *string `json:"complement" validate:"omitempty"`
	Neighborhood  *string `json:"neighborhood" validate:"omitempty"`
	// Complete overrides the automatically computed value when provided
	Complete *bool `json:"complete" validate:"omitempty"`
}

// UpdatePersonRequest defines the payload for updating an existing person.
// All fields are optional pointers: nil means "don't change this field".
type UpdatePersonRequest struct {
	Name          *string `json:"name" validate:"omitempty"`
	CPF           *string `json:"cpf" validate:"omitempty"`
	BirthDate     *string `json:"birth_date" validate:"omitempty"`
	Email         *string `json:"email" validate:"omitempty,email"`
	Phone         *string `json:"phone" validate:"omitempty"`
	ZipCode       *string `json:"zip_code" validate:"omitempty"`
	State         *string `json:"state" validate:"omitempty"`
	City          *string `json:"city" validate:"omitempty"`
	Address       *string `json:"address" validate:"omitempty"`
	AddressNumber *string `json:"address_number" validate:"omitempty"`
	Complement    *string `json:"complement" validate:"omitempty"`
	Neighborhood  *string `json:"neighborhood" validate:"omitempty"`
	// Complete overrides the automatically recomputed value when provided
	Complete *bool `json:"complete" validate:"omitempty"`
}

// PersonResponse defines how a person should be returned in the API
type PersonResponse struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	CPF           *string                  `json:"cpf"`
	BirthDate     *time.Time               `json:"birth_date"`
	Email         *string                  `json:"email"`
	Phone         *string                  `json:"phone"`
	ZipCode       *string                  `json:"zip_code"`
	State         *string                  `json:"state"`
	City          *string                  `json:"city"`
	Address       *string                  `json:"address"`
	AddressNumber *string                  `json:"address_number"`
	Complement    *string                  `json:"complement"`
	Neighborhood  *string                  `json:"neighborhood"`
	Nicknames     []PersonNicknameResponse `json:"nicknames"`
	Complete      bool                     `json:"complete"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
}

// PersonPickerResponse is the person as a combobox needs her: the id to select by, the
// name to show, and the nicknames the operator may type instead of the name.
//
// It exists because the pickers were the heaviest thing on the panel. GET /people is
// unpaginated by design - a combobox that only holds a page of names is useless - so
// every screen with a holder, administrator or passenger picker was shipping all
// fourteen columns of every person. At three thousand people that is 1.36 MB per page
// load, serialised by the API, again by Next into the HTML, and gzipped by Caddy on the
// way out. The three fields below are the ones person-combobox.tsx actually reads.
type PersonPickerResponse struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Nicknames []PersonNicknameResponse `json:"nicknames"`
}

// MergeRowRequest names the holder whose row is absorbed into the holder in the URL.
type MergeRowRequest struct {
	SourcePersonID string `json:"source_person_id" validate:"required,uuid"`
	// KeepNickname registers the absorbed holder's name as an alias of the survivor. Omitted
	// means yes: the duplicate usually exists because that name was never registered, and
	// leaving it unregistered invites the bot to recreate it.
	KeepNickname *bool `json:"keep_nickname" validate:"omitempty"`
}

// MergeRowAccountPair is one loyalty account the absorbed row has in a program the surviving
// holder already has an account in - the two get merged into the survivor's.
type MergeRowAccountPair struct {
	ProgramName        string   `json:"program_name"`
	KeepAccountID      string   `json:"keep_account_id"`
	DropAccountID      string   `json:"drop_account_id"`
	DropLogin          *string  `json:"drop_login"`
	Issuances          int64    `json:"issuances"`
	CollidingNicknames []string `json:"colliding_nicknames"`
}

// MergeRowPreviewResponse is what merging a row would do, worked out before it runs.
type MergeRowPreviewResponse struct {
	// AccountsMerged are folded into the survivor's account in the same program.
	AccountsMerged []MergeRowAccountPair `json:"accounts_merged"`
	// AccountsMoved only change holder: programs the survivor did not have yet.
	AccountsMoved int64 `json:"accounts_moved"`
	// Administered counts the accounts the absorbed person administers, which follow them.
	Administered     int64    `json:"administered"`
	Nicknames        int64    `json:"nicknames"`
	FieldsToFill     []string `json:"fields_to_fill"`
	NicknameFromName string   `json:"nickname_from_name"`
}

// MergeRowResultResponse is whatever the merge could not do, for the panel to warn about.
type MergeRowResultResponse struct {
	Warning string `json:"warning"`
}

// SetHolderAdministratorRequest names the administrator of every account a holder has.
// Empty or null clears it.
type SetHolderAdministratorRequest struct {
	AdministratorID *string `json:"administrator_id" validate:"omitempty,uuid"`
}

// PersonRemovalResponse is what removing a holder did (or would do, for the preview).
type PersonRemovalResponse struct {
	AccountsDeleted  int  `json:"accounts_deleted"`
	AccountsArchived int  `json:"accounts_archived"`
	PersonDeleted    bool `json:"person_deleted"`
}

// PersonRemovalSingleResponse is the DELETE /people/{id} and delete-preview route response.
type PersonRemovalSingleResponse struct {
	Success bool                  `json:"success" example:"true"`
	Message string                `json:"message" example:"Titular excluído"`
	Data    PersonRemovalResponse `json:"data"`
}
