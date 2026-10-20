package api

import (
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	_ "github.com/murandido/m-80-backend/docs"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	v1 "github.com/murandido/m-80-backend/internal/api/v1"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/health"
	"github.com/murandido/m-80-backend/internal/latamsync"
	"github.com/murandido/m-80-backend/internal/mcp"
	"github.com/murandido/m-80-backend/internal/smilessync"
	httpSwagger "github.com/swaggo/http-swagger"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, broker *events.Broker, bookings *booking.Registry, monitor *health.Monitor, smilesGuard *smilessync.SessionGuard, latamGuard *latamsync.SessionGuard, azulConnector *azulsync.Connector) *chi.Mux {
	r := chi.NewRouter()

	// Before Logger and the rate limiter, both of which read r.RemoteAddr and
	// would otherwise see the Caddy container on every request in production.
	r.Use(middleware.RealIP())

	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	r.Use(middleware.CORS())
	r.Use(middleware.RateLimit())

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("API On 🚀"))
		if err != nil {
			//TODO: audit this error
			fmt.Println("Error sending response: ", err)
			return
		}
	})

	// Swagger Route
	//
	// Registered before any authentication, so in production it would hand the
	// whole API surface - every route, every DTO - to anyone who asks. The JSON
	// URL is also hardcoded to localhost, so it would not even work published.
	if os.Getenv("ENV") != "production" {
		r.Get("/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("http://localhost:8080/swagger/doc.json"),
		))
	}

	r.Mount("/api/v1", v1.NewRouter(db, broker, bookings, monitor, smilesGuard, latamGuard, azulConnector))

	// MCP server for Claude, authenticated by its own personal tokens rather than the login
	// JWT. It is handed this same router: every tool calls back into /api/v1 through all of the
	// middleware above, so it can never see more than its owner sees in the panel. Like the
	// rest of the API it has no public address; it is reached only through the panel's
	// /api/mcp relay.
	// The three verbs of the Streamable HTTP transport, not r.Handle: a catch-all would also
	// register CONNECT and TRACE, which nothing here answers.
	mcpHandler := mcp.NewHandler(db, r)
	r.Post("/mcp", mcpHandler.ServeHTTP)
	r.Get("/mcp", mcpHandler.ServeHTTP)
	r.Delete("/mcp", mcpHandler.ServeHTTP)

	return r
}
