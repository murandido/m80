package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterMCPRoutes mounts the panel side of the MCP server, dev-only end to end. These routes
// sit behind the ordinary session JWT: an MCP token is accepted only by /mcp itself, so the
// agent can never mint tokens for itself.
func RegisterMCPRoutes(r chi.Router, db *gorm.DB) {
	tokenHandler := handlers.NewMCPTokenHandler(db)

	r.Route("/mcp-tokens", func(r chi.Router) {
		r.Use(middleware.RequireDev())

		r.Get("/", tokenHandler.GetMCPTokens)
		r.Post("/", tokenHandler.CreateMCPToken)
		r.Delete("/{id}", tokenHandler.RevokeMCPToken)
	})
}
