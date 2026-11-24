package schemas

import "time"

// InjunctionGroupRequest creates or replaces an entry of the Controle de Liminares. SpreadsheetURL null keeps the stored
// link on an update (the panel never has it to send back); "" removes it.
type InjunctionGroupRequest struct {
	Name             string                   `json:"name" validate:"required,max=80" example:"Tropa Azul 1"`
	WhatsAppChatName *string                  `json:"whatsapp_chat_name" validate:"omitempty,max=120"`
	SpreadsheetURL   *string                  `json:"spreadsheet_url" validate:"omitempty,url,max=2000"`
	Accounts         []InjunctionAccountLabel `json:"accounts" validate:"dive"`
	Issuers          []InjunctionIssuerLabel  `json:"issuers" validate:"dive"`
}

// InjunctionAccountLabel maps the Conta column of the workbook onto an injunction account.
// The program comes from the account itself.
type InjunctionAccountLabel struct {
	AccountLabel string `json:"account_label" validate:"required,max=80" example:"HUDSON"`
	AccountID    string `json:"account_id" validate:"required,uuid"`
}

// InjunctionIssuerLabel maps the Emissor column onto a panel user and the agency their
// issuances belong to: one workbook mixes issuers of different agencies.
type InjunctionIssuerLabel struct {
	IssuerName string `json:"issuer_name" validate:"required,max=80" example:"PEDRO"`
	UserID     string `json:"user_id" validate:"required,uuid"`
	AgencyID   string `json:"agency_id" validate:"required,uuid"`
}

// InjunctionGroupAccountResponse is one mapped account with what the panel needs to show it.
type InjunctionGroupAccountResponse struct {
	AccountLabel string `json:"account_label"`
	AccountID    string `json:"account_id"`
	ProgramCode  string `json:"program_code"`
	ProgramName  string `json:"program_name"`
	Nicknames    string `json:"nicknames"`
}

// InjunctionGroupIssuerResponse is one mapped issuer.
type InjunctionGroupIssuerResponse struct {
	IssuerName string `json:"issuer_name"`
	UserID     string `json:"user_id"`
	UserName   string `json:"user_name"`
	AgencyID   string `json:"agency_id"`
	AgencyName string `json:"agency_name"`
}

// InjunctionGroupResponse is a Tropa as the panel lists it. The workbook link is masked.
type InjunctionGroupResponse struct {
	ID               string                           `json:"id"`
	Name             string                           `json:"name"`
	WhatsAppChatName *string                          `json:"whatsapp_chat_name"`
	HasSpreadsheet   bool                             `json:"has_spreadsheet"`
	SpreadsheetHint  *string                          `json:"spreadsheet_hint" example:"onedrive.live.com/…xvHPc"`
	Accounts         []InjunctionGroupAccountResponse `json:"accounts"`
	Issuers          []InjunctionGroupIssuerResponse  `json:"issuers"`
	CreatedAt        time.Time                        `json:"created_at"`
	UpdatedAt        time.Time                        `json:"updated_at"`
}

// InjunctionImportRequest imports rows of one workbook day. Rows empty means every row of
// the Tropa that is ready to import; rows of other accounts are never imported.
type InjunctionImportRequest struct {
	Day  string                   `json:"day" validate:"required,datetime=2006-01-02" example:"2026-11-20"`
	Rows []InjunctionRowSelection `json:"rows" validate:"dive"`
}

// InjunctionRowSelection names one row of a workbook day.
type InjunctionRowSelection struct {
	BlockIndex int `json:"block_index"`
	Number     int `json:"number"`
}

// InjunctionImportPeriodRequest imports every ready row of the Tropa between two days - the
// one-off backfill before the daily routine starts.
type InjunctionImportPeriodRequest struct {
	From string `json:"from" validate:"required,datetime=2006-01-02" example:"2026-10-01"`
	To   string `json:"to" validate:"required,datetime=2006-01-02" example:"2026-11-30"`
}
