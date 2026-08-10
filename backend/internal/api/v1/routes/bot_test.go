package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
)

// The gate lives on the route (like the sales suppliers view), so it has to
// be tested through the router.
func TestRegisterBotRoutes_RequiresAdmin(t *testing.T) {
	t.Setenv("BOT_INTERNAL_URL", "http://127.0.0.1:1")

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(withRole(data.RoleUser))
		RegisterBotRoutes(r)
	})

	req := httptest.NewRequest(http.MethodGet, "/bot/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// Proves the admin path is pure pass-through: status code and body come from
// the bot's internal API untouched, with no envelope or parsing in between.
func TestRegisterBotRoutes_AdminProxiesToBotAPI(t *testing.T) {
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
		r.Use(withRole(data.RoleAdmin))
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
