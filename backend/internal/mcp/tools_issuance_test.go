package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
)

func TestRedactDropsCredentialsAndDocumentsAtAnyDepth(t *testing.T) {
	raw := json.RawMessage(`{"id":"i1","account":{"password":"x","login":"y","nickname":"JOAO"},` +
		`"passengers":[{"name":"ANA","document":"123"}],"points":5000}`)
	got := string(redact(raw))
	for _, secret := range []string{"password", "login", "document", `"x"`, `"123"`} {
		if strings.Contains(got, secret) {
			t.Errorf("redact left %s in %s", secret, got)
		}
	}
	for _, kept := range []string{"JOAO", "ANA", "5000"} {
		if !strings.Contains(got, kept) {
			t.Errorf("redact removed %s from %s", kept, got)
		}
	}
}

// Cancelling is a proposal. The PATCH route already refuses status=cancelled, but the tool
// refuses first so the agent is pointed at the right tool instead of at the panel's button.
func TestUpdateIssuanceWillNotCancel(t *testing.T) {
	db := setupTestDB(t)
	_, _, plain := seedToken(t, db, data.RoleAdmin)

	var patched atomic.Int32
	v1 := chi.NewRouter()
	v1.Use(middleware.RequireAuth(db))
	v1.Patch("/issuances/{id}", func(w http.ResponseWriter, r *http.Request) {
		patched.Add(1)
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]string{"id": chi.URLParam(r, "id")})
	})
	server := newTestServer(t, db, v1)

	session, err := connect(t, server.URL, plain)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	call := func(status string) *sdk.CallToolResult {
		result, err := session.CallTool(context.Background(), &sdk.CallToolParams{
			Name:      "update_issuance",
			Arguments: map[string]any{"issuance_id": "abc", "changes": map[string]any{"status": status}},
		})
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		return result
	}

	if result := call("cancelled"); !result.IsError || !strings.Contains(result.Content[0].(*sdk.TextContent).Text, "propose_cancel_issuance") {
		t.Errorf("status=cancelled was not refused with a pointer to the proposal: %+v", result.Content)
	}
	if patched.Load() != 0 {
		t.Fatalf("status=cancelled reached the API")
	}

	if result := call("complete"); result.IsError {
		t.Errorf("status=complete was refused: %+v", result.Content)
	}
	if patched.Load() != 1 {
		t.Errorf("status=complete did not reach the API")
	}
}
