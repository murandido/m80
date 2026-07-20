package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAirportRoutes wires up the /airports routes. The typeahead search and
// the paginated listing are reachable by any authenticated user (the search
// feeds comboboxes on other screens); write routes and get-by-id require admin,
// applied inline per route via r.With() rather than a second mount at the same
// "/airports" path, which chi rejects.
func RegisterAirportRoutes(r chi.Router, db *gorm.DB) {
	airportHandler := handlers.NewAirportHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/airports", func(r chi.Router) {
		// typeahead search (feeds comboboxes)
		r.Get("/", airportHandler.GetAirports)

		// full list (feeds client-side comboboxes)
		r.Get("/all", airportHandler.ListAllAirports)

		// paginated listing (management screen)
		r.Get("/manage", airportHandler.ListAirports)

		// create airport
		r.With(requireAdmin).Post("/", airportHandler.CreateAirport)

		// get airport
		r.With(requireAdmin).Get("/{id}", airportHandler.GetAirport)

		// update airport
		r.With(requireAdmin).Put("/{id}", airportHandler.UpdateAirport)

		// delete airport
		r.With(requireAdmin).Delete("/{id}", airportHandler.DeleteAirport)
	})
}
