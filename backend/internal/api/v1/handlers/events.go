package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/utils"
)

// heartbeatInterval keeps the connection alive through proxies and load
// balancers that drop idle sockets. It is a plain SSE comment, which browsers
// ignore, so it costs nothing on the client side.
const heartbeatInterval = 25 * time.Second

type EventHandler struct {
	Broker *events.Broker
}

func NewEventHandler(broker *events.Broker) *EventHandler {
	return &EventHandler{Broker: broker}
}

// Stream handles the GET /events route.
// @Summary      Stream de eventos (SSE)
// @Description  Conexão Server-Sent Events de longa duração. Emite eventos nomeados (ex: issuance.created, issuance.updated, issuance.cancelled, issuance.deleted, sale.updated) cujo data é um JSON com o id do recurso afetado, além de comentários de heartbeat a cada 25s. O cliente deve usar EventSource, que já reconecta sozinho.
// @Tags         Eventos
// @Produce      text/event-stream
// @Security     BearerAuth
// @Success      200  {string}  string  "Stream de eventos"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Router       /events [get]
func (h *EventHandler) Stream(w http.ResponseWriter, r *http.Request) {
	// Streaming response: the standard JSON envelope does not apply, mirroring the
	// export path. Everything below writes raw SSE frames instead of utils.SendJSON.
	flusher, ok := w.(http.Flusher)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Streaming não suportado por esta conexão")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Tells nginx-family reverse proxies not to buffer the stream; without it the
	// events only reach the browser once the proxy's buffer fills up.
	w.Header().Set("X-Accel-Buffering", "no")

	client := h.Broker.Subscribe()
	slog.Info("sse client connected", "clients", h.Broker.ClientCount())
	defer func() {
		h.Broker.Unsubscribe(client)
		// Logged rather than inferred: a client that stays in the hub after its tab
		// closed is the failure mode this endpoint is most likely to develop.
		slog.Info("sse client disconnected", "clients", h.Broker.ClientCount())
	}()

	// An immediate comment flushes the response headers, so the browser fires
	// onopen right away instead of waiting for the first real event.
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			// Tab closed, network dropped, or the proxy cut the request: the
			// deferred Unsubscribe removes this client from the hub.
			return

		case event, open := <-client:
			if !open {
				return
			}

			payload, err := json.Marshal(event.Payload)
			if err != nil {
				slog.Error("failed to marshal sse payload", "event_type", event.Type, "err", err)
				continue
			}

			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload); err != nil {
				return
			}
			flusher.Flush()

		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
