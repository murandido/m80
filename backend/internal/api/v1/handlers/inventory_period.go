package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/inventory"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type InventoryPeriodHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewInventoryPeriodHandler(db *gorm.DB) *InventoryPeriodHandler {
	return &InventoryPeriodHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toInventoryPeriodResponse(period data.InventoryPeriod) schemas.InventoryPeriodResponse {
	return schemas.InventoryPeriodResponse{
		ID:         period.ID,
		AccountID:  period.AccountID,
		StartsOn:   period.StartsOn.Format(batchDateLayout),
		EndsOn:     period.EndsOn.Format(batchDateLayout),
		ClosedAt:   period.ClosedAt,
		ClosedByID: period.ClosedByID,
		CreatedAt:  period.CreatedAt,
	}
}

// ClosePeriod handles the POST /inventory-periods route.
//
// A button, never a calendar rollover: a month ends whether or not anybody has finished
// checking it, and a period that closed itself would freeze figures mid-review.
// @Summary      Fechar período do estoque
// @Description  Recalcula o custeio da conta e congela o período. A partir daí, cancelamento de emissão dentro dele vira estorno no período aberto.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.ClosePeriodRequest true "Conta e intervalo"
// @Success      201  {object}  schemas.InventoryPeriodSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Conta sem estoque próprio ou período fora de ordem"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao fechar período"
// @Router       /inventory-periods [post]
func (h *InventoryPeriodHandler) ClosePeriod(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Não autenticado")
		return
	}

	var req schemas.ClosePeriodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	startsOn, err := time.Parse(batchDateLayout, req.StartsOn)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Data inicial inválida: use o formato AAAA-MM-DD")
		return
	}
	endsOn, err := time.Parse(batchDateLayout, req.EndsOn)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Data final inválida: use o formato AAAA-MM-DD")
		return
	}

	var account data.Account
	if err := h.DB.First(&account, "id = ?", req.AccountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch account: "+err.Error())
		return
	}
	if !account.OwnsPointInventory {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Esta conta não controla estoque próprio de pontos")
		return
	}

	period, err := inventory.Close(h.DB, req.AccountID, startsOn, endsOn, userCtx.ID)
	if err != nil {
		switch {
		case errors.Is(err, inventory.ErrPeriodOutOfOrder):
			utils.SendError(w, r, http.StatusUnprocessableEntity,
				"Já existe um período fechado que termina nesta data ou depois dela")
		case errors.Is(err, inventory.ErrPeriodInverted):
			utils.SendError(w, r, http.StatusUnprocessableEntity, "A data final é anterior à inicial")
		default:
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to close period: "+err.Error())
		}
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Período fechado com sucesso", toInventoryPeriodResponse(period))
}

// ReopenPeriod handles the DELETE /inventory-periods/{id} route.
//
// The way out of a closing pressed by mistake. Only the account's most recent closed period
// comes off - see inventory.Reopen for why the chain has to be undone from the top.
// @Summary      Reabrir período do estoque
// @Description  Desfaz o fechamento e devolve o período ao recálculo. Só o último período fechado da conta pode ser reaberto. O acerto já registrado nos lotes não é desfeito.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "ID do período"
// @Success      200  {object}  schemas.ErrorResponse "Período reaberto com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Período não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Existe período fechado mais recente"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao reabrir período"
// @Router       /inventory-periods/{id} [delete]
func (h *InventoryPeriodHandler) ReopenPeriod(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := inventory.Reopen(h.DB, id); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			utils.SendError(w, r, http.StatusNotFound, "Período não encontrado")
		case errors.Is(err, inventory.ErrPeriodNotLast):
			utils.SendError(w, r, http.StatusUnprocessableEntity,
				"Existe um período fechado mais recente. Reabra o mais novo primeiro.")
		default:
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to reopen period: "+err.Error())
		}
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Período reaberto com sucesso", nil)
}

// GetInventoryPeriods handles the GET /inventory-periods route.
// @Summary      Listar períodos fechados
// @Description  Períodos já fechados de uma conta, do mais recente para o mais antigo.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        account_id query string true "ID da conta"
// @Success      200  {object}  schemas.InventoryPeriodListResponse
// @Failure      400  {object}  schemas.ErrorResponse "account_id ausente"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao listar períodos"
// @Router       /inventory-periods [get]
func (h *InventoryPeriodHandler) GetInventoryPeriods(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		utils.SendError(w, r, http.StatusBadRequest, "Informe account_id")
		return
	}

	var periods []data.InventoryPeriod
	err := h.DB.Where("account_id = ?", accountID).
		Order("ends_on DESC").
		Find(&periods).Error
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch inventory periods: "+err.Error())
		return
	}

	response := make([]schemas.InventoryPeriodResponse, 0, len(periods))
	for _, period := range periods {
		response = append(response, toInventoryPeriodResponse(period))
	}

	utils.SendJSON(w, r, http.StatusOK, "Períodos listados com sucesso", response)
}
