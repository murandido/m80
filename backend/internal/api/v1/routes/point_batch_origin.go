package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterPointBatchOriginRoutes wires up the /point-batch-origins CRUD.
//
// Every route is admin-gated, reads included - unlike agencies or airports, whose
// comboboxes appear on operator screens. Nothing outside the points inventory has any
// use for this catalogue, and the inventory itself is admin-only because it exposes what
// the points cost and how the profit is split.
func RegisterPointBatchOriginRoutes(r chi.Router, db *gorm.DB) {
	originHandler := handlers.NewPointBatchOriginHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/point-batch-origins", func(r chi.Router) {
		r.Use(requireAdmin)

		r.Post("/", originHandler.CreatePointBatchOrigin)
		r.Get("/", originHandler.GetPointBatchOrigins)
		r.Put("/{id}", originHandler.UpdatePointBatchOrigin)
		r.Delete("/{id}", originHandler.DeletePointBatchOrigin)
	})
}
