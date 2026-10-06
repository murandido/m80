package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/health"
)

// go-chi/cors reads an empty AllowedOrigins as "allow every origin", so the
// default this project wants - no cross-site origin at all - holds only because
// CORS() special-cases the empty list. Nothing else would catch that
// regression: the panel works either way, because the browser never calls this
// API directly and server-to-server requests send no Origin.
func TestCORS_WithoutAnAllowlistDeniesEveryOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	router := NewRouter(nil, events.NewBroker(), nil, health.NewMonitor(func() {}), nil, testAzulConnector())

	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "https://atacante.exemplo")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin got %q, want it absent", got)
	}
}

// The CORS allowlist in middleware.CORS() is maintained by hand, so it drifts the
// moment a route is registered with a verb nobody remembered to add there - which is
// exactly how PATCH ended up blocked at the preflight. Rather than asserting against a
// fixed list (which would drift the same way), this walks the real router and demands
// that every verb it actually serves survives a preflight.
func TestCORS_AllowsEveryVerbTheRouterRegisters(t *testing.T) {
	// CORS() reads its allowlist at construction time, and an unset variable means
	// no cross-site origin is allowed at all - which would make every preflight
	// below fail for the wrong reason.
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://painel.exemplo")

	// A nil *gorm.DB is safe here: no handler constructor touches the database (they
	// only store it), and the CORS middleware answers preflights before the auth
	// middleware or any handler gets to run. The broker has to be real, though - the
	// SSE route subscribes to it at request time. The health monitor likewise
	// has to be real: the status handler reads its snapshot.
	router := NewRouter(nil, events.NewBroker(), nil, health.NewMonitor(func() {}), nil, testAzulConnector())

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
			// Must match the origin set in CORS_ALLOWED_ORIGINS above.
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

// testAzulConnector is a connector with no database or login behind it: the router only needs its
// tracker to mount the Azul routes, and these tests never reach a connect.
func testAzulConnector() *azulsync.Connector {
	return azulsync.NewConnector(nil, azulsync.NewConnectTracker(), nil, nil)
}
