package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/murandido/m-80-backend/internal/auth"
)

// CORS allows exactly the origins listed in CORS_ALLOWED_ORIGINS, comma separated.
//
// An empty value allows none, and that is the right default rather than an
// oversight: no page in the browser talks to this API directly. Every call
// arrives from the Next.js server, which sends no Origin and triggers no
// preflight, so a panel that works needs no entry here at all. The list exists
// so that the day something does call this API from a browser, the origin is a
// deliberate decision instead of the wildcard this used to carry - a wildcard
// plus AllowCredentials is the one combination you never want facing the
// internet.
func CORS() func(http.Handler) http.Handler {
	var origins []string
	for _, origin := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	options := cors.Options{
		AllowedOrigins: origins,
		// Must cover every verb the router registers, or the browser blocks the request
		// at the preflight - see TestCORS_AllowsEveryVerbTheRouterRegisters.
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}

	// go-chi/cors reads an empty AllowedOrigins as "allow every origin" (see
	// cors.go, where it sets allowedOriginsAll), which is the exact opposite of
	// what an empty list means here. Supplying AllowOriginFunc is what
	// suppresses that default; returning false denies every origin.
	if len(origins) == 0 {
		options.AllowOriginFunc = func(*http.Request, string) bool { return false }
	}

	return cors.Handler(options)
}

// RateLimit throttles per authenticated user rather than per IP.
//
// Per IP is the wrong axis for this API: the browser never talks to it directly.
// Every call arrives from the Next.js server (Server Components and Route
// Handlers), so r.RemoteAddr is always that one host and a per-IP budget is
// really a budget for the whole panel added together. In practice that meant a
// single operator deleting records could exhaust everyone's quota - and since a
// throttled GET /auth/me used to read as an invalid session, it logged them out.
//
// The JWT is signed, so its user_id cannot be forged to spend someone else's
// quota. Requests without a usable token (login, /health, malformed input) fall
// back to the IP, which is the only identity available for them.
func RateLimit() func(http.Handler) http.Handler {
	return httprate.LimitBy(300, 1*time.Minute, keyByTokenSubject)
}

// CredentialRateLimit caps how many account passwords one user can pull through the vault per
// minute. Thirty is far more than anyone logging into programs by hand needs, and far fewer than
// a script copying every account would want; the panel-wide RateLimit still applies on top.
func CredentialRateLimit() func(http.Handler) http.Handler {
	return httprate.LimitBy(30, 1*time.Minute, keyByTokenSubject)
}

// PassengerExtractRateLimit caps how many extraction requests one user can make per minute.
// Each one costs money and reads personal documents; twenty covers someone working through a
// large booking and stops a loop from running up the bill.
func PassengerExtractRateLimit() func(http.Handler) http.Handler {
	return httprate.Limit(20, 1*time.Minute, httprate.WithKeyFuncs(keyByTokenSubject))
}

// LoginRateLimit guards the one route where the IP really is the right identity:
// there is no authenticated user yet, and the traffic worth throttling is someone
// guessing passwords.
//
// The IP it buckets by is whatever RealIP left in r.RemoteAddr, which for a login
// is the browser address the panel forwarded (see middleware.RealIP). Without
// that header every login in the company shares one bucket.
func LoginRateLimit() func(http.Handler) http.Handler {
	return httprate.LimitBy(10, 1*time.Minute, keyByRemoteAddr)
}

// keyByTokenSubject buckets a request by the user its Bearer token belongs to.
//
// A single KeyFunc rather than two: httprate.JoinKeys concatenates the funcs it
// is given into one composite key, it does not try them in order - so the
// fallback to the IP has to happen in here.
func keyByTokenSubject(r *http.Request) (string, error) {
	parts := strings.Split(r.Header.Get("Authorization"), " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return keyByRemoteAddr(r)
	}

	claims, err := auth.ValidateToken(parts[1])
	if err != nil {
		// An invalid or expired token is going to be rejected downstream anyway;
		// counting it against the IP is what stops someone from dodging the limit
		// by sending garbage tokens.
		return keyByRemoteAddr(r)
	}

	userID, ok := claims["user_id"].(string)
	if !ok || userID == "" {
		return keyByRemoteAddr(r)
	}

	return "user:" + userID, nil
}

// keyByRemoteAddr buckets a request by the IP RealIP left in r.RemoteAddr.
//
// It returns exactly the key httprate v0.15's KeyByIP did: v0.16 deprecated it in
// favour of an explicit key function, because a library cannot know what
// RemoteAddr means behind a proxy. Here it already means the client - RealIP is
// what resolves it, and it runs before every limiter - so the right key is still
// RemoteAddr, and the explicit function just states that trust model where the
// limiter is built instead of leaving it to a library default.
//
// The port is dropped so every connection from one host shares a bucket, IPv4
// stays as is and IPv6 collapses to its /64, since one IPv6 client usually
// controls the whole /64 and could otherwise rotate into a fresh bucket per
// request. See TestKeyByRemoteAddr_MatchesHTTPRateV015.
func keyByRemoteAddr(r *http.Request) (string, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return canonicalizeClientIP(host), nil
}

// canonicalizeClientIP is httprate.CanonicalizeIP with the one input where v0.16
// changed its answer put back to v0.15's.
//
// That input is an IPv4-mapped IPv6 address ("::ffff:203.0.113.7"). v0.15 masked
// it to its /64 like any IPv6 address, which leaves "::" - so every such client,
// and ::1, shares one bucket. v0.16 returns the dotted IPv4 instead. Neither
// RealIP's source (Caddy writes plain dotted IPv4) nor net/http produces the
// mapped form, so this should never fire; it is kept so that upgrading the
// library does not quietly redraw the buckets of a security control.
func canonicalizeClientIP(ip string) string {
	if strings.Contains(ip, ":") {
		if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
			return parsed.Mask(net.CIDRMask(64, 128)).String()
		}
	}
	return httprate.CanonicalizeIP(ip)
}
