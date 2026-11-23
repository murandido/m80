package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func callCreateMCPToken(tx *gorm.DB, caller data.User, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/mcp-tokens", strings.NewReader(body))
	req = req.WithContext(asUser(req.Context(), caller))
	rec := httptest.NewRecorder()
	NewMCPTokenHandler(tx).CreateMCPToken(rec, req)
	return rec
}

// A routine's token belongs to its own user, with a policy narrower than its owner's role.
func TestCreateMCPTokenForAnotherUserWithAPolicy(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		dev := createUser(t, tx, "dev")
		agent := createUser(t, tx, "agente")

		body := `{"name":"rotina","user_id":"` + agent.ID + `","allowed_write_tools":["record_statement_batches"],"limits":{"max_batch_points":5000,"max_batches_per_day":2}}`
		rec := callCreateMCPToken(tx, dev, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: got %d (%s)", rec.Code, rec.Body.String())
		}
		var envelope struct {
			Data schemas.CreatedMCPTokenResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		got := envelope.Data
		if got.UserID != agent.ID || got.UserName == nil || *got.UserName != "agente" {
			t.Errorf("owner: got %s / %v, want the agent", got.UserID, got.UserName)
		}
		if len(got.AllowedWriteTools) != 1 || got.Limits == nil || *got.Limits.MaxBatchPoints != 5000 {
			t.Errorf("policy: got %v / %+v", got.AllowedWriteTools, got.Limits)
		}

		var stored data.MCPToken
		if err := tx.First(&stored, "id = ?", got.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Limits == nil || *stored.Limits.MaxBatchesPerDay != 2 || len(stored.AllowedWriteTools) != 1 {
			t.Errorf("stored policy: got %v / %+v", stored.AllowedWriteTools, stored.Limits)
		}

		// An empty list is "nothing directly", and must not come back as "everything".
		rec = callCreateMCPToken(tx, dev, `{"name":"tudo proposta","allowed_write_tools":[]}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("empty policy: got %d", rec.Code)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Data.AllowedWriteTools == nil {
			t.Error("an empty allow list came back as null (unrestricted)")
		}

		if rec := callCreateMCPToken(tx, dev, `{"name":"x","allowed_write_tools":["drop_everything"]}`); rec.Code != http.StatusBadRequest {
			t.Errorf("unknown tool: got %d, want 400", rec.Code)
		}
	})
}
