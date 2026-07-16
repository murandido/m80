package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterObservationPresetRoutes wires up the /observation-presets CRUD routes.
func RegisterObservationPresetRoutes(r chi.Router, db *gorm.DB) {
	observationPresetHandler := handlers.NewObservationPresetHandler(db)

	r.Route("/observation-presets", func(r chi.Router) {
		// create observation preset
		r.Post("/", observationPresetHandler.CreateObservationPreset)

		// get observation presets
		r.Get("/", observationPresetHandler.GetObservationPresets)

		// get observation preset
		r.Get("/{id}", observationPresetHandler.GetObservationPreset)

		// update observation preset
		r.Put("/{id}", observationPresetHandler.UpdateObservationPreset)

		// delete observation preset
		r.Delete("/{id}", observationPresetHandler.DeleteObservationPreset)
	})
}
