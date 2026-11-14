package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/passengerextract"
)

// RegisterPassengerRoutes wires the passenger reader the browser extension fills airline forms
// from. The extractor comes from the environment; without ANTHROPIC_API_KEY the route stays
// and answers 503, so the extension can tell "off" from "broken".
func RegisterPassengerRoutes(r chi.Router) {
	handler := handlers.NewPassengerExtractHandler()
	if extractor := passengerextract.NewFromEnv(); extractor != nil {
		// Assigned only when present: a nil *Extractor in the interface field would read as set.
		handler.Extractor = extractor
	}

	r.Route("/passengers", func(r chi.Router) {
		r.With(middleware.PassengerExtractRateLimit()).Post("/extract", handler.ExtractPassengers)
	})
}
