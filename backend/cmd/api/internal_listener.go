package main

import (
	"net/http"

	"github.com/murandido/m-80-backend/internal/events"
)

// newInternalRouter builds the internal-only listener that bridges cmd/bot's
// status changes into the SSE broker. It is not part of the public chi
// router: no auth, no envelope, no domain knowledge of what changed - the
// broker is deliberately domain-agnostic (internal/events/broker.go), and the
// browser side always refetches on receiving "bot.status" rather than trust
// the event payload. Isolation is by network only, same trust model as the
// bot's own internal API (BOT_INTERNAL_URL).
func newInternalRouter(broker *events.Broker) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /internal/bot/status-changed", func(w http.ResponseWriter, r *http.Request) {
		broker.Broadcast(events.Event{Type: "bot.status"})
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}
