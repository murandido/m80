package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// RegisterLegacyImportRoutes mounts the temporary screen that imports the hand-kept
// "Controle de Emissões" spreadsheet. Admin-only: it writes issuances dated weeks in the
// past, on behalf of other people, in batches of dozens.
//
// Mounted as a static "/issuances/legacy-import" child, like RegisterListenedChatRoutes
// under "/bot" - chi checks static children before any catch-all, so this never collides
// with the "/issuances/{id}" routes whatever the registration order.
//
// Delete this file, its handler, and the call in router.go when the migration off the
// spreadsheets is finished. See internal/legacyimport for the rest of the list.
func RegisterLegacyImportRoutes(r chi.Router, db *gorm.DB, broker *events.Broker) {
	h := handlers.NewLegacyImportHandler(db, broker)

	r.Route("/issuances/legacy-import", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())

		r.Post("/preview", h.PreviewLegacyImport)
		r.Post("/commit", h.CommitLegacyImport)
	})
}
