package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// RealIP rewrites r.RemoteAddr with the client address the reverse proxy saw,
// and only does so when TRUSTED_PROXY is "true".
//
// Why it exists: the API has no public address, so every request it serves
// arrives from the panel's container over the Compose network and r.RemoteAddr
// is that one host on every single request. That turns LoginRateLimit's "10
// attempts per minute per IP" into 10 per minute for everybody at once - one
// operator in a wrong-password loop drains the budget for the whole company, and
// brute force becomes indistinguishable from normal traffic. keyByTokenSubject
// degrades the same way for every request without a valid token.
//
// The header therefore has to be put there by the panel: Next's server-side fetch
// forwards nothing of the browser's request on its own. `clientIpHeader` in
// frontend/src/lib/client-ip.ts is the other half of this, and it sends the one
// entry the rule below is willing to read. The /health path Caddy still proxies
// straight to the API is the only request that arrives with a header this
// middleware did not ask the panel for.
//
// Why the rightmost entry and not the leftmost: a proxy *appends* the address
// of the peer it received the request from, so a client that sends its own
// X-Forwarded-For only ever adds entries to the left. Everything left of what
// Caddy wrote is attacker-authored; the last entry is the only one that cannot
// be forged. chi/middleware.RealIP reads the leftmost, which is precisely the
// rate-limiter bypass the note in internal/api/CLAUDE.md warns about - hence
// this middleware instead of that one.
//
// Why the env flag: without a proxy in front, the header is pure client input
// and honouring it would hand out the bypass for free. Off is the safe default,
// so anything other than an explicit "true" leaves RemoteAddr untouched.
func RealIP() func(http.Handler) http.Handler {
	trustedProxy := os.Getenv("TRUSTED_PROXY") == "true"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if trustedProxy {
				if ip := rightmostForwardedFor(r.Header.Get("X-Forwarded-For")); ip != "" {
					r.RemoteAddr = ip
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rightmostForwardedFor returns the last entry of an X-Forwarded-For header,
// or "" when there is none or it is not a parseable IP.
//
// It deliberately does not fall back to earlier entries when the last one is
// unusable. Those are the client-authored ones, so scanning leftwards for
// something that parses would hand back exactly the value an attacker planted.
// Falling back to the proxy address is the boring, safe failure.
func rightmostForwardedFor(header string) string {
	entries := strings.Split(header, ",")
	last := strings.TrimSpace(entries[len(entries)-1])
	if net.ParseIP(last) == nil {
		return ""
	}
	return last
}
