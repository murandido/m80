package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"gorm.io/gorm"
)

// Both programmes' statements are now synced and read from the database. The upload that used
// to live at /point-batches/statement is gone with the Smiles HTML parser it fed: nothing in
// the panel pointed at it any more, and a route nobody calls is a route nobody tests.

// RegisterAzulStatementRoutes mounts the Azul reconciliation, which reads the synced window
// from the database instead of an upload, plus the one-time connect action that discovers the
// account's customerKey. Account-scoped because the account is what selects the window;
// admin-only like the rest of the inventory.
func RegisterAzulStatementRoutes(r chi.Router, db *gorm.DB) {
	// One tracker per process, shared by both handlers: the connect handler marks an account in and
	// out while its background login runs, and the statement handler reads it into the "conectando"
	// badge. See azulsync.ConnectTracker.
	tracker := azulsync.NewConnectTracker()

	h := handlers.NewStatementHandler(db)
	h.Tracker = tracker
	connect := handlers.NewAzulConnectHandler(db, tracker)

	r.Route("/accounts/{id}/azul-statement", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())

		r.Get("/", h.PreviewAzulStatement)
		r.Post("/commit", h.CommitAzulStatement)

		// The cheap sync-state badge on its own, for the account card to poll while a connect runs
		// (the entries endpoint below is far heavier - it resolves every link).
		r.Get("/status", h.AzulConnectStatus)

		// The per-account statement page: the synced lines with each redemption resolved to
		// its issuance, plus the manual-link override (edit mode) and a CSV export.
		r.Get("/entries", h.AzulStatementEntries)
		r.Get("/export", h.ExportAzulStatement)
		r.Post("/links", h.LinkStatementBooking)
		r.Delete("/links", h.UnlinkStatementBooking)
		// The point-entry side: purchases and accruals link to a batch (lote).
		r.Post("/batch-links", h.LinkStatementBatch)
		r.Delete("/batch-links", h.UnlinkStatementBatch)
	})

	r.With(middleware.RequireAdmin()).Post("/accounts/{id}/azul-connect", connect.ConnectAzulAccount)

	// The city-to-IATA table is global, not per-account: assigning a code fixes the route display
	// for every account's statement at once.
	r.With(middleware.RequireAdmin()).Post("/azul-city-codes", h.AssignAzulCityCode)
}
