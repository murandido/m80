package mcp

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
)

func TestImportSpreadsheetIssuancesNeedsTheStatementForAnAgent(t *testing.T) {
	db := setupTestDB(t)
	_, token, plain := seedToken(t, db, data.RoleAdmin)
	setPolicy(t, db, token, []string{"import_spreadsheet_issuances"}, nil)

	var imports atomic.Int32
	v1 := chi.NewRouter()
	v1.Use(middleware.RequireAuth(db))
	v1.Get("/injunction-groups/{id}/check", func(w http.ResponseWriter, r *http.Request) {
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]any{
			"missing_in_panel_confirmed":   []map[string]any{{"block_index": 0, "number": 1, "locator": "OK0001"}},
			"missing_in_panel_unconfirmed": []map[string]any{{"block_index": 0, "number": 2, "locator": "NO0002"}},
		})
	})
	v1.Post("/injunction-groups/{id}/import", func(w http.ResponseWriter, r *http.Request) {
		imports.Add(1)
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]any{"imported": 1})
	})
	session, err := connect(t, newTestServer(t, db, v1).URL, plain)
	if err != nil {
		t.Fatal(err)
	}

	importRows := func(numbers ...int) (string, bool) {
		rows := []any{}
		for _, n := range numbers {
			rows = append(rows, map[string]any{"block_index": 0, "number": n})
		}
		return callTool(t, session, "import_spreadsheet_issuances", map[string]any{"group_id": "g1", "day": "2026-11-20", "rows": rows})
	}

	if text, isErr := importRows(); !isErr || !strings.Contains(text, "rows") {
		t.Errorf("an empty selection was accepted: %s", text)
	}
	if text, isErr := importRows(1); isErr || isProposal(text) || imports.Load() != 1 {
		t.Fatalf("a confirmed row was not imported directly: %s", text)
	}
	if text, isErr := importRows(1, 2); isErr || !isProposal(text) || !strings.Contains(text, "linha 2") {
		t.Errorf("an unconfirmed row did not become a proposal: %s", text)
	}
	if imports.Load() != 1 {
		t.Errorf("the proposal imported anyway")
	}
}
