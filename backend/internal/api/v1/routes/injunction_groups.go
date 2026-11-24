package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/onedrivesync"
	"gorm.io/gorm"
)

// RegisterInjunctionGroupRoutes mounts the Tropas. Reading is admin, like the rest of the
// points inventory; changing the setup is dev, because it decides which workbook the API
// reads and which rows of it turn into issuances.
func RegisterInjunctionGroupRoutes(r chi.Router, db *gorm.DB, workbooks *onedrivesync.Service, broker *events.Broker) {
	h := handlers.NewInjunctionGroupHandler(db, workbooks, broker)
	requireDev := middleware.RequireDev()

	r.Route("/injunction-groups", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())

		r.Get("/", h.GetInjunctionGroups)
		r.With(requireDev).Post("/", h.CreateInjunctionGroup)

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetInjunctionGroup)
			r.With(requireDev).Put("/", h.UpdateInjunctionGroup)
			r.With(requireDev).Delete("/", h.DeleteInjunctionGroup)

			r.Get("/check", h.CheckInjunctionGroup)
			r.Post("/import", h.ImportInjunctionGroupDay)
			// The backfill writes hundreds of issuances at once: dev only.
			r.With(requireDev).Post("/import-period", h.ImportInjunctionGroupPeriod)
		})
	})
}
