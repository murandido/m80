package v1

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/carriers/onedrive"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/api/v1/routes"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/health"
	"github.com/murandido/m-80-backend/internal/latamsync"
	"github.com/murandido/m-80-backend/internal/onedrivesync"
	"github.com/murandido/m-80-backend/internal/smilessync"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, broker *events.Broker, bookings *booking.Registry, monitor *health.Monitor, smilesGuard *smilessync.SessionGuard, latamGuard *latamsync.SessionGuard, azulConnector *azulsync.Connector) chi.Router {
	r := chi.NewRouter()
	// The groups below shadow r; approved MCP proposals need the whole router.
	root := r

	// One per process: the Microsoft token cache and refresh lock only hold if every route
	// shares them. A missing app registration leaves it unconfigured, not the API down.
	var graph onedrivesync.Graph
	if client, err := onedrive.New(); err == nil {
		graph = client
	} else {
		slog.Warn("microsoft connection disabled", "err", err)
	}
	workbooks := onedrivesync.New(db, graph)

	authHandler := handlers.NewAuthHandler(db)

	// One Smiles connect for the staff screens and the client portal. Its tracker holds the
	// logins waiting on a verification code, so both sides must share it: two trackers would let
	// an operator and a client start competing logins for one account. The guard comes from
	// cmd/api because it is what keeps a connect's first sync and a ticker round from renewing
	// the same session at once - the tenant treats two uses of one refresh token as theft and
	// revokes the lot.
	smilesConnect := handlers.NewSmilesConnectHandler(db, smilessync.NewLoginTracker(), smilesGuard)

	r.Route("/auth", func(r chi.Router) {
		// public routes - login carries its own, much stricter per-IP limit on top
		// of the router-wide one, since it is the only route where an unauthenticated
		// caller can guess at something (see middleware.LoginRateLimit).
		r.With(middleware.LoginRateLimit()).Post("/login", authHandler.Login)

		// private routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(db))

			r.Get("/me", authHandler.Me)
			r.Put("/me", authHandler.UpdateMe)
			r.Post("/refresh", authHandler.Refresh)
		})
	})

	// private routes - staff only. RequireStaff is the default deny for any role that is
	// not part of the team (statement_client today): a route added to this group later is
	// closed to outsiders without anyone having to remember it.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(db))
		r.Use(middleware.RequireStaff())

		// any staff user
		routes.RegisterPeopleRoutes(r, db)
		routes.RegisterAccountRoutes(r, db, azulConnector)
		routes.RegisterAirlineRoutes(r, db)
		routes.RegisterLoyaltyProgramRoutes(r, db)
		routes.RegisterObservationPresetRoutes(r, db)
		routes.RegisterAgencyRoutes(r, db)
		routes.RegisterClientRoutes(r, db)
		routes.RegisterIssuanceRoutes(r, db, broker, bookings)
		routes.RegisterSaleRoutes(r, db, broker)
		routes.RegisterDashboardRoutes(r, db)
		routes.RegisterAirportRoutes(r, db)
		routes.RegisterLoyaltyDocumentRoutes(r, db)
		routes.RegisterPromoRecipeRoutes(r, db)
		routes.RegisterPromoEnrollmentRoutes(r, db)
		// Points inventory: admin-only end to end, gated inside the register funcs.
		// It exposes what points cost us and how the profit is split, which is not
		// something the issuances grid may ever surface.
		routes.RegisterPointBatchOriginRoutes(r, db)
		routes.RegisterPointBatchRoutes(r, db)
		routes.RegisterAzulStatementRoutes(r, db, azulConnector)
		routes.RegisterSmilesStatementRoutes(r, db, smilesConnect)
		routes.RegisterLatamRoutes(r, db, latamGuard)
		routes.RegisterInventoryPeriodRoutes(r, db)
		// Reads passengers out of pasted text or document photos for the browser extension.
		routes.RegisterPassengerRoutes(r)
		// SSE stream, behind the same auth as everything else in this group
		routes.RegisterEventRoutes(r, broker)
		// Dependency lights (bot + Traveos) for everyone, not just admins -
		// the operator on the issuances grid is who needs them.
		routes.RegisterStatusRoutes(r, monitor)
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
		// Temporary: how each airline's own integration fares against the Traveos fallback,
		// dev-gated inside. Delete with the rest of migration 66 once Traveos is gone.
		routes.RegisterBookingLookupRoutes(r, db, bookings)
		// Temporary: the screen that imports the pre-panel spreadsheet, admin-gated
		// inside RegisterLegacyImportRoutes. Delete this line along with the rest of
		// the importer once the migration off the spreadsheets is done.
		routes.RegisterLegacyImportRoutes(r, db, broker)
		// The panel side of the MCP server (tokens, activity, proposals), dev-gated inside.
		// /mcp itself is mounted by internal/api, outside this group: its credential is an
		// MCP token, which these routes never accept.
		routes.RegisterMCPRoutes(r, db, root)
		// The Microsoft login the Tropas' workbooks are read with, dev-gated inside.
		routes.RegisterMicrosoftRoutes(r, workbooks)
	})

	// The statement client's own area, outside the staff group above: the only routes that role
	// reaches besides /auth/me. Gated inside RegisterPortalRoutes.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(db))
		routes.RegisterPortalRoutes(r, db, azulConnector, smilesConnect)
	})

	return r
}
