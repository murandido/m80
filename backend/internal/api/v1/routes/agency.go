package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAgencyRoutes wires up the /agencies CRUD routes. Managing agencies is an
// admin screen, so the writes require admin; reading stays open to every authenticated
// user because the agency combobox appears on the dashboard, on the issuances grid and
// on the payments screen, which plain operators use.
func RegisterAgencyRoutes(r chi.Router, db *gorm.DB) {
	agencyHandler := handlers.NewAgencyHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/agencies", func(r chi.Router) {
		// create agency
		r.With(requireAdmin).Post("/", agencyHandler.CreateAgency)

		// get agencies
		r.Get("/", agencyHandler.GetAgencies)

		// get agency
		r.Get("/{id}", agencyHandler.GetAgency)

		// update agency
		r.With(requireAdmin).Put("/{id}", agencyHandler.UpdateAgency)

		// delete agency
		r.With(requireAdmin).Delete("/{id}", agencyHandler.DeleteAgency)

		// nicknames sub-resource
		RegisterAgencyNicknameRoutes(r, db)
	})
}
