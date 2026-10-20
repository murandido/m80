package schemas

import "time"

type CreateMCPTokenRequest struct {
	Name string `json:"name" validate:"required,max=100"`
}

type MCPTokenResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreatedMCPTokenResponse is the only response that ever carries the token itself: only its
// hash is stored, so it cannot be shown again.
type CreatedMCPTokenResponse struct {
	MCPTokenResponse
	Token string `json:"token"`
}
