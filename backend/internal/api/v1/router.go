package v1

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/api/v1/routes"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()

	authHandler := handlers.NewAuthHandler(db)

	r.Route("/auth", func(r chi.Router) {
		// public routes
		r.Post("/login", authHandler.Login)

		// private routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(db))

			r.Get("/me", authHandler.Me)
		})
	})

	// private routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(db))

		// any authenticated user
		routes.RegisterPeopleRoutes(r, db)
		routes.RegisterAccountRoutes(r, db)
		routes.RegisterAirlineRoutes(r, db)
		routes.RegisterLoyaltyProgramRoutes(r, db)

		// admin routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin())

			routes.RegisterUserRoutes(r, db)
		})
	})

	return r
}
