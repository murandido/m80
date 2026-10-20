package data

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// mcpTokenPrefix makes a leaked MCP token recognizable at a glance (and by secret scanners),
// and keeps it from ever being mistaken for a login JWT.
const mcpTokenPrefix = "m80mcp_"

// MCPToken is a personal credential for the MCP server. See migration 000087 for why it is
// separate from the login JWT.
type MCPToken struct {
	ID         string     `gorm:"primaryKey;type:uuid" json:"id"`
	UserID     string     `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	Name       string     `gorm:"not null" json:"name"`
	TokenHash  string     `gorm:"column:token_hash;not null" json:"-"`
	LastUsedAt *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at" json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (MCPToken) TableName() string { return "mcp_tokens" }

func (t *MCPToken) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}

// NewMCPTokenSecret returns a fresh token and the hash to store for it.
func NewMCPTokenSecret() (token, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token = mcpTokenPrefix + hex.EncodeToString(raw)
	return token, HashMCPToken(token), nil
}

// HashMCPToken is the digest stored in mcp_tokens.token_hash.
func HashMCPToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
