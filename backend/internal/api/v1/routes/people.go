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

		// paginated listing for the management screen. Chi resolves a static
		// segment ahead of a param, so this never gets read as /people/{id}.
		r.Get("/manage", peopleHandler.ListPeople)

		// get person
		r.Get("/{id}", peopleHandler.GetPerson)

		// update person
		r.Put("/{id}", peopleHandler.UpdatePerson)

		// delete person
		r.Delete("/{id}", peopleHandler.DeletePerson)

		// merge two people into one. Admin-only, like the legacy import: it rewrites
		// who owns an account and then deletes a record for good, with no undo.
		r.With(requireAdmin).Get("/{id}/merge-preview", peopleHandler.PreviewMergePerson)
		r.With(requireAdmin).Post("/{id}/merge", peopleHandler.MergePerson)

		// merge a whole row of the accounts screen into this holder: the person (or the
		// holderless account) and every loyalty account that repeats a program. Admin-only
		// for the same reason.
		r.With(requireAdmin).Get("/{id}/merge-row-preview", peopleHandler.PreviewMergeRow)
		r.With(requireAdmin).Post("/{id}/merge-row", peopleHandler.MergeRow)

		// nicknames sub-resource
		RegisterPersonNicknameRoutes(r, db)
	})
}
