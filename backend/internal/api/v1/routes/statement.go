package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterStatementRoutes mounts the screen that reconciles an airline statement against
// the panel.
//
// Mounted as a static "/point-batches/statement" child, the same shape the legacy importer
// uses under "/issuances": chi checks static children before any catch-all, so this never
// collides with "/point-batches/{id}" whatever the registration order.
//
// Admin-only like the rest of the inventory - it reads what points cost and writes batches.
//
// Temporary in the same sense the importer is: it exists because neither programme has an
// API. When one does, the parsers go and the reconciliation stays, fed from the API instead
// of from a file.
func RegisterStatementRoutes(r chi.Router, db *gorm.DB) {
	h := handlers.NewStatementHandler(db)

	r.Route("/point-batches/statement", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())

		r.Post("/preview", h.PreviewStatement)
		r.Post("/commit", h.CommitStatement)
	})
}
