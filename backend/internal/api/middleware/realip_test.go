package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The whole value of RealIP is that a client cannot choose which rate-limit
// bucket it lands in. These cases are the ones that decide that: the header is
// only honoured behind a trusted proxy, and only its rightmost entry counts.
func TestRealIP(t *testing.T) {
	const proxyAddr = "172.18.0.5:41234"

	cases := []struct {
		name         string
		trustedProxy string
		forwardedFor string
		want         string
	}{
		{
			name:         "no proxy in front, header is pure client input and is ignored",
			trustedProxy: "",
			forwardedFor: "1.2.3.4",
			want:         proxyAddr,
		},
		{
			name:         "behind the proxy, the address the proxy appended is used",
			trustedProxy: "true",
			forwardedFor: "203.0.113.7",
			want:         "203.0.113.7",
		},
		{
			// The bypass this middleware exists to close: everything left of
			// the last entry was written by the client.
			name:         "a forged entry to the left does not move the bucket",
			trustedProxy: "true",
			forwardedFor: "1.2.3.4, 203.0.113.7",
			want:         "203.0.113.7",
		},
		{
			name:         "an unparseable last entry falls back to the proxy, not to the forged one",
			trustedProxy: "true",
			forwardedFor: "1.2.3.4, not-an-ip",
			want:         proxyAddr,
		},
		{
			name:         "no header at all leaves RemoteAddr alone",
			trustedProxy: "true",
			forwardedFor: "",
			want:         proxyAddr,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TRUSTED_PROXY", tc.trustedProxy)

			var got string
			handler := RealIP()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr
			}))

			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			req.RemoteAddr = proxyAddr
			if tc.forwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tc.forwardedFor)
			}

			handler.ServeHTTP(httptest.NewRecorder(), req)

			if got != tc.want {
				t.Errorf("RemoteAddr got %q, want %q", got, tc.want)
			}
		})
	}
}
