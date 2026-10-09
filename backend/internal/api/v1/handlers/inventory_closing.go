package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/inventory"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type InventoryClosingHandler struct {
	DB *gorm.DB
}

func NewInventoryClosingHandler(db *gorm.DB) *InventoryClosingHandler {
	return &InventoryClosingHandler{DB: db}
}

// GetInventoryClosing handles the GET /inventory-closing route.
//
// Several accounts at once because a holder owns one per programme - Marlon/Azul and
// Marlon/Smiles are two accounts - and the settlement is paid on their sum. Each is
// reported on its own so the figures can be checked, and consolidated underneath so the
// transfer can be made. The spreadsheet could only do the first half: its tabs each
// carried a total and adding them up was done by hand.
//
// Recomputes before reading. The report is what the closing button will freeze, so it has
// to describe the rows as they are now, not as they were the last time something ran.
// @Summary      Fechamento do estoque de pontos
// @Description  Lucro por lote, acerto entre agências e pendências, por conta e consolidado. Não congela nada — para isso, feche o período.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        account_ids query string true  "IDs das contas, separados por vírgula"
// @Param        from        query string true  "Início do período (AAAA-MM-DD)"
// @Param        to          query string true  "Fim do período (AAAA-MM-DD)"
// @Success      200  {object}  schemas.InventoryClosingListResponse
// @Failure      400  {object}  schemas.ErrorResponse "Parâmetros ausentes ou inválidos"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao calcular o fechamento"
// @Router       /inventory-closing [get]
func (h *InventoryClosingHandler) GetInventoryClosing(w http.ResponseWriter, r *http.Request) {
	accountIDs := splitIDs(r.URL.Query().Get("account_ids"))
	if len(accountIDs) == 0 {
		utils.SendError(w, r, http.StatusBadRequest, "Informe account_ids")
		return
	}

	startsOn, err := time.Parse(batchDateLayout, r.URL.Query().Get("from"))
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Data inicial inválida: use o formato AAAA-MM-DD")
		return
	}
	endsOn, err := time.Parse(batchDateLayout, r.URL.Query().Get("to"))
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Data final inválida: use o formato AAAA-MM-DD")
		return
	}

	scope := middleware.AgencyScopeFrom(r.Context())

	names, err := h.agencyNames()
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch agencies: "+err.Error())
		return
	}
	accountLabels, err := h.accountLabels(accountIDs)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch accounts: "+err.Error())
		return
	}
	batchLabels, err := h.batchLabels(accountIDs)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch batches: "+err.Error())
		return
	}

	data := schemas.InventoryClosingDataResponse{Accounts: []schemas.InventoryClosingResponse{}}
	// Netted across accounts, the same way ReadClosing nets across batches: one transfer
	// per pair of agencies, not one per account per batch.
	var pointSettlements, feeSettlements []inventory.Settlement
	var accountParts [][]inventory.SettlementPart
	var accountAgencies []inventory.BatchResult

	// The panel's own figure, and what somebody last read off the airline's page. Gathered
	// for the whole report in one go, after the recompute below has had its say on the
	// batches - the balance formula does not depend on the allocations, but the closing
	// the operator is looking at does.
	balances, err := inventory.ExpectedBalances(h.DB, accountIDs)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to compute the balances: "+err.Error())
		return
	}
	readings, err := h.balanceReadings(accountIDs)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to read the balances: "+err.Error())
		return
	}

	for _, accountID := range accountIDs {
		if err := inventory.Recompute(h.DB, accountID); err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to recompute: "+err.Error())
			return
		}

		closing, err := inventory.ReadClosing(h.DB, accountID, startsOn, endsOn)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to read the closing: "+err.Error())
			return
		}
		closing = scopeClosing(closing, scope)

		closedPeriodID, err := h.closedPeriodID(accountID, endsOn)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the period: "+err.Error())
			return
		}

		account := toClosingResponse(closing, closedPeriodID, names, batchLabels, accountLabels)
		account.ExpectedBalancePoints = balances[accountID]
		if reading, ok := readings[accountID]; ok {
			account.RealBalancePoints = reading.points
			account.RealBalanceCheckedOn = reading.checkedOn
		}
		data.Accounts = append(data.Accounts, account)

		data.Totals.CostCents += closing.CostCents
		data.Totals.RevenueCents += closing.RevenueCents
		data.Totals.PendingPoints += closing.PendingPoints
		data.Totals.RedemptionFeeSavedCents += closing.RedemptionFeeSavedCents
		pointSettlements = append(pointSettlements, closing.Settlements...)
		feeSettlements = append(feeSettlements, closing.FeeSettlements...)
		accountParts = append(accountParts, closing.Parts)
		// SummariseAgencies adds consumers up across batches; each account's agency totals
		// are handed to it as one batch's worth, to add them up across accounts.
		accountAgencies = append(accountAgencies, inventory.BatchResult{Consumers: closing.Agencies})
	}

	data.Totals.ProfitCents = data.Totals.RevenueCents - data.Totals.CostCents
	data.Totals.Settlements = settlementResponses(inventory.NetSettlements(pointSettlements), names)
	data.Totals.FeeSettlements = settlementResponses(inventory.NetSettlements(feeSettlements), names)
	data.Totals.TotalSettlements = settlementResponses(inventory.NetSettlements(pointSettlements, feeSettlements), names)
	data.Totals.Parts = partResponses(inventory.MergeParts(accountParts...), names)
	data.Totals.Agencies = consumerResponses(inventory.SummariseAgencies(accountAgencies), names)

	utils.SendJSON(w, r, http.StatusOK, "Fechamento calculado com sucesso", data)
}

// batchLabel is what a batch is called on the report: its origin and the day it landed.
// There is no ordinal - the spreadsheet's "Livelo 2" was a manual crutch, and a number
// derived from position would renumber every later batch when one is entered late.
type batchLabel struct {
	OriginName *string
	AcquiredOn time.Time
	SettledAt  *time.Time
}

func (h *InventoryClosingHandler) batchLabels(accountIDs []string) (map[string]batchLabel, error) {
	var batches []data.PointBatch
	if err := h.DB.Preload("Origin").Where("account_id IN ?", accountIDs).Find(&batches).Error; err != nil {
		return nil, err
	}

	labels := make(map[string]batchLabel, len(batches))
	for _, batch := range batches {
		label := batchLabel{AcquiredOn: batch.AcquiredOn, SettledAt: batch.SettledAt}
		if batch.Origin != nil {
			name := batch.Origin.Name
			label.OriginName = &name
		}
		labels[batch.ID] = label
	}
	return labels, nil
}

func (h *InventoryClosingHandler) agencyNames() (map[string]string, error) {
	var agencies []data.Agency
	if err := h.DB.Find(&agencies).Error; err != nil {
		return nil, err
	}

	names := make(map[string]string, len(agencies))
	for _, agency := range agencies {
		names[agency.ID] = agency.Name
	}
	return names, nil
}

// closedPeriodID names the closing that froze the requested range, or nil when it is open.
// The screen needs the id, not just the fact: it is what the reopen button acts on.
func (h *InventoryClosingHandler) closedPeriodID(accountID string, endsOn time.Time) (*string, error) {
	var period data.InventoryPeriod
	err := h.DB.Where("account_id = ? AND closed_at IS NOT NULL AND ends_on >= ?", accountID, endsOn).
		Order("ends_on ASC").First(&period).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &period.ID, nil
}

func toClosingResponse(
	closing inventory.Closing,
	closedPeriodID *string,
	names map[string]string,
	labels map[string]batchLabel,
	accountLabels map[string]string,
) schemas.InventoryClosingResponse {
	response := schemas.InventoryClosingResponse{
		AccountID:        closing.AccountID,
		StartsOn:         closing.StartsOn.Format(batchDateLayout),
		EndsOn:           closing.EndsOn.Format(batchDateLayout),
		Closed:           closedPeriodID != nil,
		ClosedPeriodID:   closedPeriodID,
		UnsupportedSplit: closing.UnsupportedSplit,
		Batches:          make([]schemas.InventoryBatchResultResponse, 0, len(closing.Batches)),
		PendingPoints:    closing.PendingPoints,
		CostCents:        closing.CostCents,
		RevenueCents:     closing.RevenueCents,
		ProfitCents:      closing.ProfitCents,
		Settlements:      settlementResponses(closing.Settlements, names),

		RedemptionFeeSavedCents: closing.RedemptionFeeSavedCents,
		FeeSettlements:          settlementResponses(closing.FeeSettlements, names),
		TotalSettlements:        settlementResponses(closing.TotalSettlements, names),
		Agencies:                consumerResponses(closing.Agencies, names),
		Parts:                   partResponses(closing.Parts, names),
	}

	for _, batch := range closing.Batches {
		line := schemas.InventoryBatchResultResponse{
			BatchID:            batch.BatchID,
			FundingAgencyID:    batch.FundingAgencyID,
			FundingAgencyName:  nameOf(names, batch.FundingAgencyID),
			ProfitSharePercent: batch.ProfitSharePercent,
			Points:             batch.Points,
			CostCents:          batch.CostCents,
			ConsumedPoints:     batch.ConsumedPoints,
			ChargedCents:       batch.ChargedCents,
			RevenueCents:       batch.RevenueCents,
			ProfitCents:        batch.ProfitCents,
			RemainingPoints:    batch.RemainingPoints,
			Consumers:          make([]schemas.InventoryConsumerResponse, 0, len(batch.Consumers)),
			Settlements:        settlementResponses(batch.Settlements, names),

			RedemptionFeeSavedCents: batch.RedemptionFeeSavedCents,
			FeeSettlements:          settlementResponses(batch.FeeSettlements, names),
			TotalSettlements:        settlementResponses(batch.TotalSettlements, names),
			Lines:                   make([]schemas.InventoryBatchLineResponse, 0, len(batch.Lines)),
			Parts:                   partResponses(batch.Parts, names),
		}

		if label, ok := labels[batch.BatchID]; ok {
			line.OriginName = label.OriginName
			line.AcquiredOn = label.AcquiredOn.Format(batchDateLayout)
			if label.SettledAt != nil {
				settled := label.SettledAt.Format(time.RFC3339)
				line.SettledAt = &settled
			}
		}

		line.Consumers = append(line.Consumers, consumerResponses(batch.Consumers, names)...)
		for _, slice := range batch.Lines {
			line.Lines = append(line.Lines, schemas.InventoryBatchLineResponse{
				IssuanceID:              slice.IssuanceID,
				ProgramLocator:          slice.ProgramLocator,
				OccurredOn:              slice.OccurredOn.Format(batchDateLayout),
				AgencyID:                slice.AgencyID,
				AgencyName:              nameOf(names, slice.AgencyID),
				Points:                  slice.Points,
				CostCents:               slice.CostCents,
				RevenueCents:            slice.RevenueCents,
				ProfitCents:             slice.RevenueCents - slice.CostCents,
				RedemptionFeeSavedCents: slice.RedemptionFeeSavedCents,
			})
		}

		response.Batches = append(response.Batches, line)
	}

	if label, ok := accountLabels[closing.AccountID]; ok && label != "" {
		response.AccountLabel = &label
	}

	return response
}

// balanceReading is what somebody last read off the airline's page for one account.
type balanceReading struct {
	points    *int
	checkedOn *string
}

// balanceReadings loads those readings for the accounts in the report. One query, and an
// account nobody has checked is simply absent - which the screen renders as "não conferido"
// rather than as a zero.
func (h *InventoryClosingHandler) balanceReadings(accountIDs []string) (map[string]balanceReading, error) {
	readings := make(map[string]balanceReading, len(accountIDs))
	if len(accountIDs) == 0 {
		return readings, nil
	}

	var rows []data.Account
	err := h.DB.Select("id", "real_balance_points", "real_balance_checked_on").
		Where("id IN ?", accountIDs).Find(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if row.RealBalancePoints == nil {
			continue
		}
		reading := balanceReading{points: row.RealBalancePoints}
		if row.RealBalanceCheckedOn != nil {
			day := row.RealBalanceCheckedOn.Format(batchDateLayout)
			reading.checkedOn = &day
		}
		readings[row.ID] = reading
	}
	return readings, nil
}

// accountLabels names each account the way the panel's own pickers do: the holder, plus the
// programme.
//
// The programme is not decoration here. Two accounts of one holder differ by nothing else -
// "Marlon" on Azul and "Marlon" on Smiles are two separate stocks with two separate
// closings - and an account alias is free text that need not mention it. A heading saying
// only "Marlon" leaves the operator guessing which of the two they are settling.
func (h *InventoryClosingHandler) accountLabels(accountIDs []string) (map[string]string, error) {
	type row struct {
		ID          string
		HolderName  *string
		Login       *string
		ProgramName string
	}
	var rows []row
	err := h.DB.Table("accounts").
		Select(`accounts.id,
		        holders.name AS holder_name,
		        accounts.login,
		        loyalty_programs.name AS program_name`).
		Joins("JOIN loyalty_programs ON loyalty_programs.id = accounts.loyalty_program_id").
		Joins("LEFT JOIN people AS holders ON holders.id = accounts.holder_id").
		Where("accounts.id IN ?", accountIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	labels := make(map[string]string, len(rows))
	for _, r := range rows {
		var who string
		switch {
		case r.HolderName != nil && *r.HolderName != "":
			who = *r.HolderName
		case r.Login != nil && *r.Login != "":
			who = *r.Login
		default:
			who = r.ID[:8]
		}
		labels[r.ID] = who + " · " + r.ProgramName
	}
	return labels, nil
}

// consumerResponses maps agency activity, never returning nil.
func consumerResponses(consumers []inventory.Consumer, names map[string]string) []schemas.InventoryConsumerResponse {
	responses := make([]schemas.InventoryConsumerResponse, 0, len(consumers))
	for _, consumer := range consumers {
		responses = append(responses, schemas.InventoryConsumerResponse{
			AgencyID:                consumer.AgencyID,
			AgencyName:              nameOf(names, consumer.AgencyID),
			Points:                  consumer.Points,
			CostCents:               consumer.CostCents,
			RevenueCents:            consumer.RevenueCents,
			ProfitCents:             consumer.ProfitCents,
			RedemptionFeeSavedCents: consumer.RedemptionFeeSavedCents,
		})
	}
	return responses
}

// partResponses maps settlement parts, never returning nil. A mixed percent becomes null:
// there is no single figure to print, and the screen says "parte" instead.
func partResponses(parts []inventory.SettlementPart, names map[string]string) []schemas.InventorySettlementPartResponse {
	responses := make([]schemas.InventorySettlementPartResponse, 0, len(parts))
	for _, part := range parts {
		response := schemas.InventorySettlementPartResponse{
			Kind:            part.Kind,
			PayerAgencyID:   part.PayerID,
			PayerAgencyName: nameOf(names, part.PayerID),
			PayeeAgencyID:   part.PayeeID,
			PayeeAgencyName: nameOf(names, part.PayeeID),
			BaseCents:       part.BaseCents,
			AmountCents:     part.AmountCents,
		}
		if part.Percent != inventory.MixedPercent {
			percent := part.Percent
			response.Percent = &percent
		}
		responses = append(responses, response)
	}
	return responses
}

// settlementResponses maps a list of settlements, never returning nil so the JSON carries
// an empty list rather than null.
func settlementResponses(settlements []inventory.Settlement, names map[string]string) []schemas.InventorySettlementResponse {
	responses := make([]schemas.InventorySettlementResponse, 0, len(settlements))
	for _, settlement := range settlements {
		responses = append(responses, settlementResponse(settlement, names))
	}
	return responses
}

func settlementResponse(settlement inventory.Settlement, names map[string]string) schemas.InventorySettlementResponse {
	return schemas.InventorySettlementResponse{
		AgencyID:     settlement.AgencyID,
		AgencyName:   nameOf(names, settlement.AgencyID),
		ToAgencyID:   settlement.ToAgencyID,
		ToAgencyName: nameOf(names, settlement.ToAgencyID),
		AmountCents:  settlement.AmountCents,
	}
}

// nameOf returns nil rather than an empty string for an agency that no longer exists, so
// the screen can tell "no name" from a name that happens to be blank.
func nameOf(names map[string]string, id string) *string {
	name, ok := names[id]
	if !ok {
		return nil
	}
	return &name
}

// scopeClosing narrows one account's closing to what a restricted caller may see: the batches
// their agencies paid for, and the transfers their agencies take part in. The account totals
// are summed again from the batches kept - the same sums ReadClosing makes - so the header
// never carries money from a batch the screen does not list. The counterparty of a kept
// transfer stays named: a settlement that hides who pays whom cannot be acted on.
//
// PendingPoints is left as is. It is points, not money, and it is the account's warning that
// a batch is missing - hiding it would only hide that something needs entering.
func scopeClosing(closing inventory.Closing, scope access.AgencyScope) inventory.Closing {
	if scope.All {
		return closing
	}

	involved := func(settlements []inventory.Settlement) []inventory.Settlement {
		kept := make([]inventory.Settlement, 0, len(settlements))
		for _, settlement := range settlements {
			if scope.Allows(settlement.AgencyID) || scope.Allows(settlement.ToAgencyID) {
				kept = append(kept, settlement)
			}
		}
		return kept
	}

	involvedParts := func(parts []inventory.SettlementPart) []inventory.SettlementPart {
		kept := make([]inventory.SettlementPart, 0, len(parts))
		for _, part := range parts {
			if scope.Allows(part.PayerID) || scope.Allows(part.PayeeID) {
				kept = append(kept, part)
			}
		}
		return kept
	}

	batches := make([]inventory.BatchResult, 0, len(closing.Batches))
	closing.CostCents, closing.RevenueCents, closing.RedemptionFeeSavedCents = 0, 0, 0
	for _, batch := range closing.Batches {
		if !scope.Allows(batch.FundingAgencyID) {
			continue
		}
		consumers := make([]inventory.Consumer, 0, len(batch.Consumers))
		for _, consumer := range batch.Consumers {
			if scope.Allows(consumer.AgencyID) {
				consumers = append(consumers, consumer)
			}
		}
		batch.Consumers = consumers
		// The bookings listed under a batch follow the same rule as its consumers: another
		// agency's locators and sale prices are not this caller's to read.
		lines := make([]inventory.Slice, 0, len(batch.Lines))
		for _, line := range batch.Lines {
			if scope.Allows(line.AgencyID) {
				lines = append(lines, line)
			}
		}
		batch.Lines = lines
		batch.Settlements = involved(batch.Settlements)
		batch.FeeSettlements = involved(batch.FeeSettlements)
		batch.TotalSettlements = involved(batch.TotalSettlements)
		batch.Parts = involvedParts(batch.Parts)
		batches = append(batches, batch)
		closing.CostCents += batch.ChargedCents
		closing.RevenueCents += batch.RevenueCents
		closing.RedemptionFeeSavedCents += batch.RedemptionFeeSavedCents
	}
	closing.Batches = batches
	closing.ProfitCents = closing.RevenueCents - closing.CostCents
	closing.Settlements = involved(closing.Settlements)
	closing.FeeSettlements = involved(closing.FeeSettlements)
	closing.TotalSettlements = involved(closing.TotalSettlements)
	closing.Parts = involvedParts(closing.Parts)
	// The agency totals are added up again from the kept batches, whose consumers are
	// already narrowed to the caller's agencies.
	closing.Agencies = inventory.SummariseAgencies(closing.Batches)
	return closing
}
