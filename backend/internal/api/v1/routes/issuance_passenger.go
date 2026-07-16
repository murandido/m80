package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterIssuancePassengerRoutes wires up the /issuances/{id}/passengers CRUD routes.
func RegisterIssuancePassengerRoutes(r chi.Router, db *gorm.DB) {
	issuancePassengerHandler := handlers.NewIssuancePassengerHandler(db)

	r.Route("/{id}/passengers", func(r chi.Router) {
		// create passenger
		r.Post("/", issuancePassengerHandler.CreateIssuancePassenger)

		// get passengers
		r.Get("/", issuancePassengerHandler.GetIssuancePassengers)

		// get passenger
		r.Get("/{passengerID}", issuancePassengerHandler.GetIssuancePassenger)

		// update passenger
		r.Put("/{passengerID}", issuancePassengerHandler.UpdateIssuancePassenger)

		// delete passenger
		r.Delete("/{passengerID}", issuancePassengerHandler.DeleteIssuancePassenger)
	})
}
