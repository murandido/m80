package routes

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The report reads every airline's lookups and the locators of the issuances behind them;
// it sits next to the dev-only airline screen and is gated the same way.
func TestBookingLookupReportIsDevOnly(t *testing.T) {
	register := func(r chi.Router, db *gorm.DB) { RegisterBookingLookupRoutes(r, db, booking.NewRegistry(nil)) }
	for _, role := range []string{data.RoleUser, data.RoleAdmin} {
		if code := callRoute(t, register, role, http.MethodGet, "/booking-lookups/report"); code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", role, code)
		}
	}
}
