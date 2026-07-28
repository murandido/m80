package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/murandido/m-80-backend/internal/auth"
)

func CORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins: []string{"https://*", "http://*"},
		// Must cover every verb the router registers, or the browser blocks the request
		// at the preflight - see TestCORS_AllowsEveryVerbTheRouterRegisters.
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
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
	return httprate.Limit(300, 1*time.Minute, httprate.WithKeyFuncs(keyByTokenSubject))
}

// LoginRateLimit guards the one route where the IP really is the right identity:
// there is no authenticated user yet, and the traffic worth throttling is someone
// guessing passwords.
func LoginRateLimit() func(http.Handler) http.Handler {
	return httprate.LimitByIP(10, 1*time.Minute)
}

// keyByTokenSubject buckets a request by the user its Bearer token belongs to.
//
// A single KeyFunc rather than two: httprate.WithKeyFuncs concatenates the funcs
// it is given into one composite key, it does not try them in order - so the
// fallback to the IP has to happen in here.
func keyByTokenSubject(r *http.Request) (string, error) {
	parts := strings.Split(r.Header.Get("Authorization"), " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return httprate.KeyByIP(r)
	}

	claims, err := auth.ValidateToken(parts[1])
	if err != nil {
		// An invalid or expired token is going to be rejected downstream anyway;
		// counting it against the IP is what stops someone from dodging the limit
		// by sending garbage tokens.
		return httprate.KeyByIP(r)
	}

	userID, ok := claims["user_id"].(string)
	if !ok || userID == "" {
		return httprate.KeyByIP(r)
	}

	return "user:" + userID, nil
}
