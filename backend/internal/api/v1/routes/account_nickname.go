package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAccountNicknameRoutes wires up the /accounts/{id}/nicknames CRUD routes.
func RegisterAccountNicknameRoutes(r chi.Router, db *gorm.DB) {
	accountNicknameHandler := handlers.NewAccountNicknameHandler(db)

	// nickname availability check (global, not scoped to a specific account)
	r.Get("/nicknames/availability", accountNicknameHandler.CheckAccountNicknameAvailability)

	r.Route("/{id}/nicknames", func(r chi.Router) {
		// create nickname
		r.Post("/", accountNicknameHandler.CreateAccountNickname)

		// get nicknames
		r.Get("/", accountNicknameHandler.GetAccountNicknames)

		// get nickname
		r.Get("/{nicknameID}", accountNicknameHandler.GetAccountNickname)

		// update nickname
		r.Put("/{nicknameID}", accountNicknameHandler.UpdateAccountNickname)

		// delete nickname
		r.Delete("/{nicknameID}", accountNicknameHandler.DeleteAccountNickname)
	})
}
