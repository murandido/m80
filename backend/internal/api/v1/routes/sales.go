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

		// one window of the same listing, for the panel's scrolled list
		r.Get("/page", saleHandler.GetSalesPage)

		// export filtered sales as an .xlsx spreadsheet
		r.Get("/export", saleHandler.GetSalesExport)

		// the payment statement sent to a client, with the clients view's filters
		r.Get("/clients/report", saleHandler.GetClientReportPDF)

		// aggregated balance per account (Visão Fornecedores)
		r.With(middleware.RequireAdmin()).Get("/suppliers", saleHandler.GetSupplierBalances)

		// set the payment terms on a set of issuances. Admin-only for the same
		// reason the balances are: what the agency pays a supplier is not operator data.
		r.With(middleware.RequireAdmin()).Post("/suppliers/issuances", saleHandler.UpdateSupplierIssuances)

		// the payment summary sent to the account's administrator
		r.With(middleware.RequireAdmin()).Get("/suppliers/report", saleHandler.GetSupplierReportPDF)

		// the per-administrator export counter that numbers each supplier PDF. Admin-only,
		// like everything else on the suppliers side. POST bumps it per export; PUT sets it
		// by hand.
		r.With(middleware.RequireAdmin()).Post("/suppliers/export-counter", saleHandler.IncrementSupplierExportCounter)
		r.With(middleware.RequireAdmin()).Put("/suppliers/export-counter", saleHandler.SetSupplierExportCounter)
		// bulk bump for the "all suppliers" PDF, which covers several administrators at once
		r.With(middleware.RequireAdmin()).Post("/suppliers/export-counter/bulk", saleHandler.BulkIncrementSupplierExportCounter)

		// Everything addressed by a sale id sits behind the scope check - see
		// middleware.RequireSaleInScope.
		r.Route("/{id}", func(r chi.Router) {
			r.Use(middleware.RequireSaleInScope(db))

			// link a client (and, for a panel that has not been published yet, the old
			// paid switch - translated into a payment; see settleSaleFromLegacyFlag)
			r.Patch("/", saleHandler.UpdateSale)

			// The statement of a sale: what the client has handed over, in parts. Open to
			// any authenticated user, like the clients view it belongs to - receiving money
			// from a client is the operator's own work.
			r.Get("/payments", saleHandler.GetSalePayments)
			r.Post("/payments", saleHandler.CreateSalePayment)
			r.Delete("/payments/{paymentID}", saleHandler.DeleteSalePayment)
		})

		// A lump sum from a client, spread over their open sales oldest first; what no sale
		// takes is the client's credit. Same audience as the statement above.
		r.Post("/client-payments", saleHandler.CreateClientPayment)
		// The client's payments, undone ones included, and undoing one: the way back from
		// a wrong amount, which would otherwise sit in the client's credit for good.
		r.Get("/client-payments", saleHandler.GetClientPayments)
		r.Delete("/client-payments/{paymentID}", saleHandler.DeleteClientPayment)
	})
}
