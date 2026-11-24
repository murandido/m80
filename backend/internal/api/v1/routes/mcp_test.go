package routes

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/onedrivesync"
	"gorm.io/gorm"
)

// Connecting a Microsoft account consents, for the whole installation, to the API reading
// every file shared with it - not something an admin decides for the agency owner.
func TestMicrosoftConnectionRoutesAreDevOnly(t *testing.T) {
	register := func(r chi.Router, _ *gorm.DB) { RegisterMicrosoftRoutes(r, onedrivesync.New(nil, nil)) }
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/microsoft-connection"},
		{http.MethodPost, "/microsoft-connection/start"},
		{http.MethodPost, "/microsoft-connection/callback"},
		{http.MethodDelete, "/microsoft-connection"},
	} {
		for _, role := range []string{data.RoleUser, data.RoleAdmin} {
			if code := callRoute(t, register, role, tc.method, tc.path); code != http.StatusForbidden {
				t.Errorf("%s %s as %s: got %d, want 403", tc.method, tc.path, role, code)
			}
		}
	}
}

// Reading a Tropa is admin, like the rest of the inventory; changing it is dev, because it
// decides which workbook is read and which of its rows become issuances.
func TestInjunctionGroupRouteGates(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000000"
	register := func(r chi.Router, db *gorm.DB) { RegisterInjunctionGroupRoutes(r, db, onedrivesync.New(nil, nil)) }
	for _, tc := range []struct {
		method, path string
		refused      []string
	}{
		{http.MethodGet, "/injunction-groups", []string{data.RoleUser}},
		{http.MethodPost, "/injunction-groups", []string{data.RoleUser, data.RoleAdmin}},
		{http.MethodPut, "/injunction-groups/" + id, []string{data.RoleUser, data.RoleAdmin}},
		{http.MethodDelete, "/injunction-groups/" + id, []string{data.RoleUser, data.RoleAdmin}},
	} {
		for _, role := range tc.refused {
			if code := callRoute(t, register, role, tc.method, tc.path); code != http.StatusForbidden {
				t.Errorf("%s %s as %s: got %d, want 403", tc.method, tc.path, role, code)
			}
		}
	}
	if code := callRoute(t, register, data.RoleAdmin, http.MethodGet, "/injunction-groups"); code == http.StatusForbidden {
		t.Error("an admin cannot read the Tropas")
	}
}

// Everything the panel does with the MCP server - minting tokens, reading the agent's activity,
// approving what it proposed - is dev-only. An admin who could approve a proposal could cancel
// an issuance through the agent with no more checks than cancelling it directly, and one who
// could mint a token would hand the agent their own authority.
func TestMCPRoutesAreDevOnly(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000000"
	cases := []struct{ method, path string }{
		{http.MethodGet, "/mcp-tokens"},
		{http.MethodPost, "/mcp-tokens"},
		{http.MethodDelete, "/mcp-tokens/" + id},
		{http.MethodGet, "/mcp-proposals"},
		{http.MethodPost, "/mcp-proposals/" + id + "/approve"},
		{http.MethodPost, "/mcp-proposals/" + id + "/reject"},
		{http.MethodGet, "/mcp-calls"},
		{http.MethodGet, "/mcp-reports"},
	}
	register := func(r chi.Router, db *gorm.DB) { RegisterMCPRoutes(r, db, nil) }

	for _, tc := range cases {
		for _, role := range []string{data.RoleUser, data.RoleAdmin} {
			if code := callRoute(t, register, role, tc.method, tc.path); code != http.StatusForbidden {
				t.Errorf("%s %s as %s: got %d, want 403", tc.method, tc.path, role, code)
			}
		}
		if code := callRoute(t, register, data.RoleDev, tc.method, tc.path); code == http.StatusForbidden {
			t.Errorf("%s %s as dev: got 403, want the gate to let it through", tc.method, tc.path)
		}
	}
}
