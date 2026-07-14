package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAgencyNicknameRoutes wires up the /agencies/{id}/nicknames CRUD routes.
func RegisterAgencyNicknameRoutes(r chi.Router, db *gorm.DB) {
	agencyNicknameHandler := handlers.NewAgencyNicknameHandler(db)

	// nickname availability check (global, not scoped to a specific agency)
	r.Get("/nicknames/availability", agencyNicknameHandler.CheckAgencyNicknameAvailability)

	r.Route("/{id}/nicknames", func(r chi.Router) {
		r.Post("/", agencyNicknameHandler.CreateAgencyNickname)
		r.Get("/", agencyNicknameHandler.GetAgencyNicknames)
		r.Get("/{nicknameID}", agencyNicknameHandler.GetAgencyNickname)
		r.Put("/{nicknameID}", agencyNicknameHandler.UpdateAgencyNickname)
		r.Delete("/{nicknameID}", agencyNicknameHandler.DeleteAgencyNickname)
	})
}
