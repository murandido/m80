package main

import (
	"net/http"

	"github.com/murandido/m-80-backend/internal/events"
)

// newInternalRouter builds the internal-only listener that bridges cmd/bot's
// own state changes (connection status, a chat requested via
// !registrar-chat) into the SSE broker. It is not part of the public chi
// router: no auth, no envelope, no domain knowledge of what changed - the
// broker is deliberately domain-agnostic (internal/events/broker.go), and the
// browser side always refetches on receiving an event rather than trust its
// (nonexistent) payload. Isolation is by network only, same trust model as
// the bot's own internal API (BOT_INTERNAL_URL).
func newInternalRouter(broker *events.Broker) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /internal/bot/status-changed", func(w http.ResponseWriter, r *http.Request) {
		broker.Broadcast(events.Event{Type: "bot.status"})
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /internal/bot/chat-requested", func(w http.ResponseWriter, r *http.Request) {
		broker.Broadcast(events.Event{Type: "listened_chat.requested"})
		w.WriteHeader(http.StatusNoContent)
	})

	// Same event type the issuance handlers broadcast, so the grid reacts to a
	// row created from WhatsApp exactly as it does to one created in the panel.
	// The panel's own broadcast carries the issuance id; this one carries
	// nothing, and that costs nothing either - the browser refetches with its
	// own filters regardless of the payload.
	mux.HandleFunc("POST /internal/bot/issuance-created", func(w http.ResponseWriter, r *http.Request) {
		broker.Broadcast(events.Event{Type: "issuance.created"})
		w.WriteHeader(http.StatusNoContent)
	})

	// A correction from WhatsApp - an ALTERACAO message or an edit of the
	// original EMISSAO - broadcast as the same event the panel's own PATCH
	// emits, for the same reason as the create above.
	mux.HandleFunc("POST /internal/bot/issuance-updated", func(w http.ResponseWriter, r *http.Request) {
		broker.Broadcast(events.Event{Type: "issuance.updated"})
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}
