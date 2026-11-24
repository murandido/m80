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
	"github.com/murandido/m-80-backend/internal/access"
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

// InjunctionGroupHandler manages the Controle de Liminares: groups of injunction accounts
// that keep their issuances in a OneDrive workbook and talk to the agency in a WhatsApp group.
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
		WhatsAppChatName: group.WhatsAppChatName,
		Accounts:         []schemas.InjunctionGroupAccountResponse{},
		Issuers:          []schemas.InjunctionGroupIssuerResponse{},
		CreatedAt:        group.CreatedAt,
		UpdatedAt:        group.UpdatedAt,
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
	agencyNames := map[string]string{}
	if agencyIDs := injunction.IssuerAgencyIDs(group); len(agencyIDs) > 0 {
		var agencies []data.Agency
		if err := h.DB.Unscoped().Select("id", "name").Where("id IN ?", agencyIDs).Find(&agencies).Error; err != nil {
			return resp, err
		}
		for _, agency := range agencies {
			agencyNames[agency.ID] = agency.Name
		}
	}
	for _, mapping := range group.Mappings.Issuers {
		resp.Issuers = append(resp.Issuers, schemas.InjunctionGroupIssuerResponse{
			IssuerName: mapping.IssuerName, UserID: mapping.UserID, UserName: names[mapping.UserID],
			AgencyID: mapping.AgencyID, AgencyName: agencyNames[mapping.AgencyID],
		})
	}
	return resp, nil
}

// visibleTo says whether the caller may see a group: every agency its issuers write into has
// to be in their scope, because checking and importing it writes into all of them. A group
// with no issuer agency yet (saved before migration 000096) is dev only, like its setup.
func visibleTo(scope access.AgencyScope, group data.InjunctionGroup) bool {
	if scope.All {
		return true
	}
	agencyIDs := injunction.IssuerAgencyIDs(group)
	if len(agencyIDs) == 0 {
		return false
	}
	for _, agencyID := range agencyIDs {
		if !scope.Allows(agencyID) {
			return false
		}
	}
	return true
}

// loadGroup finds a group the caller may see; one outside their agency scope answers as
// missing.
func (h *InjunctionGroupHandler) loadGroup(w http.ResponseWriter, r *http.Request) (data.InjunctionGroup, bool) {
	var group data.InjunctionGroup
	err := h.DB.First(&group, "id = ?", chi.URLParam(r, "id")).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !visibleTo(middleware.AgencyScopeFrom(r.Context()), group)) {
		utils.SendError(w, r, http.StatusNotFound, "Grupo de liminares não encontrado")
		return group, false
	}
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao buscar o grupo de liminares")
		return group, false
	}
	return group, true
}

// GetInjunctionGroups handles the GET /injunction-groups route
// @Summary      Listar grupos de liminares
// @Description  Grupos de contas liminares com planilha no OneDrive e grupo no WhatsApp, das agências visíveis. O link da planilha vem mascarado.
// @Tags         Liminares
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Router       /injunction-groups [get]
func (h *InjunctionGroupHandler) GetInjunctionGroups(w http.ResponseWriter, r *http.Request) {
	var groups []data.InjunctionGroup
	if err := h.DB.Order("name").Find(&groups).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar os grupos de liminares")
		return
	}
	scope := middleware.AgencyScopeFrom(r.Context())
	resp := make([]schemas.InjunctionGroupResponse, 0, len(groups))
	for _, group := range groups {
		if !visibleTo(scope, group) {
			continue
		}
		item, err := h.toResponse(group)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar os grupos de liminares")
			return
		}
		resp = append(resp, item)
	}
	utils.SendJSON(w, r, http.StatusOK, "Grupos de liminares listados com sucesso", resp)
}

// GetInjunctionGroup handles the GET /injunction-groups/{id} route
// @Summary      Ver grupo de liminares
// @Tags         Liminares
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "ID do grupo de liminares"
// @Success      200  {object}  object
// @Failure      404  {object}  schemas.ErrorResponse "grupo de liminares não encontrada"
// @Router       /injunction-groups/{id} [get]
func (h *InjunctionGroupHandler) GetInjunctionGroup(w http.ResponseWriter, r *http.Request) {
	group, ok := h.loadGroup(w, r)
	if !ok {
		return
	}
	resp, err := h.toResponse(group)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao buscar o grupo de liminares")
		return
	}
	utils.SendJSON(w, r, http.StatusOK, "Grupo de liminares", resp)
}

// buildMappings checks every account is an injunction, every issuer a user with an agency the
// caller can write into, and fills the program code from the account itself. The message is
// for the operator when it fails.
func (h *InjunctionGroupHandler) buildMappings(scope access.AgencyScope, req schemas.InjunctionGroupRequest) (data.InjunctionGroupMappings, string, error) {
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
			return mappings, "Só contas liminares entram no grupo (\"" + label.AccountLabel + "\" não é liminar)", nil
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
		var agency data.Agency
		err = h.DB.Select("id").First(&agency, "id = ?", label.AgencyID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !scope.Allows(agency.ID)) {
			return mappings, "Agência não encontrada para o emissor \"" + label.IssuerName + "\"", nil
		}
		if err != nil {
			return mappings, "", err
		}
		mappings.Issuers = append(mappings.Issuers, data.InjunctionIssuerMapping{
			IssuerName: strings.ToUpper(strings.TrimSpace(label.IssuerName)), UserID: user.ID, AgencyID: agency.ID,
		})
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
	mappings, message, err := h.buildMappings(middleware.AgencyScopeFrom(r.Context()), req)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao validar as contas e emissores")
		return
	}
	if message != "" {
		utils.SendError(w, r, http.StatusUnprocessableEntity, message)
		return
	}

	group.Name = strings.TrimSpace(req.Name)
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

	if err := h.DB.Save(&group).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe um grupo de liminares com esse nome")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao salvar o grupo de liminares")
		return
	}

	// Read back so the link is decrypted for the response.
	var saved data.InjunctionGroup
	if err := h.DB.First(&saved, "id = ?", group.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao salvar o grupo de liminares")
		return
	}
	resp, err := h.toResponse(saved)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao salvar o grupo de liminares")
		return
	}
	status, message := http.StatusOK, "Grupo de liminares atualizado"
	if created {
		status, message = http.StatusCreated, "Grupo de liminares criado"
	}
	utils.SendJSON(w, r, status, message, resp)
}

// CreateInjunctionGroup handles the POST /injunction-groups route
// @Summary      Criar grupo de liminares
// @Description  Cria um grupo de liminares. Só contas liminares; o programa de cada conta vem da própria conta. Emissores devem ser usuários do painel, cada um com a agência das emissões dele.
// @Tags         Liminares
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  schemas.InjunctionGroupRequest  true  "grupo de liminares"
// @Success      201  {object}  object
// @Failure      422  {object}  schemas.ErrorResponse "Conta não liminar, emissor inexistente ou nome repetido"
// @Router       /injunction-groups [post]
func (h *InjunctionGroupHandler) CreateInjunctionGroup(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, data.InjunctionGroup{}, true)
}

// UpdateInjunctionGroup handles the PUT /injunction-groups/{id} route
// @Summary      Atualizar grupo de liminares
// @Description  Substitui o grupo de liminares. spreadsheet_url null mantém o link guardado; "" remove.
// @Tags         Liminares
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                          true  "ID do grupo de liminares"
// @Param        request  body  schemas.InjunctionGroupRequest  true  "grupo de liminares"
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
// @Summary      Excluir grupo de liminares
// @Description  Apaga o cadastro do grupo de liminares. Nenhuma emissão ou conta é alterada.
// @Tags         Liminares
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "ID do grupo de liminares"
// @Success      200  {object}  object
// @Router       /injunction-groups/{id} [delete]
func (h *InjunctionGroupHandler) DeleteInjunctionGroup(w http.ResponseWriter, r *http.Request) {
	group, ok := h.loadGroup(w, r)
	if !ok {
		return
	}
	if err := h.DB.Delete(&data.InjunctionGroup{}, "id = ?", group.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao excluir o grupo de liminares")
		return
	}
	utils.SendJSON(w, r, http.StatusOK, "Grupo de liminares excluído", nil)
}

// injunctionPeriodMaxDays bounds an import-period request: the Tropa workbooks hold a sheet
// per day, and a backfill wider than this is a typo in the dates.
const injunctionPeriodMaxDays = 120

// downloadWorkbook reads the Tropa's workbook through the Microsoft connection, answering the
// client itself on failure.
func (h *InjunctionGroupHandler) downloadWorkbook(w http.ResponseWriter, r *http.Request, group data.InjunctionGroup) ([]byte, bool) {
	if group.SpreadsheetURL == nil || *group.SpreadsheetURL == "" {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "O grupo de liminares não tem link de planilha cadastrado")
		return nil, false
	}
	content, err := h.Workbooks.Download(r.Context(), *group.SpreadsheetURL)
	if err != nil {
		// Every failure, not only the unexpected ones: "not shared" and "needs login" are the
		// ones an operator asks about, and Microsoft's own reason is only in this line.
		slog.Warn("injunction: workbook download failed", "group_id", group.ID, "err", err)
	}
	switch {
	case err == nil:
		return content, true
	case errors.Is(err, onedrivesync.ErrNotConfigured):
		utils.SendError(w, r, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, onedrivesync.ErrNotConnected):
		utils.SendError(w, r, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, onedrive.ErrNeedsLogin):
		utils.SendError(w, r, http.StatusUnprocessableEntity, "A conta Microsoft precisa ser conectada de novo (Dev Mode > Controle de Liminares)")
	case errors.Is(err, onedrive.ErrNotShared):
		utils.SendError(w, r, http.StatusUnprocessableEntity, "A planilha não abre com a conta Microsoft conectada: confira o link e o compartilhamento")
	default:
		utils.SendError(w, r, http.StatusBadGateway, "A Microsoft não entregou a planilha. Tente de novo.")
	}
	return nil, false
}

func parseDay(value string) (time.Time, error) {
	return time.Parse(time.DateOnly, value)
}

// CheckInjunctionGroup handles the GET /injunction-groups/{id}/check route
// @Summary      Conferir grupo de liminares
// @Description  Baixa a planilha do grupo de liminares e cruza com o painel e o extrato das contas liminares dele. Só linhas das contas mapeadas; a taxa de resgate é ignorada. Padrão: últimos 7 dias.
// @Tags         Liminares
// @Produce      json
// @Security     BearerAuth
// @Param        id    path   string  true   "ID do grupo de liminares"
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
	utils.SendJSON(w, r, http.StatusOK, "Conferência do grupo de liminares", result)
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
// @Summary      Importar dia da planilha do grupo de liminares
// @Description  Grava como emissões as linhas de um dia da planilha (só das contas do grupo de liminares; sem rows, todas as prontas). Linhas negativas cancelam a emissão do localizador. Taxa de resgate ignorada.
// @Tags         Liminares
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                           true  "ID do grupo de liminares"
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
// @Summary      Importar período da planilha do grupo de liminares
// @Description  Importa todas as linhas prontas das contas do grupo de liminares entre dois dias. Feito para a carga inicial antes da rotina diária.
// @Tags         Liminares
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                                 true  "ID do grupo de liminares"
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
