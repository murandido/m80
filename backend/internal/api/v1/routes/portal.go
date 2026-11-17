package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// RegisterPortalRoutes mounts the statement client's area: their granted accounts, each one's
// statement without the agency's links, the export, a new airline password and the Azul
// connect. The group is closed to staff (RequireRole lets dev through, as everywhere) and every
// {id} route checks the grant first.
//
// There is no Smiles connect here on purpose: its verification code can only be requested from
// a browser running the M80 extension (every automated client gets a 406 from Smiles' edge),
// and that extension is the team's, not something handed to customers. A client updates the
// password; the team runs the code step from the accounts screen.
//
// The connector is the process-wide one, so a client and an operator connecting the same Azul
// account see the same in-flight state.
func RegisterPortalRoutes(r chi.Router, db *gorm.DB, connector *azulsync.Connector) {
	statementHandler := handlers.NewStatementHandler(db)
	statementHandler.Tracker = connector.Tracker
	portal := handlers.NewPortalHandler(db, statementHandler)
	azulConnect := handlers.NewAzulConnectHandler(connector)

	r.Route("/portal", func(r chi.Router) {
		r.Use(middleware.RequireRole(data.RoleStatementClient))

		r.Get("/accounts", portal.ListPortalAccounts)

		r.Route("/accounts/{id}", func(r chi.Router) {
			r.Use(middleware.RequireLinkedAccount(db))

			r.Get("/entries", portal.PortalStatementEntries)
			r.Get("/status", portal.PortalStatementStatus)
			r.Get("/export", portal.PortalStatementExport)
			r.Put("/password", portal.UpdatePortalPassword)

			r.With(handlers.RequirePortalProgram("AZUL")).Post("/azul-connect", azulConnect.ConnectAzulAccount)
		})
	})
}
