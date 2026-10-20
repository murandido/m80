package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/auth"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Like the handler tests, these need Postgres (tokens and users are real rows) and skip
// without one.
var (
	testDBOnce   sync.Once
	testDB       *gorm.DB
	testDBErr    error
	phoneCounter int64
)

func runInTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	testDBOnce.Do(func() {
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"), os.Getenv("DB_PORT"))
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err == nil {
			sqlDB, dbErr := db.DB()
			if err = dbErr; err == nil {
				err = sqlDB.Ping()
			}
		}
		if err != nil {
			testDBErr = err
			return
		}
		data.Connect()
		testDB = data.DB
	})
	if testDBErr != nil {
		t.Skipf("skipping MCP tests: no test database available: %v", testDBErr)
	}

	tx := testDB.Begin()
	if tx.Error != nil {
		t.Fatalf("failed to begin test transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	fn(tx)
}

func seedUser(t *testing.T, tx *gorm.DB, role string) data.User {
	t.Helper()
	phone := fmt.Sprintf("1198%07d", atomic.AddInt64(&phoneCounter, 1)%10000000)
	user := data.User{Name: "MCP " + role, Phone: phone, Password: "irrelevant", Role: role}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatalf("failed to seed the user: %v", err)
	}
	return user
}

func seedToken(t *testing.T, tx *gorm.DB, user data.User, revoked bool) string {
	t.Helper()
	secret, hash, err := data.NewMCPTokenSecret()
	if err != nil {
		t.Fatalf("failed to generate a token: %v", err)
	}
	token := data.MCPToken{UserID: user.ID, Name: "test", TokenHash: hash}
	if revoked {
		now := time.Now()
		token.RevokedAt = &now
	}
	if err := tx.Create(&token).Error; err != nil {
		t.Fatalf("failed to seed the token: %v", err)
	}
	return secret
}

// newTestAPI stands in for the API router: the real auth and role middleware in front of two
// routes, which is the part of the chain the MCP server relies on.
func newTestAPI(tx *gorm.DB) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(tx))
		r.Get("/auth/me", func(w http.ResponseWriter, r *http.Request) {
			user, _ := r.Context().Value(middleware.UserContextKey).(data.User)
			utils.SendJSON(w, r, http.StatusOK, "ok", map[string]string{"id": user.ID, "role": user.Role})
		})
		r.With(middleware.RequireAdmin()).Get("/point-batches", func(w http.ResponseWriter, r *http.Request) {
			utils.SendJSON(w, r, http.StatusOK, "ok", []string{})
		})
	})
	r.Handle("/mcp", NewHandler(tx, r))
	return r
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(req)
}

func connect(t *testing.T, serverURL, token string) (*mcp.ClientSession, error) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	return client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   serverURL + "/mcp",
		HTTPClient: &http.Client{Transport: bearerTransport{token: token}},
	}, nil)
}

func callText(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("calling %s failed at the protocol level: %v", name, err)
	}
	var text strings.Builder
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return text.String(), result.IsError
}

func TestMCP_RejectsMissingRevokedAndLoginTokens(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := seedUser(t, tx, data.RoleDev)
		revoked := seedToken(t, tx, user, true)
		loginJWT, err := auth.GenerateToken(user.ID, user.Phone, user.Role)
		if err != nil {
			t.Fatalf("failed to sign a login token: %v", err)
		}

		server := httptest.NewServer(newTestAPI(tx))
		defer server.Close()

		for name, token := range map[string]string{"missing": "", "revoked": revoked, "login JWT": loginJWT} {
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(`{}`))
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s: request failed: %v", name, err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("%s: expected 401, got %d", name, resp.StatusCode)
			}
		}
	})
}

func TestMCP_ToolsActAsTheTokenOwnerWithTheirRole(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := seedUser(t, tx, data.RoleUser)
		token := seedToken(t, tx, user, false)

		server := httptest.NewServer(newTestAPI(tx))
		defer server.Close()

		session, err := connect(t, server.URL, token)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer func() { _ = session.Close() }()

		text, isError := callText(t, session, "whoami", nil)
		if isError || !strings.Contains(text, user.ID) || !strings.Contains(text, `"role":"user"`) {
			t.Fatalf("whoami must answer as the token owner, got error=%v %s", isError, text)
		}

		// A plain user's token must hit the same admin gate the panel does.
		text, isError = callText(t, session, "list_point_batches", map[string]any{"account_id": "00000000-0000-0000-0000-000000000001"})
		if !isError || !strings.Contains(text, "403") {
			t.Fatalf("a user token must be refused the inventory, got error=%v %s", isError, text)
		}

		text, isError = callText(t, session, "api_get", map[string]any{"path": "/events"})
		if !isError || !strings.Contains(text, "SSE") {
			t.Fatalf("api_get must refuse the event stream, got error=%v %s", isError, text)
		}

		var stored data.MCPToken
		if err := tx.First(&stored, "token_hash = ?", data.HashMCPToken(token)).Error; err != nil || stored.LastUsedAt == nil {
			t.Fatalf("the token use was not recorded: %v %+v", err, stored)
		}
	})
}
