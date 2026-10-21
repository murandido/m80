package schemas

import "time"

// LatamConnectResponse is what starting a LATAM connection returns: the LATAM login page the
// operator opens.
type LatamConnectResponse struct {
	AuthorizeURL string `json:"authorize_url" example:"https://accounts.latamairlines.com/authorize?..."`
}

// LatamConnectSingleResponse is the start route's envelope.
type LatamConnectSingleResponse struct {
	Success bool                 `json:"success" example:"true"`
	Message string               `json:"message" example:"Abra o login da LATAM"`
	Data    LatamConnectResponse `json:"data"`
}

// LatamCallbackRequest carries LATAM's answer to the login: the address the login page sent the
// browser to.
type LatamCallbackRequest struct {
	CallbackURL string `json:"callback_url" validate:"required,max=4096" example:"...?code=...&state=..."`
}

// LatamCallbackResponse names the account the login linked, so the callback page can go to it.
type LatamCallbackResponse struct {
	AccountID string `json:"account_id" example:"0b8f6c1e-5a0e-4d55-9f3c-2a1f8c3e4b7d"`
	Connected bool   `json:"connected" example:"true"`
}

// LatamCallbackSingleResponse is the callback route's envelope.
type LatamCallbackSingleResponse struct {
	Success bool                  `json:"success" example:"true"`
	Message string                `json:"message" example:"Conta conectada"`
	Data    LatamCallbackResponse `json:"data"`
}

// LatamSyncStateResponse is the account's balance-read lifecycle as the panel reads it. Same field
// names as the Azul and Smiles badges, so the panel keeps one vocabulary.
type LatamSyncStateResponse struct {
	// Connected is whether a session is stored for the account.
	Connected bool `json:"connected" example:"true"`
	// State is one of pending|ok|error|needs_login|needs_attention (see data.LatamSync*).
	State        string     `json:"state" example:"ok"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	// LastBalancePoints is the account's real_balance_points: LATAM keeps no balance of its own
	// in the state, so this is the figure the last read wrote.
	LastBalancePoints   *int       `json:"last_balance_points" example:"84321"`
	ConsecutiveFailures int        `json:"consecutive_failures" example:"0"`
	Detail              *string    `json:"detail"`
	UpdatedAt           *time.Time `json:"updated_at"`
}

// LatamSyncStateSingleResponse is the status route's envelope.
type LatamSyncStateSingleResponse struct {
	Success bool                   `json:"success" example:"true"`
	Message string                 `json:"message" example:"Estado carregado"`
	Data    LatamSyncStateResponse `json:"data"`
}
