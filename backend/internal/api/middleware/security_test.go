package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/murandido/m-80-backend/internal/auth"
)

// okHandler is what the limiter wraps: if a request reaches it, it wasn't throttled.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// callWithToken sends one request through the limiter and returns its status.
// Every call uses a distinct RemoteAddr port but the same host, mirroring how the
// real traffic arrives: many connections from the single Next.js server.
func callWithToken(t *testing.T, limiter func(http.Handler) http.Handler, token string, port int) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.RemoteAddr = fmt.Sprintf("172.18.0.1:%d", 40000+port)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	limiter(okHandler).ServeHTTP(rec, req)
	return rec.Code
}

// This is the property the per-user keying exists for. Under the previous
// LimitByIP the whole panel shared one budget, because every request reaches the
// API from the same Next.js server - so one operator deleting records could
// exhaust the quota for everyone, and a throttled /auth/me logged them out.
func TestRateLimit_QuotaIsPerUserNotPerIP(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-for-rate-limit-tests")

	first, err := auth.GenerateToken("11111111-1111-1111-1111-111111111111", "11999999991", "admin")
	if err != nil {
		t.Fatalf("failed to generate the first token: %v", err)
	}
	second, err := auth.GenerateToken("22222222-2222-2222-2222-222222222222", "11999999992", "user")
	if err != nil {
		t.Fatalf("failed to generate the second token: %v", err)
	}

	limiter := RateLimit()

	// Spend the first user's entire budget, from a single IP.
	for i := range 300 {
		if code := callWithToken(t, limiter, first, i); code != http.StatusOK {
			t.Fatalf("request %d of the first user's budget was throttled with %d", i+1, code)
		}
	}
	if code := callWithToken(t, limiter, first, 300); code != http.StatusTooManyRequests {
		t.Fatalf("the first user past their budget got %d, want 429", code)
	}

	// The second user shares that IP and must be untouched by it.
	if code := callWithToken(t, limiter, second, 301); code != http.StatusOK {
		t.Fatalf("the second user got %d from the same IP, want 200 - the quotas are still shared", code)
	}
}

// A request with no usable token has no user to bill, so it falls back to the IP.
// Without this, garbage tokens would all share one bucket - or worse, bypass the
// limit entirely.
func TestRateLimit_FallsBackToIPWithoutAValidToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-for-rate-limit-tests")

	limiter := RateLimit()

	for i := range 300 {
		if code := callWithToken(t, limiter, "", i); code != http.StatusOK {
			t.Fatalf("anonymous request %d was throttled with %d", i+1, code)
		}
	}
	if code := callWithToken(t, limiter, "", 300); code != http.StatusTooManyRequests {
		t.Fatalf("anonymous request past the budget got %d, want 429", code)
	}

	// Same IP, but now with a token the signature check rejects: it must land in
	// the very same (already exhausted) bucket rather than open a fresh one.
	if code := callWithToken(t, limiter, "not-a-real-token", 301); code != http.StatusTooManyRequests {
		t.Fatalf("a forged token opened a fresh bucket (%d) - the IP fallback is not holding", code)
	}
}

func TestLoginRateLimit_IsStricterAndPerIP(t *testing.T) {
	limiter := LoginRateLimit()

	for i := range 10 {
		if code := callWithToken(t, limiter, "", i); code != http.StatusOK {
			t.Fatalf("login attempt %d was throttled with %d", i+1, code)
		}
	}
	if code := callWithToken(t, limiter, "", 10); code != http.StatusTooManyRequests {
		t.Fatalf("the 11th login attempt got %d, want 429", code)
	}
}
