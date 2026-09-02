package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterPromoRecipeRoutes wires up /promo-recipes.
//
// Dev-only, all of it. A recipe is a set of CSS selectors and regular expressions, and
// getting one wrong points the bot at the wrong form. Keeping the screen in the panel
// is what removes the deploy from the loop - which was the goal - without also handing
// selectors to whoever happens to be on shift.
func RegisterPromoRecipeRoutes(r chi.Router, db *gorm.DB) {
	handler := handlers.NewPromoRecipeHandler(db)
	requireDev := middleware.RequireDev()

	r.Route("/promo-recipes", func(r chi.Router) {
		r.Use(requireDev)

		// The dry run. Static child of the same node as /{id}, and chi resolves static
		// segments first, so "test" is never read as an id.
		r.Post("/test", handler.TestPromoRecipe)

		r.Post("/", handler.CreatePromoRecipe)
		r.Get("/", handler.GetPromoRecipes)
		r.Get("/{id}", handler.GetPromoRecipe)
		r.Put("/{id}", handler.UpdatePromoRecipe)
		r.Delete("/{id}", handler.DeletePromoRecipe)
	})
}

// RegisterLoyaltyDocumentRoutes wires up /loyalty-documents.
//
// Admin rather than dev: which CPFs are in rotation is an operational decision, made by
// whoever manages the agency's accounts, and it needs no technical knowledge.
func RegisterLoyaltyDocumentRoutes(r chi.Router, db *gorm.DB) {
	handler := handlers.NewLoyaltyDocumentHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/loyalty-documents", func(r chi.Router) {
		r.Use(requireAdmin)

		r.Post("/", handler.CreateLoyaltyDocument)
		r.Get("/", handler.GetLoyaltyDocuments)

		// paginated listing for the management screen. Chi resolves a static
		// segment ahead of a param, so this never gets read as /loyalty-documents/{id}.
		r.Get("/manage", handler.ListLoyaltyDocuments)
		r.Put("/{id}", handler.UpdateLoyaltyDocument)
		r.Delete("/{id}", handler.DeleteLoyaltyDocument)
	})
}

// RegisterPromoEnrollmentRoutes wires up /promo-enrollments. Read-only: the history is
// written by the enrolment run and by nothing else, which is what makes it trustworthy
// as the answer to "who is already in".
func RegisterPromoEnrollmentRoutes(r chi.Router, db *gorm.DB) {
	handler := handlers.NewPromoEnrollmentHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/promo-enrollments", func(r chi.Router) {
		r.Use(requireAdmin)

		r.Get("/", handler.GetPromoEnrollments)
	})
}
