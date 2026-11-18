package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterSmilesStatementRoutes mounts the Smiles reconciliation, which reads the synced window
// from the database, plus the three-step connect that trades a verification code for a session.
// Account-scoped because the account is what selects the window; admin-only like the rest of the
// inventory.
//
// It is the twin of RegisterAzulStatementRoutes, and the two share the link handlers: pinning a
// statement line to an issuance or to a batch is the same operation in both programmes, keyed by
// account and line, so there is one implementation and one pair of tables behind both paths.
func RegisterSmilesStatementRoutes(r chi.Router, db *gorm.DB, connect *handlers.SmilesConnectHandler) {
	// The connect handler comes from outside because it is shared with the statement client's
	// portal (RegisterPortalRoutes): its tracker holds the logins waiting on a verification code,
	// and an operator and a client reconnecting the same account must see the same pending login,
	// not two that each expect a different code. Its guard is the ticker's, for the reason given
	// where it is built (v1.NewRouter).
	h := handlers.NewStatementHandler(db)

	r.Route("/accounts/{id}/smiles-statement", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())

		r.Get("/", h.PreviewSmilesStatement)
		r.Post("/commit", h.CommitSmilesStatement)

		// The cheap sync-state badge on its own, for the account card to poll after a connect
		// (the entries endpoint below is far heavier - it resolves every link).
		r.Get("/status", h.SmilesConnectStatus)

		// The per-account statement page: the synced lines with each redemption resolved to its
		// issuance, plus the manual-link override (edit mode) and a CSV export.
		r.Get("/entries", h.SmilesStatementEntries)
		r.Get("/export", h.ExportSmilesStatement)
		r.Post("/links", h.LinkStatementBooking)
		r.Delete("/links", h.UnlinkStatementBooking)
		// The miles-in side: purchases and accruals link to a batch (lote).
		r.Post("/batch-links", h.LinkStatementBatch)
		r.Delete("/batch-links", h.UnlinkStatementBatch)
	})

	// The connect itself is a conversation, so it is three calls rather than one: log in, ask
	// for the code on a chosen channel, and hand back the digits. Mounted as a static child of
	// /accounts, which chi checks before any catch-all.
	r.Route("/accounts/{id}/smiles-connect", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())

		r.Post("/", connect.ConnectSmilesAccount)
		// The browser path: the extension runs the one POST the edge refuses to HTTP clients.
		r.Post("/challenge/browser", connect.BrowserChallengeSmilesLogin)
		r.Post("/challenge/complete", connect.CompleteBrowserChallengeSmilesLogin)
		r.Post("/code", connect.ConfirmSmilesLogin)
		r.Delete("/", connect.DisconnectSmilesAccount)
	})
}
