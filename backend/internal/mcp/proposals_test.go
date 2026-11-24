package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/auth"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// cancelV1 is a v1 with one issuance and a cancel route that counts, and remembers who called.
type cancelV1 struct {
	http.Handler
	cancels  atomic.Int32
	lastUser atomic.Value
}

func newCancelV1(db *gorm.DB) *cancelV1 {
	v := &cancelV1{}
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(db))
	r.Get("/issuances/{id}", func(w http.ResponseWriter, r *http.Request) {
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]any{
			"id": chi.URLParam(r, "id"), "status": "complete", "program_locator": "ABC123", "points": 50000,
			"loyalty_program": map[string]any{"code": "SMILES"},
		})
	})
	r.Post("/issuances/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		v.cancels.Add(1)
		v.lastUser.Store(r.Context().Value(middleware.UserContextKey).(data.User).ID)
		utils.SendJSON(w, r, http.StatusOK, "cancelada", map[string]any{"id": chi.URLParam(r, "id"), "status": "cancelled"})
	})
	v.Handler = r
	return v
}

func proposeCancel(t *testing.T, url, token string) string {
	t.Helper()
	session, err := connect(t, url, token)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "propose_cancel_issuance",
		Arguments: map[string]any{"issuance_id": "iss-1", "reason": "cliente desistiu", "refunded_points": 50000},
	})
	if err != nil || result.IsError {
		t.Fatalf("propose: %v %+v", err, result)
	}
	return result.Content[0].(*sdk.TextContent).Text
}

func TestProposalRunsOnlyWhenADevApprovesIt(t *testing.T) {
	db := setupTestDB(t)
	agent, _, plain := seedToken(t, db, data.RoleAdmin)
	approver, _, _ := seedToken(t, db, data.RoleDev)
	v1 := newCancelV1(db)
	server := newTestServer(t, db, v1)

	text := proposeCancel(t, server.URL, plain)
	if v1.cancels.Load() != 0 {
		t.Fatal("proposing ran the cancellation")
	}
	if !strings.Contains(text, "proposal_id") {
		t.Fatalf("no proposal id in %s", text)
	}

	var proposal data.MCPProposal
	if err := db.Where("user_id = ?", agent.ID).First(&proposal).Error; err != nil {
		t.Fatal(err)
	}
	if proposal.Status != data.MCPProposalPending || !strings.Contains(proposal.Summary, "ABC123") {
		t.Fatalf("unexpected proposal: %+v", proposal)
	}

	session, err := auth.GenerateToken(approver.ID, approver.Phone, approver.Role)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyProposal(context.Background(), db, v1, proposal.ID, approver.ID, "Bearer "+session)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Status != data.MCPProposalApplied || v1.cancels.Load() != 1 {
		t.Fatalf("want applied and one cancel, got %s and %d", applied.Status, v1.cancels.Load())
	}
	if v1.lastUser.Load() != approver.ID {
		t.Errorf("the operation ran as %v, want the approver %s", v1.lastUser.Load(), approver.ID)
	}

	if _, err := ApplyProposal(context.Background(), db, v1, proposal.ID, approver.ID, "Bearer "+session); !errors.Is(err, ErrProposalNotPending) {
		t.Errorf("second approval: got %v, want ErrProposalNotPending", err)
	}
	if v1.cancels.Load() != 1 {
		t.Errorf("a second approval ran the cancellation again")
	}
}

func TestRejectedAndExpiredProposalsNeverRun(t *testing.T) {
	db := setupTestDB(t)
	agent, _, plain := seedToken(t, db, data.RoleAdmin)
	approver, _, _ := seedToken(t, db, data.RoleDev)
	v1 := newCancelV1(db)
	server := newTestServer(t, db, v1)
	session, err := auth.GenerateToken(approver.ID, approver.Phone, approver.Role)
	if err != nil {
		t.Fatal(err)
	}

	proposeCancel(t, server.URL, plain)
	proposeCancel(t, server.URL, plain)
	var proposals []data.MCPProposal
	if err := db.Where("user_id = ?", agent.ID).Order("created_at").Find(&proposals).Error; err != nil || len(proposals) != 2 {
		t.Fatalf("want two proposals, got %d (%v)", len(proposals), err)
	}

	if _, err := RejectProposal(db, proposals[0].ID, approver.ID); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := ApplyProposal(context.Background(), db, v1, proposals[0].ID, approver.ID, "Bearer "+session); !errors.Is(err, ErrProposalNotPending) {
		t.Errorf("approving a rejected proposal: got %v", err)
	}

	old := time.Now().Add(-data.MCPProposalLifetime - time.Hour)
	if err := db.Model(&proposals[1]).Update("created_at", old).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyProposal(context.Background(), db, v1, proposals[1].ID, approver.ID, "Bearer "+session); !errors.Is(err, ErrProposalNotPending) {
		t.Errorf("approving an expired proposal: got %v", err)
	}
	if v1.cancels.Load() != 0 {
		t.Fatalf("a rejected or expired proposal ran")
	}
}

// The MCP token is not a session: the routes that approve proposals are behind RequireAuth,
// which only takes a signed JWT, so the agent cannot approve its own proposals with it.
func TestMCPTokenIsNotASession(t *testing.T) {
	db := setupTestDB(t)
	_, _, plain := seedToken(t, db, data.RoleDev)

	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(db))
	r.Post("/mcp-proposals/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		t.Error("an MCP token reached the approve route")
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp-proposals/x/approve", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rec.Code)
	}
}
