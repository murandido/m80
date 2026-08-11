package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAirportRoutes wires up the /airports routes. The typeahead search and the full
// list are reachable by any authenticated user - they feed the origin/destination
// comboboxes on the issuances grid, which is the plain operator's main screen. Everything
// that belongs to the management screen requires dev, applied inline per route via
// r.With() rather than a second mount at the same "/airports" path, which chi rejects.
func RegisterAirportRoutes(r chi.Router, db *gorm.DB) {
	airportHandler := handlers.NewAirportHandler(db)
	requireDev := middleware.RequireDev()

	r.Route("/airports", func(r chi.Router) {
		// typeahead search (feeds comboboxes)
		r.Get("/", airportHandler.GetAirports)

		// full list (feeds client-side comboboxes)
		r.Get("/all", airportHandler.ListAllAirports)

		// paginated listing (management screen)
		r.With(requireDev).Get("/manage", airportHandler.ListAirports)

		// create airport
		r.With(requireDev).Post("/", airportHandler.CreateAirport)

		// get airport
		r.With(requireDev).Get("/{id}", airportHandler.GetAirport)

		// update airport
		r.With(requireDev).Put("/{id}", airportHandler.UpdateAirport)

		// delete airport
		r.With(requireDev).Delete("/{id}", airportHandler.DeleteAirport)
	})
}
