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
	"github.com/murandido/m-80-backend/internal/sale"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// A sale is settled when its payments add up to what it is worth. sales.paid survives as a
// cache of that sum - see recalculateSalePaid - so the previous API image keeps reading the
// right thing after a rollback, and so the ?paid= filter and the export stay one column
// lookup instead of an aggregate.
//
// Everything that writes a payment goes through here, and everything here ends by
// recalculating. That is the whole of the consistency rule.

func toSalePaymentResponse(payment data.SalePayment) schemas.SalePaymentResponse {
	resp := schemas.SalePaymentResponse{
		ID:          payment.ID,
		SaleID:      payment.SaleID,
		AmountCents: payment.AmountCents,
		PaidOn:      payment.PaidOn.Format("2006-01-02"),
		Note:        payment.Note,
		CreatedByID: payment.CreatedByID,
		CreatedAt:   payment.CreatedAt,
	}
	if payment.CreatedBy != nil {
		resp.CreatedByName = payment.CreatedBy.Name
	}
	return resp
}

// GetSalePayments handles GET /sales/{id}/payments
// @Summary      Extrato de pagamentos de uma venda
// @Description  Lista os lançamentos de uma venda, do mais antigo para o mais recente. Valor negativo é dinheiro devolvido ao cliente.
// @Tags         Vendas
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "UUID da venda"
// @Success      200  {object}  schemas.SalePaymentListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Venda não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar os lançamentos"
// @Router       /sales/{id}/payments [get]
func (h *SaleHandler) GetSalePayments(w http.ResponseWriter, r *http.Request) {
	saleID := chi.URLParam(r, "id")

	var found data.Sale
	if err := h.DB.First(&found, "id = ?", saleID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Sale not found")
		return
	}

	var payments []data.SalePayment
	err := h.DB.Preload("CreatedBy").
		Where("sale_id = ?", saleID).
		Order("paid_on asc, created_at asc").
		Find(&payments).Error
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the payments")
		return
	}

	responses := make([]schemas.SalePaymentResponse, 0, len(payments))
	for _, payment := range payments {
		responses = append(responses, toSalePaymentResponse(payment))
	}

	utils.SendJSON(w, r, http.StatusOK, "Sale payments retrieved successfully", responses)
}

// CreateSalePayment handles POST /sales/{id}/payments
// @Summary      Lançar um pagamento numa venda
// @Description  Registra dinheiro recebido do cliente. O valor é em centavos e pode ser negativo, que é como se registra uma devolução — o saldo da venda volta a subir. Zero é recusado. Depois de gravar, a venda é marcada como paga ou em aberto conforme a soma dos lançamentos alcance ou não o valor dela.
// @Tags         Vendas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                            true "UUID da venda"
// @Param        payload body  schemas.CreateSalePaymentRequest  true "Lançamento"
// @Success      201  {object}  schemas.SalePaymentSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido, valor zero ou data em formato errado"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Venda não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao lançar o pagamento"
// @Router       /sales/{id}/payments [post]
func (h *SaleHandler) CreateSalePayment(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	saleID := chi.URLParam(r, "id")

	var req schemas.CreateSalePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}
	// The database refuses this too, but a check constraint surfaces as a 500 with a
	// Postgres string in it. A movement of nothing is only ever a typo.
	if req.AmountCents == 0 {
		utils.SendError(w, r, http.StatusBadRequest, "O valor do lançamento não pode ser zero")
		return
	}

	paidOn, err := time.Parse("2006-01-02", req.PaidOn)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Data inválida (use AAAA-MM-DD)")
		return
	}

	var found data.Sale
	if err := h.DB.First(&found, "id = ?", saleID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Sale not found")
		return
	}

	payment := data.SalePayment{
		SaleID:      saleID,
		AmountCents: req.AmountCents,
		PaidOn:      paidOn,
		Note:        optionalText(req.Note),
		CreatedByID: userCtx.ID,
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&payment).Error; err != nil {
			return err
		}
		return sale.Recalculate(tx, saleID)
	})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to record the payment: "+err.Error())
		return
	}

	h.broadcastSaleUpdated(saleID)

	payment.CreatedBy = &userCtx
	utils.SendJSON(w, r, http.StatusCreated, "Payment recorded successfully", toSalePaymentResponse(payment))
}

// CreateClientPayment handles POST /sales/client-payments
// @Summary      Registrar um pagamento do cliente
// @Description  Recebe um valor do cliente e abate nas vendas dele em aberto, da emissão mais antiga para a mais nova; a última alcançada pode ficar parcial. O que sobrar vira crédito do cliente e é abatido nas próximas vendas vinculadas a ele. O pagamento fica no histórico do cliente e pode ser desfeito por DELETE /sales/client-payments/{id}.
// @Tags         Vendas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body  schemas.CreateClientPaymentRequest  true "Pagamento"
// @Success      201  {object}  schemas.ClientPaymentSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido, valor não positivo ou data em formato errado"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Cliente não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao registrar o pagamento"
// @Router       /sales/client-payments [post]
func (h *SaleHandler) CreateClientPayment(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	var req schemas.CreateClientPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	paidOn, err := time.Parse("2006-01-02", req.PaidOn)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Data inválida (use AAAA-MM-DD)")
		return
	}

	var result sale.ClientPaymentResult
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		result, txErr = sale.ReceiveClientPayment(tx, req.ClientID, req.AmountCents, paidOn, userCtx.ID)
		return txErr
	})
	if errors.Is(err, sale.ErrClientNotFound) {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Cliente não encontrado")
		return
	}
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to record the client payment: "+err.Error())
		return
	}

	for _, saleID := range result.SaleIDs {
		h.broadcastSaleUpdated(saleID)
	}

	saleIDs := result.SaleIDs
	if saleIDs == nil {
		saleIDs = []string{}
	}
	utils.SendJSON(w, r, http.StatusCreated, "Client payment recorded successfully", schemas.ClientPaymentResponse{
		ClientPaymentID: result.ClientPaymentID,
		SaleIDs:         saleIDs,
		AppliedCents:    result.AppliedCents,
		SettledSales:    result.SettledSales,
		CreditCents:     result.CreditCents,
	})
}

// GetClientPayments handles GET /sales/client-payments
// @Summary      Histórico de pagamentos do cliente
// @Description  Lista os pagamentos do cliente, do mais antigo para o mais recente, incluindo os desfeitos, com as vendas em que cada um está abatido e o crédito que sobra.
// @Tags         Vendas
// @Produce      json
// @Security     BearerAuth
// @Param        client_id query string true "UUID do cliente"
// @Success      200  {object}  schemas.ClientPaymentHistorySingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "client_id ausente ou inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar os pagamentos"
// @Router       /sales/client-payments [get]
func (h *SaleHandler) GetClientPayments(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if err := h.Validator.Var(clientID, "required,uuid"); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "client_id inválido")
		return
	}

	var payments []data.ClientPayment
	err := h.DB.Preload("CreatedBy").Preload("DeletedBy").
		Where("client_id = ?", clientID).
		Order("paid_on, created_at, id").
		Find(&payments).Error
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the client payments")
		return
	}

	var allocations []struct {
		ClientPaymentID string
		SaleID          string
		ProgramLocator  string
		AmountCents     int
	}
	err = h.DB.Table("sale_payments").
		Select("sale_payments.client_payment_id, sale_payments.sale_id, issuances.program_locator, sale_payments.amount_cents").
		Joins("JOIN client_payments ON client_payments.id = sale_payments.client_payment_id").
		Joins("JOIN sales ON sales.id = sale_payments.sale_id").
		Joins("JOIN issuances ON issuances.id = sales.issuance_id").
		Where("client_payments.client_id = ?", clientID).
		Order("issuances.created_at, sales.id").
		Scan(&allocations).Error
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the client payments")
		return
	}
	byPayment := map[string][]schemas.ClientPaymentAllocation{}
	for _, a := range allocations {
		byPayment[a.ClientPaymentID] = append(byPayment[a.ClientPaymentID], schemas.ClientPaymentAllocation{
			SaleID:         a.SaleID,
			ProgramLocator: a.ProgramLocator,
			AmountCents:    a.AmountCents,
		})
	}

	credit, err := sale.CreditOf(h.DB, clientID)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the client payments")
		return
	}

	resp := schemas.ClientPaymentHistoryResponse{
		CreditCents: credit,
		Payments:    make([]schemas.ClientPaymentHistoryItem, 0, len(payments)),
	}
	for _, payment := range payments {
		item := schemas.ClientPaymentHistoryItem{
			ID:          payment.ID,
			AmountCents: payment.AmountCents,
			PaidOn:      payment.PaidOn.Format("2006-01-02"),
			CreatedAt:   payment.CreatedAt,
			DeletedAt:   payment.DeletedAt,
			Allocations: byPayment[payment.ID],
		}
		if item.Allocations == nil {
			item.Allocations = []schemas.ClientPaymentAllocation{}
		}
		if payment.CreatedBy != nil {
			item.CreatedByName = payment.CreatedBy.Name
		}
		if payment.DeletedBy != nil {
			item.DeletedByName = &payment.DeletedBy.Name
		}
		resp.Payments = append(resp.Payments, item)
	}

	utils.SendJSON(w, r, http.StatusOK, "Client payments retrieved successfully", resp)
}

// DeleteClientPayment handles DELETE /sales/client-payments/{paymentID}
// @Summary      Desfazer um pagamento do cliente
// @Description  Marca o pagamento como desfeito (ele continua no histórico) e redistribui os pagamentos restantes do cliente pelas vendas em aberto, da mais antiga para a mais nova.
// @Tags         Vendas
// @Produce      json
// @Security     BearerAuth
// @Param        paymentID path string true "UUID do pagamento do cliente"
// @Success      200  {object}  schemas.ClientPaymentUndoSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Pagamento não encontrado ou já desfeito"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao desfazer o pagamento"
// @Router       /sales/client-payments/{paymentID} [delete]
func (h *SaleHandler) DeleteClientPayment(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}
	paymentID := chi.URLParam(r, "paymentID")

	var result sale.ClientPaymentResult
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		result, txErr = sale.UndoClientPayment(tx, paymentID, userCtx.ID)
		return txErr
	})
	if errors.Is(err, sale.ErrClientPaymentNotFound) {
		utils.SendError(w, r, http.StatusNotFound, "Pagamento não encontrado ou já desfeito")
		return
	}
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to undo the client payment: "+err.Error())
		return
	}

	for _, saleID := range result.SaleIDs {
		h.broadcastSaleUpdated(saleID)
	}

	saleIDs := result.SaleIDs
	if saleIDs == nil {
		saleIDs = []string{}
	}
	utils.SendJSON(w, r, http.StatusOK, "Client payment undone successfully", schemas.ClientPaymentUndoResponse{
		SaleIDs:     saleIDs,
		CreditCents: result.CreditCents,
	})
}

// DeleteSalePayment handles DELETE /sales/{id}/payments/{paymentID}
// @Summary      Apagar um lançamento
// @Description  Remove um lançamento digitado errado e recalcula a situação da venda. Não é soft delete: um pagamento que nunca aconteceu não é história, é engano.
// @Tags         Vendas
// @Produce      json
// @Security     BearerAuth
// @Param        id        path string true "UUID da venda"
// @Param        paymentID path string true "UUID do lançamento"
// @Success      200  {object}  schemas.ErrorResponse "Lançamento apagado com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Lançamento não encontrado nessa venda"
// @Failure      409  {object}  schemas.ErrorResponse "Lançamento veio de um pagamento do cliente"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao apagar o lançamento"
// @Router       /sales/{id}/payments/{paymentID} [delete]
func (h *SaleHandler) DeleteSalePayment(w http.ResponseWriter, r *http.Request) {
	saleID := chi.URLParam(r, "id")
	paymentID := chi.URLParam(r, "paymentID")

	// Scoped by both ids: the payment has to belong to the sale in the URL, or a guessed id
	// would let one sale's statement be edited through another's.
	var payment data.SalePayment
	if err := h.DB.First(&payment, "id = ? AND sale_id = ?", paymentID, saleID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Payment not found")
		return
	}
	// A line spread from a client payment is derived: deleting it here would only last
	// until the next replay put it back. The payment is undone as a whole instead.
	if payment.ClientPaymentID != nil {
		utils.SendError(w, r, http.StatusConflict, "Esse lançamento veio de um pagamento do cliente; desfaça o pagamento no histórico")
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&payment).Error; err != nil {
			return err
		}
		return sale.Recalculate(tx, saleID)
	})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete the payment: "+err.Error())
		return
	}

	h.broadcastSaleUpdated(saleID)

	utils.SendJSON(w, r, http.StatusOK, "Payment deleted successfully", nil)
}

// settleSaleFromLegacyFlag is the one-click paid switch: true records a payment for
// whatever is missing; false clears the statement. It started as a compatibility step for
// the panel that predated the statement, and the payments screen's Pago / A Pagar button is
// built on it again - partial amounts go through CreateClientPayment instead.
func settleSaleFromLegacyFlag(tx *gorm.DB, saleID string, paid bool, actorID string) error {
	if !paid {
		// Only the sale's own lines: those spread from a client payment are rebuilt from
		// the payment and go away by undoing it (UpdateSale refuses a sale that has nothing
		// else).
		if err := tx.Where("sale_id = ? AND client_payment_id IS NULL", saleID).Delete(&data.SalePayment{}).Error; err != nil {
			return err
		}
		return sale.Recalculate(tx, saleID)
	}

	amount, err := sale.AmountOf(tx, saleID)
	if err != nil {
		return err
	}
	paidCents, err := sale.PaidCents(tx, saleID)
	if err != nil {
		return err
	}

	// On a cancelled sale the balance is what has to go back, so the old switch records the
	// refund. That is the only reading of "mark as paid" that leaves the sale with nothing
	// pending, which is what the switch meant.
	if missing := sale.BalanceCents(amount, paidCents); missing != 0 {
		note := "Baixa pelo interruptor de pago"
		payment := data.SalePayment{
			SaleID:      saleID,
			AmountCents: missing,
			PaidOn:      time.Now(),
			Note:        &note,
			CreatedByID: actorID,
		}
		if err := tx.Create(&payment).Error; err != nil {
			return err
		}
	}

	return sale.Recalculate(tx, saleID)
}

// rebalanceClient replays a client's payments after one of their sales changed hands. It
// lives out here because UpdateSale's own variable is called sale, which hides the package.
func rebalanceClient(tx *gorm.DB, clientID string) ([]string, error) {
	return sale.RebalanceClient(tx, clientID)
}

func optionalText(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	return value
}
