package schemas

import "time"

// PortalAccount is one Azul/Smiles account as a statement client sees it in their list: whose
// it is, how it is synced and the last balance read. It never carries the password - only
// whether one is saved, which is what decides if the client can press "Conectar".
type PortalAccount struct {
	ID           string `json:"id"`
	ProgramCode  string `json:"program_code" example:"AZUL"`
	ProgramID    string `json:"program_id"`
	ProgramName  string `json:"program_name" example:"Azul Fidelidade"`
	ProgramColor string `json:"program_color" example:"#1D4ED8"`
	HolderName   string `json:"holder_name" example:"MARIA SILVA"`
	// Login is the account's own number/CPF at the airline; it belongs to the client.
	Login           *string `json:"login"`
	HasPassword     bool    `json:"has_password"`
	TracksStatement bool    `json:"tracks_statement"`
	// Connected/State/LastSyncedAt/LastBalancePoints mirror the program's sync-state response.
	Connected         bool       `json:"connected"`
	State             string     `json:"state" example:"ok"`
	LastSyncedAt      *time.Time `json:"last_synced_at"`
	LastBalancePoints *int       `json:"last_balance_points"`
}

// PortalAccountListResponse wraps the list for the swagger docs.
type PortalAccountListResponse struct {
	Success bool            `json:"success" example:"true"`
	Message string          `json:"message" example:"Contas carregadas"`
	Data    []PortalAccount `json:"data"`
}

// PortalPasswordRequest is a new airline password for the account. Write-only: no route ever
// returns it to the client.
type PortalPasswordRequest struct {
	Password string `json:"password" validate:"required,max=200"`
}
