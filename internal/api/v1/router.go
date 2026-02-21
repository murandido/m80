package v1

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()

	authHandler := handlers.NewAuthHandler(db)
	userHandler := handlers.NewUserHandler(db)

	// public routes
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", authHandler.Login)
	})

	// private routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth())

		// admin routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin())

			// CRUD routes
			r.Route("/users", func(r chi.Router) {
				// create user
				r.Post("/", userHandler.CreateUser)

				// get users
				r.Get("/", userHandler.GetUsers)

				// update user
				r.Put("/{id}", userHandler.UpdateUser)

				// delete user
				r.Delete("/{id}", userHandler.DeleteUser)
			})
		})
	})

	return r
}
