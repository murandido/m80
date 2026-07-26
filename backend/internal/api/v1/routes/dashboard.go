package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterDashboardRoutes wires up the /dashboard route. Deliberately not admin-gated: the
// volume figures in miles are every operator's own work, and they land on this page. Only
// the monetary side of the payload is cut, inside the handler, from the role the auth
// middleware already resolved - which is why there is no RequireAdmin here.
func RegisterDashboardRoutes(r chi.Router, db *gorm.DB) {
	dashboardHandler := handlers.NewDashboardHandler(db)

	r.Route("/dashboard", func(r chi.Router) {
		// aggregated metrics for the dashboard home
		r.Get("/", dashboardHandler.GetDashboard)
	})
}
