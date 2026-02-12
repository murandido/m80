package v1

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()

	authHandler := handlers.NewAuthHandler(db)

	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", authHandler.Login)
	})

	return r
}
