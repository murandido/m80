package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
)

// Devs decide which agencies each account sees, so creating an agency, renaming it, moving
// is_default or editing its nicknames are theirs alone - an admin able to do any of it would
// be shaping the scope that is supposed to constrain them.
func TestRegisterAgencyRoutes_WritesRequireDev(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001"
	for _, tc := range []struct{ name, method, path string }{
		{"create", http.MethodPost, "/agencies"},
		{"update", http.MethodPut, "/agencies/" + id},
		{"delete", http.MethodDelete, "/agencies/" + id},
		{"nickname availability", http.MethodGet, "/agencies/nicknames/availability?nickname=x"},
		{"create nickname", http.MethodPost, "/agencies/" + id + "/nicknames"},
		{"update nickname", http.MethodPut, "/agencies/" + id + "/nicknames/" + id},
		{"delete nickname", http.MethodDelete, "/agencies/" + id + "/nicknames/" + id},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Group(func(r chi.Router) {
				r.Use(withRole(data.RoleAdmin))
				RegisterAgencyRoutes(r, nil)
			})

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}
