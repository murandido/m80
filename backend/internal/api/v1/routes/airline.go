package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAirlineRoutes wires up the /airlines CRUD routes.
func RegisterAirlineRoutes(r chi.Router, db *gorm.DB) {
	airlineHandler := handlers.NewAirlineHandler(db)

	r.Route("/airlines", func(r chi.Router) {
		// create airline
		r.Post("/", airlineHandler.CreateAirline)

		// get airlines
		r.Get("/", airlineHandler.GetAirlines)

		// get airline
		r.Get("/{id}", airlineHandler.GetAirline)

		// update airline
		r.Put("/{id}", airlineHandler.UpdateAirline)

		// delete airline
		r.Delete("/{id}", airlineHandler.DeleteAirline)
	})
}
