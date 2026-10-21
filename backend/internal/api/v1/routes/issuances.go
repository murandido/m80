package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// RegisterIssuanceRoutes wires up the /issuances CRUD routes. The broker is passed
// down so each mutation can announce itself on the SSE stream.
func RegisterIssuanceRoutes(r chi.Router, db *gorm.DB, broker *events.Broker, bookings *booking.Registry) {
	issuanceHandler := handlers.NewIssuanceHandler(db, broker, bookings)

	r.Route("/issuances", func(r chi.Router) {
		// create issuance
		r.Post("/", issuanceHandler.CreateIssuance)

		// get issuances
		r.Get("/", issuanceHandler.GetIssuances)

		// get issuances (resumo para listagem)
		r.Get("/summary", issuanceHandler.GetIssuancesSummary)

		// export filtered issuances as an .xlsx spreadsheet
		r.Get("/export", issuanceHandler.GetIssuancesExport)

		// preview a booking at the airline before the issuance exists - the panel's
		// form asks for the itinerary before the values an issuance needs to be saved
		r.Post("/lookup", issuanceHandler.LookupIssuanceBooking)

		// Everything addressed by id sits behind the scope check, so a route added to this
		// group is protected without having to remember it (see RequireIssuanceInScope).
		r.Route("/{id}", func(r chi.Router) {
			r.Use(middleware.RequireIssuanceInScope(db))

			// get issuance
			r.Get("/", issuanceHandler.GetIssuance)

			// update issuance (autosave, partial)
			r.Patch("/", issuanceHandler.UpdateIssuance)

			// delete issuance (soft delete)
			r.Delete("/", issuanceHandler.DeleteIssuance)

			// cancel issuance (deactivate + refund + audit)
			r.Post("/cancel", issuanceHandler.CancelIssuance)

			// uncancel issuance (revert cancellation, restore prior sale state + audit)
			r.Post("/uncancel", issuanceHandler.UncancelIssuance)

			// alterations to the ticket after it was issued: miles or money the airline
			// charged or gave back, each a dated movement of its own
			r.Post("/adjustments", issuanceHandler.CreateIssuanceAdjustment)
			r.Patch("/adjustments/{adjustmentID}", issuanceHandler.UpdateIssuanceAdjustment)
			r.Delete("/adjustments/{adjustmentID}", issuanceHandler.DeleteIssuanceAdjustment)

			// pull the itinerary from Traveos, issuing the ticket there first if needed
			r.Post("/traveos-sync", issuanceHandler.SyncIssuanceWithTraveos)

			// the passenger's ticket - the same document twice, as the page the panel
			// shows and as the file it downloads
			r.Get("/ticket", issuanceHandler.GetIssuanceTicket)
			r.Get("/pdf", issuanceHandler.GetIssuanceTicketPDF)

			// the two things about a booking the airline changes after issuing, and
			// that Traveos lets an operator correct. Each PUT replaces its whole set.
			r.Get("/seats", issuanceHandler.GetIssuanceSeats)
			r.Put("/seats", issuanceHandler.PutIssuanceSeats)
			r.Get("/baggage", issuanceHandler.GetIssuanceBaggage)
			r.Put("/baggage", issuanceHandler.PutIssuanceBaggage)

			// passengers sub-resource
			RegisterIssuancePassengerRoutes(r, db)
		})
	})
}
