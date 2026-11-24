package mcp

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
)

func intPtr(n int) *int       { return &n }
func strPtr(s string) *string { return &s }
func kinds(alerts []stockAlert) string {
	var out []string
	for _, a := range alerts {
		out = append(out, a.Kind)
	}
	return strings.Join(out, ",")
}

func TestStockAlerts(t *testing.T) {
	today := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	ok := "ok"
	reconnect := "needs_reconnect"

	cases := []struct {
		name    string
		account stockAccount
		want    string
	}{
		{
			name:    "healthy account raises nothing",
			account: stockAccount{ID: "a", ExpectedBalancePoints: intPtr(10000), RealBalancePoints: intPtr(10500), DivergencePoints: intPtr(500), RealBalanceCheckedOn: strPtr("2026-11-15"), Sync: &syncState{Connected: true, State: &ok}},
			want:    "",
		},
		{
			name:    "divergence past the threshold",
			account: stockAccount{ID: "a", ExpectedBalancePoints: intPtr(10000), RealBalancePoints: intPtr(7000), DivergencePoints: intPtr(-3000)},
			want:    "balance_divergence",
		},
		{
			name:    "negative expected balance",
			account: stockAccount{ID: "a", ExpectedBalancePoints: intPtr(-200)},
			want:    "negative_expected_balance",
		},
		{
			name:    "connected but stale and asking for a reconnect",
			account: stockAccount{ID: "a", RealBalanceCheckedOn: strPtr("2026-11-01"), Sync: &syncState{Connected: true, State: &reconnect}},
			want:    "stale_balance,sync_needs_attention",
		},
		{
			name:    "connected but never read",
			account: stockAccount{ID: "a", Sync: &syncState{Connected: true}},
			want:    "balance_never_read",
		},
		{
			name:    "an unconnected account is not stale, it is just not read",
			account: stockAccount{ID: "a", RealBalanceCheckedOn: strPtr("2026-01-01"), Sync: &syncState{Connected: false}},
			want:    "",
		},
		{
			name:    "CPF limit close",
			account: stockAccount{ID: "a", CPFUsage: map[string]any{"used": float64(23), "limit": float64(25), "remaining": float64(2)}},
			want:    "cpf_limit",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := kinds(stockAlerts([]stockAccount{tc.account}, stockAlertsInput{}, today))
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// GET /accounts hands out every account's password decrypted - the staff need it to log into
// the programs. The agent must get the balances and never the passwords.
func TestStockOverviewNeverHandsOutPasswords(t *testing.T) {
	db := setupTestDB(t)
	_, _, plain := seedToken(t, db, data.RoleAdmin)

	v1 := chi.NewRouter()
	v1.Use(middleware.RequireAuth(db))
	v1.Get("/accounts", func(w http.ResponseWriter, r *http.Request) {
		utils.SendJSON(w, r, http.StatusOK, "ok", []map[string]any{{ //nolint:gosec // G101: fake credentials the test asserts never reach the agent
			"id":                      "acc-1",
			"loyalty_program":         map[string]any{"code": "SMILES", "name": "Smiles"},
			"login":                   "login-que-nao-pode-vazar",
			"password":                "senha-que-nao-pode-vazar",
			"nicknames":               []map[string]any{{"nickname": "JOAO"}},
			"owns_point_inventory":    true,
			"expected_balance_points": 120000,
			"real_balance_points":     118000,
		}})
	})
	server := newTestServer(t, db, v1)

	session, err := connect(t, server.URL, plain)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: "stock_overview"})
	if err != nil || result.IsError {
		t.Fatalf("stock_overview: %v %+v", err, result)
	}
	text := result.Content[0].(*sdk.TextContent).Text
	if strings.Contains(text, "nao-pode-vazar") {
		t.Fatalf("credentials reached the agent: %s", text)
	}
	if !strings.Contains(text, `"divergence_points":-2000`) || !strings.Contains(text, "JOAO") {
		t.Errorf("overview is missing the balance figures: %s", text)
	}
}
