package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/carriers/onedrive"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/businessday"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/injunction"
	"github.com/murandido/m-80-backend/internal/legacyimport"
	"github.com/murandido/m-80-backend/internal/onedrivesync"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// InjunctionGroupHandler manages the Tropas: groups of injunction accounts that keep their
// issuances in a OneDrive workbook and talk to the agency in a WhatsApp group.
type InjunctionGroupHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
	Workbooks *onedrivesync.Service
	Broker    *events.Broker
}

func NewInjunctionGroupHandler(db *gorm.DB, workbooks *onedrivesync.Service, broker *events.Broker) *InjunctionGroupHandler {
	return &InjunctionGroupHandler{DB: db, Validator: validator.New(), Workbooks: workbooks, Broker: broker}
}

// spreadsheetHint shows enough of a link to recognise it, and not enough to open it.
func spreadsheetHint(link string) string {
	trimmed := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(link, "https://"), "http://"), "/", 2)[0]
	tail := link
	if index := strings.Index(link, "?"); index > 0 {
		tail = link[:index]
	}
	if len(tail) > 5 {
		tail = tail[len(tail)-5:]
	}
	return trimmed + "/…" + tail
}

func (h *InjunctionGroupHandler) toResponse(group data.InjunctionGroup) (schemas.InjunctionGroupResponse, error) {
	resp := schemas.InjunctionGroupResponse{
		ID:               group.ID,
		Name:             group.Name,
		AgencyID:         group.AgencyID,
		WhatsAppChatName: group.WhatsAppChatName,
		Accounts:         []schemas.InjunctionGroupAccountResponse{},
		Issuers:          []schemas.InjunctionGroupIssuerResponse{},
		CreatedAt:        group.CreatedAt,
		UpdatedAt:        group.UpdatedAt,
	}
	if group.Agency != nil {
		resp.AgencyName = group.Agency.Name
	}
	if group.SpreadsheetURL != nil && *group.SpreadsheetURL != "" {
		resp.HasSpreadsheet = true
		hint := spreadsheetHint(*group.SpreadsheetURL)
		resp.SpreadsheetHint = &hint
	}

	accountIDs := make([]string, 0, len(group.Mappings.Accounts))
	for _, mapping := range group.Mappings.Accounts {
		accountIDs = append(accountIDs, mapping.AccountID)
	}
	accounts := map[string]data.Account{}
	if len(accountIDs) > 0 {
		var rows []data.Account
		if err := h.DB.Unscoped().Preload("LoyaltyProgram").Preload("Nicknames").Where("id IN ?", accountIDs).Find(&rows).Error; err != nil {
			return resp, err
		}
		for _, account := range rows {
			accounts[account.ID] = account
		}
	}
	for _, mapping := range group.Mappings.Accounts {
		item := schemas.InjunctionGroupAccountResponse{AccountLabel: mapping.AccountLabel, AccountID: mapping.AccountID, ProgramCode: mapping.ProgramCode}
		if account, ok := accounts[mapping.AccountID]; ok {
			if account.LoyaltyProgram != nil {
				item.ProgramName = account.LoyaltyProgram.Name
			}
			names := make([]string, 0, len(account.Nicknames))
			for _, nickname := range account.Nicknames {
				names = append(names, nickname.Nickname)
			}
			item.Nicknames = strings.Join(names, ", ")
		}
		resp.Accounts = append(resp.Accounts, item)
	}

	userIDs := make([]string, 0, len(group.Mappings.Issuers))
	for _, mapping := range group.Mappings.Issuers {
		userIDs = append(userIDs, mapping.UserID)
	}
	names := map[string]string{}
	if len(userIDs) > 0 {
		var users []data.User
		if err := h.DB.Unscoped().Select("id", "name").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return resp, err
		}
		for _, user := range users {
			names[user.ID] = user.Name
		}
	}
	for _, mapping := range group.Mappings.Issuers {
		resp.Issuers = append(resp.Issuers, schemas.InjunctionGroupIssuerResponse{IssuerName: mapping.IssuerName, UserID: mapping.UserID, UserName: names[mapping.UserID]})
	}
	return resp, nil
}

// loadGroup finds a Tropa in the caller's agency scope; another agency's answers as missing.
func (h *InjunctionGroupHandler) loadGroup(w http.ResponseWriter, r *http.Request) (data.InjunctionGroup, bool) {
	var group data.InjunctionGroup
	query := middleware.AgencyScopeFrom(r.Context()).Restrict(h.DB.Preload("Agency"), "injunction_groups.agency_id")
	err := query.First(&group, "injunction_groups.id = ?", chi.URLParam(r, "id")).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.SendError(w, r, http.StatusNotFound, "Tropa não encontrada")
		return group, false
	}
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao buscar a Tropa")
		return group, false
	}
	return group, true
}

// GetInjunctionGroups handles the GET /injunction-groups route
// @Summary      Listar Tropas
// @Description  Tropas (grupos de contas liminares com planilha no OneDrive e grupo no WhatsApp) das agências visíveis. O link da planilha vem mascarado.
// @Tags         Tropas
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Router       /injunction-groups [get]
func (h *InjunctionGroupHandler) GetInjunctionGroups(w http.ResponseWriter, r *http.Request) {
	var groups []data.InjunctionGroup
	query := middleware.AgencyScopeFrom(r.Context()).Restrict(h.DB.Preload("Agency"), "injunction_groups.agency_id")
	if err := query.Order("name").Find(&groups).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar Tropas")
		return
	}
	resp := make([]schemas.InjunctionGroupResponse, 0, len(groups))
	for _, group := range groups {
		item, err := h.toResponse(group)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar Tropas")
			return
		}
		resp = append(resp, item)
	}
	utils.SendJSON(w, r, http.StatusOK, "Tropas listadas com sucesso", resp)
}

// GetInjunctionGroup handles the GET /injunction-groups/{id} route
// @Summary      Ver Tropa
// @Tags         Tropas
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "ID da Tropa"
// @Success      200  {object}  object
// @Failure      404  {object}  schemas.ErrorResponse "Tropa não encontrada"
// @Router       /injunction-groups/{id} [get]
func (h *InjunctionGroupHandler) GetInjunctionGroup(w http.ResponseWriter, r *http.Request) {
	group, ok := h.loadGroup(w, r)
	if !ok {
		return
	}
	resp, err := h.toResponse(group)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao buscar a Tropa")
		return
	}
	utils.SendJSON(w, r, http.StatusOK, "Tropa", resp)
}

// buildMappings checks every account is an injunction and every issuer a user, and fills the
// program code from the account itself. The message is for the operator when it fails.
func (h *InjunctionGroupHandler) buildMappings(req schemas.InjunctionGroupRequest) (data.InjunctionGroupMappings, string, error) {
	mappings := data.InjunctionGroupMappings{Accounts: []data.InjunctionAccountMapping{}, Issuers: []data.InjunctionIssuerMapping{}}
	seen := map[string]bool{}
	for _, label := range req.Accounts {
		var account data.Account
		err := h.DB.Preload("LoyaltyProgram").First(&account, "id = ?", label.AccountID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mappings, "Conta não encontrada: " + label.AccountID, nil
		}
		if err != nil {
			return mappings, "", err
		}
		if account.Category != data.AccountCategoryInjunction {
			return mappings, "Só contas liminares entram numa Tropa (\"" + label.AccountLabel + "\" não é liminar)", nil
		}
		code := ""
		if account.LoyaltyProgram != nil {
			code = account.LoyaltyProgram.Code
		}
		normalized := strings.ToUpper(strings.TrimSpace(label.AccountLabel))
		key := code + "|" + normalized
		if seen[key] {
			return mappings, "O rótulo \"" + normalized + "\" aparece duas vezes no mesmo programa", nil
		}
		seen[key] = true
		mappings.Accounts = append(mappings.Accounts, data.InjunctionAccountMapping{ProgramCode: code, AccountLabel: normalized, AccountID: account.ID})
	}
	for _, label := range req.Issuers {
		var user data.User
		err := h.DB.Select("id").First(&user, "id = ?", label.UserID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mappings, "Usuário não encontrado para o emissor \"" + label.IssuerName + "\"", nil
		}
		if err != nil {
			return mappings, "", err
		}
		mappings.Issuers = append(mappings.Issuers, data.InjunctionIssuerMapping{IssuerName: strings.ToUpper(strings.TrimSpace(label.IssuerName)), UserID: user.ID})
	}
	return mappings, "", nil
}

func (h *InjunctionGroupHandler) save(w http.ResponseWriter, r *http.Request, group data.InjunctionGroup, created bool) {
	var req schemas.InjunctionGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "JSON inválido")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Dados inválidos: "+err.Error())
		return
	}
	if !middleware.AgencyScopeFrom(r.Context()).Allows(req.AgencyID) {
		utils.SendError(w, r, http.StatusNotFound, "Agência não encontrada")
		return
	}
	mappings, message, err := h.buildMappings(req)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao validar as contas e emissores")
		return
	}
	if message != "" {
		utils.SendError(w, r, http.StatusUnprocessableEntity, message)
		return
	}

	group.Name = strings.TrimSpace(req.Name)
	group.AgencyID = req.AgencyID
	group.WhatsAppChatName = req.WhatsAppChatName
	group.Mappings = mappings
	if req.SpreadsheetURL != nil {
		if *req.SpreadsheetURL == "" {
			group.SpreadsheetURL = nil
		} else {
			link := strings.TrimSpace(*req.SpreadsheetURL)
			group.SpreadsheetURL = &link
		}
	}

	if err := h.DB.Omit("Agency").Save(&group).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe uma Tropa com esse nome")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao salvar a Tropa")
		return
	}

	// Read back so the link is decrypted and the agency loaded for the response.
	var saved data.InjunctionGroup
	if err := h.DB.Preload("Agency").First(&saved, "id = ?", group.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao salvar a Tropa")
		return
	}
	resp, err := h.toResponse(saved)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao salvar a Tropa")
		return
	}
	status, message := http.StatusOK, "Tropa atualizada"
	if created {
		status, message = http.StatusCreated, "Tropa criada"
	}
	utils.SendJSON(w, r, status, message, resp)
}

// CreateInjunctionGroup handles the POST /injunction-groups route
// @Summary      Criar Tropa
// @Description  Cria uma Tropa. Só contas liminares; o programa de cada conta vem da própria conta. Emissores devem ser usuários do painel.
// @Tags         Tropas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  schemas.InjunctionGroupRequest  true  "Tropa"
// @Success      201  {object}  object
// @Failure      422  {object}  schemas.ErrorResponse "Conta não liminar, emissor inexistente ou nome repetido"
// @Router       /injunction-groups [post]
func (h *InjunctionGroupHandler) CreateInjunctionGroup(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, data.InjunctionGroup{}, true)
}

// UpdateInjunctionGroup handles the PUT /injunction-groups/{id} route
// @Summary      Atualizar Tropa
// @Description  Substitui a Tropa. spreadsheet_url null mantém o link guardado; "" remove.
// @Tags         Tropas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                          true  "ID da Tropa"
// @Param        request  body  schemas.InjunctionGroupRequest  true  "Tropa"
// @Success      200  {object}  object
// @Router       /injunction-groups/{id} [put]
func (h *InjunctionGroupHandler) UpdateInjunctionGroup(w http.ResponseWriter, r *http.Request) {
	group, ok := h.loadGroup(w, r)
	if !ok {
		return
	}
	h.save(w, r, group, false)
}

// DeleteInjunctionGroup handles the DELETE /injunction-groups/{id} route
// @Summary      Excluir Tropa
// @Description  Apaga o cadastro da Tropa. Nenhuma emissão ou conta é alterada.
// @Tags         Tropas
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "ID da Tropa"
// @Success      200  {object}  object
// @Router       /injunction-groups/{id} [delete]
func (h *InjunctionGroupHandler) DeleteInjunctionGroup(w http.ResponseWriter, r *http.Request) {
	group, ok := h.loadGroup(w, r)
	if !ok {
		return
	}
	if err := h.DB.Delete(&data.InjunctionGroup{}, "id = ?", group.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao excluir a Tropa")
		return
	}
	utils.SendJSON(w, r, http.StatusOK, "Tropa excluída", nil)
}

// injunctionPeriodMaxDays bounds an import-period request: the Tropa workbooks hold a sheet
// per day, and a backfill wider than this is a typo in the dates.
const injunctionPeriodMaxDays = 120

// downloadWorkbook reads the Tropa's workbook through the Microsoft connection, answering the
// client itself on failure.
func (h *InjunctionGroupHandler) downloadWorkbook(w http.ResponseWriter, r *http.Request, group data.InjunctionGroup) ([]byte, bool) {
	if group.SpreadsheetURL == nil || *group.SpreadsheetURL == "" {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "A Tropa não tem link de planilha cadastrado")
		return nil, false
	}
	content, err := h.Workbooks.Download(r.Context(), *group.SpreadsheetURL)
	switch {
	case err == nil:
		return content, true
	case errors.Is(err, onedrivesync.ErrNotConfigured):
		utils.SendError(w, r, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, onedrivesync.ErrNotConnected):
		utils.SendError(w, r, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, onedrive.ErrNeedsLogin):
		utils.SendError(w, r, http.StatusUnprocessableEntity, "A conta Microsoft precisa ser conectada de novo (Dev Mode > Tropas)")
	case errors.Is(err, onedrive.ErrNotShared):
		utils.SendError(w, r, http.StatusUnprocessableEntity, "A planilha não abre com a conta Microsoft conectada: confira o link e o compartilhamento")
	default:
		slog.Error("injunction: workbook download failed", "group_id", group.ID, "err", err)
		utils.SendError(w, r, http.StatusBadGateway, "A Microsoft não entregou a planilha. Tente de novo.")
	}
	return nil, false
}

func parseDay(value string) (time.Time, error) {
	return time.Parse(time.DateOnly, value)
}

// CheckInjunctionGroup handles the GET /injunction-groups/{id}/check route
// @Summary      Conferir Tropa
// @Description  Baixa a planilha da Tropa e cruza com o painel e o extrato das contas liminares dela. Só linhas das contas mapeadas; a taxa de resgate é ignorada. Padrão: últimos 7 dias.
// @Tags         Tropas
// @Produce      json
// @Security     BearerAuth
// @Param        id    path   string  true   "ID da Tropa"
// @Param        from  query  string  false  "Primeiro dia (AAAA-MM-DD)"
// @Param        to    query  string  false  "Último dia (AAAA-MM-DD)"
// @Success      200  {object}  object
// @Failure      422  {object}  schemas.ErrorResponse "Sem planilha, sem conexão Microsoft ou planilha inacessível"
// @Router       /injunction-groups/{id}/check [get]
func (h *InjunctionGroupHandler) CheckInjunctionGroup(w http.ResponseWriter, r *http.Request) {
	group, ok := h.loadGroup(w, r)
	if !ok {
		return
	}
	today := businessday.Of(time.Now())
	from, to := today.AddDate(0, 0, -7), today
	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := parseDay(raw)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "from inválido: use AAAA-MM-DD")
			return
		}
		from = parsed
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := parseDay(raw)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "to inválido: use AAAA-MM-DD")
			return
		}
		to = parsed
	}
	workbook, ok := h.downloadWorkbook(w, r, group)
	if !ok {
		return
	}
	result, err := injunction.Check(h.DB, group, workbook, from, to)
	if err != nil {
		utils.SendError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}
	utils.SendJSON(w, r, http.StatusOK, "Conferência da Tropa", result)
}

func (h *InjunctionGroupHandler) announce(result legacyimport.CommitResult) {
	if h.Broker == nil {
		return
	}
	if result.Imported+result.CreatedAndCancelled > 0 {
		h.Broker.Broadcast(events.Event{Type: eventIssuanceCreated, Payload: map[string]string{"id": ""}})
	}
	if result.Cancelled+result.CreatedAndCancelled > 0 {
		h.Broker.Broadcast(events.Event{Type: eventIssuanceCancelled, Payload: map[string]string{"id": ""}})
	}
}

// ImportInjunctionGroupDay handles the POST /injunction-groups/{id}/import route
// @Summary      Importar dia da planilha da Tropa
// @Description  Grava como emissões as linhas de um dia da planilha (só das contas da Tropa; sem rows, todas as prontas). Linhas negativas cancelam a emissão do localizador. Taxa de resgate ignorada.
// @Tags         Tropas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                           true  "ID da Tropa"
// @Param        request  body  schemas.InjunctionImportRequest  true  "Dia e linhas"
// @Success      200  {object}  object
// @Router       /injunction-groups/{id}/import [post]
func (h *InjunctionGroupHandler) ImportInjunctionGroupDay(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}
	group, ok := h.loadGroup(w, r)
	if !ok {
		return
	}
	var req schemas.InjunctionImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || h.Validator.Struct(req) != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Informe day (AAAA-MM-DD) e, se quiser, rows")
		return
	}
	day, _ := parseDay(req.Day)
	var rows []legacyimport.RowSelection
	if len(req.Rows) > 0 {
		rows = make([]legacyimport.RowSelection, 0, len(req.Rows))
		for _, row := range req.Rows {
			rows = append(rows, legacyimport.RowSelection{BlockIndex: row.BlockIndex, Number: row.Number})
		}
	}
	workbook, ok := h.downloadWorkbook(w, r, group)
	if !ok {
		return
	}
	result, err := injunction.Import(h.DB, group, workbook, day, rows, user.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}
	h.announce(result)
	utils.SendJSON(w, r, http.StatusOK, "Dia importado", result)
}

// ImportInjunctionGroupPeriod handles the POST /injunction-groups/{id}/import-period route
// @Summary      Importar período da planilha da Tropa
// @Description  Importa todas as linhas prontas das contas da Tropa entre dois dias. Feito para a carga inicial antes da rotina diária.
// @Tags         Tropas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                                 true  "ID da Tropa"
// @Param        request  body  schemas.InjunctionImportPeriodRequest  true  "Período"
// @Success      200  {object}  object
// @Router       /injunction-groups/{id}/import-period [post]
func (h *InjunctionGroupHandler) ImportInjunctionGroupPeriod(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}
	group, ok := h.loadGroup(w, r)
	if !ok {
		return
	}
	var req schemas.InjunctionImportPeriodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || h.Validator.Struct(req) != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Informe from e to (AAAA-MM-DD)")
		return
	}
	from, _ := parseDay(req.From)
	to, _ := parseDay(req.To)
	if to.Before(from) || to.Sub(from) > injunctionPeriodMaxDays*24*time.Hour {
		utils.SendError(w, r, http.StatusBadRequest, fmt.Sprintf("Período inválido (até %d dias)", injunctionPeriodMaxDays))
		return
	}
	workbook, ok := h.downloadWorkbook(w, r, group)
	if !ok {
		return
	}

	type dayResult struct {
		Day    string                    `json:"day"`
		Result legacyimport.CommitResult `json:"result"`
		Error  string                    `json:"error,omitempty"`
	}
	total := legacyimport.CommitResult{Skipped: []legacyimport.SkippedRow{}}
	days := []dayResult{}
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		result, err := injunction.Import(h.DB, group, workbook, day, nil, user.ID)
		if err != nil {
			// A day the workbook does not have is normal (weekends, future days).
			if !strings.Contains(err.Error(), "não tem a aba") {
				days = append(days, dayResult{Day: day.Format(time.DateOnly), Error: err.Error()})
			}
			continue
		}
		if result.Imported+result.Cancelled+result.CreatedAndCancelled+len(result.Skipped) == 0 {
			continue
		}
		total.Imported += result.Imported
		total.Cancelled += result.Cancelled
		total.CreatedAndCancelled += result.CreatedAndCancelled
		total.Skipped = append(total.Skipped, result.Skipped...)
		days = append(days, dayResult{Day: day.Format(time.DateOnly), Result: result})
	}
	h.announce(total)
	utils.SendJSON(w, r, http.StatusOK, "Período importado", map[string]any{"total": total, "days": days})
}
