package handlers

import (
	"net/http"
	"strings"

	"encoding/json"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/smilessync"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// Program codes the portal dispatches on. They are fixed by the loyalty-program seeds.
const (
	portalProgramAzul   = "AZUL"
	portalProgramSmiles = "SMILES"
)

// PortalHandler serves the statement client's own area. Every route but the list sits behind
// middleware.RequireLinkedAccount, so by the time a handler runs the account is one granted to
// the caller; the handlers reuse the staff statement code with the internal links left out.
type PortalHandler struct {
	DB        *gorm.DB
	Statement *StatementHandler
	Validator *validator.Validate
}

func NewPortalHandler(db *gorm.DB, statementHandler *StatementHandler) *PortalHandler {
	return &PortalHandler{DB: db, Statement: statementHandler, Validator: validator.New()}
}

// ListPortalAccounts handles GET /portal/accounts
// @Summary      Contas do cliente de extrato
// @Description  Lista as contas Azul/Smiles concedidas ao usuário logado (perfil statement_client), com o estado da sincronização e o último saldo lido. Nunca devolve a senha.
// @Tags         Portal
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.PortalAccountListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Só para clientes de extrato"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao carregar as contas"
// @Router       /portal/accounts [get]
func (h *PortalHandler) ListPortalAccounts(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}

	var accounts []data.Account
	err := h.DB.
		Joins("JOIN user_accounts ON user_accounts.account_id = accounts.id").
		Where("user_accounts.user_id = ?", user.ID).
		Preload("LoyaltyProgram").Preload("Holder").
		Order("user_accounts.created_at, accounts.id").
		Find(&accounts).Error
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui carregar as contas")
		return
	}

	resp := make([]schemas.PortalAccount, 0, len(accounts))
	for _, account := range accounts {
		item := schemas.PortalAccount{
			ID:              account.ID,
			Login:           account.Login,
			HasPassword:     account.Password != nil && *account.Password != "",
			TracksStatement: account.TracksStatement,
		}
		if account.LoyaltyProgram != nil {
			item.ProgramID = account.LoyaltyProgram.ID
			item.ProgramCode = account.LoyaltyProgram.Code
			item.ProgramName = account.LoyaltyProgram.Name
			item.ProgramColor = account.LoyaltyProgram.Color
		}
		if account.Holder != nil {
			item.HolderName = account.Holder.Name
		}
		if err := h.fillSyncState(&item, account); err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da sincronização")
			return
		}
		resp = append(resp, item)
	}

	utils.SendJSON(w, r, http.StatusOK, "Contas carregadas", resp)
}

func (h *PortalHandler) fillSyncState(item *schemas.PortalAccount, account data.Account) error {
	switch item.ProgramCode {
	case portalProgramAzul:
		state, err := azulsync.LoadState(h.DB, account.ID)
		if err != nil {
			return err
		}
		synced := h.Statement.toAzulSyncStateResponse(account, state)
		item.Connected, item.State, item.LastSyncedAt, item.LastBalancePoints =
			synced.Connected, synced.State, synced.LastSyncedAt, synced.LastBalancePoints
	case portalProgramSmiles:
		state, err := smilessync.LoadState(h.DB, account.ID)
		if err != nil {
			return err
		}
		synced := toSmilesSyncStateResponse(account, state)
		item.Connected, item.State, item.LastSyncedAt, item.LastBalancePoints =
			synced.Connected, synced.State, synced.LastSyncedAt, synced.LastBalancePoints
	}
	return nil
}

// PortalStatementEntries handles GET /portal/accounts/{id}/entries
// @Summary      Extrato da conta, visão do cliente
// @Description  O mesmo extrato sincronizado da tela da equipe, sem os vínculos com emissões e lotes (passageiro, custo dos pontos). Azul devolve o formato de AzulStatementEntriesResponse; Smiles, o de SmilesStatementEntriesResponse.
// @Tags         Portal
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.AzulStatementEntriesSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Só para clientes de extrato"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada ou não concedida"
// @Failure      422  {object}  schemas.ErrorResponse "Conta não acompanha extrato"
// @Router       /portal/accounts/{id}/entries [get]
func (h *PortalHandler) PortalStatementEntries(w http.ResponseWriter, r *http.Request) {
	account, ok := h.Statement.statementAccount(w, r)
	if !ok {
		return
	}
	// The scope only matters for links, which are not built here; an empty one keeps it that
	// way even if a later change forgets to pass includeLinks=false.
	noScope := access.AgencyScope{}
	switch middleware.LinkedProgramFrom(r.Context()) {
	case portalProgramAzul:
		resp, message, err := h.Statement.buildAzulStatementEntries(account, noScope, false)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, message+err.Error())
			return
		}
		resp.SyncState.Detail = nil // see PortalStatementStatus
		utils.SendJSON(w, r, http.StatusOK, "Extrato carregado", resp)
	case portalProgramSmiles:
		resp, message, err := h.Statement.buildSmilesStatementEntries(account, noScope, false)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, message+err.Error())
			return
		}
		resp.SyncState.Detail = nil // see PortalStatementStatus
		utils.SendJSON(w, r, http.StatusOK, "Extrato carregado", resp)
	default:
		utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
	}
}

// PortalStatementStatus handles GET /portal/accounts/{id}/status
// @Summary      Estado da sincronização da conta, visão do cliente
// @Tags         Portal
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.AzulSyncStateSingleResponse
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada ou não concedida"
// @Router       /portal/accounts/{id}/status [get]
func (h *PortalHandler) PortalStatementStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := h.Statement.loadAccount(w, r)
	if !ok {
		return
	}
	// The sync state without its detail: that is the raw error of the last login or read
	// (internal service names, proxy errors), meant for the team, not for a customer. The
	// state itself already says what the client needs to know.
	switch middleware.LinkedProgramFrom(r.Context()) {
	case portalProgramAzul:
		state, err := azulsync.LoadState(h.DB, account.ID)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da sincronização")
			return
		}
		resp := h.Statement.toAzulSyncStateResponse(account, state)
		resp.Detail = nil
		utils.SendJSON(w, r, http.StatusOK, "Estado carregado", resp)
	case portalProgramSmiles:
		state, err := smilessync.LoadState(h.DB, account.ID)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da sincronização")
			return
		}
		resp := toSmilesSyncStateResponse(account, state)
		resp.Detail = nil
		utils.SendJSON(w, r, http.StatusOK, "Estado carregado", resp)
	default:
		utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
	}
}

// PortalStatementExport handles GET /portal/accounts/{id}/export
// @Summary      Exportar o extrato da conta em CSV, visão do cliente
// @Description  O mesmo CSV da equipe (Conta, Data, Operação, Localizador, Valor, Origem, Destino, Descrição), que não traz vínculos nem custos.
// @Tags         Portal
// @Produce      text/csv
// @Security     BearerAuth
// @Param        id         path   string  true   "Conta"
// @Param        date_from  query  string  false  "Primeiro dia (YYYY-MM-DD, dia de Brasília)"
// @Param        date_to    query  string  false  "Último dia, inclusive (YYYY-MM-DD, dia de Brasília)"
// @Success      200  {string}  string  "CSV do extrato"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada ou não concedida"
// @Router       /portal/accounts/{id}/export [get]
func (h *PortalHandler) PortalStatementExport(w http.ResponseWriter, r *http.Request) {
	switch middleware.LinkedProgramFrom(r.Context()) {
	case portalProgramAzul:
		h.Statement.ExportAzulStatement(w, r)
	case portalProgramSmiles:
		h.Statement.ExportSmilesStatement(w, r)
	default:
		utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
	}
}

// UpdatePortalPassword handles PUT /portal/accounts/{id}/password
// @Summary      Trocar a senha da conta na companhia
// @Description  Grava a nova senha (criptografada) para o próximo "Conectar". Não conecta sozinho e nunca devolve a senha.
// @Tags         Portal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                         true  "Conta"
// @Param        payload  body  schemas.PortalPasswordRequest  true  "Nova senha"
// @Success      200  {object}  schemas.ErrorResponse "Senha salva"
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada ou não concedida"
// @Router       /portal/accounts/{id}/password [put]
func (h *PortalHandler) UpdatePortalPassword(w http.ResponseWriter, r *http.Request) {
	var req schemas.PortalPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Informe a senha")
		return
	}

	// Encrypted here and written with UpdateColumn: Account.BeforeSave would encrypt a full
	// struct save, but this route touches the one column and nothing else on the account.
	encrypted, err := utils.Encrypt(req.Password)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui salvar a senha")
		return
	}
	if err := h.DB.Model(&data.Account{}).Where("id = ?", chi.URLParam(r, "id")).
		UpdateColumn("password", encrypted).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui salvar a senha")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Senha salva", nil)
}

// RequirePortalProgram 404s a connect route mounted for one program when the account is of
// another: an Azul login on a Smiles account would only fail later, inside a browser.
func RequirePortalProgram(code string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if middleware.LinkedProgramFrom(r.Context()) != code {
				utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
