package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/inventory"
	"github.com/murandido/m-80-backend/internal/statement"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// statementMaxBatches bounds what one commit can write. A month brings a dozen credits; a
// request asking for hundreds is not a statement being reconciled.
const statementMaxBatches = 200

type StatementHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
	// Tracker is the shared connect-in-flight registry, set on the Azul routes so the sync-state
	// badge can report "conectando". Nil on the Smiles routes, which never connect - IsConnecting
	// is nil-safe.
	Tracker *azulsync.ConnectTracker
}

func NewStatementHandler(db *gorm.DB) *StatementHandler {
	return &StatementHandler{DB: db, Validator: validator.New()}
}

// PreviewAzulStatement handles GET /accounts/{id}/azul-statement
//
// There is no upload: the statement is the window the sync wrote into azul_statement_lines.
// Reads it, compares it with the panel over its own period, and writes nothing.
// @Summary      Conciliar extrato sincronizado da Azul
// @Description  Concilia o extrato que o coletor sincronizou da Azul (tabela azul_statement_lines) com o que o painel tem da conta, no período que a janela sincronizada cobre. Não há upload — a Azul deixou de subir CSV. Devolve as três listas com o veredito de cada linha e nada é gravado. Crédito sem lote é o achado que importa. Conta sem janela sincronizada devolve uma comparação vazia.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "Conta a conciliar"
// @Success      200  {object}  schemas.AzulStatementSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao conciliar"
// @Router       /accounts/{id}/azul-statement [get]
func (h *StatementHandler) PreviewAzulStatement(w http.ResponseWriter, r *http.Request) {
	account, ok := h.inventoryAccount(w, r)
	if !ok {
		return
	}

	reconciliation, from, to, err := h.reconcileAzul(account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui conciliar: "+err.Error())
		return
	}

	// The badge rides along with the reconciliation: the panel opens on this one read, so the
	// sync state comes with it rather than as a second round-trip.
	state, err := azulsync.LoadState(h.DB, account.ID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui ler o estado da sincronização: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Extrato conciliado", schemas.AzulStatementResponse{
		SyncState:      h.toAzulSyncStateResponse(account, state),
		Reconciliation: toStatementResponse(statement.Statement{}, reconciliation, from, to),
	})
}

// CommitAzulStatement handles POST /accounts/{id}/azul-statement/commit
//
// Records the chosen credits as batches, exactly like the Smiles commit, but the statement is
// re-read from the synced window rather than from a re-uploaded file, and the selection arrives
// as a JSON body.
// @Summary      Lançar lotes a partir do extrato sincronizado da Azul
// @Description  Soma os créditos marcados que compartilham origem e agência num único lote, datado no crédito mais antigo do grupo. O extrato é relido da janela sincronizada e conciliado de novo do zero, então uma seleção que já deixou de ser um crédito sem lote é pulada em vez de duplicar o lançamento. Ao fim, recalcula o custeio da conta.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                              true  "Conta"
// @Param        request  body  schemas.StatementAzulCommitRequest  true  "Créditos marcados"
// @Success      200  {object}  schemas.StatementCommitSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Seleção inválida"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio ou seleção grande demais"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao lançar os lotes"
// @Router       /accounts/{id}/azul-statement/commit [post]
func (h *StatementHandler) CommitAzulStatement(w http.ResponseWriter, r *http.Request) {
	account, ok := h.inventoryAccount(w, r)
	if !ok {
		return
	}

	var body schemas.StatementAzulCommitRequest
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

	reconciliation, _, _, err := h.reconcileAzul(account.ID)
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

// loadAccount fetches the account named in the URL, answering the client itself when it is not
// there. The two guards below add the question each route actually asks.
func (h *StatementHandler) loadAccount(w http.ResponseWriter, r *http.Request) (data.Account, bool) {
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

// statementAccount is the guard for reading a statement: connecting the account, the badge, the
// lines, the export and the issuance links. It asks whether the account is tracked at all, which
// is a different question from whether its points are ours to price.
func (h *StatementHandler) statementAccount(w http.ResponseWriter, r *http.Request) (data.Account, bool) {
	account, ok := h.loadAccount(w, r)
	if !ok {
		return account, false
	}
	if !account.TracksStatement {
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Esta conta não acompanha extrato. Ligue \"Acompanhar extrato\" no cadastro dela.")
		return account, false
	}
	return account, true
}

// inventoryAccount is the guard for anything that prices points: the reconciliation panel, the
// batch commit and the batch links. Tracking a statement is not enough - a batch on an account
// whose points are not ours would be costing somebody else's acquisition.
func (h *StatementHandler) inventoryAccount(w http.ResponseWriter, r *http.Request) (data.Account, bool) {
	account, ok := h.loadAccount(w, r)
	if !ok {
		return account, false
	}
	if !account.OwnsPointInventory {
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Esta conta não controla estoque próprio de pontos")
		return account, false
	}
	return account, true
}

// reconcileAzul reads the synced window and reconciles it against the panel over its own
// period. A window that has not been synced yet is an empty comparison, not an error.
func (h *StatementHandler) reconcileAzul(
	accountID string,
) (statement.Reconciliation, time.Time, time.Time, error) {
	parsed, err := statement.LoadAzulStatement(h.DB, accountID)
	if err != nil {
		return statement.Reconciliation{}, time.Time{}, time.Time{}, err
	}
	from, to, ok := statement.PeriodOf(parsed)
	if !ok {
		return statement.Reconciliation{}, time.Time{}, time.Time{}, nil
	}

	bookings, batches, err := statement.LoadPanel(h.DB, accountID, from, to)
	if err != nil {
		return statement.Reconciliation{}, from, to, err
	}
	return statement.Reconcile(parsed.Movements, bookings, batches), from, to, nil
}

// recordBatches writes the chosen credits as batches, one per origin and funding agency
// rather than one per credit.
//
// A purchase reaches the statement in instalments - the Inter batch arrives as four credits
// of the same day, flight accruals as one credit per flight across the week - and recording
// each of them separately fills the screen with rows that are one acquisition to everybody
// who reads them. So credits that share the origin and the payer are summed into a single
// batch, dated at the earliest of them: the points are then in the FIFO queue from the day
// the first of them landed, and no issuance made in between goes uncovered.
//
// A selection whose credit is no longer missing is skipped rather than written: the same
// file submitted twice would otherwise duplicate every batch it created the first time.
func (h *StatementHandler) recordBatches(
	account data.Account,
	credits []statement.CreditLine,
	selections []schemas.StatementBatchSelection,
) (schemas.StatementCommitResponse, error) {
	result := schemas.StatementCommitResponse{}

	// Insertion order is kept on purpose: the batches come out in the order the operator
	// saw the credits, not in whatever order a map iterates.
	type group struct {
		batch data.PointBatch
		// lines is where the batch came from: the credits summed into it. Written onto the
		// batch so the same file reconciled again finds them already recorded - a summed
		// batch resembles none of its credits, so date and amount alone cannot.
		lines []statement.BatchLine
		// note is what the operator typed for these credits, if anything.
		note string
		// description is the credits' own wording while they all say the same thing, and
		// empty once two of them disagree - a label that describes half of a batch is
		// worse than none.
		description string
		sameWording bool
	}
	groups := make(map[string]*group)
	order := make([]string, 0, len(selections))

	for _, selection := range selections {
		if selection.Index < 0 || selection.Index >= len(credits) {
			result.Skipped++
			continue
		}
		credit := credits[selection.Index]
		if credit.Verdict != statement.VerdictMissingInPanel {
			result.Skipped++
			continue
		}
		result.Credits++

		key := selection.OriginID + "|" + selection.FundingAgencyID + "|" + selection.Note
		existing, found := groups[key]
		if !found {
			originID := selection.OriginID
			groups[key] = &group{
				batch: data.PointBatch{
					AccountID:          account.ID,
					OriginID:           &originID,
					AcquiredOn:         credit.OccurredOn,
					Points:             credit.Points,
					CostCents:          selection.CostCents,
					FundingAgencyID:    selection.FundingAgencyID,
					ProfitSharePercent: account.DefaultProfitSharePercent,
				},
				lines:       []statement.BatchLine{{OccurredOn: credit.OccurredOn, Points: credit.Points}},
				note:        selection.Note,
				description: credit.Description,
				sameWording: true,
			}
			order = append(order, key)
			continue
		}

		existing.lines = append(existing.lines,
			statement.BatchLine{OccurredOn: credit.OccurredOn, Points: credit.Points})
		existing.batch.Points += credit.Points
		existing.batch.CostCents += selection.CostCents
		if credit.OccurredOn.Before(existing.batch.AcquiredOn) {
			existing.batch.AcquiredOn = credit.OccurredOn
		}
		if credit.Description != existing.description {
			existing.sameWording = false
		}
	}

	rows := make([]data.PointBatch, 0, len(order))
	for _, key := range order {
		entry := groups[key]
		batch := entry.batch
		// The operator's own note wins; failing that, the airline's wording, which is what
		// tells two batches of the same origin apart.
		note := entry.note
		if note == "" && entry.sameWording {
			note = entry.description
		}
		if note != "" {
			batch.Note = &note
		}
		encoded, err := statement.EncodeBatchLines(entry.lines)
		if err != nil {
			return result, err
		}
		batch.StatementLines = encoded
		rows = append(rows, batch)
	}

	if len(rows) == 0 {
		return result, nil
	}
	if err := h.DB.Create(&rows).Error; err != nil {
		return result, err
	}
	result.Created = len(rows)
	return result, nil
}

func toStatementResponse(
	parsed statement.Statement,
	reconciliation statement.Reconciliation,
	from, to time.Time,
) schemas.StatementReconciliationResponse {
	response := schemas.StatementReconciliationResponse{
		AccountNumber:      parsed.AccountNumber,
		From:               formatDayOrEmpty(from),
		To:                 formatDayOrEmpty(to),
		Bookings:           make([]schemas.StatementBookingLine, 0, len(reconciliation.Bookings)),
		Refunds:            make([]schemas.StatementRefundLine, 0, len(reconciliation.Refunds)),
		Credits:            make([]schemas.StatementCreditLine, 0, len(reconciliation.Credits)),
		StatementNetPoints: reconciliation.StatementNetPoints,
		PanelNetPoints:     reconciliation.PanelNetPoints,
	}

	for _, line := range reconciliation.Bookings {
		occurred := ""
		if !line.OccurredOn.IsZero() {
			occurred = line.OccurredOn.Format(batchDateLayout)
		}
		response.Bookings = append(response.Bookings, schemas.StatementBookingLine{
			Locator:         line.Locator,
			Verdict:         string(line.Verdict),
			StatementPoints: line.StatementPoints,
			PanelPoints:     line.PanelPoints,
			IssuanceID:      line.IssuanceID,
			OccurredOn:      occurred,
		})
	}
	for _, line := range reconciliation.Refunds {
		response.Refunds = append(response.Refunds, schemas.StatementRefundLine{
			Locator:         line.Locator,
			Verdict:         string(line.Verdict),
			StatementPoints: line.StatementPoints,
			PanelPoints:     line.PanelPoints,
			IssuanceID:      line.IssuanceID,
		})
	}
	// The index is the position in this list, and it is what the commit takes back. Both
	// routes reconcile the same file the same way, so the numbering agrees.
	for index, line := range reconciliation.Credits {
		response.Credits = append(response.Credits, schemas.StatementCreditLine{
			Index:       index,
			OccurredOn:  line.OccurredOn.Format(batchDateLayout),
			Points:      line.Points,
			Description: line.Description,
			Verdict:     string(line.Verdict),
			BatchID:     line.BatchID,
		})
	}

	return response
}

// toAzulSyncStateResponse turns the stored sync state into what the panel's badge reads, adding
// whether the account has a customerKey at all (account.AzulCustomerKey is never returned in the
// clear, so "connected" is the one bit the panel gets from it). A never-synced account carries a
// zero UpdatedAt, which is reported as null rather than as the epoch.
func (h *StatementHandler) toAzulSyncStateResponse(account data.Account, state *data.AzulSyncState) schemas.AzulSyncStateResponse {
	response := schemas.AzulSyncStateResponse{
		Connected:           account.AzulCustomerKey != nil && *account.AzulCustomerKey != "",
		Connecting:          h.Tracker.IsConnecting(account.ID),
		ConnectingSince:     h.Tracker.ConnectingSince(account.ID),
		State:               state.State,
		LastSyncedAt:        state.LastSyncedAt,
		LastBalancePoints:   state.LastBalancePoints,
		ConsecutiveFailures: state.ConsecutiveFailures,
		Detail:              state.Detail,
	}
	if !state.UpdatedAt.IsZero() {
		response.UpdatedAt = &state.UpdatedAt
	}
	return response
}

// formatDayOrEmpty renders a day, or "" for the zero time - which is what an account whose
// statement has not been synced yet produces (no period to name).
func formatDayOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(batchDateLayout)
}
