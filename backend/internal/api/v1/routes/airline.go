package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAirlineRoutes wires up the /airlines CRUD routes. The airline catalogue is
// seeded from a vendor list and mirrors the Traveos ids, so editing it is a dev screen;
// reading stays open because the airline combobox is on the dashboard, on the issuances
// grid and on the loyalty programs screen.
func RegisterAirlineRoutes(r chi.Router, db *gorm.DB) {
	airlineHandler := handlers.NewAirlineHandler(db)
	requireDev := middleware.RequireDev()

	r.Route("/airlines", func(r chi.Router) {
		// create airline
		r.With(requireDev).Post("/", airlineHandler.CreateAirline)

		// get airlines
		r.Get("/", airlineHandler.GetAirlines)

		// get airline
		r.Get("/{id}", airlineHandler.GetAirline)

		// update airline
		r.With(requireDev).Put("/{id}", airlineHandler.UpdateAirline)

		// delete airline
		r.With(requireDev).Delete("/{id}", airlineHandler.DeleteAirline)
	})
}
