package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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

	var rebalancedSaleIDs []string
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&payment).Error; err != nil {
			return err
		}
		var err error
		rebalancedSaleIDs, err = recalculateWithClient(tx, found)
		return err
	})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to record the payment: "+err.Error())
		return
	}

	h.broadcastSalesUpdated(saleID, rebalancedSaleIDs)

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

	if !h.clientInScope(w, r, req.ClientID) {
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
// @Description  Com client_id, lista os pagamentos do cliente, do mais antigo para o mais recente, incluindo os desfeitos, com as vendas em que cada um está abatido e o crédito que sobra. Sem client_id, lista os pagamentos de todos os clientes da agência (ou dos de client_ids) recebidos no período, do mais recente para o mais antigo, e o crédito somado deles.
// @Tags         Vendas
// @Produce      json
// @Security     BearerAuth
// @Param        client_id  query string false "UUID do cliente"
// @Param        client_ids query string false "Sem client_id: UUIDs de clientes separados por vírgula (padrão: todos da agência)"
// @Param        agency_id  query string false "Sem client_id: UUID da agência (padrão: a agência principal)"
// @Param        date_from  query string false "Sem client_id: primeiro dia de recebimento (YYYY-MM-DD), inclusivo"
// @Param        date_to    query string false "Sem client_id: último dia de recebimento (YYYY-MM-DD), inclusivo"
// @Success      200  {object}  schemas.ClientPaymentHistorySingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "client_id inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar os pagamentos"
// @Router       /sales/client-payments [get]
func (h *SaleHandler) GetClientPayments(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		h.getClientPaymentsAcrossClients(w, r)
		return
	}
	if err := h.Validator.Var(clientID, "required,uuid"); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "client_id inválido")
		return
	}
	if !h.clientInScope(w, r, clientID) {
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

	h.sendClientPaymentHistory(w, r, payments, []string{clientID})
}

// getClientPaymentsAcrossClients is the history with no single client chosen: every
// client of the agency, or the ones the filter picked, narrowed to the payments received
// in the period. The period is what keeps it readable - a whole agency's history, since
// the first payment, is not something anyone scrolls through. Newest first, since the
// question it answers is "what came in lately".
func (h *SaleHandler) getClientPaymentsAcrossClients(w http.ResponseWriter, r *http.Request) {
	agencyID, err := resolveAgencyFilter(h.DB, r)
	if err != nil {
		sendAgencyFilterError(w, r, err)
		return
	}

	q := r.URL.Query()
	clients := h.DB.Model(&data.Client{}).Where("agency_id = ?", agencyID)
	if raw := strings.TrimSpace(q.Get("client_ids")); raw != "" {
		ids := make([]string, 0, 4)
		for _, id := range splitIDs(raw) {
			if _, err := uuid.Parse(id); err == nil {
				ids = append(ids, id)
			}
		}
		// Only "Sem cliente registrado" (or garbage) was picked: nobody's payments.
		if len(ids) == 0 {
			h.sendClientPaymentHistory(w, r, nil, nil)
			return
		}
		clients = clients.Where("id IN ?", ids)
	}
	var clientIDs []string
	if err := clients.Pluck("id", &clientIDs).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the client payments")
		return
	}
	if len(clientIDs) == 0 {
		h.sendClientPaymentHistory(w, r, nil, nil)
		return
	}

	// paid_on is a calendar day with no time zone, so the period's days compare as they are
	// - unlike issuances.created_at, which parseDateRange has to shift into UTC.
	query := h.DB.Preload("CreatedBy").Preload("DeletedBy").Where("client_id IN ?", clientIDs)
	if day, err := time.Parse("2006-01-02", q.Get("date_from")); err == nil {
		query = query.Where("paid_on >= ?", day)
	}
	if day, err := time.Parse("2006-01-02", q.Get("date_to")); err == nil {
		query = query.Where("paid_on <= ?", day)
	}

	var payments []data.ClientPayment
	if err := query.Order("paid_on DESC, created_at DESC, id").Find(&payments).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the client payments")
		return
	}

	h.sendClientPaymentHistory(w, r, payments, clientIDs)
}

// sendClientPaymentHistory adds to the payments where each one's money sits, whose it is,
// and the credit the clients hold, and answers with the history.
func (h *SaleHandler) sendClientPaymentHistory(w http.ResponseWriter, r *http.Request, payments []data.ClientPayment, clientIDs []string) {
	resp := schemas.ClientPaymentHistoryResponse{
		Payments: make([]schemas.ClientPaymentHistoryItem, 0, len(payments)),
	}
	if len(payments) == 0 && len(clientIDs) == 0 {
		utils.SendJSON(w, r, http.StatusOK, "Client payments retrieved successfully", resp)
		return
	}

	paymentIDs := make([]string, 0, len(payments))
	for _, payment := range payments {
		paymentIDs = append(paymentIDs, payment.ID)
	}

	var allocations []struct {
		ClientPaymentID string
		SaleID          string
		ProgramLocator  string
		AmountCents     int
	}
	err := h.DB.Table("sale_payments").
		Select("sale_payments.client_payment_id, sale_payments.sale_id, issuances.program_locator, sale_payments.amount_cents").
		Joins("JOIN sales ON sales.id = sale_payments.sale_id").
		Joins("JOIN issuances ON issuances.id = sales.issuance_id").
		Where("sale_payments.client_payment_id IN ?", paymentIDs).
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

	var clients []data.Client
	if err := h.DB.Select("id", "name").Where("id IN ?", clientIDs).Find(&clients).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the client payments")
		return
	}
	names := make(map[string]string, len(clients))
	for _, client := range clients {
		names[client.ID] = client.Name
	}

	resp.CreditCents, err = sale.CreditOfClients(h.DB, clientIDs)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the client payments")
		return
	}

	for _, payment := range payments {
		item := schemas.ClientPaymentHistoryItem{
			ID:          payment.ID,
			ClientID:    payment.ClientID,
			ClientName:  names[payment.ClientID],
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

	var payment data.ClientPayment
	if err := h.DB.Select("client_id").First(&payment, "id = ?", paymentID).Error; err == nil {
		if !h.clientInScope(w, r, payment.ClientID) {
			return
		}
	}

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
	// The same for a line moving a sale's surplus: it follows the payments of the sale it
	// came from, and changes when those do.
	if payment.CreditFromSaleID != nil {
		utils.SendError(w, r, http.StatusConflict, "Esse lançamento é crédito do cliente vindo de outra emissão; ele muda sozinho com os pagamentos daquela emissão")
		return
	}

	var found data.Sale
	if err := h.DB.First(&found, "id = ?", saleID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Sale not found")
		return
	}

	var rebalancedSaleIDs []string
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&payment).Error; err != nil {
			return err
		}
		var err error
		rebalancedSaleIDs, err = recalculateWithClient(tx, found)
		return err
	})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete the payment: "+err.Error())
		return
	}

	h.broadcastSalesUpdated(saleID, rebalancedSaleIDs)

	utils.SendJSON(w, r, http.StatusOK, "Payment deleted successfully", nil)
}

// settleSaleFromLegacyFlag is the one-click paid switch: true records a payment for
// whatever is missing; false clears the statement. It started as a compatibility step for
// the panel that predated the statement, and the payments screen's Pago / A Pagar button is
// built on it again - partial amounts go through CreateClientPayment instead.
func settleSaleFromLegacyFlag(tx *gorm.DB, saleID string, paid bool, actorID string) error {
	if !paid {
		// Only the sale's own lines: those spread from a client payment, or moving a sale's
		// surplus, are rebuilt by the replay (UpdateSale refuses a sale that has nothing
		// else).
		if err := tx.Where("sale_id = ? AND client_payment_id IS NULL AND credit_from_sale_id IS NULL", saleID).Delete(&data.SalePayment{}).Error; err != nil {
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

// salePartlyPaid is the rule behind the panel's yellow button: a live sale worth something,
// with some money on it and some still missing. It mirrors saleStatus on the panel, and
// lives out here for the same reason rebalanceClient does.
func salePartlyPaid(db *gorm.DB, saleID string) (bool, error) {
	amount, err := sale.AmountOf(db, saleID)
	if err != nil {
		return false, err
	}
	if amount.Cancelled || amount.Cents <= 0 {
		return false, nil
	}
	paidCents, err := sale.PaidCents(db, saleID)
	if err != nil {
		return false, err
	}
	// What is missing only because the ticket was altered after the booking was paid is not
	// where a lump sum ran out: the booking is covered, and the difference is a new charge
	// the operator collects and settles by hand like any other.
	bookingCents := amount.Cents - amount.AdjustmentCents
	return paidCents > 0 && paidCents < bookingCents && sale.BalanceCents(amount, paidCents) > 0, nil
}

// recalculateWithClient follows a change to a sale's own lines: sales.paid first, then the
// replay of its client, since money put on or taken off one sale can free credit for the
// others or need it. It returns the other sales the replay changed.
func recalculateWithClient(tx *gorm.DB, found data.Sale) ([]string, error) {
	if err := sale.Recalculate(tx, found.ID); err != nil {
		return nil, err
	}
	if found.ClientID == nil {
		return nil, nil
	}
	return sale.RebalanceClient(tx, *found.ClientID)
}

// rebalanceClient replays a client's payments after one of their sales changed hands. It
// lives out here because UpdateSale's own variable is called sale, which hides the package.
func rebalanceClient(tx *gorm.DB, clientID string) ([]string, error) {
	return sale.RebalanceClient(tx, clientID)
}

// saleAmountOf and recalculateSale live out here for the same reason.
func saleAmountOf(db *gorm.DB, saleID string) (sale.Amount, error) {
	return sale.AmountOf(db, saleID)
}

func recalculateRepricedSale(tx *gorm.DB, issuanceID string) error {
	return sale.RecalculateRepriced(tx, issuanceID)
}

func optionalText(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	return value
}

// clientInScope answers 404 itself when the client is outside the caller's agencies, and
// reports whether to carry on. A client payment is spread over the client's open sales, and
// since a client and their sales share one agency, checking the client is checking all of
// them. A client still waiting for an agency (NULL) belongs to nobody's scope but a dev's.
func (h *SaleHandler) clientInScope(w http.ResponseWriter, r *http.Request, clientID string) bool {
	scope := middleware.AgencyScopeFrom(r.Context())
	if scope.All {
		return true
	}
	var client data.Client
	err := scope.Restrict(h.DB, "clients.agency_id").Select("id").First(&client, "id = ?", clientID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.SendError(w, r, http.StatusNotFound, "Cliente não encontrado")
		return false
	}
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load the client: "+err.Error())
		return false
	}
	return true
}
