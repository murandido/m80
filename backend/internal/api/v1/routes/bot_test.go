package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
)

// The gate lives on the route (like the sales suppliers view), so it has to be tested
// through the router. admin is in this table on purpose: the WhatsApp session used to be
// an admin screen and is now dev-only, so admin being turned away is the change, not an
// oversight.
func TestRegisterBotRoutes_RequiresDev(t *testing.T) {
	t.Setenv("BOT_INTERNAL_URL", "http://127.0.0.1:1")

	for _, role := range []string{data.RoleUser, data.RoleAdmin} {
		t.Run(role, func(t *testing.T) {
			r := chi.NewRouter()
			r.Group(func(r chi.Router) {
				r.Use(withRole(role))
				RegisterBotRoutes(r)
			})

			req := httptest.NewRequest(http.MethodGet, "/bot/status", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}

// Proves the dev path is pure pass-through: status code and body come from
// the bot's internal API untouched, with no envelope or parsing in between.
func TestRegisterBotRoutes_DevProxiesToBotAPI(t *testing.T) {
	botServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/status" {
			t.Errorf("upstream path: got %q, want %q", req.URL.Path, "/status")
		}
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"success":true,"data":{"state":"disconnected"}}`))
	}))
	defer botServer.Close()

	t.Setenv("BOT_INTERNAL_URL", botServer.URL)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(withRole(data.RoleDev))
		RegisterBotRoutes(r)
	})

	req := httptest.NewRequest(http.MethodGet, "/bot/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusTeapot, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"state":"disconnected"`) {
		t.Fatalf("body was not forwarded untouched: %s", rec.Body.String())
	}
}
