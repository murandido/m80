package handlers

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/azul"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/statement"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// AzulStatementEntries handles GET /accounts/{id}/azul-statement/entries
//
// The synced statement itself, as a pretty per-account view rather than the reconciliation: the
// raw movements in day order, each redemption/refund resolved to the real issuance it maps to
// (by locator, ignoring the date - see statement.ResolveBookingLinks), plus the sync-state
// header. A never-synced account is an empty window, not an error.
// @Summary      Extrato sincronizado da Azul, com vínculo de emissões
// @Description  Lista as linhas do extrato que o coletor sincronizou da Azul (tabela azul_statement_lines), em ordem de dia, cada resgate/reembolso já vinculado à emissão real do banco quando o localizador bate (ignorando a data, como o lote). Traz o estado da sincronização para o cabeçalho e o período que a janela cobre. Nada é gravado. Conta sem janela sincronizada devolve lista vazia.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.AzulStatementEntriesSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao carregar o extrato"
// @Router       /accounts/{id}/azul-statement/entries [get]
func (h *StatementHandler) AzulStatementEntries(w http.ResponseWriter, r *http.Request) {
	account, ok := h.azulAccount(w, r)
	if !ok {
		return
	}

	var rows []data.AzulStatementLine
	if err := h.DB.Where("account_id = ?", account.ID).
		Order("occurred_on ASC, id ASC").
		Find(&rows).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o extrato: "+err.Error())
		return
	}

	// Each line's link key: its locator when it has one (redemptions, refunds, flight accruals),
	// otherwise a stable signature so a locator-less line (a promotion) can still be linked by
	// hand. The same key is what the frontend sends back to link or unlink. Point-entry lines
	// (purchases, accruals) resolve to a batch instead of an issuance, matched by day and points.
	keys := make([]string, 0, len(rows))
	batchRefs := make([]statement.BatchLineRef, 0, len(rows))
	for _, row := range rows {
		key := statementLineKey(row)
		keys = append(keys, key)
		if kind := statementKind(row); kind == data.AzulLineAccrual || kind == data.AzulLinePurchase {
			batchRefs = append(batchRefs, statement.BatchLineRef{
				Key: key, OccurredOn: row.OccurredOn, Points: row.Points,
			})
		}
	}
	resolved, err := statement.ResolveBookingLinks(h.DB, account.ID, keys)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui resolver os vínculos: "+err.Error())
		return
	}
	resolvedBatches, err := statement.ResolveBatchLinks(h.DB, account.ID, batchRefs)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui resolver os lotes: "+err.Error())
		return
	}

	state, err := azulsync.LoadState(h.DB, account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da sincronização: "+err.Error())
		return
	}

	// The city-to-IATA lookup, read once for the whole page; an unassigned city shows its raw name.
	cityCodes, err := data.LoadAzulCityCodes(h.DB)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler os códigos de cidade: "+err.Error())
		return
	}

	entries := make([]schemas.AzulStatementEntry, 0, len(rows))
	var from, to string
	for i, row := range rows {
		day := row.OccurredOn.Format(batchDateLayout)
		if i == 0 {
			from = day
		}
		to = day // rows are ordered ascending, so the last one seen is the window's end

		key := statementLineKey(row)
		// Derive the kind from the airline's type code rather than trusting the stored one: the
		// mapping (refunds, purchases, club/card accruals) may be newer than a row that was synced
		// before it, and this keeps the page correct without waiting for a re-sync.
		kind := statementKind(row)
		entry := schemas.AzulStatementEntry{
			Key:             key,
			OccurredOn:      day,
			Kind:            kind,
			TypeCode:        row.TypeCode,
			TypeDescription: row.TypeDescription,
			Points:          row.Points,
			Locator:         effectiveLocator(row),
			// Origin/destination arrive as Azul's own city or airport names; show the IATA code
			// when the city is assigned one, otherwise the raw name (which the operator can assign).
			Origin:      azul.ResolveCity(cityCodes, derefString(row.Origin)),
			Destination: azul.ResolveCity(cityCodes, derefString(row.Destination)),
			PartnerName: derefString(row.PartnerName),
			Description: derefString(row.Description),
			Note:        azul.NoteFromComment(derefString(row.Comment)),
			DayBalance:  row.DayBalance,
		}
		if booking, found := resolved[key]; found {
			entry.LinkedIssuance = toLinkedIssuance(booking)
		}
		if batch, found := resolvedBatches[key]; found {
			entry.LinkedBatch = toLinkedBatch(batch)
		}
		entries = append(entries, entry)
	}

	utils.SendJSON(w, r, http.StatusOK, "Extrato carregado", schemas.AzulStatementEntriesResponse{
		SyncState: h.toAzulSyncStateResponse(account, state),
		From:      from,
		To:        to,
		Lines:     entries,
	})
}

// ExportAzulStatement handles GET /accounts/{id}/azul-statement/export
//
// The synced window as a CSV in the same shape the old Azul upload used
// (Conta,Data,Operação,Localizador,Valor,Origem,Destino,Descrição), so the operator can keep a
// copy or re-import it elsewhere. It is one of the few non-JSON routes: a file, not a resource.
// @Summary      Exportar o extrato sincronizado da Azul em CSV
// @Description  Baixa o extrato sincronizado da conta como CSV no mesmo formato que a Azul exportava (Conta, Data, Operação, Localizador, Valor, Origem, Destino, Descrição), em ordem de dia. As quatro operações antigas (Resgate, Reembolso, Acúmulo de voo, Acúmulo de pontos) são reimportáveis; expiração e outros saem rotulados.
// @Tags         Estoque de pontos
// @Produce      text/csv
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {string}  string  "CSV do extrato"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao gerar o CSV"
// @Router       /accounts/{id}/azul-statement/export [get]
func (h *StatementHandler) ExportAzulStatement(w http.ResponseWriter, r *http.Request) {
	account, ok := h.azulAccount(w, r)
	if !ok {
		return
	}

	var rows []data.AzulStatementLine
	if err := h.DB.Where("account_id = ?", account.ID).
		Order("occurred_on ASC, id ASC").
		Find(&rows).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o extrato: "+err.Error())
		return
	}

	cityCodes, err := data.LoadAzulCityCodes(h.DB)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler os códigos de cidade: "+err.Error())
		return
	}

	accountNumber := derefString(account.Login)
	filename := fmt.Sprintf("extrato-azul-%s.csv", account.ID)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	// A UTF-8 BOM so a spreadsheet opens the accents right; the old importer strips it, so a
	// re-import still works.
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}

	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"Conta", "Data", "Operação", "Localizador", "Valor", "Origem", "Destino", "Descrição"})
	for _, row := range rows {
		_ = writer.Write([]string{
			accountNumber,
			row.OccurredOn.Format("02/01/2006"),
			azulOperationLabel(row),
			effectiveLocator(row),
			strconv.Itoa(row.Points),
			azul.ResolveCity(cityCodes, derefString(row.Origin)),
			azul.ResolveCity(cityCodes, derefString(row.Destination)),
			derefString(row.Description),
		})
	}
	writer.Flush()
}

// azulOperationLabel is the Portuguese operation word for a line, matching the old export's
// vocabulary so the four known ones re-import cleanly. Flight accrual and a point purchase are
// distinguished by the type code, the way the old CSV separated "Acúmulo de voo" from "de pontos".
func azulOperationLabel(row data.AzulStatementLine) string {
	switch statementKind(row) {
	case data.AzulLineRedemption:
		return "Resgate"
	case data.AzulLineRefund:
		return "Reembolso"
	case data.AzulLineAccrual:
		if strings.EqualFold(strings.TrimSpace(derefString(row.TypeCode)), "AI") {
			return "Acúmulo de voo"
		}
		return "Acúmulo de pontos"
	case data.AzulLinePurchase:
		return "Acúmulo de pontos"
	case data.AzulLineExpiration:
		return "Expiração"
	default:
		return "Outro"
	}
}

// AssignAzulCityCode handles POST /azul-city-codes
//
// Assigns (or reassigns) the IATA code for a statement place name, so a redemption route stops
// showing the raw city and shows the code everywhere. Global, not account-scoped: the mapping is
// the same for every account. Admin-only.
// @Summary      Atribuir o código IATA de uma cidade do extrato
// @Description  Define o código IATA de um nome de cidade que aparece no extrato da Azul, para o extrato mostrar o código em vez do nome cru. Vale para todas as contas. Reatribuir sobrescreve.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  schemas.AzulCityCodeRequest  true  "Cidade e código"
// @Success      200  {object}  schemas.AzulCityCodeSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Corpo inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao salvar"
// @Router       /azul-city-codes [post]
func (h *StatementHandler) AssignAzulCityCode(w http.ResponseWriter, r *http.Request) {
	var body schemas.AzulCityCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Corpo inválido")
		return
	}
	if err := h.Validator.Struct(body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Dados inválidos: o código precisa ter 3 letras")
		return
	}
	iata := strings.ToUpper(strings.TrimSpace(body.IATA))
	city := strings.TrimSpace(body.City)
	if city == "" {
		utils.SendError(w, r, http.StatusBadRequest, "Informe a cidade")
		return
	}

	if err := data.UpsertAzulCityCode(h.DB, city, iata); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui salvar o código: "+err.Error())
		return
	}
	utils.SendJSON(w, r, http.StatusOK, "Código atribuído", schemas.AzulCityCodeResponse{City: city, IATA: iata})
}

// LinkAzulBooking handles POST /accounts/{id}/azul-statement/links
//
// Pins a statement locator to a real issuance, overriding the automatic match. Only reachable
// from the statement page's edit mode; the pinned issuance must belong to the account.
// @Summary      Vincular manualmente uma linha do extrato a uma emissão
// @Description  Fixa o vínculo de um localizador do extrato a uma emissão real da conta, substituindo o vínculo automático. A emissão precisa ser da própria conta. Usado pelo modo de edição da página de extrato.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                          true  "Conta"
// @Param        request  body  schemas.AzulBookingLinkRequest  true  "Localizador e emissão"
// @Success      200  {object}  schemas.AzulBookingLinkSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Corpo inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio ou emissão não é da conta"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao salvar o vínculo"
// @Router       /accounts/{id}/azul-statement/links [post]
func (h *StatementHandler) LinkAzulBooking(w http.ResponseWriter, r *http.Request) {
	account, ok := h.azulAccount(w, r)
	if !ok {
		return
	}

	var body schemas.AzulBookingLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Corpo inválido")
		return
	}
	if err := h.Validator.Struct(body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Dados inválidos: "+err.Error())
		return
	}

	// The issuance must belong to this account: a locator on one account cannot be pinned to
	// another account's booking. Cheaper and clearer than trusting the client's scoping.
	var issuance data.Issuance
	if err := h.DB.First(&issuance, "id = ? AND account_id = ?", body.IssuanceID, account.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Emissão não encontrada nesta conta")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui buscar a emissão: "+err.Error())
		return
	}

	var createdBy *string
	if user, ok := r.Context().Value(middleware.UserContextKey).(data.User); ok {
		createdBy = &user.ID
	}
	if err := statement.UpsertBookingLink(h.DB, account.ID, body.Locator, body.IssuanceID, createdBy); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui salvar o vínculo: "+err.Error())
		return
	}

	h.respondBookingLink(w, r, account.ID, body.Locator, "Vínculo salvo")
}

// UnlinkAzulBooking handles DELETE /accounts/{id}/azul-statement/links
//
// Removes a manual pin, handing the locator back to the automatic match. The response reports
// how the locator resolves now (the automatic match, or null when there is none).
// @Summary      Remover o vínculo manual de uma linha do extrato
// @Description  Apaga o vínculo manual de um localizador, devolvendo-o ao vínculo automático por localizador. A resposta traz como o localizador passa a resolver (o automático, ou nulo se não houver).
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                                true  "Conta"
// @Param        request  body  schemas.AzulBookingLinkDeleteRequest  true  "Localizador"
// @Success      200  {object}  schemas.AzulBookingLinkSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Corpo inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao remover o vínculo"
// @Router       /accounts/{id}/azul-statement/links [delete]
func (h *StatementHandler) UnlinkAzulBooking(w http.ResponseWriter, r *http.Request) {
	account, ok := h.azulAccount(w, r)
	if !ok {
		return
	}

	var body schemas.AzulBookingLinkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Corpo inválido")
		return
	}
	if err := h.Validator.Struct(body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Dados inválidos: "+err.Error())
		return
	}

	if err := statement.DeleteBookingLink(h.DB, account.ID, body.Locator); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui remover o vínculo: "+err.Error())
		return
	}

	h.respondBookingLink(w, r, account.ID, body.Locator, "Vínculo removido")
}

// respondBookingLink resolves one locator and answers with how it now maps, shared by the link
// and unlink routes so the page can update the line in place without a full refetch.
func (h *StatementHandler) respondBookingLink(w http.ResponseWriter, r *http.Request, accountID, locator, message string) {
	resolved, err := statement.ResolveBookingLinks(h.DB, accountID, []string{locator})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui resolver o vínculo: "+err.Error())
		return
	}
	response := schemas.AzulBookingLinkResponse{Locator: locator}
	if booking, found := resolved[locator]; found {
		response.LinkedIssuance = toLinkedIssuance(booking)
	}
	utils.SendJSON(w, r, http.StatusOK, message, response)
}

// LinkAzulBatch handles POST /accounts/{id}/azul-statement/batch-links
//
// Pins a point-entry line (a purchase or accrual) to a batch, overriding the automatic match.
// The batch must belong to the account. Reached from the statement page's edit mode.
// @Summary      Vincular manualmente uma linha de entrada a um lote
// @Description  Fixa o vínculo de uma linha de compra ou acúmulo a um lote da conta, substituindo o vínculo automático (que casa pelos créditos que o lote guardou). O lote precisa ser da própria conta.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                         true  "Conta"
// @Param        request  body  schemas.AzulBatchLinkRequest   true  "Chave da linha e lote"
// @Success      200  {object}  schemas.AzulBatchLinkSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Corpo inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio ou lote não é da conta"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao salvar o vínculo"
// @Router       /accounts/{id}/azul-statement/batch-links [post]
func (h *StatementHandler) LinkAzulBatch(w http.ResponseWriter, r *http.Request) {
	account, ok := h.azulAccount(w, r)
	if !ok {
		return
	}

	var body schemas.AzulBatchLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Corpo inválido")
		return
	}
	if err := h.Validator.Struct(body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Dados inválidos: "+err.Error())
		return
	}

	// The batch must belong to this account.
	var batch data.PointBatch
	if err := h.DB.First(&batch, "id = ? AND account_id = ?", body.BatchID, account.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Lote não encontrado nesta conta")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui buscar o lote: "+err.Error())
		return
	}

	var createdBy *string
	if user, ok := r.Context().Value(middleware.UserContextKey).(data.User); ok {
		createdBy = &user.ID
	}
	if err := statement.UpsertBatchLink(h.DB, account.ID, body.Key, body.BatchID, createdBy); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui salvar o vínculo: "+err.Error())
		return
	}

	h.respondBatchLink(w, r, account.ID, body.Key, body.OccurredOn, body.Points, "Vínculo salvo")
}

// UnlinkAzulBatch handles DELETE /accounts/{id}/azul-statement/batch-links
//
// Removes a manual batch pin, handing the line back to the automatic match.
// @Summary      Remover o vínculo manual de uma linha de entrada com o lote
// @Description  Apaga o vínculo manual de uma linha de compra ou acúmulo, devolvendo-a ao casamento automático por créditos. A resposta traz como a linha passa a resolver.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                              true  "Conta"
// @Param        request  body  schemas.AzulBatchLinkDeleteRequest  true  "Chave da linha"
// @Success      200  {object}  schemas.AzulBatchLinkSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Corpo inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao remover o vínculo"
// @Router       /accounts/{id}/azul-statement/batch-links [delete]
func (h *StatementHandler) UnlinkAzulBatch(w http.ResponseWriter, r *http.Request) {
	account, ok := h.azulAccount(w, r)
	if !ok {
		return
	}

	var body schemas.AzulBatchLinkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Corpo inválido")
		return
	}
	if err := h.Validator.Struct(body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Dados inválidos: "+err.Error())
		return
	}

	if err := statement.DeleteBatchLink(h.DB, account.ID, body.Key); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui remover o vínculo: "+err.Error())
		return
	}

	h.respondBatchLink(w, r, account.ID, body.Key, body.OccurredOn, body.Points, "Vínculo removido")
}

// respondBatchLink resolves one line against the batches and answers with how it now maps,
// shared by the batch link and unlink routes so the page can update the line in place. The
// day/points come from the request so an automatic match still resolves after an unlink.
func (h *StatementHandler) respondBatchLink(w http.ResponseWriter, r *http.Request, accountID, key, occurredOn string, points int, message string) {
	ref := statement.BatchLineRef{Key: key, Points: points}
	if day, err := time.Parse(batchDateLayout, occurredOn); err == nil {
		ref.OccurredOn = day
	}
	resolved, err := statement.ResolveBatchLinks(h.DB, accountID, []statement.BatchLineRef{ref})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui resolver o vínculo: "+err.Error())
		return
	}
	response := schemas.AzulBatchLinkResponse{Key: key}
	if batch, found := resolved[key]; found {
		response.LinkedBatch = toLinkedBatch(batch)
	}
	utils.SendJSON(w, r, http.StatusOK, message, response)
}

// statementKind derives a line's kind from its type code, partner/description and comment, the
// single place the page's kind comes from.
func statementKind(row data.AzulStatementLine) string {
	return data.AzulKindFromTypeCode(
		derefString(row.TypeCode),
		derefString(row.PartnerName)+" "+derefString(row.TypeDescription),
		derefString(row.Comment),
	)
}

// statementLineKey is the key a line is linked by. A line with a booking reference is keyed by
// its locator - so it auto-matches an issuance and so a refund keyed by the same PNR as its
// redemption lands on the same booking. A locator-less line (a promotion) is keyed by a stable
// signature of its content instead, which never auto-matches but can be pinned by hand. Two
// identical locator-less movements on one day share a signature, and so a link; that is rare and
// preferred over a row id, which the window replacement changes on every sync.
// effectiveLocator is the row's stored locator, or - for a row synced before the parser learned a
// comment format - the locator re-derived from the stored comment. This keeps Azul pelo Mundo
// ("Debit Points Pnr: …") and Azul Viagens ("Juniper Azul Viagens | … - …") redemptions keyed and
// displayed by their PNR without waiting for a re-sync, the same way the kind is re-derived above.
func effectiveLocator(row data.AzulStatementLine) string {
	if row.Locator != nil && *row.Locator != "" {
		return *row.Locator
	}
	return azul.LocatorFromComment(derefString(row.Comment))
}

func statementLineKey(row data.AzulStatementLine) string {
	if loc := effectiveLocator(row); loc != "" {
		return loc
	}
	return fmt.Sprintf("sig:%s:%s:%d",
		row.OccurredOn.Format(batchDateLayout), derefString(row.TypeCode), row.Points)
}

// toLinkedIssuance summarises a resolved booking for the statement page.
func toLinkedIssuance(booking statement.ResolvedBooking) *schemas.AzulStatementLinkedIssuance {
	issuance := booking.Issuance
	return &schemas.AzulStatementLinkedIssuance{
		IssuanceID:    issuance.ID,
		PassengerName: derefString(issuance.MainPassengerName),
		Origin:        derefString(issuance.OriginIATACode),
		Destination:   derefString(issuance.DestinationIATACode),
		IssuedOn:      issuance.CreatedAt.Format(batchDateLayout),
		Points:        issuance.Points,
		Manual:        booking.Manual,
	}
}

// toLinkedBatch summarises a resolved batch (lote) for the statement page.
func toLinkedBatch(resolved statement.ResolvedBatch) *schemas.AzulStatementLinkedBatch {
	batch := resolved.Batch
	origin := ""
	if batch.Origin != nil {
		origin = batch.Origin.Name
	}
	return &schemas.AzulStatementLinkedBatch{
		BatchID:    batch.ID,
		Origin:     origin,
		AcquiredOn: batch.AcquiredOn.Format(batchDateLayout),
		Points:     batch.Points,
		CostCents:  batch.CostCents,
		Manual:     resolved.Manual,
	}
}
