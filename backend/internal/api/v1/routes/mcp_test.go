package routes

import (
	"net/http"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
)

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
	}

	for _, tc := range cases {
		for _, role := range []string{data.RoleUser, data.RoleAdmin} {
			if code := callRoute(t, RegisterMCPRoutes, role, tc.method, tc.path); code != http.StatusForbidden {
				t.Errorf("%s %s as %s: got %d, want 403", tc.method, tc.path, role, code)
			}
		}
		if code := callRoute(t, RegisterMCPRoutes, data.RoleDev, tc.method, tc.path); code == http.StatusForbidden {
			t.Errorf("%s %s as dev: got 403, want the gate to let it through", tc.method, tc.path)
		}
	}
}
