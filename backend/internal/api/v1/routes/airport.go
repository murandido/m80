package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAirportRoutes wires up the /airports search route.
func RegisterAirportRoutes(r chi.Router, db *gorm.DB) {
	airportHandler := handlers.NewAirportHandler(db)

	r.Route("/airports", func(r chi.Router) {
		// search airports
		r.Get("/", airportHandler.GetAirports)
	})
}
