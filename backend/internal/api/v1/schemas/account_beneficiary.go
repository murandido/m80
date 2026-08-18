package schemas

import "time"

// CreateAccountBeneficiaryRequest adds someone the operator knows was issued on this
// account, for the case where the issuance itself is not in the system.
type CreateAccountBeneficiaryRequest struct {
	Name string `json:"name" validate:"required"`
	// The full CPF. It is used to compute a fingerprint and a mask and is then discarded -
	// the number itself is never written to the database.
	CPF *string `json:"cpf" validate:"omitempty"`
	// When the issuance happened, which is what decides the window it falls in. Defaults to
	// now, which is right for someone recording today's issuance.
	UsedAt *time.Time `json:"used_at" validate:"omitempty"`
}

// UpdateAccountBeneficiaryRequest corrects what is known about a beneficiary. All fields
// optional: nil means "leave this alone".
type UpdateAccountBeneficiaryRequest struct {
	Name *string `json:"name" validate:"omitempty"`
	CPF  *string `json:"cpf" validate:"omitempty"`
	// IsHolder takes the person out of the count: none of the programs charges the account
	// holder against the cap.
	IsHolder *bool `json:"is_holder" validate:"omitempty"`
}

// MergeAccountBeneficiaryRequest names the beneficiary to absorb into the one in the URL.
type MergeAccountBeneficiaryRequest struct {
	SourceID string `json:"source_id" validate:"required,uuid"`
}

// AccountBeneficiaryResponse is one person on the account's ledger.
//
// The CPF is never returned in full because it is never stored in full: CPFMasked is built
// from the number at write time and HasCPF says whether the row is identified by a document
// at all - which is what tells the operator how much to trust the match.
type AccountBeneficiaryResponse struct {
	ID        string  `json:"id"`
	AccountID string  `json:"account_id"`
	Name      string  `json:"name"`
	CPFMasked *string `json:"cpf_masked"`
	HasCPF    bool    `json:"has_cpf"`
	IsHolder  bool    `json:"is_holder"`
	// Uses is how many issuances this person accounts for, over the whole history rather
	// than the current window - the panel shows it as context, not as the cap counter.
	Uses       int        `json:"uses"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// AccountBeneficiaryListResponse documents the envelope for swagger.
type AccountBeneficiaryListResponse struct {
	Success bool                         `json:"success"`
	Message string                       `json:"message"`
	Data    []AccountBeneficiaryResponse `json:"data"`
}

// AccountBeneficiarySingleResponse documents the envelope for swagger.
type AccountBeneficiarySingleResponse struct {
	Success bool                       `json:"success"`
	Message string                     `json:"message"`
	Data    AccountBeneficiaryResponse `json:"data"`
}
