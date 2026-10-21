package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/carriers/latam"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/latamsync"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// LatamConnectHandler owns linking a LATAM account for its balance.
//
// The login itself happens on LATAM's own page, in the operator's browser, so there is no call here
// that takes a password. What this handler does is hand out the page and later receive the answer;
// how the answer gets back to the panel is documented in the private submodules.
type LatamConnectHandler struct {
	DB        *gorm.DB
	Client    *latam.AccountClient
	Tracker   *latamsync.LoginTracker
	Guard     *latamsync.SessionGuard
	Validator *validator.Validate
}

func NewLatamConnectHandler(db *gorm.DB, client *latam.AccountClient, tracker *latamsync.LoginTracker, guard *latamsync.SessionGuard) *LatamConnectHandler {
	return &LatamConnectHandler{DB: db, Client: client, Tracker: tracker, Guard: guard, Validator: validator.New()}
}

// StartLatamLogin handles POST /accounts/{id}/latam-connect
// @Summary      Conectar conta LATAM (abrir o login)
// @Description  Prepara um login na página oficial da LATAM e devolve o endereço para o painel abrir. O operador faz o login (e a verificação, se a LATAM pedir) na própria LATAM; no fim o navegador pergunta se pode abrir o programa instalado no computador, que devolve a resposta ao painel em /latam-connect/callback. O login vale por 15 minutos.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta a conectar"
// @Success      200  {object}  schemas.LatamConnectSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Router       /accounts/{id}/latam-connect [post]
func (h *LatamConnectHandler) StartLatamLogin(w http.ResponseWriter, r *http.Request) {
	account, ok := h.account(w, r)
	if !ok {
		return
	}
	login, err := h.Client.BeginLogin()
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui preparar o login: "+err.Error())
		return
	}
	h.Tracker.Begin(login.State, latamsync.PendingLogin{AccountID: account.ID, Verifier: login.Verifier})
	utils.SendJSON(w, r, http.StatusOK, "Abra o login da LATAM",
		schemas.LatamConnectResponse{AuthorizeURL: login.AuthorizeURL})
}

// FinishLatamLogin handles POST /latam-connect/callback
//
// Not under /accounts/{id}: the page that calls it has nothing but LATAM's answer, so the account
// is found from the login's state.
// @Summary      Concluir a conexão LATAM
// @Description  Recebe o endereço com que a LATAM respondeu ao login, acha a conta pelo login pendente, troca o código pela sessão, guarda a sessão cifrada e lê o saldo uma vez. O mesmo endereço só vale uma vez.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  schemas.LatamCallbackRequest  true  "Resposta da LATAM"
// @Success      200  {object}  schemas.LatamCallbackSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Corpo inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      409  {object}  schemas.ErrorResponse "Login expirado, já usado ou código recusado"
// @Failure      422  {object}  schemas.ErrorResponse "Não é uma resposta de login da LATAM"
// @Failure      429  {object}  schemas.ErrorResponse "A LATAM está limitando as tentativas"
// @Failure      503  {object}  schemas.ErrorResponse "A LATAM não respondeu"
// @Router       /latam-connect/callback [post]
func (h *LatamConnectHandler) FinishLatamLogin(w http.ResponseWriter, r *http.Request) {
	var body schemas.LatamCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Corpo inválido")
		return
	}
	if err := h.Validator.Struct(body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Dados inválidos: "+err.Error())
		return
	}

	parsed, err := url.Parse(strings.TrimSpace(body.CallbackURL))
	if err != nil || parsed.Query().Get("state") == "" {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Isso não é uma resposta de login da LATAM")
		return
	}
	pending, ok := h.Tracker.Take(parsed.Query().Get("state"))
	if !ok {
		utils.SendError(w, r, http.StatusConflict, latamExpiredLoginMessage)
		return
	}
	code, err := latam.CodeFromCallback(body.CallbackURL, parsed.Query().Get("state"))
	if err != nil {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "A LATAM não concluiu o login: "+err.Error())
		return
	}

	session, err := h.Client.ExchangeCode(r.Context(), code, pending.Verifier)
	if err != nil {
		slog.Warn("latam connect: code exchange failed", "account_id", pending.AccountID, "err", err)
		switch {
		case errors.Is(err, latam.ErrCodeRejected):
			utils.SendError(w, r, http.StatusConflict, latamExpiredLoginMessage)
		case errors.Is(err, latam.ErrRateLimited):
			utils.SendError(w, r, http.StatusTooManyRequests,
				"A LATAM está limitando as tentativas. Espere alguns minutos antes de tentar de novo.")
		case errors.Is(err, latam.ErrUnavailable):
			utils.SendError(w, r, http.StatusServiceUnavailable, "A LATAM não respondeu. Tente de novo em instantes.")
		default:
			utils.SendError(w, r, http.StatusInternalServerError, "Não consegui concluir o login: "+err.Error())
		}
		return
	}

	// The first balance read is part of the request: it is two quick calls, and the operator is
	// looking at the screen waiting for the number. A failed read still leaves the account linked
	// - the ticker retries it - so it is logged, not returned.
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := latamsync.Connect(ctx, h.DB, h.Client, h.Guard, pending.AccountID, session, time.Now()); err != nil {
		slog.Warn("latam connect: linked, but the first read failed", "account_id", pending.AccountID, "err", err)
	}
	slog.Info("latam connect: account linked", "account_id", pending.AccountID)
	utils.SendJSON(w, r, http.StatusOK, "Conta conectada",
		schemas.LatamCallbackResponse{AccountID: pending.AccountID, Connected: true})
}

// DisconnectLatamAccount handles DELETE /accounts/{id}/latam-connect
// @Summary      Desconectar a conta LATAM
// @Description  Apaga a sessão guardada, deixando a conta como antes de ser conectada. O último saldo lido continua na conta. Reconectar exige um login novo na LATAM.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.LatamCallbackSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao desconectar"
// @Router       /accounts/{id}/latam-connect [delete]
func (h *LatamConnectHandler) DisconnectLatamAccount(w http.ResponseWriter, r *http.Request) {
	account, ok := h.account(w, r)
	if !ok {
		return
	}
	if err := latamsync.Disconnect(h.DB, h.Guard, account.ID); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui desconectar: "+err.Error())
		return
	}
	slog.Info("latam connect: account disconnected", "account_id", account.ID)
	utils.SendJSON(w, r, http.StatusOK, "Conta desconectada", schemas.LatamCallbackResponse{AccountID: account.ID})
}

// LatamConnectStatus handles GET /accounts/{id}/latam-status
// @Summary      Estado da leitura do saldo LATAM
// @Description  O estado da leitura do saldo da conta (conectada, última leitura, falhas), para o cartão da conta.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.LatamSyncStateSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao carregar o estado"
// @Router       /accounts/{id}/latam-status [get]
func (h *LatamConnectHandler) LatamConnectStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := h.account(w, r)
	if !ok {
		return
	}
	state, err := latamsync.LoadState(h.DB, account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da leitura: "+err.Error())
		return
	}
	response := schemas.LatamSyncStateResponse{
		Connected:           account.LatamRefreshToken != nil && *account.LatamRefreshToken != "",
		State:               state.State,
		LastSyncedAt:        state.LastSyncedAt,
		LastBalancePoints:   account.RealBalancePoints,
		ConsecutiveFailures: state.ConsecutiveFailures,
		Detail:              state.Detail,
	}
	if !state.UpdatedAt.IsZero() {
		updated := state.UpdatedAt
		response.UpdatedAt = &updated
	}
	utils.SendJSON(w, r, http.StatusOK, "Estado carregado", response)
}

// latamExpiredLoginMessage names the recovery, because "409" on its own reads as a bug.
const latamExpiredLoginMessage = "Esse login da LATAM expirou ou já foi usado. Clique em Conectar de novo."

func (h *LatamConnectHandler) account(w http.ResponseWriter, r *http.Request) (data.Account, bool) {
	var account data.Account
	if err := h.DB.First(&account, "id = ?", chi.URLParam(r, "id")).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
			return account, false
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch account: "+err.Error())
		return account, false
	}
	return account, true
}
