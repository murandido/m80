package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()

	authHandler := handlers.NewAuthHandler(db)

	// public routes
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", authHandler.Login)
	})

	// private routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth())

		// endpoint just to test
		// TODO: remove this endpoint
		r.Get("/me", func(w http.ResponseWriter, r *http.Request) {
			utils.SendJSON(w, r, http.StatusOK, "You are logged in and accessed a private route!", nil)
		})

		// admin routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin())

			// endpoint just to test
			// TODO: remove this endpoint
			r.Post("/adm", func(w http.ResponseWriter, r *http.Request) {
				utils.SendJSON(w, r, http.StatusOK, "You are admin!", nil)
			})
		})
	})

	return r
}
