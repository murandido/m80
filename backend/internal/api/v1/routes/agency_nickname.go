package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAgencyNicknameRoutes wires up the /agencies/{id}/nicknames CRUD routes.
func RegisterAgencyNicknameRoutes(r chi.Router, db *gorm.DB) {
	agencyNicknameHandler := handlers.NewAgencyNicknameHandler(db)

	// nickname availability check (global, not scoped to a specific agency)
	r.Get("/nicknames/availability", agencyNicknameHandler.CheckAgencyNicknameAvailability)

	requireAdmin := middleware.RequireAdmin()

	r.Route("/{id}/nicknames", func(r chi.Router) {
		// Writes follow the agencies screen they are edited from; reads stay open
		// because the bot resolves an agency by nickname on behalf of any operator.
		r.With(requireAdmin).Post("/", agencyNicknameHandler.CreateAgencyNickname)
		r.Get("/", agencyNicknameHandler.GetAgencyNicknames)
		r.Get("/{nicknameID}", agencyNicknameHandler.GetAgencyNickname)
		r.With(requireAdmin).Put("/{nicknameID}", agencyNicknameHandler.UpdateAgencyNickname)
		r.With(requireAdmin).Delete("/{nicknameID}", agencyNicknameHandler.DeleteAgencyNickname)
	})
}
