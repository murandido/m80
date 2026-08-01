package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"gorm.io/gorm"
)

// RegisterListenedChatRoutes mounts /bot/chats/* - admin-only endpoints to
// list, approve, deny and stop monitoring WhatsApp chats requested via the
// bot's !registrar-chat command. Mounted as a static child under the same
// "bot" router node as RegisterBotRoutes' "/bot/*" catch-all proxy; chi's
// route tree always checks static children before the catch-all bucket, so
// this never falls through to the proxy regardless of registration order.
func RegisterListenedChatRoutes(r chi.Router, db *gorm.DB) {
	h := handlers.NewListenedChatHandler(db)

	r.Route("/bot/chats", func(r chi.Router) {
		r.Use(middleware.RequireAdmin())

		r.Get("/pending", h.GetPendingListenedChats)
		r.Post("/{id}/approve", h.ApproveListenedChat)
		r.Post("/{id}/deny", h.DenyListenedChat)

		r.Get("/active", h.GetMonitoredListenedChats)
		r.Post("/{id}/stop", h.StopListenedChat)
		r.Post("/{id}/deactivate", h.DeactivateListenedChat)
		r.Post("/{id}/reactivate", h.ReactivateListenedChat)

		// the other half of the bot's !pdf command - same column, two ways in
		r.Patch("/{id}/pdf-mode", h.SetListenedChatPDFMode)
	})
}
