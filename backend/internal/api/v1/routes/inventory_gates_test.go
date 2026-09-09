package routes

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The points inventory is the one management area where the READ is gated too, which is
// why it cannot ride along in TestManagementRouteGates - that table asserts the opposite
// invariant, that a plain user can always fill the comboboxes on the issuances grid.
//
// Here there is no combobox to fill. These endpoints say what points cost us and how the
// profit splits between agencies, the same reason issuances.cost_of_the_thousand is kept
// out of IssuanceResponse. Over-gating a picker empties a screen; under-gating this one
// hands an operator the margin on every booking they write.
func TestPointsInventoryRoutesAreAdminOnlyIncludingReads(t *testing.T) {
	cases := []struct {
		name     string
		register func(r chi.Router, db *gorm.DB)
		method   string
		path     string
	}{
		{
			name:     "origins listing refuses a plain user",
			register: RegisterPointBatchOriginRoutes,
			method:   http.MethodGet,
			path:     "/point-batch-origins",
		},
		{
			name:     "origins write refuses a plain user",
			register: RegisterPointBatchOriginRoutes,
			method:   http.MethodDelete,
			path:     "/point-batch-origins/00000000-0000-0000-0000-000000000000",
		},
		{
			name:     "batches listing refuses a plain user",
			register: RegisterPointBatchRoutes,
			method:   http.MethodGet,
			path:     "/point-batches?account_id=00000000-0000-0000-0000-000000000000",
		},
		{
			name:     "batches write refuses a plain user",
			register: RegisterPointBatchRoutes,
			method:   http.MethodDelete,
			path:     "/point-batches/00000000-0000-0000-0000-000000000000",
		},
		{
			name:     "batch settlement refuses a plain user",
			register: RegisterPointBatchRoutes,
			method:   http.MethodPatch,
			path:     "/point-batches/00000000-0000-0000-0000-000000000000/settlement",
		},
		{
			name:     "period listing refuses a plain user",
			register: RegisterInventoryPeriodRoutes,
			method:   http.MethodGet,
			path:     "/inventory-periods?account_id=00000000-0000-0000-0000-000000000000",
		},
		{
			name:     "closing a period refuses a plain user",
			register: RegisterInventoryPeriodRoutes,
			method:   http.MethodPost,
			path:     "/inventory-periods",
		},
		{
			name:     "the statement preview refuses a plain user",
			register: RegisterStatementRoutes,
			method:   http.MethodPost,
			path:     "/point-batches/statement/preview",
		},
		{
			name:     "the statement commit refuses a plain user",
			register: RegisterStatementRoutes,
			method:   http.MethodPost,
			path:     "/point-batches/statement/commit",
		},
		{
			name:     "the closing report refuses a plain user",
			register: RegisterInventoryPeriodRoutes,
			method:   http.MethodGet,
			path:     "/inventory-closing?account_ids=00000000-0000-0000-0000-000000000000&from=2026-08-01&to=2026-08-31",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := callRoute(t, tc.register, data.RoleUser, tc.method, tc.path); code != http.StatusForbidden {
				t.Fatalf("%s %s status = %d, want 403 for a plain user", tc.method, tc.path, code)
			}

			// An admin has to get through, or the gate is simply broken rather than strict.
			if code := callRoute(t, tc.register, data.RoleAdmin, tc.method, tc.path); code == http.StatusForbidden {
				t.Fatalf("%s %s answered 403 to an admin", tc.method, tc.path)
			}
		})
	}
}
