package schemas

import (
	"encoding/json"
	"time"
)

// CreateMCPTokenRequest names a new MCP token. The token always belongs to the caller.
type CreateMCPTokenRequest struct {
	Name string `json:"name" validate:"required,max=80" example:"Claude Code - notebook"`
}

// MCPTokenResponse is a token as the panel lists it. The plain value never appears here.
type MCPTokenResponse struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	UserName   *string    `json:"user_name"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
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
