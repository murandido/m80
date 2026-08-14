package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// RegisterSaleRoutes wires up the /sales routes. The clients view is open to any
// authenticated user; the suppliers view is admin-only - what the agency owes each account
// administrator is not an operator's business. The broker is passed down so the update
// route can announce itself on the SSE stream.
func RegisterSaleRoutes(r chi.Router, db *gorm.DB, broker *events.Broker) {
	saleHandler := handlers.NewSaleHandler(db, broker)

	r.Route("/sales", func(r chi.Router) {
		// get sales (Visão Clientes)
		r.Get("/", saleHandler.GetSales)

		// export filtered sales as an .xlsx spreadsheet
		r.Get("/export", saleHandler.GetSalesExport)

		// aggregated balance per account (Visão Fornecedores)
		r.With(middleware.RequireAdmin()).Get("/suppliers", saleHandler.GetSupplierBalances)

		// set the payment terms on a set of issuances. Admin-only for the same
		// reason the balances are: what the agency pays a supplier is not operator data.
		r.With(middleware.RequireAdmin()).Post("/suppliers/issuances", saleHandler.UpdateSupplierIssuances)

		// the payment summary sent to the account's administrator
		r.With(middleware.RequireAdmin()).Get("/suppliers/report", saleHandler.GetSupplierReportPDF)

		// link a client (and, for a panel that has not been published yet, the old
		// paid switch - translated into a payment; see settleSaleFromLegacyFlag)
		r.Patch("/{id}", saleHandler.UpdateSale)

		// The statement of a sale: what the client has handed over, in parts. Open to
		// any authenticated user, like the clients view it belongs to - receiving money
		// from a client is the operator's own work.
		r.Get("/{id}/payments", saleHandler.GetSalePayments)
		r.Post("/{id}/payments", saleHandler.CreateSalePayment)
		r.Delete("/{id}/payments/{paymentID}", saleHandler.DeleteSalePayment)
	})
}
