package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// stockV1 is a v1 with one Azul account whose statement shows two credits still missing in
// the panel, and a batches route that records what it is asked to write.
type stockV1 struct {
	http.Handler
	mu        sync.Mutex
	batches   []map[string]any
	issuances int
}

const (
	stockAccountID = "acc-1"
	creditDay      = "2026-11-20"
)

func newStockV1(db *gorm.DB) *stockV1 {
	v := &stockV1{}
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(db))
	r.Get("/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]any{"id": chi.URLParam(r, "id"), "loyalty_program": map[string]any{"code": "AZUL"}})
	})
	r.Get("/accounts/{id}/azul-statement", func(w http.ResponseWriter, r *http.Request) {
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]any{"reconciliation": map[string]any{"credits": []map[string]any{
			{"index": 0, "occurred_on": creditDay, "points": 500, "verdict": "missing_in_panel"},
			{"index": 1, "occurred_on": creditDay, "points": 700, "verdict": "missing_in_panel"},
		}}})
	})
	r.Get("/point-batches", func(w http.ResponseWriter, r *http.Request) {
		v.mu.Lock()
		defer v.mu.Unlock()
		utils.SendJSON(w, r, http.StatusOK, "ok", v.batches)
	})
	r.Post("/point-batches", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		v.mu.Lock()
		v.batches = append(v.batches, body)
		v.mu.Unlock()
		utils.SendJSON(w, r, http.StatusCreated, "ok", body)
	})
	r.Post("/issuances", func(w http.ResponseWriter, r *http.Request) {
		v.mu.Lock()
		v.issuances++
		v.mu.Unlock()
		utils.SendJSON(w, r, http.StatusCreated, "ok", map[string]any{"id": "new"})
	})
	v.Handler = r
	return v
}

func (v *stockV1) written() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.batches)
}

func setPolicy(t *testing.T, db *gorm.DB, token data.MCPToken, allowed []string, limits *data.MCPTokenLimits) {
	t.Helper()
	token.AllowedWriteTools = pq.StringArray(allowed)
	if allowed != nil && len(allowed) == 0 {
		token.AllowedWriteTools = pq.StringArray{}
	}
	token.Limits = limits
	if err := db.Model(&token).Select("allowed_write_tools", "limits").Updates(&token).Error; err != nil {
		t.Fatalf("set policy: %v", err)
	}
}

func callTool(t *testing.T, session *sdk.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result.Content[0].(*sdk.TextContent).Text, result.IsError
}

func batchArgs(points int, withSource bool) map[string]any {
	args := map[string]any{
		"account_id": stockAccountID, "origin_id": "origin", "acquired_on": creditDay,
		"points": points, "cost_cents": points * 2, "funding_agency_id": "agency",
	}
	if withSource {
		args["source"] = map[string]any{"channel": "whatsapp", "chat": "Compras", "message_at": "2026-11-20 10:15", "excerpt": "comprei 500 pts"}
	}
	return args
}

func isProposal(text string) bool { return strings.Contains(text, "proposal_id") }

// A person's own token has no policy: every free write runs, as in phase 1.
func TestTokenWithoutPolicyWritesDirectly(t *testing.T) {
	db := setupTestDB(t)
	_, _, plain := seedToken(t, db, data.RoleAdmin)
	v1 := newStockV1(db)
	session, err := connect(t, newTestServer(t, db, v1).URL, plain)
	if err != nil {
		t.Fatal(err)
	}

	text, isErr := callTool(t, session, "add_point_batch", batchArgs(9999, false))
	if isErr || isProposal(text) || v1.written() != 1 {
		t.Fatalf("want a direct write without source, got %s (written %d)", text, v1.written())
	}
}

func TestPolicyTurnsDisallowedWritesIntoProposals(t *testing.T) {
	db := setupTestDB(t)
	_, token, plain := seedToken(t, db, data.RoleAdmin)
	setPolicy(t, db, token, []string{}, nil)
	v1 := newStockV1(db)
	session, err := connect(t, newTestServer(t, db, v1).URL, plain)
	if err != nil {
		t.Fatal(err)
	}

	text, isErr := callTool(t, session, "create_issuance", map[string]any{
		"program_locator": "ABC", "loyalty_program_id": "p", "account_id": stockAccountID, "points": 1000, "value_of_the_thousand": 2000,
	})
	if isErr || !isProposal(text) || !strings.Contains(text, "não liberada") {
		t.Fatalf("want a proposal saying the tool is not allowed, got %s", text)
	}
	if v1.issuances != 0 {
		t.Fatal("a disallowed write reached the API")
	}

	var call data.MCPCall
	if err := db.Where("token_id = ? AND tool = ?", token.ID, "create_issuance").First(&call).Error; err != nil {
		t.Fatal(err)
	}
	var args map[string]any
	if err := json.Unmarshal(call.Args, &args); err != nil || args["_outcome"] != "proposed" {
		t.Errorf("the trail does not mark the call as proposed: %s", call.Args)
	}
}

func TestAgentBatchesNeedSourceStatementAndLimits(t *testing.T) {
	db := setupTestDB(t)
	_, token, plain := seedToken(t, db, data.RoleAdmin)
	maxPoints, perDay := 600, 1
	setPolicy(t, db, token, []string{"add_point_batch"}, &data.MCPTokenLimits{MaxBatchPoints: &maxPoints, MaxBatchesPerDay: &perDay})
	v1 := newStockV1(db)
	session, err := connect(t, newTestServer(t, db, v1).URL, plain)
	if err != nil {
		t.Fatal(err)
	}

	if text, isErr := callTool(t, session, "add_point_batch", batchArgs(500, false)); !isErr || !strings.Contains(text, "source") {
		t.Fatalf("want an error asking for the source, got %s", text)
	}

	// Matches a statement credit, under both ceilings: runs, with the source in the note.
	text, isErr := callTool(t, session, "add_point_batch", batchArgs(500, true))
	if isErr || isProposal(text) || v1.written() != 1 {
		t.Fatalf("want a direct write, got %s", text)
	}
	if note, _ := v1.batches[0]["note"].(string); !strings.HasPrefix(note, "[Agente] whatsapp Compras") {
		t.Errorf("the source did not reach the note: %q", note)
	}

	cases := map[string]map[string]any{
		"second batch of the day":               batchArgs(700, true),
		"same day and points again (duplicate)": batchArgs(500, true),
		"no matching statement credit": func() map[string]any {
			args := batchArgs(300, true)
			args["acquired_on"] = "2026-11-21"
			return args
		}(),
		"over the points ceiling": batchArgs(5000, true),
	}
	for name, args := range cases {
		if text, isErr := callTool(t, session, "add_point_batch", args); isErr || !isProposal(text) {
			t.Errorf("%s: want a proposal, got %s", name, text)
		}
	}
	if v1.written() != 1 {
		t.Errorf("proposals wrote batches: %d written", v1.written())
	}
}
