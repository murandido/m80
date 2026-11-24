package mcp

import (
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
)

func TestStatementReconciliationShowsOnlyWhatDoesNotMatch(t *testing.T) {
	db := setupTestDB(t)
	_, _, plain := seedToken(t, db, data.RoleAdmin)
	session, err := connect(t, newTestServer(t, db, newStockV1(db)).URL, plain)
	if err != nil {
		t.Fatal(err)
	}

	text, isErr := callTool(t, session, "statement_reconciliation", map[string]any{"account_id": stockAccountID})
	if isErr {
		t.Fatalf("reconciliation: %s", text)
	}
	if strings.Contains(text, "b-old") || strings.Contains(text, "OK1") {
		t.Errorf("matched lines reached the agent: %s", text)
	}
	if !strings.Contains(text, `"points":700`) || !strings.Contains(text, "MISS") {
		t.Errorf("unmatched lines are missing: %s", text)
	}
}

func TestRecordStatementBatchesFollowsThePolicy(t *testing.T) {
	db := setupTestDB(t)
	_, token, plain := seedToken(t, db, data.RoleAdmin)
	maxPoints := 600
	setPolicy(t, db, token, []string{"record_statement_batches"}, &data.MCPTokenLimits{MaxBatchPoints: &maxPoints})
	v1 := newStockV1(db)
	session, err := connect(t, newTestServer(t, db, v1).URL, plain)
	if err != nil {
		t.Fatal(err)
	}

	choice := func(index int) map[string]any {
		return map[string]any{
			"credit_index": index, "origin_id": "origin", "cost_cents": 1000, "funding_agency_id": "agency",
			"source": map[string]any{"channel": "whatsapp", "chat": "Compras", "excerpt": "comprei"},
		}
	}

	if text, isErr := callTool(t, session, "record_statement_batches", map[string]any{
		"account_id": stockAccountID, "batches": []any{choice(2)},
	}); !isErr || !strings.Contains(text, "não está sem lote") {
		t.Errorf("an already matched credit was accepted: %s", text)
	}

	text, isErr := callTool(t, session, "record_statement_batches", map[string]any{
		"account_id": stockAccountID, "batches": []any{choice(0)},
	})
	if isErr || isProposal(text) || len(v1.commits) != 1 {
		t.Fatalf("a credit under the ceiling did not commit: %s", text)
	}
	if note, _ := v1.commits[0]["note"].(string); !strings.HasPrefix(note, "[Agente] whatsapp Compras") {
		t.Errorf("the source did not reach the note: %q", note)
	}

	if text, isErr := callTool(t, session, "record_statement_batches", map[string]any{
		"account_id": stockAccountID, "batches": []any{choice(1)},
	}); isErr || !isProposal(text) || len(v1.commits) != 1 {
		t.Errorf("a credit over the ceiling was not proposed: %s", text)
	}
}
