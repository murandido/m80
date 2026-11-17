package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem login/senha"
// @Router       /accounts/{id}/azul-connect [post]
func (h *AzulConnectHandler) ConnectAzulAccount(w http.ResponseWriter, r *http.Request) {
	err := h.Connector.Start(r.Context(), chi.URLParam(r, "id"))
	switch {
	case err == nil:
		utils.SendJSON(w, r, http.StatusAccepted, "Login iniciado", schemas.AzulConnectResponse{Connecting: true})
	case errors.Is(err, azulsync.ErrAccountNotFound):
		utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
	case errors.Is(err, azulsync.ErrMissingCredentials):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"A conta precisa de login e senha salvos para conectar")
	case errors.Is(err, azulsync.ErrAlreadyConnecting):
		utils.SendError(w, r, http.StatusConflict, "Já há um login em andamento para esta conta")
	default:
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch account: "+err.Error())
	}
}

// maxBrowserKeyBody bounds the request: a customerKey and a CPF are well under a kilobyte.
const maxBrowserKeyBody = 4 << 10

// StoreAzulBrowserKey handles POST /accounts/{id}/azul-customer-key
//
// The fallback for when the login service cannot get through: the operator logs in on the Azul
// site in their own browser, the extension hands the key that login produced to the panel, and
// the panel sends it here. The key is checked before it is kept (see
// azulsync.Connector.StoreBrowserKey): the login must have been made with this account's CPF, and
// Azul must answer for it. The key is never logged.
// @Summary      Conectar conta Azul com a chave do login feito no navegador
// @Description  Recebe a customerKey capturada de um login feito pelo funcionário no próprio navegador (pela extensão) e o CPF usado nesse login. Só grava se o CPF for o da conta e se a Azul responder o saldo com essa chave; depois roda a primeira sincronização em segundo plano.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  string                         true  "Conta"
// @Param        body  body  schemas.AzulBrowserKeyRequest  true  "Chave e CPF do login"
// @Success      200  {object}  schemas.AzulBrowserKeySingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      409  {object}  schemas.ErrorResponse "Já existe um login em andamento para esta conta"
// @Failure      422  {object}  schemas.ErrorResponse "O login não foi feito com o CPF da conta, ou a conta não tem login salvo"
// @Failure      502  {object}  schemas.ErrorResponse "A Azul não reconheceu a chave, ou não deu para conferir agora"
// @Failure      503  {object}  schemas.ErrorResponse "Leitura da Azul não configurada neste servidor"
// @Router       /accounts/{id}/azul-customer-key [post]
func (h *AzulConnectHandler) StoreAzulBrowserKey(w http.ResponseWriter, r *http.Request) {
	var req schemas.AzulBrowserKeyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBrowserKeyBody)).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Payload inválido")
		return
	}
	if req.CustomerKey == "" || req.LoginDocument == "" {
		utils.SendError(w, r, http.StatusBadRequest, "A chave e o CPF do login são obrigatórios")
		return
	}

	accountID := chi.URLParam(r, "id")
	err := h.Connector.StoreBrowserKey(r.Context(), accountID, req.CustomerKey, req.LoginDocument)
	switch {
	case err == nil:
		// The statement is pulled after the answer, on a context that outlives the request.
		go h.Connector.FirstSync(context.WithoutCancel(r.Context()), accountID)
		utils.SendJSON(w, r, http.StatusOK, "Conta conectada", schemas.AzulBrowserKeyResponse{Connected: true})
	case errors.Is(err, azulsync.ErrAccountNotFound):
		utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
	case errors.Is(err, azulsync.ErrMissingCredentials):
		utils.SendError(w, r, http.StatusUnprocessableEntity, "A conta precisa do CPF salvo como login para conectar")
	case errors.Is(err, azulsync.ErrDocumentMismatch):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"O login feito na Azul não foi com o CPF desta conta. Nada foi gravado.")
	case errors.Is(err, azulsync.ErrAlreadyConnecting):
		utils.SendError(w, r, http.StatusConflict, "Já há um login em andamento para esta conta. Espere ele terminar.")
	case errors.Is(err, azulsync.ErrKeyRejected):
		utils.SendError(w, r, http.StatusBadGateway, "A Azul não reconheceu a chave deste login. Tente conectar de novo.")
	case errors.Is(err, azulsync.ErrKeyUnverified):
		slog.Warn("azul browser connect: could not check the key", "account_id", accountID, "err", err)
		utils.SendError(w, r, http.StatusBadGateway,
			"Não deu para conferir a chave com a Azul agora. Nada foi gravado; tente de novo em alguns minutos.")
	case errors.Is(err, azulsync.ErrVerifierUnavailable):
		utils.SendError(w, r, http.StatusServiceUnavailable, "A leitura da Azul não está configurada neste servidor.")
	default:
		slog.Error("azul browser connect: storing the key", "account_id", accountID, "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui gravar a conexão. Tente de novo.")
	}
}
