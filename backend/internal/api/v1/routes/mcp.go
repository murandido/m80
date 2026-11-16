package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterMCPRoutes mounts the panel side of the MCP server, dev-only end to end. These routes
// sit behind the ordinary session JWT: an MCP token is accepted only by /mcp itself, so the
// agent can never mint tokens for itself or approve its own proposals.
//
// v1 is the whole /api/v1 router: an approved proposal runs through it again as the approver.
func RegisterMCPRoutes(r chi.Router, db *gorm.DB, v1 http.Handler) {
	tokenHandler := handlers.NewMCPTokenHandler(db)
	proposalHandler := handlers.NewMCPProposalHandler(db, v1)
	requireDev := middleware.RequireDev()

	r.Route("/mcp-tokens", func(r chi.Router) {
		r.Use(requireDev)

		r.Get("/", tokenHandler.GetMCPTokens)
		r.Post("/", tokenHandler.CreateMCPToken)
		r.Delete("/{id}", tokenHandler.RevokeMCPToken)
	})

	r.Route("/mcp-proposals", func(r chi.Router) {
		r.Use(requireDev)

		r.Get("/", proposalHandler.GetMCPProposals)
		r.Post("/{id}/approve", proposalHandler.ApproveMCPProposal)
		r.Post("/{id}/reject", proposalHandler.RejectMCPProposal)
	})

	r.With(requireDev).Get("/mcp-calls", proposalHandler.GetMCPCalls)
}
