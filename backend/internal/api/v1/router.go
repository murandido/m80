package v1

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/api/v1/routes"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, broker *events.Broker) chi.Router {
	r := chi.NewRouter()

	authHandler := handlers.NewAuthHandler(db)

	r.Route("/auth", func(r chi.Router) {
		// public routes
		r.Post("/login", authHandler.Login)

		// private routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(db))

			r.Get("/me", authHandler.Me)
		})
	})

	// private routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(db))

		// any authenticated user
		routes.RegisterPeopleRoutes(r, db)
		routes.RegisterAccountRoutes(r, db)
		routes.RegisterAirlineRoutes(r, db)
		routes.RegisterLoyaltyProgramRoutes(r, db)
		routes.RegisterObservationPresetRoutes(r, db)
		routes.RegisterAgencyRoutes(r, db)
		routes.RegisterClientRoutes(r, db)
		routes.RegisterIssuanceRoutes(r, db, broker)
		routes.RegisterSaleRoutes(r, db, broker)
		routes.RegisterDashboardRoutes(r, db)
		routes.RegisterAirportRoutes(r, db)
		// SSE stream, behind the same auth as everything else in this group
		routes.RegisterEventRoutes(r, broker)
		// GET /users is open to any authenticated user; its write routes are
		// admin-gated inline within RegisterUserRoutes itself.
		routes.RegisterUserRoutes(r, db)
		// Pure admin-only proxy to the bot's internal API - the gate lives
		// inside RegisterBotRoutes itself, same as the suppliers view above.
		routes.RegisterBotRoutes(r)
		// Admin-only chat approval flow for !registrar-chat - a static
		// "/bot/chats" child, never shadowed by RegisterBotRoutes' "/bot/*"
		// catch-all proxy above (see RegisterListenedChatRoutes doc comment).
		routes.RegisterListenedChatRoutes(r, db)
	})

	return r
}
