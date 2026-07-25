package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
)

// withRole stands in for RequireAuth: it injects an already-authenticated user so the
// route's own RequireAdmin gate is what the test exercises.
func withRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), middleware.UserContextKey,
				data.User{ID: "00000000-0000-0000-0000-000000000000", Role: role})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// The suppliers view exposes what the agency owes each account administrator, which is not
// an operator's business - the gate lives on the route, so it has to be tested through the
// router rather than by calling the handler.
//
// Only the rejection is asserted here: a nil *gorm.DB and a nil broker are enough to prove
// the request never reaches the handler, and the admin path is already covered by the
// handler tests in internal/api/v1/handlers/sales_test.go, which have a real database.
func TestRegisterSaleRoutes_SuppliersRequireAdmin(t *testing.T) {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(withRole("user"))
		RegisterSaleRoutes(r, nil, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/sales/suppliers", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}
