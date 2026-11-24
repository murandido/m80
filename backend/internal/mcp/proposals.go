package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Operation is one request a proposal runs on approval, against /api/v1.
type Operation struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// propose records an operation for a dev to decide on. Nothing runs here: the agent's only
// power over a destructive operation is asking for it.
func (s *Server) propose(who caller, tool, summary string, op Operation, preview any) (any, error) {
	operations, err := json.Marshal([]Operation{op})
	if err != nil {
		return nil, fmt.Errorf("mcp: encode operations: %w", err)
	}
	var previewJSON datatypes.JSON
	if preview != nil {
		raw, err := json.Marshal(preview)
		if err != nil {
			return nil, fmt.Errorf("mcp: encode preview: %w", err)
		}
		previewJSON = datatypes.JSON(raw)
	}

	proposal := data.MCPProposal{
		UserID:     who.UserID,
		Tool:       tool,
		Summary:    summary,
		Operations: datatypes.JSON(operations),
		Preview:    previewJSON,
		Status:     data.MCPProposalPending,
	}
	if err := s.db.Create(&proposal).Error; err != nil {
		return nil, fmt.Errorf("mcp: record proposal: %w", err)
	}
	return map[string]any{
		"proposal_id": proposal.ID,
		"status":      proposal.Status,
		"summary":     proposal.Summary,
		"expires_at":  proposal.CreatedAt.Add(data.MCPProposalLifetime),
		"next_step":   "Aguardando aprovação de um dev no painel (Propostas MCP). Nada foi alterado ainda.",
	}, nil
}

// ErrProposalNotPending is returned when a proposal was already decided or has expired.
var ErrProposalNotPending = errors.New("proposta já decidida ou expirada")

// ErrProposalNotFound is returned for an unknown proposal id.
var ErrProposalNotFound = errors.New("proposta não encontrada")

// ApplyProposal runs a pending proposal's operations through v1 with the approver's own
// Authorization header, so the role gates and the agency scope checked are the approver's,
// not the agent's. The proposal row stays locked for the duration, so a double click cannot
// run the operations twice.
//
// An operation the API refuses marks the proposal failed with the API's message; that is a
// decision made, not an error of this function, so the proposal is returned with a nil error.
func ApplyProposal(ctx context.Context, db *gorm.DB, v1 http.Handler, proposalID, approverID, authorization string) (data.MCPProposal, error) {
	now := time.Now()
	if err := data.ExpireStaleMCPProposals(db, now); err != nil {
		return data.MCPProposal{}, err
	}

	var proposal data.MCPProposal
	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&proposal, "id = ?", proposalID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProposalNotFound
		}
		if err != nil {
			return err
		}
		if proposal.Status != data.MCPProposalPending {
			return ErrProposalNotPending
		}

		var operations []Operation
		if err := json.Unmarshal(proposal.Operations, &operations); err != nil {
			return fmt.Errorf("mcp: decode operations of proposal %s: %w", proposal.ID, err)
		}

		status := data.MCPProposalApplied
		results := make([]json.RawMessage, 0, len(operations))
		var failure string
		for _, op := range operations {
			var body any
			if len(op.Body) > 0 && string(op.Body) != "null" {
				body = op.Body
			}
			raw, err := serveV1(ctx, v1, op.Method, op.Path, nil, body, authorization)
			if err != nil {
				status = data.MCPProposalFailed
				failure = err.Error()
				break
			}
			results = append(results, redact(raw))
		}

		result := map[string]any{"responses": results}
		if failure != "" {
			result["error"] = failure
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}

		proposal.Status = status
		proposal.Result = datatypes.JSON(resultJSON)
		proposal.DecidedAt = &now
		proposal.DecidedByID = &approverID
		return tx.Model(&proposal).Updates(map[string]any{
			"status":        proposal.Status,
			"result":        proposal.Result,
			"decided_at":    now,
			"decided_by_id": approverID,
		}).Error
	})
	return proposal, err
}

// RejectProposal closes a pending proposal without running anything.
func RejectProposal(db *gorm.DB, proposalID, approverID string) (data.MCPProposal, error) {
	now := time.Now()
	if err := data.ExpireStaleMCPProposals(db, now); err != nil {
		return data.MCPProposal{}, err
	}

	var proposal data.MCPProposal
	if err := db.First(&proposal, "id = ?", proposalID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return proposal, ErrProposalNotFound
		}
		return proposal, err
	}

	// Conditional on still being pending, so a reject racing an approve cannot overwrite it.
	result := db.Model(&data.MCPProposal{}).
		Where("id = ? AND status = ?", proposalID, data.MCPProposalPending).
		Updates(map[string]any{"status": data.MCPProposalRejected, "decided_at": now, "decided_by_id": approverID})
	if result.Error != nil {
		return proposal, result.Error
	}
	if result.RowsAffected == 0 {
		return proposal, ErrProposalNotPending
	}
	proposal.Status = data.MCPProposalRejected
	proposal.DecidedAt = &now
	proposal.DecidedByID = &approverID
	return proposal, nil
}
