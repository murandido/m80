package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterPointBatchRoutes wires up the /point-batches CRUD.
//
// Admin-only including the reads, for the reason spelled out in
// RegisterPointBatchOriginRoutes: a batch says what the points cost us.
func RegisterPointBatchRoutes(r chi.Router, db *gorm.DB) {
	batchHandler := handlers.NewPointBatchHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/point-batches", func(r chi.Router) {
		r.Use(requireAdmin)

		r.Post("/", batchHandler.CreatePointBatch)
		r.Get("/", batchHandler.GetPointBatches)
		r.Put("/{id}", batchHandler.UpdatePointBatch)
		r.Patch("/{id}/settlement", batchHandler.SettlePointBatch)
		r.Delete("/{id}", batchHandler.DeletePointBatch)
	})
}
