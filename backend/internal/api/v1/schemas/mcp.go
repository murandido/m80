package schemas

import (
	"encoding/json"
	"time"
)

// CreateMCPTokenRequest names a new MCP token. The token always belongs to the caller.
type CreateMCPTokenRequest struct {
	Name string `json:"name" validate:"required,max=80" example:"Claude Code - notebook"`
	// UserID is whose authority the token carries. Omitted means the caller. A dedicated
	// agent user is how an unattended routine gets a narrower role and its own trail.
	UserID *string `json:"user_id" validate:"omitempty,uuid"`
	// AllowedWriteTools is which free-write tools run directly; the rest become proposals.
	// Omitted (null) means all of them - the right default for a person's own token. An empty
	// list means none.
	AllowedWriteTools *[]string `json:"allowed_write_tools"`
	// Limits cap the direct writes; past them a write becomes a proposal.
	Limits *MCPTokenLimits `json:"limits"`
}

// MCPTokenLimits are the ceilings on a token's direct writes. Null is no ceiling.
type MCPTokenLimits struct {
	MaxBatchPoints   *int `json:"max_batch_points" validate:"omitempty,gt=0" example:"200000"`
	MaxBatchesPerDay *int `json:"max_batches_per_day" validate:"omitempty,gt=0" example:"5"`
}

// MCPTokenResponse is a token as the panel lists it. The plain value never appears here.
type MCPTokenResponse struct {
	ID       string  `json:"id"`
	UserID   string  `json:"user_id"`
	UserName *string `json:"user_name"`
	Name     string  `json:"name"`
	// AllowedWriteTools null means every free write runs directly.
	AllowedWriteTools []string        `json:"allowed_write_tools"`
	Limits            *MCPTokenLimits `json:"limits"`
	LastUsedAt        *time.Time      `json:"last_used_at"`
	RevokedAt         *time.Time      `json:"revoked_at"`
	CreatedAt         time.Time       `json:"created_at"`
}

// CreatedMCPTokenResponse is the only response that carries the plain token: it is not stored,
// so this is the one chance to copy it.
type CreatedMCPTokenResponse struct {
	MCPTokenResponse
	Token string `json:"token" example:"m80mcp_..."`
}

// MCPCallResponse is one tool call from the MCP activity trail.
type MCPCallResponse struct {
	ID         string          `json:"id"`
	TokenID    string          `json:"token_id"`
	TokenName  *string         `json:"token_name"`
	Tool       string          `json:"tool"`
	Args       json.RawMessage `json:"args" swaggertype:"object"`
	OK         bool            `json:"ok"`
	Error      *string         `json:"error"`
	DurationMs int             `json:"duration_ms"`
	CreatedAt  time.Time       `json:"created_at"`
}

// MCPProposalResponse is a proposal as the review page shows it.
type MCPProposalResponse struct {
	ID            string          `json:"id"`
	UserID        string          `json:"user_id"`
	UserName      *string         `json:"user_name"`
	Tool          string          `json:"tool"`
	Summary       string          `json:"summary"`
	Operations    json.RawMessage `json:"operations" swaggertype:"object"`
	Preview       json.RawMessage `json:"preview" swaggertype:"object"`
	Status        string          `json:"status" example:"pending"`
	Result        json.RawMessage `json:"result" swaggertype:"object"`
	CreatedAt     time.Time       `json:"created_at"`
	ExpiresAt     time.Time       `json:"expires_at"`
	DecidedAt     *time.Time      `json:"decided_at"`
	DecidedByID   *string         `json:"decided_by_id"`
	DecidedByName *string         `json:"decided_by_name"`
}

// MCPReportResponse is one day's report from an agent's routine.
type MCPReportResponse struct {
	ID         string          `json:"id"`
	TokenID    string          `json:"token_id"`
	TokenName  *string         `json:"token_name"`
	ReportDate string          `json:"report_date" example:"2026-11-23"`
	Title      string          `json:"title"`
	Summary    string          `json:"summary"`
	Findings   json.RawMessage `json:"findings" swaggertype:"array,object"`
	Actions    json.RawMessage `json:"actions" swaggertype:"array,object"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// MicrosoftConnectionResponse is the Microsoft login as the panel shows it. The token never
// leaves the API.
type MicrosoftConnectionResponse struct {
	Configured   bool       `json:"configured"`
	Connected    bool       `json:"connected"`
	State        *string    `json:"state" example:"ok"`
	Detail       *string    `json:"detail"`
	AccountName  *string    `json:"account_name"`
	AccountEmail *string    `json:"account_email"`
	LastOKAt     *time.Time `json:"last_ok_at"`
	ConnectedAt  *time.Time `json:"connected_at"`
}

// MicrosoftCallbackRequest is what the panel forwards from Microsoft's redirect.
type MicrosoftCallbackRequest struct {
	Code  string `json:"code" validate:"required"`
	State string `json:"state" validate:"required"`
}
