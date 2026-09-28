package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/azul"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/promotion"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// AzulConnectHandler owns the one-time "connect this Azul account" action: log the account in
// through the login service to discover its customerKey, and store it so the read-only sync can run
// from the datacentre IP thereafter.
type AzulConnectHandler struct {
	DB      *gorm.DB
	Promo   *promotion.Client
	Tracker *azulsync.ConnectTracker
}

func NewAzulConnectHandler(db *gorm.DB, tracker *azulsync.ConnectTracker) *AzulConnectHandler {
	return &AzulConnectHandler{DB: db, Promo: promotion.NewClient(), Tracker: tracker}
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
	var account data.Account
	if err := h.DB.First(&account, "id = ?", chi.URLParam(r, "id")).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch account: "+err.Error())
		return
	}
	if !account.TracksStatement {
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Esta conta não acompanha extrato. Ligue \"Acompanhar extrato\" no cadastro dela.")
		return
	}
	if account.Login == nil || *account.Login == "" || account.Password == nil || *account.Password == "" {
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"A conta precisa de login e senha salvos para conectar")
		return
	}

	// Refuse a duplicate rather than launch a second browser for the same account.
	if !h.Tracker.Begin(account.ID) {
		utils.SendError(w, r, http.StatusConflict, "Já há um login em andamento para esta conta")
		return
	}

	// account.Login/Password come back decrypted from the AfterFind hook, ready to send.
	login, password := *account.Login, *account.Password
	// WithoutCancel keeps the request's values but drops its cancellation: the request returns now,
	// and a browser login taking minutes must not be aborted because the operator closed the tab.
	go h.runConnect(context.WithoutCancel(r.Context()), account, login, password)

	utils.SendJSON(w, r, http.StatusAccepted, "Login iniciado", schemas.AzulConnectResponse{Connecting: true})
}

// runConnect performs the background login and records the outcome. The tracker mark is always
// cleared at the end.
func (h *AzulConnectHandler) runConnect(ctx context.Context, account data.Account, login, password string) {
	defer h.Tracker.End(account.ID)

	key, err := h.Promo.AzulLogin(ctx, promotion.AzulLoginRequest{
		DocumentNumber: login,
		Password:       password,
	})
	if err != nil {
		if markErr := azulsync.MarkLoginFailure(h.DB, account.ID, err); markErr != nil {
			slog.Error("azul connect: failed to record login failure", "account_id", account.ID, "err", markErr)
		}
		// The credentials are never logged; err from the login service is already credential-free.
		slog.Warn("azul connect: login did not complete", "account_id", account.ID, "err", err)
		return
	}

	// BeforeSave encrypts the customerKey the same way it does the password.
	account.AzulCustomerKey = &key
	if err := h.DB.Save(&account).Error; err != nil {
		slog.Error("azul connect: connected but failed to store the customerKey", "account_id", account.ID, "err", err)
		return
	}
	if err := azulsync.MarkConnected(h.DB, account.ID); err != nil {
		slog.Error("azul connect: connected but failed to record the state", "account_id", account.ID, "err", err)
		return
	}
	slog.Info("azul connect: account connected", "account_id", account.ID)

	// Pull the first statement window (up to a month) right away, so the panel shows the
	// reconciliation as soon as the connect finishes instead of waiting for the next ticker round.
	// This runs whether or not the periodic ticker (AZUL_SYNC_ENABLED) is on - connecting is an
	// explicit action, and the operator expects the extract to be there.
	h.firstSync(ctx, account.ID)
}

// firstSync runs one read round for a freshly connected account. It builds its own read client;
// if the Firebase config is absent (azul.New fails) the account is left connected and the periodic
// ticker will pull the window later, so a missing read config never turns a good connect into an
// error. The account is re-read so the AfterFind hook hands SyncAccount the decrypted customerKey.
func (h *AzulConnectHandler) firstSync(ctx context.Context, accountID string) {
	reader, err := azul.New()
	if err != nil {
		slog.Warn("azul connect: read client unavailable; first sync deferred to the ticker",
			"account_id", accountID, "err", err)
		return
	}
	var account data.Account
	if err := h.DB.First(&account, "id = ?", accountID).Error; err != nil {
		slog.Error("azul connect: reloading account for the first sync", "account_id", accountID, "err", err)
		return
	}
	if err := azulsync.SyncAccount(ctx, h.DB, reader, account, time.Now()); err != nil {
		// SyncAccount already records the failure in the sync state; log and move on.
		slog.Warn("azul connect: first sync did not complete", "account_id", accountID, "err", err)
		return
	}
	slog.Info("azul connect: first sync done", "account_id", accountID)
}
