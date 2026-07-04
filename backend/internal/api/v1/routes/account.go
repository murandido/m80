package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAccountRoutes wires up the /accounts CRUD routes.
func RegisterAccountRoutes(r chi.Router, db *gorm.DB) {
	accountHandler := handlers.NewAccountHandler(db)

	r.Route("/accounts", func(r chi.Router) {
		// create account
		r.Post("/", accountHandler.CreateAccount)

		// get accounts
		r.Get("/", accountHandler.GetAccounts)

		// get account
		r.Get("/{id}", accountHandler.GetAccount)

		// update account
		r.Put("/{id}", accountHandler.UpdateAccount)

		// delete account
		r.Delete("/{id}", accountHandler.DeleteAccount)

		// nicknames sub-resource
		RegisterAccountNicknameRoutes(r, db)
	})
}
