package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The whole point of these routes is the split between reading and writing. Reading has to
// stay open to a plain user because the issuances grid - the operator's main screen - fills
// seven comboboxes from these very endpoints; a gate that is one route too wide empties
// that screen. So every case here is asserted twice: the write is refused and the read is
// not.
//
// A 403 proves the gate fired before the handler. A nil *gorm.DB is deliberate: anything
// that gets past the gate would panic on it, so "not 403" can never be a false pass.
func TestManagementRouteGates(t *testing.T) {
	cases := []struct {
		name string
		// register mounts the routes under test on a router that already has a role.
		register func(r chi.Router, db *gorm.DB)
		method   string
		path     string
		// roles that must be refused with 403
		refused []string
		// a read on the same resource that must never be refused, whoever asks
		openMethod string
		openPath   string
	}{
		{
			name:       "agency write is admin only",
			register:   RegisterAgencyRoutes,
			method:     http.MethodDelete,
			path:       "/agencies/00000000-0000-0000-0000-000000000000",
			refused:    []string{data.RoleUser},
			openMethod: http.MethodGet,
			openPath:   "/agencies",
		},
		{
			name:       "airline write is dev only",
			register:   RegisterAirlineRoutes,
			method:     http.MethodDelete,
			path:       "/airlines/00000000-0000-0000-0000-000000000000",
			refused:    []string{data.RoleUser, data.RoleAdmin},
			openMethod: http.MethodGet,
			openPath:   "/airlines",
		},
		{
			name:       "loyalty program write is dev only",
			register:   RegisterLoyaltyProgramRoutes,
			method:     http.MethodDelete,
			path:       "/loyalty-programs/00000000-0000-0000-0000-000000000000",
			refused:    []string{data.RoleUser, data.RoleAdmin},
			openMethod: http.MethodGet,
			openPath:   "/loyalty-programs",
		},
		{
			// Merging a row rewrites who owns accounts and then deletes records for good.
			// The rest of /people stays open: the pickers on the issuances grid read it.
			name:       "row merge is admin only",
			register:   RegisterPeopleRoutes,
			method:     http.MethodPost,
			path:       "/people/00000000-0000-0000-0000-000000000000/merge-row",
			refused:    []string{data.RoleUser},
			openMethod: http.MethodGet,
			openPath:   "/people",
		},
		{
			name:       "row merge preview is admin only",
			register:   RegisterPeopleRoutes,
			method:     http.MethodGet,
			path:       "/people/00000000-0000-0000-0000-000000000000/merge-row-preview",
			refused:    []string{data.RoleUser},
			openMethod: http.MethodGet,
			openPath:   "/people",
		},
		{
			// Correcting a beneficiary is the operator's daily work and stays open; only
			// the reconcile - which discards every correction anyone ever made - is gated.
			name:       "beneficiary reconcile is admin only",
			register:   registerAccountRoutes,
			method:     http.MethodPost,
			path:       "/accounts/00000000-0000-0000-0000-000000000000/beneficiaries/reconcile",
			refused:    []string{data.RoleUser},
			openMethod: http.MethodGet,
			openPath:   "/accounts/00000000-0000-0000-0000-000000000000/beneficiaries",
		},
		{
			name:       "airport management listing is dev only",
			register:   RegisterAirportRoutes,
			method:     http.MethodGet,
			path:       "/airports/manage",
			refused:    []string{data.RoleUser, data.RoleAdmin},
			openMethod: http.MethodGet,
			openPath:   "/airports/all",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, role := range tc.refused {
				t.Run("refuses "+role, func(t *testing.T) {
					code := callRoute(t, tc.register, role, tc.method, tc.path)
					if code != http.StatusForbidden {
						t.Fatalf("status = %d, want 403", code)
					}
				})
			}

			// The read is the guard against over-gating. It reaches the handler and dies
			// on the nil DB, which is fine: what matters is that it was not turned away.
			t.Run("read stays open to a plain user", func(t *testing.T) {
				code := callRoute(t, tc.register, data.RoleUser, tc.openMethod, tc.openPath)
				if code == http.StatusForbidden {
					t.Fatalf("%s %s answered 403 to a plain user; the comboboxes on the issuances grid read it", tc.openMethod, tc.openPath)
				}
			})
		})
	}
}

// callRoute mounts the routes behind the given role and returns the status. A panic from
// the nil *gorm.DB means the request reached the handler, which for these tests is the
// same information as "the gate let it through".
func callRoute(t *testing.T, register func(chi.Router, *gorm.DB), role, method, path string) (code int) {
	t.Helper()

	defer func() {
		if recover() != nil {
			code = http.StatusOK
		}
	}()

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(withRole(role))
		register(r, nil)
	})

	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}
