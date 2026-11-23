package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// injunctionV1 has one injunction account with inventory and a statement, and answers the
// account list only when asked for the injunction category - so a check that forgot the
// filter would come back empty.
func injunctionV1(db *gorm.DB) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(db))
	r.Get("/accounts", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("category") != categoryInjunction {
			utils.SendJSON(w, r, http.StatusOK, "ok", []any{})
			return
		}
		utils.SendJSON(w, r, http.StatusOK, "ok", []map[string]any{{
			"id": "liminar-1", "loyalty_program": map[string]any{"code": "AZUL"}, "category": categoryInjunction,
			"owns_point_inventory": true, "expected_balance_points": 100000, "real_balance_points": 90000,
			"real_balance_checked_on": "2026-11-23", "azul_connected": true, "azul_sync_state": "ok",
		}})
	})
	r.Get("/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]any{"id": chi.URLParam(r, "id"), "loyalty_program": map[string]any{"code": "AZUL"}})
	})
	r.Get("/accounts/{id}/azul-statement", func(w http.ResponseWriter, r *http.Request) {
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]any{"reconciliation": map[string]any{"credits": []map[string]any{
			{"index": 0, "occurred_on": "2026-11-20", "points": 10000, "verdict": "missing_in_panel"},
		}}})
	})
	r.Get("/inventory-closing", func(w http.ResponseWriter, r *http.Request) {
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]any{"accounts": []map[string]any{{
			"account_id": "liminar-1", "pending_points": 2500,
			"batches": []map[string]any{
				{"batch_id": "b-open", "acquired_on": "2026-11-02", "points": 50000, "settled_at": nil},
				{"batch_id": "b-done", "acquired_on": "2026-11-01", "points": 40000, "settled_at": "2026-11-10T12:00:00Z"},
			},
		}}})
	})
	r.Get("/point-batches", func(w http.ResponseWriter, r *http.Request) {
		utils.SendJSON(w, r, http.StatusOK, "ok", []map[string]any{
			{"id": "b-agent", "points": 10000, "note": "[Agente] whatsapp Compras: comprei", "created_at": "2026-11-23T10:00:00Z"},
			{"id": "b-human", "points": 5000, "note": "Livelo", "created_at": "2026-11-23T10:00:00Z"},
		})
	})
	return r
}

func TestInjunctionDailyCheckPutsEverythingTogether(t *testing.T) {
	db := setupTestDB(t)
	_, _, plain := seedToken(t, db, data.RoleAdmin)
	session, err := connect(t, newTestServer(t, db, injunctionV1(db)).URL, plain)
	if err != nil {
		t.Fatal(err)
	}

	text, isErr := callTool(t, session, "injunction_daily_check", map[string]any{"date": "2026-11-23"})
	if isErr {
		t.Fatalf("check: %s", text)
	}
	var check struct {
		Accounts []accountCheck `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(text), &check); err != nil || len(check.Accounts) != 1 {
		t.Fatalf("want one injunction account, got %s", text)
	}
	got := check.Accounts[0]

	if !strings.Contains(kinds(got.Alerts), "balance_divergence") {
		t.Errorf("the 10k divergence was not flagged: %+v", got.Alerts)
	}
	if got.Statement == nil || len(got.Statement.CreditsWithoutLink) != 1 {
		t.Errorf("the credit without a batch is missing: %+v / %s", got.Statement, got.StatementError)
	}
	if got.Closing == nil || got.Closing.PendingPoints != 2500 || len(got.Closing.UnsettledBatches) != 1 || got.Closing.UnsettledBatches[0].BatchID != "b-open" {
		t.Errorf("closing: got %+v / %s", got.Closing, got.ClosingError)
	}
	if len(got.AgentBatches) != 1 || got.AgentBatches[0].ID != "b-agent" {
		t.Errorf("agent batches: got %+v", got.AgentBatches)
	}
}

func TestSaveReportKeepsOnePerDay(t *testing.T) {
	db := setupTestDB(t)
	_, token, plain := seedToken(t, db, data.RoleAdmin)
	setPolicy(t, db, token, []string{}, nil) // a policy allowing no writes still saves reports
	t.Cleanup(func() { db.Where("token_id = ?", token.ID).Delete(&data.MCPReport{}) })
	session, err := connect(t, newTestServer(t, db, injunctionV1(db)).URL, plain)
	if err != nil {
		t.Fatal(err)
	}

	save := func(title string) {
		text, isErr := callTool(t, session, "save_report", map[string]any{
			"report_date": "2026-11-23", "title": title, "summary": "tudo certo",
			"findings": []any{map[string]any{"kind": "balance_divergence", "severity": "warning", "detail": "10 mil a menos"}},
		})
		if isErr {
			t.Fatalf("save %s: %s", title, text)
		}
	}
	save("primeira")
	save("segunda")

	var reports []data.MCPReport
	if err := db.Where("token_id = ?", token.ID).Find(&reports).Error; err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Title != "segunda" {
		t.Errorf("want one report replaced in place, got %d (%+v)", len(reports), reports)
	}

	if text, isErr := callTool(t, session, "save_report", map[string]any{
		"title": "x", "summary": "y", "findings": []any{map[string]any{"kind": "k", "severity": "grave", "detail": "d"}},
	}); !isErr || !strings.Contains(text, "severity") {
		t.Errorf("an unknown severity was accepted: %s", text)
	}
}
