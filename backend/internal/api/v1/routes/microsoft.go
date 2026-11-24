package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/onedrivesync"
)

// RegisterMicrosoftRoutes mounts the Microsoft connection, dev-only: connecting consents,
// for the whole installation, to the API reading every file shared with that account.
func RegisterMicrosoftRoutes(r chi.Router, workbooks *onedrivesync.Service) {
	h := handlers.NewMicrosoftConnectionHandler(workbooks)

	r.Route("/microsoft-connection", func(r chi.Router) {
		r.Use(middleware.RequireDev())

		r.Get("/", h.GetMicrosoftConnection)
		r.Delete("/", h.DeleteMicrosoftConnection)
		r.Post("/start", h.StartMicrosoftConnection)
		r.Post("/callback", h.FinishMicrosoftConnection)
	})
}
