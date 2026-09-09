package handlers

import (
	"errors"
	"net/http"
	"time"

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

	names, err := h.agencyNames()
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch agencies: "+err.Error())
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
	type pair struct{ from, to string }
	owed := map[pair]int{}
	order := make([]pair, 0)

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

		closedPeriodID, err := h.closedPeriodID(accountID, endsOn)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the period: "+err.Error())
			return
		}

		data.Accounts = append(data.Accounts, toClosingResponse(closing, closedPeriodID, names, batchLabels))

		data.Totals.CostCents += closing.CostCents
		data.Totals.RevenueCents += closing.RevenueCents
		data.Totals.PendingPoints += closing.PendingPoints
		for _, settlement := range closing.Settlements {
			key := pair{from: settlement.AgencyID, to: settlement.ToAgencyID}
			if _, seen := owed[key]; !seen {
				order = append(order, key)
			}
			owed[key] += settlement.AmountCents
		}
	}

	data.Totals.ProfitCents = data.Totals.RevenueCents - data.Totals.CostCents
	data.Totals.Settlements = make([]schemas.InventorySettlementResponse, 0, len(order))
	for _, key := range order {
		data.Totals.Settlements = append(data.Totals.Settlements, settlementResponse(
			inventory.Settlement{AgencyID: key.from, ToAgencyID: key.to, AmountCents: owed[key]}, names,
		))
	}

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
		Settlements:      make([]schemas.InventorySettlementResponse, 0, len(closing.Settlements)),
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
			Settlements:        make([]schemas.InventorySettlementResponse, 0, len(batch.Settlements)),
		}

		if label, ok := labels[batch.BatchID]; ok {
			line.OriginName = label.OriginName
			line.AcquiredOn = label.AcquiredOn.Format(batchDateLayout)
			if label.SettledAt != nil {
				settled := label.SettledAt.Format(time.RFC3339)
				line.SettledAt = &settled
			}
		}

		for _, consumer := range batch.Consumers {
			line.Consumers = append(line.Consumers, schemas.InventoryConsumerResponse{
				AgencyID:     consumer.AgencyID,
				AgencyName:   nameOf(names, consumer.AgencyID),
				Points:       consumer.Points,
				CostCents:    consumer.CostCents,
				RevenueCents: consumer.RevenueCents,
				ProfitCents:  consumer.ProfitCents,
			})
		}
		for _, settlement := range batch.Settlements {
			line.Settlements = append(line.Settlements, settlementResponse(settlement, names))
		}

		response.Batches = append(response.Batches, line)
	}

	for _, settlement := range closing.Settlements {
		response.Settlements = append(response.Settlements, settlementResponse(settlement, names))
	}

	return response
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
