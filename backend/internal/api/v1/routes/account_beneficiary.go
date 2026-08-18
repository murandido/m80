package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterAccountBeneficiaryRoutes wires up /accounts/{id}/beneficiaries.
//
// Everything here is open to any authenticated user except the reconcile: correcting a name
// or attaching a CPF is the operator's daily work, while rebuilding the ledger throws away
// every correction anyone ever made, with no undo.
func RegisterAccountBeneficiaryRoutes(
	r chi.Router,
	db *gorm.DB,
	requireAdmin func(http.Handler) http.Handler,
) {
	beneficiaryHandler := handlers.NewAccountBeneficiaryHandler(db)

	r.Route("/{id}/beneficiaries", func(r chi.Router) {
		r.Get("/", beneficiaryHandler.GetAccountBeneficiaries)
		r.Post("/", beneficiaryHandler.CreateAccountBeneficiary)

		// Before /{beneficiaryID}, or chi would read "reconcile" as an id.
		r.With(requireAdmin).Post("/reconcile", beneficiaryHandler.ReconcileAccountBeneficiaries)

		r.Patch("/{beneficiaryID}", beneficiaryHandler.UpdateAccountBeneficiary)
		r.Delete("/{beneficiaryID}", beneficiaryHandler.DeleteAccountBeneficiary)
		r.Post("/{beneficiaryID}/merge", beneficiaryHandler.MergeAccountBeneficiaries)
	})
}
