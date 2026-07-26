package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/murandido/m-80-backend/internal/bot"
	"github.com/murandido/m-80-backend/internal/utils"
)

// newInternalRouter builds the bot's internal-only HTTP API: status, pairing
// and logout. It has no auth of its own - it is only reachable from the
// Docker internal network, and admin auth happens one layer up, in cmd/api's
// proxy at /api/v1/bot/*.
func newInternalRouter(b *bot.Bot) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)

	r.Get("/status", func(w http.ResponseWriter, req *http.Request) {
		utils.SendJSON(w, req, http.StatusOK, "status", b.Status.Get())
	})

	r.Post("/pair", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Method bot.PairMethod `json:"method"`
			Phone  string         `json:"phone"`
		}
		if req.Body != nil {
			// A missing/empty body just falls back to the default below.
			_ = json.NewDecoder(req.Body).Decode(&body)
		}
		if body.Method == "" {
			body.Method = bot.PairMethodPhone
		}

		if body.Method == bot.PairMethodPhone && !utils.ValidateBRPhone(body.Phone) {
			utils.SendError(w, req, http.StatusBadRequest, "Telefone inválido - use DDD + número")
			return
		}

		//nolint:contextcheck // StartPairing deliberately does not take req.Context(): pairing completes asynchronously, well after this request returns, and whatsmeow disconnects the client the moment a context it was given is cancelled.
		status, err := b.StartPairing(body.Method, body.Phone)
		if err != nil {
			if errors.Is(err, bot.ErrAlreadyConnected) {
				utils.SendError(w, req, http.StatusUnprocessableEntity, err.Error())
				return
			}
			utils.SendError(w, req, http.StatusInternalServerError, err.Error())
			return
		}

		utils.SendJSON(w, req, http.StatusOK, "pairing started", status)
	})

	r.Post("/pair/cancel", func(w http.ResponseWriter, req *http.Request) {
		status := b.CancelPairing(req.Context())
		utils.SendJSON(w, req, http.StatusOK, "pairing cancelled", status)
	})

	r.Post("/logout", func(w http.ResponseWriter, req *http.Request) {
		status, err := b.Logout(req.Context())
		if err != nil {
			utils.SendError(w, req, http.StatusInternalServerError, err.Error())
			return
		}
		utils.SendJSON(w, req, http.StatusOK, "logged out", status)
	})

	return r
}
