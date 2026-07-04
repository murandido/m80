package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterLoyaltyProgramRoutes wires up the /loyalty-programs CRUD routes.
func RegisterLoyaltyProgramRoutes(r chi.Router, db *gorm.DB) {
	loyaltyProgramHandler := handlers.NewLoyaltyProgramHandler(db)

	r.Route("/loyalty-programs", func(r chi.Router) {
		// create loyalty program
		r.Post("/", loyaltyProgramHandler.CreateLoyaltyProgram)

		// get loyalty programs
		r.Get("/", loyaltyProgramHandler.GetLoyaltyPrograms)

		// get loyalty program
		r.Get("/{id}", loyaltyProgramHandler.GetLoyaltyProgram)

		// update loyalty program
		r.Put("/{id}", loyaltyProgramHandler.UpdateLoyaltyProgram)

		// delete loyalty program
		r.Delete("/{id}", loyaltyProgramHandler.DeleteLoyaltyProgram)
	})
}
