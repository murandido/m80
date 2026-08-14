package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// The importer writes issuances dated weeks in the past, on somebody else's behalf, dozens
// at a time. Both of its routes are admin-only, and both are POSTs - there is no read half
// to keep open, which is why this does not join the table in management_gates_test.go.
func TestLegacyImportRoutesAreAdminOnly(t *testing.T) {
	paths := []string{
		"/issuances/legacy-import/preview",
		"/issuances/legacy-import/commit",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Run("refuses a plain user", func(t *testing.T) {
				if code := callLegacyImport(t, data.RoleUser, path); code != http.StatusForbidden {
					t.Fatalf("status = %d, want 403", code)
				}
			})

			// The guard against over-gating: an admin has to get past the middleware
			// and reach the handler, which then answers 400 on the empty body.
			t.Run("lets an admin through", func(t *testing.T) {
				if code := callLegacyImport(t, data.RoleAdmin, path); code == http.StatusForbidden {
					t.Fatalf("%s answered 403 to an admin", path)
				}
			})
		})
	}
}

// callLegacyImport mounts the routes behind the given role and posts an empty body. A
// panic from the nil *gorm.DB would mean the request reached the handler; it does not
// happen here because the handler rejects the malformed upload before touching the
// database, which is itself the point of reading the file first.
func callLegacyImport(t *testing.T, role, path string) (code int) {
	t.Helper()

	defer func() {
		if recover() != nil {
			code = http.StatusOK
		}
	}()

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(withRole(role))
		RegisterLegacyImportRoutes(r, (*gorm.DB)(nil), events.NewBroker())
	})

	req := httptest.NewRequest(http.MethodPost, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}
