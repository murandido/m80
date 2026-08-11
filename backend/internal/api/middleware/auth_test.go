package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/murandido/m-80-backend/internal/auth"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// unreachableDB returns a *gorm.DB pointed at a port nothing listens on, so every
// query fails with a connection error. That is the whole point: it reproduces an
// infrastructure failure - the database being down, a dropped connection, a lock
// held by someone working in psql - without needing a Postgres to be up, which
// means this test actually runs everywhere instead of skipping in silence.
func unreachableDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(
		postgres.Open("host=127.0.0.1 port=1 user=nobody password=nobody dbname=nothing sslmode=disable"),
		&gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
			// Without this gorm.Open itself fails, and we need the failure to
			// happen later, on the query the middleware runs.
			DisableAutomaticPing: true,
		},
	)
	if err != nil {
		t.Fatalf("failed to open the unreachable database handle: %v", err)
	}
	return db
}

func callRequireAuth(t *testing.T, db *gorm.DB, authorization string) int {
	t.Helper()

	reached := false
	handler := RequireAuth(db)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if reached && rec.Code != http.StatusOK {
		t.Fatalf("the handler ran but the status is %d", rec.Code)
	}
	return rec.Code
}

// The regression this guards: a database that cannot answer is not the same as a
// user who is not there. Returning 401 here told the panel the session had
// expired, and the panel responds to 401 by deleting the auth cookie - so a
// database hiccup used to log everyone out.
func TestRequireAuth_DatabaseFailureIsNotUnauthorized(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-for-auth-middleware-tests")

	token, err := auth.GenerateToken("11111111-1111-1111-1111-111111111111", "11999999991", "admin")
	if err != nil {
		t.Fatalf("failed to generate a token: %v", err)
	}

	code := callRequireAuth(t, unreachableDB(t), "Bearer "+token)

	if code == http.StatusUnauthorized {
		t.Fatal("a database failure answered 401, which makes the panel destroy a valid session")
	}
	if code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", code)
	}
}

// The token-level rejections need no database at all: they are decided before the
// lookup, and all of them are genuine 401s.
func TestRequireAuth_RejectsBadTokens(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-for-auth-middleware-tests")

	tests := []struct {
		name          string
		authorization string
	}{
		{"no header", ""},
		{"no bearer prefix", "abcdef"},
		{"wrong scheme", "Basic abcdef"},
		{"garbage token", "Bearer not-a-real-token"},
		{"token signed with another secret", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiMSJ9.wrong"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The database is unreachable on purpose: none of these may get far
			// enough to touch it.
			if code := callRequireAuth(t, unreachableDB(t), tt.authorization); code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", code)
			}
		})
	}
}

// callGate runs a gate against a context user of the given role and reports the status.
// No database is involved: the gates decide from the user RequireAuth already loaded.
func callGate(t *testing.T, gate func(http.Handler) http.Handler, role string) int {
	t.Helper()

	handler := gate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req = req.WithContext(context.WithValue(
		req.Context(), UserContextKey, data.User{ID: "00000000-0000-0000-0000-000000000000", Role: role},
	))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}

// The two gates side by side, because what matters is the difference between them:
// dev passes both, admin passes only the admin one, and a plain user passes neither.
// dev is never named in a RequireRole call - if it stops passing a gate here, the
// implicit allowance was lost.
func TestRoleGates(t *testing.T) {
	tests := []struct {
		name string
		gate func(http.Handler) http.Handler
		role string
		want int
	}{
		{"admin gate/admin", RequireAdmin(), data.RoleAdmin, http.StatusOK},
		{"admin gate/dev", RequireAdmin(), data.RoleDev, http.StatusOK},
		{"admin gate/user", RequireAdmin(), data.RoleUser, http.StatusForbidden},
		{"dev gate/dev", RequireDev(), data.RoleDev, http.StatusOK},
		{"dev gate/admin", RequireDev(), data.RoleAdmin, http.StatusForbidden},
		{"dev gate/user", RequireDev(), data.RoleUser, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code := callGate(t, tt.gate, tt.role); code != tt.want {
				t.Fatalf("status = %d, want %d", code, tt.want)
			}
		})
	}
}

// A request that never went through RequireAuth has no user in the context at all.
// That is a wiring mistake in the router, not a permission problem, so it must not
// masquerade as a 403.
func TestRequireAdmin_MissingContextIsNotForbidden(t *testing.T) {
	handler := RequireAdmin()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
