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
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/health"
	"github.com/murandido/m-80-backend/internal/traveos"
	httpSwagger "github.com/swaggo/http-swagger"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, broker *events.Broker, traveosClient *traveos.Client, monitor *health.Monitor) *chi.Mux {
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

	r.Mount("/api/v1", v1.NewRouter(db, broker, traveosClient, monitor))

	return r
}
