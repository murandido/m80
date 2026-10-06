package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"gorm.io/gorm"
)

// RegisterAccountRoutes wires up the /accounts CRUD routes. azulConnector may be nil (the route
// tests have none); saving a password then just never starts an Azul login.
func RegisterAccountRoutes(r chi.Router, db *gorm.DB, azulConnector *azulsync.Connector) {
	accountHandler := handlers.NewAccountHandler(db)
	if azulConnector != nil {
		// Assigned only when present: a nil *Connector in the interface field would read as set.
		accountHandler.AzulConnector = azulConnector
	}
	requireAdmin := middleware.RequireAdmin()

	r.Route("/accounts", func(r chi.Router) {
		// create account
		r.Post("/", accountHandler.CreateAccount)

		// get accounts — the whole list, for the pickers on the issuances grid,
		// the sales screens and the importer
		r.Get("/", accountHandler.GetAccounts)

		// the management screen: one row per holder with all of their accounts. Chi resolves
		// a static segment ahead of a param, so this never gets read as /accounts/{id}.
		r.Get("/holders", accountHandler.ListAccountHolders)

		// get account
		r.Get("/{id}", accountHandler.GetAccount)

		// update account
		r.Put("/{id}", accountHandler.UpdateAccount)

		// delete account
		r.Delete("/{id}", accountHandler.DeleteAccount)

		// nicknames sub-resource
		RegisterAccountNicknameRoutes(r, db)

		// beneficiaries sub-resource: the manual half of the CPF control
		RegisterAccountBeneficiaryRoutes(r, db, requireAdmin)
	})
}
