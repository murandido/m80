package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// A sale changes through two doors: the issuance behind it, which already announces
// itself as issuance.*, and this handler's PATCH. Only the second one needs an event of
// its own - linking a client or marking a sale as paid touches nothing on the issuance,
// so no other stream would carry it. The payload is just the id, as everywhere else:
// whoever is listening refetches with their own filters.
const eventSaleUpdated = "sale.updated"

type SaleHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
	Broker    *events.Broker
}

func NewSaleHandler(db *gorm.DB, broker *events.Broker) *SaleHandler {
	return &SaleHandler{
		DB:        db,
		Validator: validator.New(),
		Broker:    broker,
	}
}

// defaultAgencyID resolves the "agência principal" - the single agency flagged is_default.
// It is what the sales views fall back to when no agency_id is asked for, so the operator
// lands on their own agency instead of a mix of every partner's issuances.
func defaultAgencyID(db *gorm.DB) (string, error) {
	var agency data.Agency
	if err := db.Select("id").Where("is_default = ?", true).First(&agency).Error; err != nil {
		return "", err
	}
	return agency.ID, nil
}

// saleRow is the flat shape scanned out of the sales listing join. A hand-written join
// beats preloading data.Sale.Issuance here because every column the table renders comes
// back in one round trip - at the cost of having to write the soft-delete scope by hand,
// since GORM only applies it to models it resolves itself (see saleListQuery).
type saleRow struct {
	ID         string
	IssuanceID string
	ClientID   *string
	ClientName *string
	Paid       bool

	ProgramLocator    string
	MainPassengerName *string
	IssuedAt          time.Time
	IssuerName        string
	Points            int

	ValueOfTheThousand int
	Taxes              *int
	CancelledAt        *time.Time

	AccountID           string
	AccountNickname     *string
	AccountLogin        *string
	HolderName          *string
	LoyaltyProgramName  string
	LoyaltyProgramColor string
}

// saleListQuery builds the base sales join. Everything the table renders is denormalized
// here in one round trip; the caller adds the filters.
func saleListQuery(db *gorm.DB) *gorm.DB {
	return db.Table("sales").
		Select(`sales.id,
			sales.issuance_id,
			sales.client_id,
			sales.paid,
			clients.name AS client_name,
			issuances.program_locator,
			issuances.main_passenger_name,
			issuances.created_at AS issued_at,
			issuances.points,
			issuances.value_of_the_thousand,
			issuances.taxes,
			issuances.cancelled_at,
			issuances.account_id,
			users.name AS issuer_name,
			account_nicknames.nickname AS account_nickname,
			accounts.login AS account_login,
			holders.name AS holder_name,
			loyalty_programs.name AS loyalty_program_name,
			loyalty_programs.color AS loyalty_program_color`).
		Joins("JOIN issuances ON issuances.id = sales.issuance_id").
		Joins("JOIN accounts ON accounts.id = issuances.account_id").
		Joins("JOIN loyalty_programs ON loyalty_programs.id = issuances.loyalty_program_id").
		Joins("JOIN users ON users.id = issuances.issuer_id").
		Joins("LEFT JOIN clients ON clients.id = sales.client_id").
		Joins("LEFT JOIN account_nicknames ON account_nicknames.id = issuances.account_nickname_id").
		Joins("LEFT JOIN people AS holders ON holders.id = accounts.holder_id").
		// Deleting an issuance also deletes its sale, so this only matters for rows that
		// predate that rule - but the join is hand-written, so nothing else would filter
		// them out. A sale whose issuance was erased has nothing left to charge for.
		Where("issuances.deleted_at IS NULL")
}

// saleAccountLabel mirrors the grid's account labelling (and export.accountLabel): the
// alias picked on the issuance when there is one, then the holder, then the login.
func saleAccountLabel(row saleRow) string {
	if row.AccountNickname != nil && *row.AccountNickname != "" {
		return *row.AccountNickname
	}
	if row.HolderName != nil && *row.HolderName != "" {
		return *row.HolderName
	}
	if row.AccountLogin != nil && *row.AccountLogin != "" {
		return *row.AccountLogin
	}
	return row.AccountID
}

// pointsValueCents converts miles to money at a given price per thousand, rounding up to
// the cent. Rounding up (rather than truncating) matches the ROUNDUP the export
// spreadsheet applies, so the two never disagree by a cent on the same issuance.
func pointsValueCents(points, valueOfTheThousand int) int {
	total := points * valueOfTheThousand
	if total <= 0 {
		return total / 1000
	}
	return (total + 999) / 1000
}

func toSaleResponse(row saleRow) schemas.SaleResponse {
	amount := pointsValueCents(row.Points, row.ValueOfTheThousand)
	if row.Taxes != nil {
		amount += *row.Taxes
	}

	return schemas.SaleResponse{
		ID:                  row.ID,
		IssuanceID:          row.IssuanceID,
		ClientID:            row.ClientID,
		ClientName:          row.ClientName,
		Paid:                row.Paid,
		ProgramLocator:      row.ProgramLocator,
		MainPassengerName:   row.MainPassengerName,
		IssuedAt:            row.IssuedAt,
		IssuerName:          row.IssuerName,
		Points:              row.Points,
		ValueOfTheThousand:  row.ValueOfTheThousand,
		Taxes:               row.Taxes,
		AmountCents:         amount,
		AccountID:           row.AccountID,
		AccountLabel:        saleAccountLabel(row),
		LoyaltyProgramName:  row.LoyaltyProgramName,
		LoyaltyProgramColor: row.LoyaltyProgramColor,
		CancelledAt:         row.CancelledAt,
	}
}

// resolveAgencyFilter reads the agency_id query param, falling back to the main agency.
// The fallback is what makes the operator land on their own agency instead of a mix of
// every partner's issuances, while still allowing them to switch.
func resolveAgencyFilter(db *gorm.DB, r *http.Request) (string, error) {
	if value := r.URL.Query().Get("agency_id"); value != "" {
		return value, nil
	}
	return defaultAgencyID(db)
}

// GetSales handles the GET /sales route.
// @Summary      Listar vendas (Visão Clientes)
// @Description  Lista as vendas com os dados da emissão associada. Sem agency_id, filtra pela agência principal (is_default=true). cancelled_at preenchido significa que a venda foi reaberta pelo cancelamento da emissão. Emissões deletadas não têm venda e nunca aparecem aqui.
// @Tags         Vendas
// @Produce      json
// @Security     BearerAuth
// @Param        date_from           query  string  false  "Data inicial da emissão (YYYY-MM-DD), inclusiva"
// @Param        date_to             query  string  false  "Data final da emissão (YYYY-MM-DD), inclusiva"
// @Param        agency_id           query  string  false  "UUID da agência (padrão: a agência principal)"
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        account_id          query  string  false  "UUID da conta"
// @Param        issuer_id           query  string  false  "UUID do emissor (usuário)"
// @Param        client_id           query  string  false  "UUID do cliente vinculado"
// @Param        paid                query  boolean false  "Filtra por vendas pagas (true) ou em aberto (false)"
// @Success      200  {object}  schemas.SaleListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Nenhuma agência principal configurada ou erro ao buscar vendas"
// @Router       /sales [get]
func (h *SaleHandler) GetSales(w http.ResponseWriter, r *http.Request) {
	agencyID, err := resolveAgencyFilter(h.DB, r)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "No default agency configured")
		return
	}

	query := saleListQuery(h.DB).Where("issuances.agency_id = ?", agencyID)

	q := r.URL.Query()
	for column, param := range map[string]string{
		"issuances.loyalty_program_id": "loyalty_program_id",
		"issuances.account_id":         "account_id",
		"issuances.issuer_id":          "issuer_id",
		"sales.client_id":              "client_id",
	} {
		if value := q.Get(param); value != "" {
			query = query.Where(column+" = ?", value)
		}
	}

	if value := q.Get("paid"); value != "" {
		if paid, err := strconvParseBool(value); err == nil {
			query = query.Where("sales.paid = ?", paid)
		}
	}

	from, to := parseDateRange(r)
	if from != nil {
		query = query.Where("issuances.created_at >= ?", *from)
	}
	if to != nil {
		query = query.Where("issuances.created_at < ?", *to)
	}

	var rows []saleRow
	if err := query.Order("issuances.created_at desc").Scan(&rows).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch sales")
		return
	}

	resp := make([]schemas.SaleResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, toSaleResponse(row))
	}

	utils.SendJSON(w, r, http.StatusOK, "Sales fetched successfully", resp)
}

// UpdateSale handles the PATCH /sales/{id} route.
// @Summary      Atualizar venda
// @Description  Vincula ou desvincula o cliente e/ou marca a venda como paga. Campos ausentes ficam inalterados; client_id explicitamente null desvincula o cliente.
// @Tags         Vendas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                      true  "UUID da venda"
// @Param        payload body  schemas.UpdateSaleRequest   true  "Campos a atualizar"
// @Success      200  {object}  schemas.SaleSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido ou nenhum campo informado"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Venda não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Cliente não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar venda"
// @Router       /sales/{id} [patch]
func (h *SaleHandler) UpdateSale(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// The body is decoded twice on purpose: the struct carries the values, the raw map
	// tells an omitted client_id (leave the link alone) from an explicit null (unlink).
	// A single *string cannot express that difference.
	var present map[string]json.RawMessage
	if err := json.Unmarshal(body, &present); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	var req schemas.UpdateSaleRequest
	if err := json.Unmarshal(body, &req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	var sale data.Sale
	if err := h.DB.First(&sale, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Sale not found")
		return
	}

	updates := map[string]any{}
	if _, ok := present["client_id"]; ok {
		if req.ClientID != nil {
			var client data.Client
			if err := h.DB.First(&client, "id = ?", *req.ClientID).Error; err != nil {
				utils.SendError(w, r, http.StatusUnprocessableEntity, "Cliente não encontrado")
				return
			}
		}
		updates["client_id"] = req.ClientID
	}
	if req.Paid != nil {
		updates["paid"] = *req.Paid
	}

	if len(updates) == 0 {
		utils.SendError(w, r, http.StatusBadRequest, "No fields to update")
		return
	}

	if err := h.DB.Model(&sale).Updates(updates).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update sale: "+err.Error())
		return
	}

	// Re-read through the listing join so the response carries the same denormalized shape
	// the table renders. Deliberately not scoped to any agency: the listing can now be
	// switched to a partner agency, and those rows have to stay editable.
	var row saleRow
	if err := saleListQuery(h.DB).Where("sales.id = ?", id).Scan(&row).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch updated sale")
		return
	}
	if row.ID == "" {
		utils.SendError(w, r, http.StatusNotFound, "Sale not found")
		return
	}

	// Announced only after the write landed, so no panel is ever told about a change that
	// failed.
	h.Broker.Broadcast(events.Event{
		Type:    eventSaleUpdated,
		Payload: map[string]string{"id": sale.ID},
	})

	utils.SendJSON(w, r, http.StatusOK, "Sale updated successfully", toSaleResponse(row))
}

// supplierIssuanceRow is one issuance's contribution to an account's balance. Rows are
// aggregated in Go rather than with SQL SUMs so the period rule - issued in it, refunded
// in it - lives in one testable function instead of inside a CASE expression.
type supplierIssuanceRow struct {
	AccountID      string
	Points         int
	RefundedPoints *int
	CreatedAt      time.Time
	CancelledAt    *time.Time

	AccountLogin        *string
	AccountNickname     *string
	HolderName          *string
	AdministratorName   *string
	LoyaltyProgramName  string
	LoyaltyProgramColor string
}

// GetSupplierBalances handles the GET /sales/suppliers route.
// @Summary      Saldo por fornecedor (Visão Fornecedores)
// @Description  Agrega, por conta, as milhas emitidas no período menos as devolvidas por cancelamentos no mesmo período. Sem agency_id, filtra pela agência principal (is_default=true). Emissões excluídas não entram. Rota restrita a administradores.
// @Tags         Vendas
// @Produce      json
// @Security     BearerAuth
// @Param        date_from           query  string  false  "Data inicial do período (YYYY-MM-DD), inclusiva"
// @Param        date_to             query  string  false  "Data final do período (YYYY-MM-DD), inclusiva"
// @Param        agency_id           query  string  false  "UUID da agência (padrão: a agência principal)"
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        account_id          query  string  false  "UUID da conta"
// @Success      200  {object}  schemas.SupplierBalanceListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Usuário não é administrador"
// @Failure      500  {object}  schemas.ErrorResponse "Nenhuma agência principal configurada ou erro ao calcular saldos"
// @Router       /sales/suppliers [get]
func (h *SaleHandler) GetSupplierBalances(w http.ResponseWriter, r *http.Request) {
	agencyID, err := resolveAgencyFilter(h.DB, r)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "No default agency configured")
		return
	}

	from, to := parseDateRange(r)

	query := h.DB.Table("issuances").
		Select(`issuances.account_id,
			issuances.points,
			issuances.refunded_points,
			issuances.created_at,
			issuances.cancelled_at,
			accounts.login AS account_login,
			-- Sub-select rather than a join: an account can hold several nicknames, and
			-- joining would multiply the issuance rows this aggregation counts. Oldest
			-- wins so the label does not move when someone registers a second alias.
			(SELECT nickname FROM account_nicknames
				WHERE account_nicknames.account_id = accounts.id
				  AND account_nicknames.deleted_at IS NULL
				ORDER BY account_nicknames.created_at ASC
				LIMIT 1) AS account_nickname,
			holders.name AS holder_name,
			administrators.name AS administrator_name,
			loyalty_programs.name AS loyalty_program_name,
			loyalty_programs.color AS loyalty_program_color`).
		Joins("JOIN accounts ON accounts.id = issuances.account_id").
		Joins("JOIN loyalty_programs ON loyalty_programs.id = issuances.loyalty_program_id").
		Joins("LEFT JOIN people AS holders ON holders.id = accounts.holder_id").
		Joins("LEFT JOIN people AS administrators ON administrators.id = accounts.administrator_id").
		// A deleted issuance is a typo being erased, not a debt with the account's
		// administrator - unlike a cancellation, which really did consume and give back
		// miles. The join is hand-written, so this scope has to be explicit.
		Where("issuances.deleted_at IS NULL").
		Where("issuances.agency_id = ?", agencyID)

	q := r.URL.Query()
	for column, param := range map[string]string{
		"issuances.loyalty_program_id": "loyalty_program_id",
		"issuances.account_id":         "account_id",
	} {
		if value := q.Get(param); value != "" {
			query = query.Where(column+" = ?", value)
		}
	}

	// Narrow to rows that can contribute to the period at all: issued in it, or
	// cancelled in it. Without this the aggregation would read the whole table.
	if from != nil || to != nil {
		created := h.DB.Session(&gorm.Session{NewDB: true})
		cancelled := h.DB.Session(&gorm.Session{NewDB: true})
		if from != nil {
			created = created.Where("issuances.created_at >= ?", *from)
			cancelled = cancelled.Where("issuances.cancelled_at >= ?", *from)
		}
		if to != nil {
			created = created.Where("issuances.created_at < ?", *to)
			cancelled = cancelled.Where("issuances.cancelled_at < ?", *to)
		}
		query = query.Where(created.Or(cancelled))
	}

	var rows []supplierIssuanceRow
	if err := query.Scan(&rows).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch supplier balances")
		return
	}

	balances := aggregateSupplierBalances(rows, from, to)

	utils.SendJSON(w, r, http.StatusOK, "Supplier balances fetched successfully", balances)
}

// aggregateSupplierBalances folds per-issuance rows into one balance per account: miles
// issued inside the period minus miles refunded by cancellations inside the same period.
// An issuance created and cancelled in the same period contributes to both sides, which
// is what makes the two cancel out to zero without any same-day special case.
func aggregateSupplierBalances(rows []supplierIssuanceRow, from, to *time.Time) []schemas.SupplierBalanceResponse {
	type accumulator struct {
		row            supplierIssuanceRow
		issuedPoints   int
		refundedPoints int
	}

	byAccount := map[string]*accumulator{}
	for _, row := range rows {
		acc, ok := byAccount[row.AccountID]
		if !ok {
			acc = &accumulator{row: row}
			byAccount[row.AccountID] = acc
		}

		if inRange(&row.CreatedAt, from, to) {
			acc.issuedPoints += row.Points
		}
		if inRange(row.CancelledAt, from, to) && row.RefundedPoints != nil {
			acc.refundedPoints += *row.RefundedPoints
		}
	}

	balances := make([]schemas.SupplierBalanceResponse, 0, len(byAccount))
	for accountID, acc := range byAccount {
		balance := schemas.SupplierBalanceResponse{
			AccountID:           accountID,
			AccountLabel:        supplierAccountLabel(acc.row),
			LoyaltyProgramName:  acc.row.LoyaltyProgramName,
			LoyaltyProgramColor: acc.row.LoyaltyProgramColor,
			AdministratorName:   acc.row.AdministratorName,
			IssuedPoints:        acc.issuedPoints,
			RefundedPoints:      acc.refundedPoints,
			NetPoints:           acc.issuedPoints - acc.refundedPoints,
		}
		balances = append(balances, balance)
	}

	// Biggest debt first; the label breaks ties so the order is stable across requests
	// (ranging over a map is deliberately randomised in Go).
	sort.Slice(balances, func(i, j int) bool {
		if balances[i].NetPoints != balances[j].NetPoints {
			return balances[i].NetPoints > balances[j].NetPoints
		}
		return balances[i].AccountLabel < balances[j].AccountLabel
	})

	return balances
}

// supplierAccountLabel identifies an account in the suppliers view. Unlike the sales
// listing it never uses the issuance's alias: the row is the account itself, aggregated
// over many issuances that may each have picked a different alias. The nickname here is
// the account's own, chosen deterministically by the query.
//
// The chain matches the account picker in the panel (account-combobox.tsx), down to
// truncating the id, so the same account reads the same way wherever it appears.
func supplierAccountLabel(row supplierIssuanceRow) string {
	if row.HolderName != nil && *row.HolderName != "" {
		return *row.HolderName
	}
	if row.AccountLogin != nil && *row.AccountLogin != "" {
		return *row.AccountLogin
	}
	if row.AccountNickname != nil && *row.AccountNickname != "" {
		return *row.AccountNickname
	}
	// Last resort for an account with no holder, no login and no alias. A full UUID
	// is unreadable and identifies nothing to the operator; the first block is enough
	// to tell two such rows apart.
	if len(row.AccountID) > 8 {
		return row.AccountID[:8]
	}
	return row.AccountID
}

// strconvParseBool accepts only the two spellings the frontend sends, so a stray
// "paid=maybe" leaves the listing unfiltered instead of silently meaning false.
func strconvParseBool(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, errors.New("not a boolean")
}
