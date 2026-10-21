package schemas

// VaultAccountResponse is one row of the browser extension's vault list: enough to find an
// account and to know which site it logs into, and no password. The password is fetched one
// account at a time, through an audited endpoint (AccountCredentialsResponse).
type VaultAccountResponse struct {
	ID          string       `json:"id"`
	Login       *string      `json:"login"`
	HolderName  *string      `json:"holder_name"`
	Program     VaultProgram `json:"program"`
	Nicknames   []string     `json:"nicknames"`
	HasPassword bool         `json:"has_password"`
}

// VaultProgram is the slice of a loyalty program the vault shows and matches sites by.
type VaultProgram struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Code  string `json:"code"`
	Color string `json:"color"`
}

// AccountCredentialsRequest says what the password is being fetched for, which is what the
// access trail records.
type AccountCredentialsRequest struct {
	Action string `json:"action" validate:"required,oneof=copy fill"`
}

// AccountCredentialsResponse is one account's login and decrypted password.
type AccountCredentialsResponse struct {
	Login    *string `json:"login"`
	Password string  `json:"password"`
}
