package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAgencyRoutes wires up the /agencies CRUD routes. The writes are dev-only: devs
// decide which agencies each account may see (user_agencies), and an admin able to create
// an agency - or to flip is_default - would be deciding part of that for them. Reading stays
// open to every authenticated user because the agency combobox appears on the dashboard, the
// issuances grid and the payments screen; the handlers narrow it to the caller's scope.
func RegisterAgencyRoutes(r chi.Router, db *gorm.DB) {
	agencyHandler := handlers.NewAgencyHandler(db)
	requireDev := middleware.RequireDev()

	r.Route("/agencies", func(r chi.Router) {
		// create agency
		r.With(requireDev).Post("/", agencyHandler.CreateAgency)

		// get agencies
		r.Get("/", agencyHandler.GetAgencies)

		// get agency
		r.Get("/{id}", agencyHandler.GetAgency)

		// update agency
		r.With(requireDev).Put("/{id}", agencyHandler.UpdateAgency)

		// delete agency
		r.With(requireDev).Delete("/{id}", agencyHandler.DeleteAgency)

		// nicknames sub-resource
		RegisterAgencyNicknameRoutes(r, db)
	})
}
