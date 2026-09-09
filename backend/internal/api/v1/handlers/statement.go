package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/inventory"
	"github.com/murandido/m-80-backend/internal/statement"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// A month of Azul's CSV is 14 KB and a month of Smiles' HTML about the same. The ceiling is
// generous against that and still small enough that no request can make the parsers chew
// through an arbitrary amount of memory - the HTML one builds a DOM, and this box runs the
// database too.
const statementMaxUpload = 4 << 20 // 4 MiB

// statementMaxBatches bounds what one commit can write. A month brings a dozen credits; a
// request asking for hundreds is not a statement being reconciled.
const statementMaxBatches = 200

// Statement formats the screen can send.
const (
	statementFormatAzul   = "azul"
	statementFormatSmiles = "smiles"
)

type StatementHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewStatementHandler(db *gorm.DB) *StatementHandler {
	return &StatementHandler{DB: db, Validator: validator.New()}
}

// PreviewStatement handles POST /point-batches/statement/preview
//
// Reads the file, compares it with the panel, and writes nothing.
// @Summary      Conciliar extrato da companhia
// @Description  Lê o extrato enviado (CSV da Azul ou HTML da Smiles) e compara com o que o painel tem da conta, no período que o próprio arquivo cobre. Devolve três listas — emissões, reembolsos e créditos — cada linha com o veredito. Nada é gravado. Crédito sem lote é o achado que importa: são pontos que entraram e nunca foram precificados.
// @Tags         Estoque de pontos
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file       formData  file    true  "Extrato (.csv da Azul ou .html da Smiles)"
// @Param        account_id formData  string  true  "Conta a conciliar"
// @Param        format     formData  string  true  "azul ou smiles"
// @Success      200  {object}  schemas.StatementReconciliationSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Arquivo ausente, grande demais ou de outro formato"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao conciliar"
// @Router       /point-batches/statement/preview [post]
func (h *StatementHandler) PreviewStatement(w http.ResponseWriter, r *http.Request) {
	account, parsed, ok := h.read(w, r)
	if !ok {
		return
	}

	reconciliation, from, to, err := h.reconcile(account.ID, parsed)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui conciliar: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Extrato conciliado",
		toStatementResponse(parsed, reconciliation, from, to))
}

// CommitStatement handles POST /point-batches/statement/commit
//
// Records the chosen credits as batches. The file is read and reconciled again from
// scratch, exactly as the legacy importer does: the preview is advice, not contract, so the
// client sends which credits it wants and what they cost - never their contents.
// @Summary      Lançar lotes a partir do extrato
// @Description  Soma os créditos marcados que compartilham origem e agência num único lote, datado no crédito mais antigo do grupo — uma compra parcelada em quatro entra como um lote só. O custo informado em cada crédito é somado. O arquivo é lido e conciliado de novo do zero, então uma seleção que já deixou de ser um crédito sem lote é pulada em vez de duplicar o lançamento. Ao fim, recalcula o custeio da conta.
// @Tags         Estoque de pontos
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file       formData  file    true  "O mesmo extrato da prévia"
// @Param        account_id formData  string  true  "Conta"
// @Param        format     formData  string  true  "azul ou smiles"
// @Param        batches    formData  string  true  "JSON com os créditos marcados ([{index, origin_id, cost_cents, funding_agency_id, note}])"
// @Success      200  {object}  schemas.StatementCommitSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Arquivo ou seleção inválidos"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio ou seleção grande demais"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao lançar os lotes"
// @Router       /point-batches/statement/commit [post]
func (h *StatementHandler) CommitStatement(w http.ResponseWriter, r *http.Request) {
	account, parsed, ok := h.read(w, r)
	if !ok {
		return
	}

	var selections []schemas.StatementBatchSelection
	if err := json.Unmarshal([]byte(r.FormValue("batches")), &selections); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Seleção de créditos inválida")
		return
	}
	if len(selections) == 0 {
		utils.SendError(w, r, http.StatusBadRequest, "Nenhum crédito marcado")
		return
	}
	if len(selections) > statementMaxBatches {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Créditos demais numa importação só")
		return
	}
	for _, selection := range selections {
		if err := h.Validator.Struct(selection); err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Seleção inválida: "+err.Error())
			return
		}
	}

	reconciliation, _, _, err := h.reconcile(account.ID, parsed)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui conciliar: "+err.Error())
		return
	}

	result, err := h.recordBatches(account, reconciliation.Credits, selections)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Não consegui lançar os lotes: "+err.Error())
		return
	}

	// The point of doing it here: the pending points the operator came to fix disappear in
	// the same action, rather than on whatever later request happens to recompute.
	if result.Created > 0 {
		if err := inventory.Recompute(h.DB, account.ID); err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Lotes lançados, mas o custeio falhou: "+err.Error())
			return
		}
	}

	utils.SendJSON(w, r, http.StatusOK, "Lotes lançados a partir do extrato", result)
}

// read does what both routes share: bound and read the upload, parse it in the format the
// screen said, and load the account. It answers the client itself on failure.
func (h *StatementHandler) read(w http.ResponseWriter, r *http.Request) (data.Account, statement.Statement, bool) {
	var account data.Account
	var parsed statement.Statement

	r.Body = http.MaxBytesReader(w, r.Body, statementMaxUpload)
	//nolint:gosec // G120: the body is bounded on the line above - MaxBytesReader caps the
	// total read, so the parse cannot pull in more than statementMaxUpload however the
	// parts are laid out. gosec does not follow the wrap through this helper's parameters.
	if err := r.ParseMultipartForm(statementMaxUpload); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Não consegui ler o envio. O extrato precisa ter menos de 4 MB")
		return account, parsed, false
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Envie o extrato no campo \"file\"")
		return account, parsed, false
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Não consegui ler o arquivo enviado")
		return account, parsed, false
	}

	switch r.FormValue("format") {
	case statementFormatAzul:
		parsed, err = statement.ParseAzul(content)
	case statementFormatSmiles:
		parsed, err = statement.ParseSmiles(content)
	default:
		utils.SendError(w, r, http.StatusBadRequest, "Informe o formato: azul ou smiles")
		return account, parsed, false
	}
	if err != nil {
		// The two "not this format" errors are the common mistake - the Azul file sent as
		// Smiles, or a spreadsheet sent by accident - and deserve the plain sentence.
		if errors.Is(err, statement.ErrNotAzulStatement) || errors.Is(err, statement.ErrNotSmilesStatement) {
			utils.SendError(w, r, http.StatusBadRequest,
				"O arquivo não é um extrato desse programa. Confira o formato escolhido.")
			return account, parsed, false
		}
		utils.SendError(w, r, http.StatusBadRequest, "Não consegui ler o extrato: "+err.Error())
		return account, parsed, false
	}
	if len(parsed.Movements) == 0 {
		utils.SendError(w, r, http.StatusBadRequest, "O extrato não tem nenhuma movimentação")
		return account, parsed, false
	}

	if err := h.DB.First(&account, "id = ?", r.FormValue("account_id")).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
			return account, parsed, false
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch account: "+err.Error())
		return account, parsed, false
	}
	if !account.OwnsPointInventory {
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Esta conta não controla estoque próprio de pontos")
		return account, parsed, false
	}

	return account, parsed, true
}

func (h *StatementHandler) reconcile(
	accountID string,
	parsed statement.Statement,
) (statement.Reconciliation, time.Time, time.Time, error) {
	from, to, _ := statement.PeriodOf(parsed)

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
		From:               from.Format(batchDateLayout),
		To:                 to.Format(batchDateLayout),
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
