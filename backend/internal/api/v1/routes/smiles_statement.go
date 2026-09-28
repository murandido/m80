package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/smilessync"
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
func RegisterSmilesStatementRoutes(r chi.Router, db *gorm.DB, guard *smilessync.SessionGuard) {
	// The tracker is per-process and belongs to these routes alone: it holds the logins that
	// are waiting on a verification code, and nothing outside the connect flow has any use for
	// it.
	//
	// The guard comes from outside, and that is the point. It is what stops a connect's first
	// sync and a ticker round from renewing the same session at once - the tenant treats two
	// uses of one refresh token as a stolen token and revokes the lot, which costs the operator
	// a new code. A guard created here would protect these handlers from each other and leave
	// the ticker on the other side of the fence.
	tracker := smilessync.NewLoginTracker()

	h := handlers.NewStatementHandler(db)
	connect := handlers.NewSmilesConnectHandler(db, tracker, guard)

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
		r.Post("/challenge", connect.ChallengeSmilesLogin)
		r.Post("/code", connect.ConfirmSmilesLogin)
		r.Delete("/", connect.DisconnectSmilesAccount)
	})
}
