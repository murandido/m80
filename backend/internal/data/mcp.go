package data

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// MCPToken is a credential for the personal MCP server (see migration 000093). Only the hash
// is stored; HashMCPToken is the one place the two are related.
type MCPToken struct {
	ID         string     `gorm:"primaryKey;type:uuid" json:"id"`
	UserID     string     `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	User       *User      `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Name       string     `gorm:"not null" json:"name"`
	TokenHash  string     `gorm:"column:token_hash;not null" json:"-"`
	LastUsedAt *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at" json:"revoked_at"`
	// AllowedWriteTools is which free-write tools the token runs directly (migration 000094).
	// nil means all of them; an empty list means none, and every write becomes a proposal.
	AllowedWriteTools pq.StringArray `gorm:"column:allowed_write_tools;type:text[]" json:"allowed_write_tools"`
	// Limits caps what the token writes directly; past a ceiling a write becomes a proposal.
	Limits    *MCPTokenLimits `gorm:"column:limits;type:jsonb;serializer:json" json:"limits"`
	CreatedAt time.Time       `json:"created_at"`
}

// MCPTokenLimits are the ceilings on a token's direct writes. A nil field is no ceiling.
type MCPTokenLimits struct {
	MaxBatchPoints   *int `json:"max_batch_points,omitempty"`
	MaxBatchesPerDay *int `json:"max_batches_per_day,omitempty"`
}

func (t *MCPToken) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}

// HashMCPToken is sha256 rather than a password hash on purpose: the token is 32 random bytes,
// so there is nothing to brute-force, and the lookup has to be an indexed equality.
func HashMCPToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// MCPCall is one tool call served by the MCP server.
type MCPCall struct {
	ID         string         `gorm:"primaryKey;type:uuid" json:"id"`
	TokenID    string         `gorm:"column:token_id;type:uuid;not null" json:"token_id"`
	Tool       string         `gorm:"not null" json:"tool"`
	Args       datatypes.JSON `gorm:"type:jsonb" json:"args"`
	OK         bool           `gorm:"column:ok;not null" json:"ok"`
	Error      *string        `json:"error"`
	DurationMs int            `gorm:"column:duration_ms;not null" json:"duration_ms"`
	CreatedAt  time.Time      `json:"created_at"`
}

func (c *MCPCall) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return nil
}

// The values mcp_proposals.status may take - the table constrains it to exactly these.
const (
	MCPProposalPending  = "pending"
	MCPProposalApplied  = "applied"
	MCPProposalRejected = "rejected"
	MCPProposalFailed   = "failed"
	MCPProposalExpired  = "expired"
)

// MCPProposalLifetime is how long a proposal waits for a decision. An older one is expired on
// read instead of by a job: a stale cancellation approved days later would act on a world that
// has moved on.
const MCPProposalLifetime = 48 * time.Hour

// MCPProposal is an operation the agent may not run on its own, waiting for a dev's decision.
type MCPProposal struct {
	ID          string         `gorm:"primaryKey;type:uuid" json:"id"`
	UserID      string         `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	Tool        string         `gorm:"not null" json:"tool"`
	Summary     string         `gorm:"not null" json:"summary"`
	Operations  datatypes.JSON `gorm:"type:jsonb;not null" json:"operations"`
	Preview     datatypes.JSON `gorm:"type:jsonb" json:"preview"`
	Status      string         `gorm:"not null;default:pending" json:"status"`
	Result      datatypes.JSON `gorm:"type:jsonb" json:"result"`
	CreatedAt   time.Time      `json:"created_at"`
	DecidedAt   *time.Time     `gorm:"column:decided_at" json:"decided_at"`
	DecidedByID *string        `gorm:"column:decided_by_id;type:uuid" json:"decided_by_id"`
}

func (p *MCPProposal) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

// ExpireStaleMCPProposals marks every pending proposal older than MCPProposalLifetime as
// expired. Called before reading or deciding, so nothing can act on one.
func ExpireStaleMCPProposals(db *gorm.DB, now time.Time) error {
	return db.Model(&MCPProposal{}).
		Where("status = ? AND created_at < ?", MCPProposalPending, now.Add(-MCPProposalLifetime)).
		Updates(map[string]any{"status": MCPProposalExpired, "decided_at": now}).Error
}

// MCPReport is what an agent's routine found on one day (migration 000094). One per token per
// day: writing again on the same day replaces it.
type MCPReport struct {
	ID         string         `gorm:"primaryKey;type:uuid" json:"id"`
	TokenID    string         `gorm:"column:token_id;type:uuid;not null" json:"token_id"`
	ReportDate time.Time      `gorm:"column:report_date;type:date;not null" json:"report_date"`
	Title      string         `gorm:"not null" json:"title"`
	Summary    string         `gorm:"not null" json:"summary"`
	Findings   datatypes.JSON `gorm:"type:jsonb" json:"findings"`
	Actions    datatypes.JSON `gorm:"type:jsonb" json:"actions"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func (r *MCPReport) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	return nil
}
