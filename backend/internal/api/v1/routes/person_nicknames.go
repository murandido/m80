package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterPersonNicknameRoutes wires up the /people/{id}/nicknames CRUD routes.
func RegisterPersonNicknameRoutes(r chi.Router, db *gorm.DB) {
	personNicknameHandler := handlers.NewPersonNicknameHandler(db)

	r.Route("/{id}/nicknames", func(r chi.Router) {
		// create nickname
		r.Post("/", personNicknameHandler.CreatePersonNickname)

		// get nicknames
		r.Get("/", personNicknameHandler.GetPersonNicknames)

		// get nickname
		r.Get("/{nicknameID}", personNicknameHandler.GetPersonNickname)

		// update nickname
		r.Put("/{nicknameID}", personNicknameHandler.UpdatePersonNickname)

		// delete nickname
		r.Delete("/{nicknameID}", personNicknameHandler.DeletePersonNickname)
	})
}
