package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/smiles"
	"github.com/murandido/m-80-backend/internal/smilessync"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// SmilesConnectHandler owns connecting a Smiles account, which is a conversation rather than a
// call: the password login answers "send a code", the operator picks a channel, the code is
// typed, and only then is there a session to store.
//
// Unlike the Azul connect this is synchronous at every step. Nothing here needs a browser, so
// each step is an HTTP round trip of a second or two; what takes time is the human, and the
// state that waits for them lives in the LoginTracker rather than in a background goroutine.
type SmilesConnectHandler struct {
	DB        *gorm.DB
	Client    *smiles.Client
	Tracker   *smilessync.LoginTracker
	Guard     *smilessync.SessionGuard
	Validator *validator.Validate

	// AfterConnect is what runs once a session is stored - in production the first read round,
	// on a background context. It is a field because it is the one part of connecting that
	// outlives the request, and a test that wants to assert on what was stored should not have
	// a goroutine reading the database behind it.
	AfterConnect func(ctx context.Context, accountID string)
}

func NewSmilesConnectHandler(db *gorm.DB, tracker *smilessync.LoginTracker, guard *smilessync.SessionGuard) *SmilesConnectHandler {
	handler := &SmilesConnectHandler{
		DB:        db,
		Client:    smiles.New(),
		Tracker:   tracker,
		Guard:     guard,
		Validator: validator.New(),
	}
	handler.AfterConnect = handler.firstSync
	return handler
}

// ConnectSmilesAccount handles POST /accounts/{id}/smiles-connect
//
// Logs the account in with its stored credentials. Almost always the tenant answers "a code is
// required" and the panel is handed the channels to choose from; an account without a second
// factor is connected outright.
// @Summary      Conectar conta Smiles (primeiro passo do login)
// @Description  Faz o login na Smiles com o login e a senha já salvos na conta. Normalmente a Smiles exige um código de verificação: a resposta traz mfa_required e a lista de canais cadastrados pelo titular (SMS/e-mail), com o destino mascarado. O painel então escolhe um canal em /smiles-connect/challenge e envia os dígitos em /smiles-connect/code. Se a conta não exigir segundo fator, ela já fica conectada.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta a conectar"
// @Success      200  {object}  schemas.SmilesConnectSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem login/senha, ou credenciais recusadas"
// @Failure      429  {object}  schemas.ErrorResponse "A Smiles está limitando as tentativas"
// @Failure      503  {object}  schemas.ErrorResponse "A Smiles não respondeu"
// @Router       /accounts/{id}/smiles-connect [post]
func (h *SmilesConnectHandler) ConnectSmilesAccount(w http.ResponseWriter, r *http.Request) {
	account, ok := h.account(w, r)
	if !ok {
		return
	}
	if account.Login == nil || *account.Login == "" || account.Password == nil || *account.Password == "" {
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"A conta precisa de login e senha salvos para conectar")
		return
	}

	// account.Login/Password come back decrypted from the AfterFind hook, ready to send.
	start, err := h.Client.BeginLogin(r.Context(), *account.Login, *account.Password)
	if err != nil {
		// Nothing about the credentials is logged, and the client has already redacted the
		// tenant's body before it got here.
		slog.Warn("smiles connect: login did not start", "account_id", account.ID, "err", err)
		h.sendClientError(w, r, err, "Não consegui iniciar o login na Smiles")
		return
	}

	if start.Session != nil {
		if err := h.storeSession(r.Context(), account.ID, *start.Session); err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Conectei, mas não consegui guardar a sessão: "+err.Error())
			return
		}
		utils.SendJSON(w, r, http.StatusOK, "Conta conectada",
			schemas.SmilesConnectResponse{Connected: true})
		return
	}

	h.Tracker.Begin(account.ID, smilessync.PendingLogin{
		MFAToken:       start.MFAToken,
		Authenticators: start.Authenticators,
	})
	utils.SendJSON(w, r, http.StatusOK, "Escolha onde receber o código", schemas.SmilesConnectResponse{
		MFARequired:    true,
		Authenticators: toAuthenticators(start.Authenticators),
	})
}

// ChallengeSmilesLogin handles POST /accounts/{id}/smiles-connect/challenge
//
// Asks Smiles to send the code on the chosen channel. It is the call that costs a real SMS or
// e-mail, so it only ever happens because an operator pressed a button - never on a retry.
// @Summary      Pedir o código de verificação da Smiles
// @Description  Pede à Smiles que envie o código no canal escolhido (um dos ids devolvidos por /smiles-connect). Cada chamada dispara um SMS ou e-mail de verdade, e a Smiles limita reenvios por canal — então o painel não reenvia sozinho. Responde com o canal e o destino mascarado, para a tela dizer onde o código chegou.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                          true  "Conta"
// @Param        request  body  schemas.SmilesChallengeRequest   true  "Canal escolhido"
// @Success      202  {object}  schemas.SmilesChallengeSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Corpo inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      409  {object}  schemas.ErrorResponse "Não há login em andamento, ou ele expirou"
// @Failure      422  {object}  schemas.ErrorResponse "Canal não é da conta"
// @Failure      429  {object}  schemas.ErrorResponse "A Smiles está limitando os envios"
// @Router       /accounts/{id}/smiles-connect/challenge [post]
func (h *SmilesConnectHandler) ChallengeSmilesLogin(w http.ResponseWriter, r *http.Request) {
	account, ok := h.account(w, r)
	if !ok {
		return
	}

	var body schemas.SmilesChallengeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Corpo inválido")
		return
	}
	if err := h.Validator.Struct(body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Dados inválidos: "+err.Error())
		return
	}

	pending, ok := h.Tracker.Pending(account.ID)
	if !ok {
		utils.SendError(w, r, http.StatusConflict, expiredLoginMessage)
		return
	}
	// The channel must be one the tenant offered for this account: taking the id on trust would
	// let a caller aim the challenge at an authenticator we never listed.
	chosen, found := findAuthenticator(pending.Authenticators, body.AuthenticatorID)
	if !found {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Esse canal não é um dos cadastrados nesta conta")
		return
	}

	oobCode, err := h.Client.Challenge(r.Context(), pending.MFAToken, chosen.ID)
	if err != nil {
		slog.Warn("smiles connect: challenge refused", "account_id", account.ID, "err", err)
		if errors.Is(err, smiles.ErrMFAExpired) {
			h.Tracker.End(account.ID)
			utils.SendError(w, r, http.StatusConflict, expiredLoginMessage)
			return
		}
		h.sendClientError(w, r, err, "Não consegui pedir o código à Smiles")
		return
	}
	if !h.Tracker.Challenged(account.ID, oobCode) {
		utils.SendError(w, r, http.StatusConflict, expiredLoginMessage)
		return
	}

	utils.SendJSON(w, r, http.StatusAccepted, "Código enviado", schemas.SmilesChallengeResponse{
		Channel: chosen.Channel,
		SentTo:  chosen.Target,
	})
}

// ConfirmSmilesLogin handles POST /accounts/{id}/smiles-connect/code
//
// Trades the digits the operator typed for a session, stores it and pulls the first window.
// @Summary      Confirmar o código de verificação da Smiles
// @Description  Troca o código digitado pela sessão da conta, guarda a sessão cifrada e dispara em segundo plano a primeira sincronização do extrato. Código errado devolve 422 com a recusa da Smiles, sem reenviar nada: a Smiles conta tentativas e um reenvio automático bloquearia o titular. Código expirado devolve 409 e o login recomeça.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                      true  "Conta"
// @Param        request  body  schemas.SmilesCodeRequest   true  "O código recebido"
// @Success      200  {object}  schemas.SmilesConnectSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Corpo inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      409  {object}  schemas.ErrorResponse "O código expirou; recomece o login"
// @Failure      422  {object}  schemas.ErrorResponse "Código não confere"
// @Router       /accounts/{id}/smiles-connect/code [post]
func (h *SmilesConnectHandler) ConfirmSmilesLogin(w http.ResponseWriter, r *http.Request) {
	account, ok := h.account(w, r)
	if !ok {
		return
	}

	var body schemas.SmilesCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Corpo inválido")
		return
	}
	if err := h.Validator.Struct(body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Código inválido")
		return
	}

	pending, ok := h.Tracker.Pending(account.ID)
	if !ok || pending.OOBCode == "" {
		utils.SendError(w, r, http.StatusConflict, expiredLoginMessage)
		return
	}

	session, err := h.Client.ConfirmMFA(r.Context(), pending.MFAToken, pending.OOBCode, body.Code)
	if err != nil {
		// The code itself is never logged, and neither is the tenant's body.
		slog.Warn("smiles connect: code refused", "account_id", account.ID, "err", err)
		switch {
		case errors.Is(err, smiles.ErrInvalidCode):
			// Deliberately not a resend: Smiles counts failed attempts and throttles per
			// channel, so answering a typo with another SMS is how the holder gets locked out.
			utils.SendError(w, r, http.StatusUnprocessableEntity,
				"O código não confere. Confira os dígitos e tente de novo — a Smiles limita as tentativas.")
		case errors.Is(err, smiles.ErrMFAExpired):
			h.Tracker.End(account.ID)
			utils.SendError(w, r, http.StatusConflict, expiredLoginMessage)
		default:
			h.sendClientError(w, r, err, "Não consegui confirmar o código")
		}
		return
	}

	h.Tracker.End(account.ID)
	if err := h.storeSession(r.Context(), account.ID, session); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Conectei, mas não consegui guardar a sessão: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Conta conectada", schemas.SmilesConnectResponse{Connected: true})
}

// DisconnectSmilesAccount handles DELETE /accounts/{id}/smiles-connect
//
// Forgets the stored session and the synced window, leaving the account as it was before it was
// ever connected.
// @Summary      Desconectar a conta Smiles
// @Description  Apaga a sessão guardada e a janela de extrato sincronizada, deixando a conta como antes de ser conectada. Os lotes já lançados a partir do extrato não são tocados — só o extrato sincronizado some. Reconectar exige um código novo.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.SmilesConnectSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao desconectar"
// @Router       /accounts/{id}/smiles-connect [delete]
func (h *SmilesConnectHandler) DisconnectSmilesAccount(w http.ResponseWriter, r *http.Request) {
	account, ok := h.account(w, r)
	if !ok {
		return
	}

	h.Tracker.End(account.ID)
	if err := smilessync.Disconnect(h.DB, h.Guard, account.ID); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui desconectar: "+err.Error())
		return
	}
	slog.Info("smiles connect: account disconnected", "account_id", account.ID)

	utils.SendJSON(w, r, http.StatusOK, "Conta desconectada", schemas.SmilesConnectResponse{})
}

// expiredLoginMessage is what an operator sees when the tenant's short-lived token has aged out
// between steps. It names the recovery, because "409" on its own reads as a bug.
const expiredLoginMessage = "O login expirou. Clique em conectar de novo para receber um código novo."

// storeSession persists the session and kicks off the first read. The read runs on a background
// context: it takes seconds and the operator should get their answer now, but a closed tab must
// not abort it halfway.
func (h *SmilesConnectHandler) storeSession(ctx context.Context, accountID string, session smiles.Session) error {
	if err := smilessync.StoreSession(h.DB, h.Guard, accountID, session.RefreshToken); err != nil {
		return err
	}
	if err := smilessync.MarkConnected(h.DB, accountID); err != nil {
		return err
	}
	slog.Info("smiles connect: account connected", "account_id", accountID)

	if h.AfterConnect != nil {
		// WithoutCancel keeps the request's values but drops its cancellation: the response
		// goes out now, and a first read must not be aborted because the operator closed the
		// tab.
		go h.AfterConnect(context.WithoutCancel(ctx), accountID)
	}
	return nil
}

// firstSync runs one read round for a freshly connected account, so the statement is on screen
// as soon as the connect finishes instead of waiting for the next ticker round. It runs whether
// or not the periodic ticker is on: connecting is an explicit action, and the operator expects
// the extract to be there.
func (h *SmilesConnectHandler) firstSync(ctx context.Context, accountID string) {
	// Re-read the account so the AfterFind hook hands SyncAccount the decrypted session.
	var account data.Account
	if err := h.DB.First(&account, "id = ?", accountID).Error; err != nil {
		slog.Error("smiles connect: reloading account for the first sync", "account_id", accountID, "err", err)
		return
	}
	if err := smilessync.SyncAccount(ctx, h.DB, h.Client, h.Guard, account, time.Now()); err != nil {
		// SyncAccount already recorded the failure in the sync state; log and move on.
		slog.Warn("smiles connect: first sync did not complete", "account_id", accountID, "err", err)
		return
	}
	slog.Info("smiles connect: first sync done", "account_id", accountID)
}

// account loads the account, answering the request itself when it does not exist. Any account
// may connect: the balance is read for every connected account, and "Acompanhar extrato" only
// decides whether the statement is pulled on top of it.
func (h *SmilesConnectHandler) account(w http.ResponseWriter, r *http.Request) (data.Account, bool) {
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

// sendClientError turns a client error into the status that tells the operator what to do:
// wait (429), try later (503), or fix the credentials (422). Anything else is ours to explain.
func (h *SmilesConnectHandler) sendClientError(w http.ResponseWriter, r *http.Request, err error, prefix string) {
	switch {
	case errors.Is(err, smiles.ErrRateLimited):
		utils.SendError(w, r, http.StatusTooManyRequests,
			"A Smiles está limitando as tentativas. Espere alguns minutos antes de tentar de novo.")
	case errors.Is(err, smiles.ErrUnavailable), errors.Is(err, smiles.ErrHostsUnavailable):
		utils.SendError(w, r, http.StatusServiceUnavailable,
			"A Smiles não respondeu agora. Tente de novo em instantes.")
	case errors.Is(err, smiles.ErrAccessBlocked):
		utils.SendError(w, r, http.StatusServiceUnavailable,
			"A Smiles recusou o acesso HTTP deste servidor. Verifique a saída de rede antes de tentar de novo.")
	case errors.Is(err, smiles.ErrInvalidCredentials):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"A Smiles recusou o login salvo na conta: "+err.Error())
	default:
		utils.SendError(w, r, http.StatusInternalServerError, prefix+": "+err.Error())
	}
}

func toAuthenticators(list []smiles.Authenticator) []schemas.SmilesAuthenticator {
	out := make([]schemas.SmilesAuthenticator, 0, len(list))
	for _, item := range list {
		out = append(out, schemas.SmilesAuthenticator{ID: item.ID, Channel: item.Channel, Target: item.Target})
	}
	return out
}

func findAuthenticator(list []smiles.Authenticator, id string) (smiles.Authenticator, bool) {
	for _, item := range list {
		if item.ID == id {
			return item, true
		}
	}
	return smiles.Authenticator{}, false
}
