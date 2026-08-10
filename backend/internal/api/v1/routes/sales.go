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

		// link a client and/or mark as paid
		r.Patch("/{id}", saleHandler.UpdateSale)
	})
}
