package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterUserRoutes wires up the /users routes. Only the listing is reachable
// by any authenticated user (e.g. to populate the "emissor" combobox on other
// screens) - get-by-id and all write routes require admin, applied inline per
// route via r.With() rather than a second mount at the same "/users" path,
// which chi rejects.
func RegisterUserRoutes(r chi.Router, db *gorm.DB) {
	userHandler := handlers.NewUserHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/users", func(r chi.Router) {
		// get users
		r.Get("/", userHandler.GetUsers)

		// create user
		r.With(requireAdmin).Post("/", userHandler.CreateUser)

		// get user
		r.With(requireAdmin).Get("/{id}", userHandler.GetUser)

		// update user
		r.With(requireAdmin).Put("/{id}", userHandler.UpdateUser)

		// delete user
		r.With(requireAdmin).Delete("/{id}", userHandler.DeleteUser)
	})
}
