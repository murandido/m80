package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/utils"
)

// AzulConnectHandler owns the "connect this Azul account" action: log the account in through the
// login service to discover its customerKey, and store it so the read-only sync can run from the
// datacentre IP thereafter. The flow itself is azulsync.Connector, shared with the ticker and the
// account update, which also start it.
type AzulConnectHandler struct {
	Connector *azulsync.Connector
}

func NewAzulConnectHandler(connector *azulsync.Connector) *AzulConnectHandler {
	return &AzulConnectHandler{Connector: connector}
}

// ConnectAzulAccount handles POST /accounts/{id}/azul-connect
//
// The login takes minutes and can retry across several browser sessions, far longer than a request
// should stay open (the panel's own fetch caps at ~300s). So this is asynchronous: it validates the
// account, kicks the login off in the background and returns 202 at once. The panel reads progress
// from the sync-state badge (GET /accounts/{id}/azul-statement, field connecting), which flips to
// connected on success or to an error state on failure. Running the login on a background context
// rather than the request's also means a client that navigates away no longer cancels a browser
// mid-login. Idempotent per account: a second request while one is in flight is refused with 409.
// @Summary      Conectar conta Azul (descobrir o customerKey)
// @Description  Dispara, em segundo plano, o login que descobre o customerKey da conta e o guarda cifrado. Retorna 202 na hora; o painel acompanha pelo estado da sincronização (campo connecting em GET /accounts/{id}/azul-statement), que vira "conectado" no sucesso ou um estado de erro na falha. Após N falhas seguidas a conta é marcada como "precisa de atenção". Uma segunda chamada enquanto um login já está em andamento é recusada com 409.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta a conectar"
// @Success      202  {object}  schemas.AzulConnectSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      409  {object}  schemas.ErrorResponse "Já existe um login em andamento para esta conta"
// @Failure      422  {object}  schemas.ErrorResponse "Conta que não acompanha extrato, ou sem login/senha"
// @Router       /accounts/{id}/azul-connect [post]
func (h *AzulConnectHandler) ConnectAzulAccount(w http.ResponseWriter, r *http.Request) {
	err := h.Connector.Start(r.Context(), chi.URLParam(r, "id"))
	switch {
	case err == nil:
		utils.SendJSON(w, r, http.StatusAccepted, "Login iniciado", schemas.AzulConnectResponse{Connecting: true})
	case errors.Is(err, azulsync.ErrAccountNotFound):
		utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
	case errors.Is(err, azulsync.ErrNotTracked):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Esta conta não acompanha extrato. Ligue \"Acompanhar extrato\" no cadastro dela.")
	case errors.Is(err, azulsync.ErrMissingCredentials):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"A conta precisa de login e senha salvos para conectar")
	case errors.Is(err, azulsync.ErrAlreadyConnecting):
		utils.SendError(w, r, http.StatusConflict, "Já há um login em andamento para esta conta")
	default:
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch account: "+err.Error())
	}
}
