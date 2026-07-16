package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterIssuanceRoutes wires up the /issuances CRUD routes.
func RegisterIssuanceRoutes(r chi.Router, db *gorm.DB) {
	issuanceHandler := handlers.NewIssuanceHandler(db)

	r.Route("/issuances", func(r chi.Router) {
		// create issuance
		r.Post("/", issuanceHandler.CreateIssuance)

		// get issuances
		r.Get("/", issuanceHandler.GetIssuances)

		// get issuance
		r.Get("/{id}", issuanceHandler.GetIssuance)

		// update issuance (autosave, partial)
		r.Patch("/{id}", issuanceHandler.UpdateIssuance)

		// delete issuance (soft delete)
		r.Delete("/{id}", issuanceHandler.DeleteIssuance)

		// passengers sub-resource
		RegisterIssuancePassengerRoutes(r, db)
	})
}
