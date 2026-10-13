package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"github.com/murandido/m-80-backend/internal/sale"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// toIssuanceAdjustmentResponse renders one alteration, with what it adds to the sale.
func toIssuanceAdjustmentResponse(adjustment data.IssuanceAdjustment) schemas.IssuanceAdjustmentResponse {
	resp := schemas.IssuanceAdjustmentResponse{
		ID:                  adjustment.ID,
		IssuanceID:          adjustment.IssuanceID,
		OccurredOn:          adjustment.OccurredOn.Format(issuanceDateLayout),
		Points:              adjustment.Points,
		Taxes:               adjustment.Taxes,
		ValueOfTheThousand:  adjustment.ValueOfTheThousand,
		TaxesPassedToClient: adjustment.TaxesPassedToClient,
		Reason:              adjustment.Reason,
		ClientAmountCents: sale.AdjustmentClientCents(
			adjustment.Points, adjustment.ValueOfTheThousand, adjustment.Taxes, adjustment.TaxesPassedToClient),
		CreatedByID: adjustment.CreatedByID,
		CreatedAt:   adjustment.CreatedAt,
		UpdatedAt:   adjustment.UpdatedAt,
	}
	if adjustment.CreatedBy != nil {
		name := adjustment.CreatedBy.Name
		resp.CreatedByName = &name
	}
	return resp
}

// adjustmentErrorResponse maps the alteration rules onto HTTP, the way issuanceErrorResponse
// does for creation. Returns (0, "") for an error the rules do not classify.
func adjustmentErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, issuancesvc.ErrAdjustmentOnCancelledIssuance):
		return http.StatusUnprocessableEntity, "Emissão cancelada não aceita alteração. Reverta o cancelamento primeiro."
	case errors.Is(err, issuancesvc.ErrAdjustmentEmpty):
		return http.StatusUnprocessableEntity, "Informe pontos ou taxas da alteração."
	case errors.Is(err, issuancesvc.ErrAdjustmentReasonRequired):
		return http.StatusUnprocessableEntity, "Informe o motivo da alteração."
	case errors.Is(err, issuancesvc.ErrAdjustmentNegativeRate):
		return http.StatusUnprocessableEntity, "O milheiro não pode ser negativo."
	default:
		return 0, ""
	}
}

// decodeAdjustment reads and validates the body. ok is false when a response was already sent.
func (h *IssuanceHandler) decodeAdjustment(w http.ResponseWriter, r *http.Request) (issuancesvc.AdjustmentInput, time.Time, bool) {
	var req schemas.IssuanceAdjustmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return issuancesvc.AdjustmentInput{}, time.Time{}, false
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return issuancesvc.AdjustmentInput{}, time.Time{}, false
	}
	day, err := parseIssuanceDay(req.OccurredOn)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid occurred_on")
		return issuancesvc.AdjustmentInput{}, time.Time{}, false
	}

	return issuancesvc.AdjustmentInput{
		// Stored as the calendar day itself. occurred_on is a DATE, and handing the driver
		// local midnight would leave the day to whichever zone it converts through.
		OccurredOn:          time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
		Points:              req.Points,
		Taxes:               req.Taxes,
		ValueOfTheThousand:  req.ValueOfTheThousand,
		TaxesPassedToClient: req.TaxesPassedToClient,
		Reason:              req.Reason,
	}, day, true
}

// guardAdjustmentDays refuses a write touching a closed period, the same guard a backdated
// cancellation gets: the recompute never replays an event inside a frozen period, so the
// alteration would silently never be costed - or, for one being moved out or removed, its
// settled cost would stay behind. ok is false when a response was already sent.
func (h *IssuanceHandler) guardAdjustmentDays(w http.ResponseWriter, r *http.Request, issuance data.Issuance, days ...time.Time) bool {
	frozenUntil, err := refuseClosedPeriod(h.DB, issuance.AccountID, days...)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the inventory period: "+err.Error())
		return false
	}
	if !frozenUntil.IsZero() {
		utils.SendError(w, r, http.StatusUnprocessableEntity, closedPeriodMessage(frozenUntil))
		return false
	}
	return true
}

// loadAdjustmentTarget reads the issuance and, when the route names one, its alteration.
// ok is false when a response was already sent.
func (h *IssuanceHandler) loadAdjustmentTarget(w http.ResponseWriter, r *http.Request, withAdjustment bool) (data.Issuance, data.IssuanceAdjustment, bool) {
	var issuance data.Issuance
	if err := h.DB.First(&issuance, "id = ?", chi.URLParam(r, "id")).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return data.Issuance{}, data.IssuanceAdjustment{}, false
	}
	if !withAdjustment {
		return issuance, data.IssuanceAdjustment{}, true
	}

	var adjustment data.IssuanceAdjustment
	if err := h.DB.First(&adjustment, "id = ? AND issuance_id = ?",
		chi.URLParam(r, "adjustmentID"), issuance.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Adjustment not found")
		return data.Issuance{}, data.IssuanceAdjustment{}, false
	}
	return issuance, adjustment, true
}

// respondAdjustment finishes a write: re-reads the alteration with its author, announces the
// issuance changed, and answers.
func (h *IssuanceHandler) respondAdjustment(w http.ResponseWriter, r *http.Request, status int, message string, adjustment data.IssuanceAdjustment) {
	if err := h.DB.Preload("CreatedBy").First(&adjustment, "id = ?", adjustment.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the adjustment")
		return
	}
	h.broadcastIssuance(eventIssuanceUpdated, adjustment.IssuanceID)
	utils.SendJSON(w, r, status, message, toIssuanceAdjustmentResponse(adjustment))
}

func (h *IssuanceHandler) sendAdjustmentError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	if status, message := adjustmentErrorResponse(err); status != 0 {
		utils.SendError(w, r, status, message)
		return
	}
	utils.SendError(w, r, http.StatusInternalServerError, fallback+": "+err.Error())
}

// CreateIssuanceAdjustment handles the POST /issuances/{id}/adjustments route.
// @Summary      Registrar alteração de passagem
// @Description  Registra uma alteração feita na passagem depois da emissão: pontos e/ou taxas cobrados a mais pela cia (positivos) ou devolvidos por ela (negativos), datados pelo dia em que aconteceram. value_of_the_thousand é o milheiro cobrado do cliente por esses pontos (0 quando a agência absorve) e taxes_passed_to_client diz se as taxas vão para a conta do cliente. A venda é recalculada, o custeio FIFO consome ou devolve na data informada e a ação vai para o audit log. Emissão cancelada não aceita alteração; data dentro de período fechado é recusada.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                              true  "UUID da emissão"
// @Param        payload  body  schemas.IssuanceAdjustmentRequest  true  "A alteração"
// @Success      201  {object}  schemas.IssuanceAdjustmentSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Emissão cancelada, alteração vazia, sem motivo ou em período fechado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao registrar a alteração"
// @Router       /issuances/{id}/adjustments [post]
func (h *IssuanceHandler) CreateIssuanceAdjustment(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}
	issuance, _, ok := h.loadAdjustmentTarget(w, r, false)
	if !ok {
		return
	}
	in, day, ok := h.decodeAdjustment(w, r)
	if !ok || !h.guardAdjustmentDays(w, r, issuance, day) {
		return
	}

	var created data.IssuanceAdjustment
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		created, txErr = issuancesvc.CreateAdjustment(tx, issuance, in, userCtx.ID)
		return txErr
	})
	if err != nil {
		h.sendAdjustmentError(w, r, err, "Failed to record the adjustment")
		return
	}

	h.respondAdjustment(w, r, http.StatusCreated, "Adjustment recorded successfully", created)
}

// UpdateIssuanceAdjustment handles the PATCH /issuances/{id}/adjustments/{adjustmentID} route.
// @Summary      Corrigir alteração de passagem
// @Description  Substitui todos os campos de uma alteração já registrada (corrige o que foi digitado). As mesmas regras do registro valem, e tanto a data antiga quanto a nova precisam estar fora de período fechado.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id            path  string                              true  "UUID da emissão"
// @Param        adjustmentID  path  string                              true  "UUID da alteração"
// @Param        payload       body  schemas.IssuanceAdjustmentRequest  true  "A alteração"
// @Success      200  {object}  schemas.IssuanceAdjustmentSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão ou alteração não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Emissão cancelada, alteração vazia, sem motivo ou em período fechado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao corrigir a alteração"
// @Router       /issuances/{id}/adjustments/{adjustmentID} [patch]
func (h *IssuanceHandler) UpdateIssuanceAdjustment(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}
	issuance, adjustment, ok := h.loadAdjustmentTarget(w, r, true)
	if !ok {
		return
	}
	in, day, ok := h.decodeAdjustment(w, r)
	if !ok || !h.guardAdjustmentDays(w, r, issuance, day, adjustment.OccurredOn) {
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		adjustment, txErr = issuancesvc.UpdateAdjustment(tx, issuance, adjustment, in, userCtx.ID)
		return txErr
	})
	if err != nil {
		h.sendAdjustmentError(w, r, err, "Failed to update the adjustment")
		return
	}

	h.respondAdjustment(w, r, http.StatusOK, "Adjustment updated successfully", adjustment)
}

// DeleteIssuanceAdjustment handles the DELETE /issuances/{id}/adjustments/{adjustmentID} route.
// @Summary      Excluir alteração de passagem
// @Description  Remove (soft delete) uma alteração registrada por engano. A venda e o custeio voltam a não contar com ela, e a remoção vai para o audit log. Recusada se a data da alteração estiver em período fechado.
// @Tags         Emissões
// @Produce      json
// @Security     BearerAuth
// @Param        id            path  string  true  "UUID da emissão"
// @Param        adjustmentID  path  string  true  "UUID da alteração"
// @Success      200  {object}  schemas.ErrorResponse "Alteração excluída com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão ou alteração não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Emissão cancelada ou período fechado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao excluir a alteração"
// @Router       /issuances/{id}/adjustments/{adjustmentID} [delete]
func (h *IssuanceHandler) DeleteIssuanceAdjustment(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}
	issuance, adjustment, ok := h.loadAdjustmentTarget(w, r, true)
	if !ok || !h.guardAdjustmentDays(w, r, issuance, adjustment.OccurredOn) {
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		return issuancesvc.DeleteAdjustment(tx, issuance, adjustment, userCtx.ID)
	})
	if err != nil {
		h.sendAdjustmentError(w, r, err, "Failed to delete the adjustment")
		return
	}

	h.broadcastIssuance(eventIssuanceUpdated, issuance.ID)
	utils.SendJSON(w, r, http.StatusOK, "Adjustment deleted successfully", nil)
}
