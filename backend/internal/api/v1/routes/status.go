package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/health"
)

// RegisterStatusRoutes mounts GET /status, the panel's dependency lights.
//
// Registered outside the RequireAdmin sub-group on purpose: this is the one
// piece of bot/vendor health that everyone gets to see. The admin-only surface
// (pairing, logout, the pairing code itself) stays where it was, in
// RegisterBotRoutes.
//
// It does not collide with the unauthenticated GET /health in
// internal/api/router.go - that one is the load balancer's, answers a plain
// string, and knows nothing about dependencies.
func RegisterStatusRoutes(r chi.Router, monitor *health.Monitor) {
	statusHandler := handlers.NewStatusHandler(monitor)

	r.Get("/status", statusHandler.GetSystemStatus)
}
