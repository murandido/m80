package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	v1 "github.com/murandido/m-80-backend/internal/api/v1"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RealIP)

	r.Use(middleware.CORS())
	r.Use(middleware.RateLimit())

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("API On 🚀"))
		if err != nil {
			//TODO: audit this error
			fmt.Println("Error sending response: ", err)
			return
		}
	})

	r.Mount("/api/v1", v1.NewRouter(db))

	return r
}
