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

// The vault's limit is per user like the panel-wide one, but an order of magnitude tighter:
// thirty passwords a minute is plenty by hand and nowhere near a bulk copy.
func TestCredentialRateLimit_IsPerUserAndTight(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-for-rate-limit-tests")

	first, err := auth.GenerateToken("33333333-3333-3333-3333-333333333333", "11999999993", "user")
	if err != nil {
		t.Fatalf("failed to generate the first token: %v", err)
	}
	second, err := auth.GenerateToken("44444444-4444-4444-4444-444444444444", "11999999994", "user")
	if err != nil {
		t.Fatalf("failed to generate the second token: %v", err)
	}

	limiter := CredentialRateLimit()

	for i := range 30 {
		if code := callWithToken(t, limiter, first, i); code != http.StatusOK {
			t.Fatalf("credential read %d was throttled with %d", i+1, code)
		}
	}
	if code := callWithToken(t, limiter, first, 30); code != http.StatusTooManyRequests {
		t.Fatalf("the 31st credential read in a minute got %d, want 429", code)
	}
	if code := callWithToken(t, limiter, second, 31); code != http.StatusOK {
		t.Fatalf("another user got %d, want 200 - the vault limit must be per user", code)
	}
}

// keyByRemoteAddr replaced httprate's KeyByIP when v0.16 deprecated it, and a security control
// must not redraw its buckets just because a dependency moved. Every expected key below was
// produced by running httprate v0.15.0's KeyByIP on the same RemoteAddr, so this table is the
// proof the swap kept the old keys - including where v0.16's own KeyByIP would now disagree.
func TestKeyByRemoteAddr_MatchesHTTPRateV015(t *testing.T) {
	cases := []struct {
		remoteAddr string
		want       string
	}{
		// IPv4: the port is dropped, the address is kept whole. RealIP leaves no port,
		// a direct connection has one; both land in the same bucket.
		{"203.0.113.7", "203.0.113.7"},
		{"203.0.113.7:51234", "203.0.113.7"},
		{"172.18.0.1:40001", "172.18.0.1"},
		// IPv6: bucketed by /64, so rotating the interface id does not buy a new bucket.
		{"2001:db8:abcd:12::1", "2001:db8:abcd:12::"},
		{"[2001:db8:abcd:12::1]:443", "2001:db8:abcd:12::"},
		{"2001:db8:abcd:12:ffff:ffff:ffff:ffff", "2001:db8:abcd:12::"},
		{"2001:db8:abcd:13::1", "2001:db8:abcd:13::"},
		{"::1", "::"},
		{"[::1]:8080", "::"},
		{"::1.2.3.4", "::"},
		// IPv4-mapped IPv6: v0.15 masked these to "::" like any other IPv6 address;
		// v0.16's CanonicalizeIP would return the dotted IPv4. Kept at v0.15's answer.
		{"::ffff:203.0.113.7", "::"},
		{"[::ffff:203.0.113.7]:443", "::"},
		{"::ffff:198.51.100.9", "::"},
		// Not an IP at all: passed through untouched.
		{"fe80::1%eth0", "fe80::1%eth0"},
		{"not-an-ip", "not-an-ip"},
		{"", ""},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tc.remoteAddr

		got, err := keyByRemoteAddr(req)
		if err != nil {
			t.Fatalf("keyByRemoteAddr(%q) returned an error: %v", tc.remoteAddr, err)
		}
		if got != tc.want {
			t.Errorf("keyByRemoteAddr(%q) = %q, want %q (httprate v0.15's KeyByIP)", tc.remoteAddr, got, tc.want)
		}
	}
}

// callFrom sends one anonymous request from remoteAddr through the limiter and returns its status.
func callFrom(t *testing.T, limiter func(http.Handler) http.Handler, remoteAddr string) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = remoteAddr

	rec := httptest.NewRecorder()
	limiter(okHandler).ServeHTTP(rec, req)
	return rec.Code
}

// The login budget is per client, and "client" means what keyByRemoteAddr says it does: the
// same IPv4 from any port, or anything inside the same IPv6 /64, spends one shared budget,
// while a different address starts fresh.
func TestLoginRateLimit_BucketsByClientIP(t *testing.T) {
	t.Run("same IPv4 from different ports shares a bucket", func(t *testing.T) {
		limiter := LoginRateLimit()
		for i := range 10 {
			if code := callFrom(t, limiter, fmt.Sprintf("198.51.100.20:%d", 50000+i)); code != http.StatusOK {
				t.Fatalf("attempt %d was throttled with %d", i+1, code)
			}
		}
		if code := callFrom(t, limiter, "198.51.100.20"); code != http.StatusTooManyRequests {
			t.Fatalf("the 11th attempt from the same IP (no port, as RealIP leaves it) got %d, want 429", code)
		}
		if code := callFrom(t, limiter, "198.51.100.21:50000"); code != http.StatusOK {
			t.Fatalf("a different IPv4 got %d, want 200 - it must have its own bucket", code)
		}
	})

	t.Run("one IPv6 /64 shares a bucket", func(t *testing.T) {
		limiter := LoginRateLimit()
		for i := range 10 {
			if code := callFrom(t, limiter, fmt.Sprintf("[2001:db8:1:2::%x]:443", i+1)); code != http.StatusOK {
				t.Fatalf("attempt %d was throttled with %d", i+1, code)
			}
		}
		if code := callFrom(t, limiter, "2001:db8:1:2:dead:beef:0:1"); code != http.StatusTooManyRequests {
			t.Fatalf("a new address inside the same /64 got %d, want 429 - rotating within a /64 must not reset the budget", code)
		}
		if code := callFrom(t, limiter, "2001:db8:1:3::1"); code != http.StatusOK {
			t.Fatalf("an address in the next /64 got %d, want 200 - it must have its own bucket", code)
		}
	})
}

// Without a usable token the panel-wide limiter falls back to the same IP key as login, so the
// /64 rule holds there too: a token-less client cannot spread across its /64 to dodge it.
func TestRateLimit_AnonymousFallbackUsesTheSameIPKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-for-rate-limit-tests")

	limiter := RateLimit()
	for i := range 300 {
		if code := callFrom(t, limiter, fmt.Sprintf("[2001:db8:9:9::%x]:443", i+1)); code != http.StatusOK {
			t.Fatalf("anonymous request %d was throttled with %d", i+1, code)
		}
	}
	if code := callFrom(t, limiter, "2001:db8:9:9::ffff"); code != http.StatusTooManyRequests {
		t.Fatalf("anonymous request past the budget from the same /64 got %d, want 429", code)
	}
}
