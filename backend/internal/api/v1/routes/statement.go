package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/azulsync"
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

	// Smiles still uploads a file - the /point-batches/statement path is now Smiles-only.
	r.Route("/point-batches/statement", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())

		r.Post("/preview", h.PreviewStatement)
		r.Post("/commit", h.CommitStatement)
	})
}

// RegisterAzulStatementRoutes mounts the Azul reconciliation, which reads the synced window
// from the database instead of an upload, plus the one-time connect action that discovers the
// account's customerKey. Account-scoped because the account is what selects the window;
// admin-only like the rest of the inventory.
func RegisterAzulStatementRoutes(r chi.Router, db *gorm.DB) {
	// One tracker per process, shared by both handlers: the connect handler marks an account in and
	// out while its background login runs, and the statement handler reads it into the "conectando"
	// badge. See azulsync.ConnectTracker.
	tracker := azulsync.NewConnectTracker()

	h := handlers.NewStatementHandler(db)
	h.Tracker = tracker
	connect := handlers.NewAzulConnectHandler(db, tracker)

	r.Route("/accounts/{id}/azul-statement", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())

		r.Get("/", h.PreviewAzulStatement)
		r.Post("/commit", h.CommitAzulStatement)
	})

	r.With(middleware.RequireAdmin()).Post("/accounts/{id}/azul-connect", connect.ConnectAzulAccount)
}
