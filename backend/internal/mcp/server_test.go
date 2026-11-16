package mcp

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/testdb"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These tests drive /mcp end to end with the SDK's own client, against the test database.
// Fixtures are committed rather than rolled back: the server reads them from another
// goroutine on its own connections, which a transaction would hide. Each test cleans up after
// itself, and every unique column carries a random suffix.

func TestMain(m *testing.M) {
	if os.Getenv("JWT_SECRET") == "" {
		// Not recoverable and not interesting: every test below would fail on it anyway.
		_ = os.Setenv("JWT_SECRET", "mcp-test-secret")
	}
	os.Exit(m.Run())
}

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testdb.RequireTestDatabase(t)

	testDBOnce.Do(func() {
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"), os.Getenv("DB_PORT"),
		)
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			testDBErr = err
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			testDBErr = err
			return
		}
		if err := sqlDB.Ping(); err != nil {
			testDBErr = err
			return
		}
		data.Connect()
		testDB = data.DB
	})

	if testDBErr != nil {
		t.Skipf("skipping mcp database tests: no test database available: %v", testDBErr)
	}
	return testDB
}

// seedToken creates a user with the given role and a token for them, and removes both (and
// the token's call trail) when the test ends.
func seedToken(t *testing.T, db *gorm.DB, role string) (data.User, data.MCPToken, string) {
	t.Helper()
	user := data.User{
		Name:     "mcp test " + role,
		Phone:    fmt.Sprintf("1199%07d", randomBelow(t, 10000000)),
		Password: "irrelevant",
		Role:     role,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	plain := fmt.Sprintf("%stest-%d", tokenPrefix, randomBelow(t, 1<<62))
	token := data.MCPToken{UserID: user.ID, Name: "test", TokenHash: data.HashMCPToken(plain)}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}

	t.Cleanup(func() {
		db.Where("token_id = ?", token.ID).Delete(&data.MCPCall{})
		db.Where("user_id = ?", user.ID).Delete(&data.MCPProposal{})
		db.Delete(&token)
		db.Unscoped().Delete(&user)
	})
	return user, token, plain
}

func randomBelow(t *testing.T, limit int64) int64 {
	t.Helper()
	n, err := rand.Int(rand.Reader, big.NewInt(limit))
	if err != nil {
		t.Fatal(err)
	}
	return n.Int64()
}

// fakeV1 stands in for the /api/v1 router: the real RequireAuth in front of a /auth/me that
// echoes who it authenticated. That is enough to prove the loopback signs a token the real
// middleware accepts, for the right user.
func fakeV1(db *gorm.DB) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(db))
	r.Get("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(middleware.UserContextKey).(data.User)
		utils.SendJSON(w, r, http.StatusOK, "ok", map[string]string{"id": user.ID, "role": user.Role})
	})
	return r
}

// newTestServer mounts /mcp the way internal/api does: on a chi router, so the tool context
// carries an outer chi route context, which the loopback must not route against.
func newTestServer(t *testing.T, db *gorm.DB, v1 http.Handler) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	r.Handle("/mcp", NewHandler(db, v1))
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	return server
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(req)
}

func connect(t *testing.T, url, token string) (*sdk.ClientSession, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:             url + "/mcp",
		HTTPClient:           &http.Client{Transport: bearerTransport{token: token}},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err == nil {
		t.Cleanup(func() { _ = session.Close() })
	}
	return session, err
}

func postStatus(t *testing.T, url, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestTokenGate(t *testing.T) {
	db := setupTestDB(t)
	_, token, plain := seedToken(t, db, data.RoleDev)
	server := newTestServer(t, db, fakeV1(db))

	cases := map[string]string{
		"no token":           "",
		"unknown token":      tokenPrefix + "nope",
		"panel JWT shape":    "eyJhbGciOiJIUzI1NiJ9.e30.x",
		"right hash, no pfx": strings.TrimPrefix(plain, tokenPrefix),
	}
	for name, candidate := range cases {
		if code := postStatus(t, server.URL, candidate); code != http.StatusUnauthorized {
			t.Errorf("%s: got %d, want 401", name, code)
		}
	}

	if code := postStatus(t, server.URL, plain); code == http.StatusUnauthorized {
		t.Fatalf("valid token: got 401")
	}

	if err := db.Model(&token).Update("revoked_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if code := postStatus(t, server.URL, plain); code != http.StatusUnauthorized {
		t.Errorf("revoked token: got %d, want 401", code)
	}
}

func TestToolRunsAsTokenOwnerAndIsRecorded(t *testing.T) {
	db := setupTestDB(t)
	user, token, plain := seedToken(t, db, data.RoleAdmin)
	server := newTestServer(t, db, fakeV1(db))

	session, err := connect(t, server.URL, plain)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("call whoami: %v", err)
	}
	if result.IsError {
		t.Fatalf("whoami failed: %+v", result.Content)
	}
	text := result.Content[0].(*sdk.TextContent).Text
	if !strings.Contains(text, user.ID) || !strings.Contains(text, `"admin"`) {
		t.Errorf("whoami ran as someone else: %s", text)
	}

	var calls []data.MCPCall
	if err := db.Where("token_id = ?", token.ID).Find(&calls).Error; err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Tool != "whoami" || !calls[0].OK {
		t.Errorf("want one successful whoami call recorded, got %+v", calls)
	}
}

func TestAPIRefusalReachesTheAgentAndIsRecorded(t *testing.T) {
	db := setupTestDB(t)
	_, token, plain := seedToken(t, db, data.RoleUser)

	refusing := chi.NewRouter()
	refusing.Get("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "período fechado")
	})
	server := newTestServer(t, db, refusing)

	session, err := connect(t, server.URL, plain)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !result.IsError {
		t.Fatalf("want a tool error, got %+v", result.Content)
	}
	if text := result.Content[0].(*sdk.TextContent).Text; !strings.Contains(text, "período fechado") {
		t.Errorf("the handler's message did not reach the agent: %s", text)
	}

	var call data.MCPCall
	if err := db.Where("token_id = ?", token.ID).First(&call).Error; err != nil {
		t.Fatal(err)
	}
	if call.OK || call.Error == nil {
		t.Errorf("want a failed call recorded, got %+v", call)
	}
}
