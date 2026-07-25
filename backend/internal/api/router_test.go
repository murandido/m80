package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/events"
)

// The CORS allowlist in middleware.CORS() is maintained by hand, so it drifts the
// moment a route is registered with a verb nobody remembered to add there - which is
// exactly how PATCH ended up blocked at the preflight. Rather than asserting against a
// fixed list (which would drift the same way), this walks the real router and demands
// that every verb it actually serves survives a preflight.
func TestCORS_AllowsEveryVerbTheRouterRegisters(t *testing.T) {
	// A nil *gorm.DB is safe here: no handler constructor touches the database (they
	// only store it), and the CORS middleware answers preflights before the auth
	// middleware or any handler gets to run. The broker has to be real, though - the
	// SSE route subscribes to it at request time.
	router := NewRouter(nil, events.NewBroker())

	// One representative route per verb is enough - CORS is configured globally, not
	// per route. The route is kept only to make a failure message actionable.
	routeForMethod := map[string]string{}
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routeForMethod[method] = route
		return nil
	}
	if err := chi.Walk(router, walk); err != nil {
		t.Fatalf("failed to walk the router: %v", err)
	}

	if len(routeForMethod) == 0 {
		t.Fatal("walked the router and found no routes - this test would pass vacuously")
	}

	for method, route := range routeForMethod {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, route, nil)
			// Must match the configured "https://*" origin pattern.
			req.Header.Set("Origin", "https://painel.exemplo")
			req.Header.Set("Access-Control-Request-Method", method)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			// go-chi/cors echoes the requested method back when it is allowed, and
			// omits the header entirely when it is not - so an empty value here means
			// the browser would refuse to send the real request.
			got := rec.Header().Get("Access-Control-Allow-Methods")
			if got != method {
				t.Errorf(
					"preflight for %s %s: Access-Control-Allow-Methods got %q, want %q",
					method, route, got, method,
				)
			}
		})
	}
}
