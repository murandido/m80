package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/inventory"
	"github.com/murandido/m-80-backend/internal/smilessync"
	"github.com/murandido/m-80-backend/internal/statement"
	"github.com/murandido/m-80-backend/internal/utils"
)

// SmilesConnectStatus handles GET /accounts/{id}/smiles-statement/status
//
// The sync badge on its own - whether the account is connected, how the last round went, the
// last balance and when. It is the cheap read the account card polls; the entries endpoint
// below resolves every link and loads the whole window, which is wasted work for a card that
// only wants to know "is it connected, and did the first sync land".
// @Summary      Estado da conexão/sincronização da conta Smiles
// @Description  Devolve só o "crachá" da sincronização (conectada, estado, último saldo e última sincronização), sem carregar as linhas do extrato. É o que o card da conta consulta em intervalo depois de conectar, para saber quando a primeira sincronização terminou. Conta nunca sincronizada devolve estado "pending", não erro.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.SmilesSyncStateSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao carregar o estado"
// @Router       /accounts/{id}/smiles-statement/status [get]
func (h *StatementHandler) SmilesConnectStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := h.inventoryAccount(w, r)
	if !ok {
		return
	}

	state, err := smilessync.LoadState(h.DB, account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da sincronização: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Estado carregado", toSmilesSyncStateResponse(account, state))
}

// PreviewSmilesStatement handles GET /accounts/{id}/smiles-statement
//
// Reconciles the synced window against the panel and writes nothing.
// @Summary      Conciliar o extrato sincronizado da Smiles
// @Description  Compara o extrato que o sync leu da Smiles com o que o painel tem da conta, no período que a janela cobre. Devolve três listas — emissões, reembolsos e créditos — cada linha com o veredito, mais o estado da sincronização. Nada é gravado. Crédito sem lote é o achado que importa: são milhas que entraram e nunca foram precificadas.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.SmilesStatementSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao conciliar"
// @Router       /accounts/{id}/smiles-statement [get]
func (h *StatementHandler) PreviewSmilesStatement(w http.ResponseWriter, r *http.Request) {
	account, ok := h.inventoryAccount(w, r)
	if !ok {
		return
	}

	reconciliation, from, to, err := h.reconcileSmiles(account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui conciliar: "+err.Error())
		return
	}

	// The badge rides along with the reconciliation: the panel opens on this one read, so the
	// sync state comes with it rather than as a second round-trip.
	state, err := smilessync.LoadState(h.DB, account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da sincronização: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Extrato conciliado", schemas.SmilesStatementResponse{
		SyncState:      toSmilesSyncStateResponse(account, state),
		Reconciliation: toStatementResponse(statement.Statement{}, reconciliation, from, to),
	})
}

// CommitSmilesStatement handles POST /accounts/{id}/smiles-statement/commit
//
// Records the chosen credits as batches. The window is re-read and reconciled from scratch:
// the preview is advice, not contract, so the client sends which credits it wants and what they
// cost - never their contents.
// @Summary      Lançar lotes a partir do extrato da Smiles
// @Description  Soma os créditos marcados que compartilham origem e agência num único lote, datado no crédito mais antigo do grupo — uma compra parcelada em quatro entra como um lote só. O custo informado em cada crédito é somado. A janela é lida e conciliada de novo do zero, então uma seleção que já deixou de ser um crédito sem lote é pulada em vez de duplicar o lançamento. Ao fim, recalcula o custeio da conta.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                               true  "Conta"
// @Param        request  body  schemas.StatementSmilesCommitRequest  true  "Créditos marcados"
// @Success      200  {object}  schemas.StatementCommitSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Seleção inválida"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio ou seleção grande demais"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao lançar os lotes"
// @Router       /accounts/{id}/smiles-statement/commit [post]
func (h *StatementHandler) CommitSmilesStatement(w http.ResponseWriter, r *http.Request) {
	account, ok := h.inventoryAccount(w, r)
	if !ok {
		return
	}

	var body schemas.StatementSmilesCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Seleção de créditos inválida")
		return
	}
	if len(body.Batches) == 0 {
		utils.SendError(w, r, http.StatusBadRequest, "Nenhum crédito marcado")
		return
	}
	if len(body.Batches) > statementMaxBatches {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Créditos demais numa importação só")
		return
	}
	for _, selection := range body.Batches {
		if err := h.Validator.Struct(selection); err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Seleção inválida: "+err.Error())
			return
		}
	}

	reconciliation, _, _, err := h.reconcileSmiles(account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui conciliar: "+err.Error())
		return
	}

	result, err := h.recordBatches(account, reconciliation.Credits, body.Batches)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui lançar os lotes: "+err.Error())
		return
	}

	if result.Created > 0 {
		if err := inventory.Recompute(h.DB, account.ID); err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Lotes lançados, mas o custeio falhou: "+err.Error())
			return
		}
	}

	utils.SendJSON(w, r, http.StatusOK, "Lotes lançados a partir do extrato", result)
}

// SmilesStatementEntries handles GET /accounts/{id}/smiles-statement/entries
//
// The synced statement as a per-account view rather than a reconciliation: the raw movements in
// day order, each redemption/refund resolved to the issuance it maps to, plus the badge. A
// never-synced account is an empty window, not an error.
// @Summary      Extrato sincronizado da Smiles, com vínculo de emissões
// @Description  Lista as linhas do extrato que o sync leu da Smiles, em ordem de dia, cada resgate/estorno já vinculado à emissão real do banco quando o localizador bate, e cada crédito vinculado ao lote correspondente. Traz o estado da sincronização e o período que a janela cobre. Nada é gravado.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta"
// @Success      200  {object}  schemas.SmilesStatementEntriesSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao carregar o extrato"
// @Router       /accounts/{id}/smiles-statement/entries [get]
func (h *StatementHandler) SmilesStatementEntries(w http.ResponseWriter, r *http.Request) {
	account, ok := h.inventoryAccount(w, r)
	if !ok {
		return
	}

	rows, err := h.smilesRows(account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o extrato: "+err.Error())
		return
	}

	// Each line's link key, and - for the miles-in lines - the reference a batch is matched on.
	// The same key is what the panel sends back to link or unlink.
	keys := make([]string, 0, len(rows))
	batchRefs := make([]statement.BatchLineRef, 0, len(rows))
	for _, row := range rows {
		key := row.LineKey()
		keys = append(keys, key)
		if kind := smilesRowKind(row); kind == data.SmilesLineAccrual || kind == data.SmilesLinePurchase {
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

	state, err := smilessync.LoadState(h.DB, account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da sincronização: "+err.Error())
		return
	}

	entries := make([]schemas.SmilesStatementEntry, 0, len(rows))
	var from, to string
	for i, row := range rows {
		day := row.OccurredOn.Format(batchDateLayout)
		if i == 0 {
			from = day
		}
		to = day // rows are ordered ascending, so the last one seen is the window's end

		key := row.LineKey()
		entry := schemas.SmilesStatementEntry{
			Key:        key,
			OccurredOn: day,
			// Derive the kind from the airline's own classification rather than trusting the
			// stored one: the mapping may be newer than a row synced before it, and this keeps
			// the page correct without waiting for a re-sync.
			Kind:            smilesRowKind(row),
			TypeCode:        row.TypeCode,
			SubType:         row.SubType,
			TypeDescription: row.TypeDescription,
			Points:          row.Points,
			Locator:         derefString(row.Locator),
			Origin:          derefString(row.Origin),
			Destination:     derefString(row.Destination),
			PartnerName:     derefString(row.PartnerName),
			Description:     derefString(row.Description),
			Channel:         derefString(row.Channel),
			DayBalance:      row.DayBalance,
		}
		if booking, found := resolved[key]; found {
			entry.LinkedIssuance = toLinkedIssuance(booking)
		}
		if batch, found := resolvedBatches[key]; found {
			entry.LinkedBatch = toLinkedBatch(batch)
		}
		entries = append(entries, entry)
	}

	utils.SendJSON(w, r, http.StatusOK, "Extrato carregado", schemas.SmilesStatementEntriesResponse{
		SyncState: toSmilesSyncStateResponse(account, state),
		From:      from,
		To:        to,
		Lines:     entries,
	})
}

// ExportSmilesStatement handles GET /accounts/{id}/smiles-statement/export
//
// The synced window as a CSV, in the same column shape the Azul export uses so an operator
// works with one file format whichever programme the account belongs to. One of the few
// non-JSON routes: a file, not a resource.
// @Summary      Exportar o extrato sincronizado da Smiles em CSV
// @Description  Baixa o extrato sincronizado da conta como CSV (Conta, Data, Operação, Localizador, Valor, Origem, Destino, Descrição), em ordem de dia — as mesmas colunas do export da Azul, para o operador lidar com um formato só.
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
// @Router       /accounts/{id}/smiles-statement/export [get]
func (h *StatementHandler) ExportSmilesStatement(w http.ResponseWriter, r *http.Request) {
	account, ok := h.inventoryAccount(w, r)
	if !ok {
		return
	}

	rows, err := h.smilesRows(account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o extrato: "+err.Error())
		return
	}

	// Not utils.SendJSON: this route answers with a file, so the envelope has no meaning here.
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", fmt.Sprintf("extrato-smiles-%s.csv", account.ID)))
	// The byte-order mark is what makes Excel open a UTF-8 CSV with the accents intact.
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}

	writer := csv.NewWriter(w)
	defer writer.Flush()
	header := []string{"Conta", "Data", "Operação", "Localizador", "Valor", "Origem", "Destino", "Descrição"}
	if err := writer.Write(header); err != nil {
		return
	}
	accountNumber := derefString(account.Login)
	for _, row := range rows {
		record := []string{
			accountNumber,
			row.OccurredOn.Format("02/01/2006"),
			smilesOperationLabel(row),
			derefString(row.Locator),
			strconv.Itoa(row.Points),
			derefString(row.Origin),
			derefString(row.Destination),
			derefString(row.Description),
		}
		if err := writer.Write(record); err != nil {
			return
		}
	}
}

// --- shared pieces ---

// smilesRows reads the whole stored window in day order.
func (h *StatementHandler) smilesRows(accountID string) ([]data.SmilesStatementLine, error) {
	var rows []data.SmilesStatementLine
	err := h.DB.Where("account_id = ?", accountID).
		Order("occurred_on ASC, id ASC").
		Find(&rows).Error
	return rows, err
}

// smilesRowKind re-derives a row's kind from the airline's own words, so a mapping rule added
// after a row was synced takes effect without a re-sync.
func smilesRowKind(row data.SmilesStatementLine) string {
	return data.SmilesKindFromType(
		derefString(row.TypeCode),
		derefString(row.SubType),
		derefString(row.TypeDescription),
		derefString(row.PartnerName),
	)
}

// reconcileSmiles reads the synced window and reconciles it against the panel over its own
// period. A window that has not been synced yet is an empty comparison, not an error.
func (h *StatementHandler) reconcileSmiles(accountID string) (statement.Reconciliation, time.Time, time.Time, error) {
	parsed, err := statement.LoadSmilesStatement(h.DB, accountID)
	if err != nil {
		return statement.Reconciliation{}, time.Time{}, time.Time{}, err
	}
	from, to, ok := statement.PeriodOf(parsed)
	if !ok {
		return statement.Reconciliation{}, time.Time{}, time.Time{}, nil
	}
	bookings, batches, err := statement.LoadPanel(h.DB, accountID, from, to)
	if err != nil {
		return statement.Reconciliation{}, time.Time{}, time.Time{}, err
	}
	return statement.Reconcile(parsed.Movements, bookings, batches), from, to, nil
}

// smilesOperationLabel is the Portuguese operation word for a line, matching the Azul export's
// vocabulary so both programmes' files read the same.
func smilesOperationLabel(row data.SmilesStatementLine) string {
	switch smilesRowKind(row) {
	case data.SmilesLineRedemption:
		return "Resgate"
	case data.SmilesLineRefund:
		return "Reembolso"
	case data.SmilesLineAccrual:
		return "Acúmulo de voo"
	case data.SmilesLinePurchase:
		return "Acúmulo de pontos"
	default:
		return "Outro"
	}
}

// toSmilesSyncStateResponse maps the stored state onto the badge the panel draws. Unlike the
// Azul one it has no "connecting" to report: every step of a Smiles connect answers within a
// request, so there is no background login for the panel to wait on.
func toSmilesSyncStateResponse(account data.Account, state *data.SmilesSyncState) schemas.SmilesSyncStateResponse {
	response := schemas.SmilesSyncStateResponse{
		Connected:           account.SmilesRefreshToken != nil && *account.SmilesRefreshToken != "",
		State:               state.State,
		LastSyncedAt:        state.LastSyncedAt,
		LastBalancePoints:   state.LastBalanceMiles,
		ConsecutiveFailures: state.ConsecutiveFailures,
		Detail:              state.Detail,
	}
	if !state.UpdatedAt.IsZero() {
		response.UpdatedAt = &state.UpdatedAt
	}
	return response
}
