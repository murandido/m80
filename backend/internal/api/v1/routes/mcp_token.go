package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterMCPTokenRoutes wires up /mcp-tokens. Dev-only: an MCP token reads everything its
// owner can, from outside the panel, and for now the MCP server is a tool for the people who
// build the system, not for the staff using it.
func RegisterMCPTokenRoutes(r chi.Router, db *gorm.DB) {
	handler := handlers.NewMCPTokenHandler(db)

	r.Route("/mcp-tokens", func(r chi.Router) {
		r.Use(middleware.RequireDev())

		r.Get("/", handler.GetMCPTokens)
		r.Post("/", handler.CreateMCPToken)
		r.Delete("/{id}", handler.RevokeMCPToken)
	})
}
