package routes

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
)

// RegisterBotRoutes mounts /bot/* as a dev-only, pure reverse proxy to the
// bot's internal API (BOT_INTERNAL_URL) - status, pairing and logout for the
// WhatsApp session. No business logic here: whatever the bot answers is what
// the panel gets, status code and body untouched.
func RegisterBotRoutes(r chi.Router) {
	proxy, err := handlers.NewBotProxy(os.Getenv("BOT_INTERNAL_URL"))
	if err != nil {
		panic("routes: invalid BOT_INTERNAL_URL: " + err.Error())
	}

	forward := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Rewrite to the path relative to this route (via chi's wildcard
		// param) rather than assuming the absolute mount prefix, so the
		// proxy target always sees e.g. "/status" regardless of where "/bot"
		// ends up mounted upstream.
		req.URL.Path = "/" + chi.URLParam(req, "*")
		proxy.ServeHTTP(w, req)
	})

	r.Route("/bot", func(r chi.Router) {
		r.Use(middleware.RequireDev())
		// Only the verbs the bot's internal API actually serves - GET
		// /status and the POST actions - not a catch-all for every method.
		r.Get("/*", forward)
		r.Post("/*", forward)
	})
}
