package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterInventoryPeriodRoutes wires up closing a period of the points inventory.
//
// Reopening is a DELETE on the period, because that is what it is: the closing goes away
// and the months underneath are recomputable again. Restricted to the account's most recent
// closed period, so the chain is always undone from the top - see inventory.Reopen.
func RegisterInventoryPeriodRoutes(r chi.Router, db *gorm.DB) {
	periodHandler := handlers.NewInventoryPeriodHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/inventory-periods", func(r chi.Router) {
		r.Use(requireAdmin)

		r.Post("/", periodHandler.ClosePeriod)
		r.Get("/", periodHandler.GetInventoryPeriods)
		r.Delete("/{id}", periodHandler.ReopenPeriod)
	})

	// The report the closing button acts on. Read-only, and deliberately reachable
	// before anything is frozen: the figures have to be checkable first.
	closingHandler := handlers.NewInventoryClosingHandler(db)
	r.With(requireAdmin).Get("/inventory-closing", closingHandler.GetInventoryClosing)
}
