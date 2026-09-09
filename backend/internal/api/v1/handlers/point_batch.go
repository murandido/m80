package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// batchDateLayout is the wire format for acquired_on. A plain date, not a timestamp: what
// orders the FIFO queue is the day the points landed, and a timezone on it would let the
// same acquisition sort differently depending on who is looking.
const batchDateLayout = "2006-01-02"

type PointBatchHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewPointBatchHandler(db *gorm.DB) *PointBatchHandler {
	return &PointBatchHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toPointBatchResponse(batch data.PointBatch) schemas.PointBatchResponse {
	response := schemas.PointBatchResponse{
		ID:                 batch.ID,
		AccountID:          batch.AccountID,
		OriginID:           batch.OriginID,
		AcquiredOn:         batch.AcquiredOn.Format(batchDateLayout),
		Points:             batch.Points,
		CostCents:          batch.CostCents,
		FundingAgencyID:    batch.FundingAgencyID,
		ProfitSharePercent: batch.ProfitSharePercent,
		Opening:            batch.Opening,
		SettledAt:          batch.SettledAt,
		Note:               batch.Note,
		CostOfTheThousand:  batch.CostOfTheThousand(),
		CreatedAt:          batch.CreatedAt,
		UpdatedAt:          batch.UpdatedAt,
	}

	if batch.Origin != nil {
		origin := toPointBatchOriginResponse(*batch.Origin)
		response.Origin = &origin
	}
	if batch.FundingAgency != nil {
		response.FundingAgency = &schemas.AgencyResponse{
			ID:        batch.FundingAgency.ID,
			Name:      batch.FundingAgency.Name,
			IsDefault: batch.FundingAgency.IsDefault,
			CreatedAt: batch.FundingAgency.CreatedAt,
			UpdatedAt: batch.FundingAgency.UpdatedAt,
		}
	}

	return response
}

// inventoryAccount loads an account and refuses it unless the inventory regime is on.
//
// The refusal matters: a batch on an ordinary account would be costing that nothing
// reads, sitting next to a cost_of_the_thousand that somebody still types by hand. Two
// answers to the same question, one of them invisible.
func (h *PointBatchHandler) inventoryAccount(id string) (data.Account, error) {
	var account data.Account
	if err := h.DB.First(&account, "id = ?", id).Error; err != nil {
		return account, err
	}
	if !account.OwnsPointInventory {
		return account, errAccountWithoutInventory
	}
	return account, nil
}

var errAccountWithoutInventory = errors.New("account does not track its own points inventory")

// CreatePointBatch handles the POST /point-batches route
// @Summary      Lançar lote de pontos
// @Description  Registra uma aquisição de pontos numa conta com estoque próprio. Use opening=true para o saldo de partida, que dispensa origem.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreatePointBatchRequest true "Dados do lote"
// @Success      201  {object}  schemas.PointBatchSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio, origem ausente ou referência inválida"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar lote"
// @Router       /point-batches [post]
func (h *PointBatchHandler) CreatePointBatch(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreatePointBatchRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// Mirrors the CHECK in migration 49 so the operator gets a sentence instead of a
	// constraint name.
	if req.OriginID == nil && !req.Opening {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Informe a origem do lote")
		return
	}

	acquiredOn, err := time.Parse(batchDateLayout, req.AcquiredOn)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Data inválida: use o formato AAAA-MM-DD")
		return
	}

	account, err := h.inventoryAccount(req.AccountID)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
		case errors.Is(err, errAccountWithoutInventory):
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Esta conta não controla estoque próprio de pontos")
		default:
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch account: "+err.Error())
		}
		return
	}

	// The account's default is a starting value, copied once. From here the batch owns
	// its own percent and a later change to the account never reaches back into it.
	share := account.DefaultProfitSharePercent
	if req.ProfitSharePercent != nil {
		share = *req.ProfitSharePercent
	}

	batch := data.PointBatch{
		AccountID:          req.AccountID,
		OriginID:           req.OriginID,
		AcquiredOn:         acquiredOn,
		Points:             req.Points,
		CostCents:          req.CostCents,
		FundingAgencyID:    req.FundingAgencyID,
		ProfitSharePercent: share,
		Opening:            req.Opening,
		Note:               req.Note,
	}

	if err := h.DB.Create(&batch).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Origem ou agência não encontrada")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create point batch: "+err.Error())
		return
	}

	h.preloadBatch(&batch)
	utils.SendJSON(w, r, http.StatusCreated, "Lote criado com sucesso", toPointBatchResponse(batch))
}

// preloadBatch fills in the origin and funding agency for the response. Best effort: the
// batch was already written, and a failed preload is a thinner payload, not a failed
// request.
func (h *PointBatchHandler) preloadBatch(batch *data.PointBatch) {
	_ = h.DB.Preload("Origin").Preload("FundingAgency").First(batch, "id = ?", batch.ID).Error
}

// GetPointBatches handles the GET /point-batches route.
// @Summary      Listar lotes de pontos
// @Description  Lotes de uma conta, do mais novo para o mais antigo. Aceita o mesmo recorte de datas do fechamento: informados from e to, só aparecem os lotes adquiridos no período.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        account_id query string true "ID da conta"
// @Param        from       query string false "Início do período (AAAA-MM-DD)"
// @Param        to         query string false "Fim do período (AAAA-MM-DD)"
// @Success      200  {object}  schemas.PointBatchListResponse
// @Failure      400  {object}  schemas.ErrorResponse "account_id ausente"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao listar lotes"
// @Router       /point-batches [get]
func (h *PointBatchHandler) GetPointBatches(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		utils.SendError(w, r, http.StatusBadRequest, "Informe account_id")
		return
	}

	// The same window the closing report reads, and optional for the same reason: a client
	// that wants the whole history of an account still gets it by leaving both out. A date
	// that does not parse is ignored rather than refused - the panel always sends a valid
	// pair, and a half-typed one is not worth an error screen.
	query := h.DB.Preload("Origin").Preload("FundingAgency").Where("account_id = ?", accountID)
	if from, err := time.Parse(batchDateLayout, r.URL.Query().Get("from")); err == nil {
		query = query.Where("acquired_on >= ?", from)
	}
	if to, err := time.Parse(batchDateLayout, r.URL.Query().Get("to")); err == nil {
		query = query.Where("acquired_on <= ?", to)
	}

	var batches []data.PointBatch
	// Newest first: the screen is a ledger being read, and what somebody just recorded is
	// what they came to check. Consumption order is FIFO's business, and internal/inventory
	// builds its own queue - this list feeds no calculation. created_at breaks the tie so
	// two batches of the same day always come back in the same order.
	err := query.Order("acquired_on DESC").Order("created_at DESC").Find(&batches).Error
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch point batches: "+err.Error())
		return
	}

	response := make([]schemas.PointBatchResponse, 0, len(batches))
	for _, batch := range batches {
		response = append(response, toPointBatchResponse(batch))
	}

	utils.SendJSON(w, r, http.StatusOK, "Lotes listados com sucesso", response)
}

// UpdatePointBatch handles the PUT /point-batches/{id} route.
// @Summary      Atualizar lote de pontos
// @Description  Corrige um lote. Conta e saldo de partida não são editáveis — para mudá-los, exclua e lance de novo.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                            true  "ID do lote"
// @Param        payload body  schemas.UpdatePointBatchRequest    true  "Campos a atualizar"
// @Success      200  {object}  schemas.PointBatchSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Lote não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Referência inválida"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar lote"
// @Router       /point-batches/{id} [put]
func (h *PointBatchHandler) UpdatePointBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var batch data.PointBatch
	if err := h.DB.First(&batch, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Lote não encontrado")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch point batch: "+err.Error())
		return
	}

	var req schemas.UpdatePointBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.OriginID != nil {
		updates["origin_id"] = *req.OriginID
	}
	if req.AcquiredOn != nil {
		acquiredOn, err := time.Parse(batchDateLayout, *req.AcquiredOn)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Data inválida: use o formato AAAA-MM-DD")
			return
		}
		updates["acquired_on"] = acquiredOn
	}
	if req.Points != nil {
		updates["points"] = *req.Points
	}
	if req.CostCents != nil {
		updates["cost_cents"] = *req.CostCents
	}
	if req.FundingAgencyID != nil {
		updates["funding_agency_id"] = *req.FundingAgencyID
	}
	if req.ProfitSharePercent != nil {
		updates["profit_share_percent"] = *req.ProfitSharePercent
	}
	if req.Note != nil {
		updates["note"] = *req.Note
	}

	if len(updates) > 0 {
		if err := h.DB.Model(&batch).Updates(updates).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
				utils.SendError(w, r, http.StatusUnprocessableEntity, "Origem ou agência não encontrada")
				return
			}
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to update point batch: "+err.Error())
			return
		}
	}

	h.preloadBatch(&batch)
	utils.SendJSON(w, r, http.StatusOK, "Lote atualizado com sucesso", toPointBatchResponse(batch))
}

// SettlePointBatch handles the PATCH /point-batches/{id}/settlement route.
//
// The spreadsheet's "ACERTADO" column, as its own route rather than a field on the update:
// marking a settlement is a different act from correcting a typo in the cost, and the two
// should not be able to happen in the same careless request.
// @Summary      Marcar lote como acertado
// @Description  Registra (ou desfaz) o acerto do lote entre as agências.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                            true  "ID do lote"
// @Param        payload body  schemas.SettlePointBatchRequest    true  "Acertado ou não"
// @Success      200  {object}  schemas.PointBatchSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Lote não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao registrar acerto"
// @Router       /point-batches/{id}/settlement [patch]
func (h *PointBatchHandler) SettlePointBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var batch data.PointBatch
	if err := h.DB.First(&batch, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Lote não encontrado")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch point batch: "+err.Error())
		return
	}

	var req schemas.SettlePointBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	var settledAt *time.Time
	if req.Settled {
		now := time.Now()
		settledAt = &now
	}

	if err := h.DB.Model(&batch).Update("settled_at", settledAt).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to settle point batch: "+err.Error())
		return
	}

	h.preloadBatch(&batch)
	utils.SendJSON(w, r, http.StatusOK, "Acerto registrado com sucesso", toPointBatchResponse(batch))
}

// DeletePointBatch handles the DELETE /point-batches/{id} route.
//
// Archives rather than erases (the model carries gorm.DeletedAt), so a batch that already
// priced issuances leaves the screen without taking that history with it. The costing
// skips archived batches, which is what makes the deletion effective: the next recompute
// re-queues everything it had consumed against the batches that remain.
// @Summary      Excluir lote de pontos
// @Description  Arquiva o lote. O custeio deixa de consumi-lo e as emissões que dependiam dele voltam para a fila no próximo cálculo.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "ID do lote"
// @Success      200  {object}  schemas.ErrorResponse "Lote excluído com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Lote não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao excluir lote"
// @Router       /point-batches/{id} [delete]
func (h *PointBatchHandler) DeletePointBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var batch data.PointBatch
	if err := h.DB.First(&batch, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Lote não encontrado")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch point batch: "+err.Error())
		return
	}

	if err := h.DB.Delete(&batch).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete point batch: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Lote excluído com sucesso", nil)
}
