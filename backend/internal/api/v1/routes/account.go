package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAccountRoutes wires up the /accounts CRUD routes.
func RegisterAccountRoutes(r chi.Router, db *gorm.DB) {
	accountHandler := handlers.NewAccountHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/accounts", func(r chi.Router) {
		// create account
		r.Post("/", accountHandler.CreateAccount)

		// get accounts — the whole list, for the pickers on the issuances grid,
		// the sales screens and the importer
		r.Get("/", accountHandler.GetAccounts)

		// paginated listing for the management screen. Chi resolves a static
		// segment ahead of a param, so this never gets read as /accounts/{id}.
		r.Get("/manage", accountHandler.ListAccounts)

		// get account
		r.Get("/{id}", accountHandler.GetAccount)

		// update account
		r.Put("/{id}", accountHandler.UpdateAccount)

		// delete account
		r.Delete("/{id}", accountHandler.DeleteAccount)

		// merge two accounts into one. Admin-only: it moves issuance history from one
		// account to another and then removes a record for good, with no undo.
		r.With(requireAdmin).Get("/{id}/merge-preview", accountHandler.PreviewMergeAccount)
		r.With(requireAdmin).Post("/{id}/merge", accountHandler.MergeAccount)

		// nicknames sub-resource
		RegisterAccountNicknameRoutes(r, db)

		// beneficiaries sub-resource: the manual half of the CPF control
		RegisterAccountBeneficiaryRoutes(r, db, requireAdmin)
	})
}
