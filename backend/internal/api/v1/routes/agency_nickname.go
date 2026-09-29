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

	requireDev := middleware.RequireDev()

	// Nickname availability check. It is global - it answers across every agency - and only
	// the dev-only agency form asks it, so it follows that form instead of leaking to a
	// restricted user that a nickname exists in an agency they cannot see.
	r.With(requireDev).Get("/nicknames/availability", agencyNicknameHandler.CheckAgencyNicknameAvailability)

	r.Route("/{id}/nicknames", func(r chi.Router) {
		// Writes follow the dev-only agencies screen they are edited from; reads stay
		// open to any role, narrowed by the handler to the caller's agency scope.
		r.With(requireDev).Post("/", agencyNicknameHandler.CreateAgencyNickname)
		r.Get("/", agencyNicknameHandler.GetAgencyNicknames)
		r.Get("/{nicknameID}", agencyNicknameHandler.GetAgencyNickname)
		r.With(requireDev).Put("/{nicknameID}", agencyNicknameHandler.UpdateAgencyNickname)
		r.With(requireDev).Delete("/{nicknameID}", agencyNicknameHandler.DeleteAgencyNickname)
	})
}
