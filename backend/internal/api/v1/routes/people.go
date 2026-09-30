package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterPeopleRoutes wires up the /people CRUD routes.
func RegisterPeopleRoutes(r chi.Router, db *gorm.DB) {
	peopleHandler := handlers.NewPeopleHandler(db)
	requireAdmin := middleware.RequireAdmin()

	r.Route("/people", func(r chi.Router) {
		// create person
		r.Post("/", peopleHandler.CreatePerson)

		// get people — the whole list, for the pickers
		r.Get("/", peopleHandler.GetPeople)

		// get person
		r.Get("/{id}", peopleHandler.GetPerson)

		// update person
		r.Put("/{id}", peopleHandler.UpdatePerson)

		// the administrator of every account the holder has - one per holder
		r.Put("/{id}/administrator", peopleHandler.SetHolderAdministrator)

		// delete person: the holder and their accounts - deleted where nothing refers to them,
		// archived otherwise
		r.Get("/{id}/delete-preview", peopleHandler.PreviewDeletePerson)
		r.Delete("/{id}", peopleHandler.DeletePerson)

		// merge a whole row of the accounts screen into this holder: the other holder and every
		// loyalty account that repeats a program. Admin-only: it moves issuance history between
		// accounts and deletes records for good, with no undo.
		r.With(requireAdmin).Get("/{id}/merge-row-preview", peopleHandler.PreviewMergeRow)
		r.With(requireAdmin).Post("/{id}/merge-row", peopleHandler.MergeRow)

		// nicknames sub-resource
		RegisterPersonNicknameRoutes(r, db)
	})
}
