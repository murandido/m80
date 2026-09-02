package routes

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// The paginated management listings live at /manage, one static segment next to a
// /{id} on the same resource. Chi resolves a static segment ahead of a param, so the
// two coexist - but nothing in the handler tests can prove that, because they call
// handlers directly and never go through the router.
//
// It is worth proving because the failure is silent. If /manage were read as an id,
// the request would reach the single-item handler, find nothing and answer 404 - and
// the screen would just render as empty, with no error anywhere.
//
// Match rather than ServeHTTP: it resolves the route and reports which pattern won
// without running the handler, so a nil *gorm.DB is safe and the assertion is about
// routing alone.
func TestManageListingsAreRoutedAheadOfTheIDParam(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		register func(chi.Router, *gorm.DB)
		path     string
		want     string
	}{
		{"accounts", RegisterAccountRoutes, "/accounts/manage", "/accounts/manage"},
		{"clients", RegisterClientRoutes, "/clients/manage", "/clients/manage"},
		{"people", RegisterPeopleRoutes, "/people/manage", "/people/manage"},
		{"airports", RegisterAirportRoutes, "/airports/manage", "/airports/manage"},
		{
			// This one has no /{id} GET to lose to, but it does have /{id} on PUT and
			// DELETE - so the pattern is the same and so is the reason to pin it.
			"loyalty documents", RegisterLoyaltyDocumentRoutes,
			"/loyalty-documents/manage", "/loyalty-documents/manage",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := chi.NewRouter()
			tc.register(r, nil)

			rctx := chi.NewRouteContext()
			if !r.Match(rctx, http.MethodGet, tc.path) {
				t.Fatalf("GET %s matched no route at all", tc.path)
			}
			if got := rctx.RoutePattern(); got != tc.want {
				t.Fatalf("GET %s resolved to %q, want %q - the static segment must win over the id param",
					tc.path, got, tc.want)
			}
		})
	}
}

// The other half of every split: the unpaginated route is what the pickers read, and
// it has to keep answering on the bare path.
func TestPickerListingsKeepTheBarePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		register func(chi.Router, *gorm.DB)
		path     string
	}{
		{"accounts", RegisterAccountRoutes, "/accounts"},
		{"clients", RegisterClientRoutes, "/clients"},
		{"people", RegisterPeopleRoutes, "/people"},
		{"airports", RegisterAirportRoutes, "/airports/all"},
		{"loyalty documents", RegisterLoyaltyDocumentRoutes, "/loyalty-documents"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := chi.NewRouter()
			tc.register(r, nil)

			rctx := chi.NewRouteContext()
			if !r.Match(rctx, http.MethodGet, tc.path) {
				t.Fatalf("GET %s matched no route", tc.path)
			}
			if got := rctx.RoutePattern(); got != tc.path {
				t.Fatalf("GET %s resolved to %q, want the bare path", tc.path, got)
			}
		})
	}
}
