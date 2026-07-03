package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterUserRoutes wires up the /users CRUD routes.
func RegisterUserRoutes(r chi.Router, db *gorm.DB) {
	userHandler := handlers.NewUserHandler(db)

	r.Route("/users", func(r chi.Router) {
		// create user
		r.Post("/", userHandler.CreateUser)

		// get users
		r.Get("/", userHandler.GetUsers)

		// get user
		r.Get("/{id}", userHandler.GetUser)

		// update user
		r.Put("/{id}", userHandler.UpdateUser)

		// delete user
		r.Delete("/{id}", userHandler.DeleteUser)
	})
}
