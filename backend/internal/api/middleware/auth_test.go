package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/murandido/m-80-backend/internal/auth"
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
