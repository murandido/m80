package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/events"
)

// RegisterEventRoutes wires up the SSE stream. It takes the broker instead of a
// *gorm.DB because the stream never touches the database - it only fans out what
// other handlers publish.
func RegisterEventRoutes(r chi.Router, broker *events.Broker) {
	eventHandler := handlers.NewEventHandler(broker)

	// single long-lived connection; every module publishes through the same broker
	r.Get("/events", eventHandler.Stream)
}
