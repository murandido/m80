package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAgencyRoutes wires up the /agencies CRUD routes.
func RegisterAgencyRoutes(r chi.Router, db *gorm.DB) {
	agencyHandler := handlers.NewAgencyHandler(db)

	r.Route("/agencies", func(r chi.Router) {
		// create agency
		r.Post("/", agencyHandler.CreateAgency)

		// get agencies
		r.Get("/", agencyHandler.GetAgencies)

		// get agency
		r.Get("/{id}", agencyHandler.GetAgency)

		// update agency
		r.Put("/{id}", agencyHandler.UpdateAgency)

		// delete agency
		r.Delete("/{id}", agencyHandler.DeleteAgency)

		// nicknames sub-resource
		RegisterAgencyNicknameRoutes(r, db)
	})
}
