package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterLoyaltyProgramRoutes wires up the /loyalty-programs CRUD routes. Same split as
// the airlines: editing the catalogue is a dev screen, reading feeds the comboboxes on the
// dashboard, the issuances grid and the accounts screen.
func RegisterLoyaltyProgramRoutes(r chi.Router, db *gorm.DB) {
	loyaltyProgramHandler := handlers.NewLoyaltyProgramHandler(db)
	requireDev := middleware.RequireDev()

	r.Route("/loyalty-programs", func(r chi.Router) {
		// create loyalty program
		r.With(requireDev).Post("/", loyaltyProgramHandler.CreateLoyaltyProgram)

		// get loyalty programs
		r.Get("/", loyaltyProgramHandler.GetLoyaltyPrograms)

		// get loyalty program
		r.Get("/{id}", loyaltyProgramHandler.GetLoyaltyProgram)

		// update loyalty program
		r.With(requireDev).Put("/{id}", loyaltyProgramHandler.UpdateLoyaltyProgram)

		// delete loyalty program
		r.With(requireDev).Delete("/{id}", loyaltyProgramHandler.DeleteLoyaltyProgram)
	})
}
