package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterPeopleRoutes wires up the /people CRUD routes.
func RegisterPeopleRoutes(r chi.Router, db *gorm.DB) {
	peopleHandler := handlers.NewPeopleHandler(db)

	r.Route("/people", func(r chi.Router) {
		// create person
		r.Post("/", peopleHandler.CreatePerson)

		// get people
		r.Get("/", peopleHandler.GetPeople)

		// get person
		r.Get("/{id}", peopleHandler.GetPerson)

		// update person
		r.Put("/{id}", peopleHandler.UpdatePerson)

		// delete person
		r.Delete("/{id}", peopleHandler.DeletePerson)

		// nicknames sub-resource
		RegisterPersonNicknameRoutes(r, db)
	})
}
