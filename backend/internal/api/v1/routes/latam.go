package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/latam"
	"github.com/murandido/m-80-backend/internal/latamsync"
	"gorm.io/gorm"
)

// RegisterLatamRoutes mounts linking a LATAM account for its balance: start a login, receive its
// answer, disconnect, and the badge. Admin-only like the Azul and Smiles connects - they log into
// a real airline account.
//
// The guard comes from outside, shared with the ticker in cmd/api, so a connect's first read and a
// ticker round never renew one session at once. The tracker and the client belong to these routes
// alone; the client goes direct, with no egress proxy (see internal/latam/account.go).
func RegisterLatamRoutes(r chi.Router, db *gorm.DB, guard *latamsync.SessionGuard) {
	connect := handlers.NewLatamConnectHandler(db, latam.NewAccountClient(), latamsync.NewLoginTracker(), guard)

	r.Route("/accounts/{id}/latam-connect", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())
		r.Post("/", connect.StartLatamLogin)
		r.Delete("/", connect.DisconnectLatamAccount)
	})
	r.With(middleware.RequireAdmin()).Get("/accounts/{id}/latam-status", connect.LatamConnectStatus)
	r.With(middleware.RequireAdmin()).Post("/latam-connect/callback", connect.FinishLatamLogin)
}
