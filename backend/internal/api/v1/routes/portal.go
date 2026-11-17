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
// statement without the agency's links, the export, a new airline password and the connects.
// The group is closed to staff (RequireRole lets dev through, as everywhere) and every {id}
// route checks the grant first.
//
// The connects are the staff ones, mounted again under the grant check: the handlers only load
// the account by id, so RequireLinkedAccount in front of them is the whole difference. The
// Smiles verification code can only be requested from a real browser (Smiles' edge answers 406
// to every automated client), so the client runs it through the M80 Extrato extension, the
// trimmed build of the team's one that carries no vault. Disconnecting is not offered: a client
// who wants to stop the sync asks the team, who also stops the grant.
//
// The Azul connector and the Smiles connect handler are the process-wide ones, so a client and
// an operator connecting the same account see the same in-flight state.
func RegisterPortalRoutes(r chi.Router, db *gorm.DB, connector *azulsync.Connector, smilesConnect *handlers.SmilesConnectHandler) {
	statementHandler := handlers.NewStatementHandler(db)
	statementHandler.Tracker = connector.Tracker
	portal := handlers.NewPortalHandler(db, statementHandler, connector)
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

			r.Group(func(r chi.Router) {
				r.Use(handlers.RequirePortalProgram("AZUL"))
				r.Post("/azul-connect", azulConnect.ConnectAzulAccount)
				r.Post("/azul-customer-key", azulConnect.StoreAzulBrowserKey)
			})

			r.Route("/smiles-connect", func(r chi.Router) {
				r.Use(handlers.RequirePortalProgram("SMILES"))
				r.Post("/", smilesConnect.ConnectSmilesAccount)
				r.Post("/challenge/browser", smilesConnect.BrowserChallengeSmilesLogin)
				r.Post("/challenge/complete", smilesConnect.CompleteBrowserChallengeSmilesLogin)
				r.Post("/code", smilesConnect.ConfirmSmilesLogin)
			})
		})
	})
}
