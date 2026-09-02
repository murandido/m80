package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterClientRoutes wires up the /clients CRUD routes.
func RegisterClientRoutes(r chi.Router, db *gorm.DB) {
	clientHandler := handlers.NewClientHandler(db)

	r.Route("/clients", func(r chi.Router) {
		// create client
		r.Post("/", clientHandler.CreateClient)

		// list clients — the whole list, for the client picker on the sales screen
		r.Get("/", clientHandler.ListClients)

		// paginated listing for the management screen. Chi resolves a static
		// segment ahead of a param, so this never gets read as /clients/{id}.
		r.Get("/manage", clientHandler.ManageClients)

		// get client
		r.Get("/{id}", clientHandler.GetClient)

		// update client
		r.Put("/{id}", clientHandler.UpdateClient)

		// delete client
		r.Delete("/{id}", clientHandler.DeleteClient)
	})
}
