package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// RegisterIssuanceRoutes wires up the /issuances CRUD routes. The broker is passed
// down so each mutation can announce itself on the SSE stream.
func RegisterIssuanceRoutes(r chi.Router, db *gorm.DB, broker *events.Broker) {
	issuanceHandler := handlers.NewIssuanceHandler(db, broker)

	r.Route("/issuances", func(r chi.Router) {
		// create issuance
		r.Post("/", issuanceHandler.CreateIssuance)

		// get issuances
		r.Get("/", issuanceHandler.GetIssuances)

		// get issuances (resumo para listagem)
		r.Get("/summary", issuanceHandler.GetIssuancesSummary)

		// export filtered issuances as an .xlsx spreadsheet
		r.Get("/export", issuanceHandler.GetIssuancesExport)

		// get issuance
		r.Get("/{id}", issuanceHandler.GetIssuance)

		// update issuance (autosave, partial)
		r.Patch("/{id}", issuanceHandler.UpdateIssuance)

		// delete issuance (soft delete)
		r.Delete("/{id}", issuanceHandler.DeleteIssuance)

		// cancel issuance (deactivate + refund + audit)
		r.Post("/{id}/cancel", issuanceHandler.CancelIssuance)

		// uncancel issuance (revert cancellation, restore prior sale state + audit)
		r.Post("/{id}/uncancel", issuanceHandler.UncancelIssuance)

		// passengers sub-resource
		RegisterIssuancePassengerRoutes(r, db)
	})
}
