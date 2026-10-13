package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/export"
	"github.com/murandido/m-80-backend/internal/inventory"
	"github.com/murandido/m-80-backend/internal/sale"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// broadcastSaleUpdated announces a persisted change. Called only after the write landed, so
// no panel is ever told about a change that failed. The payload is just the id, as
// everywhere else: whoever is listening refetches with their own filters.
func (h *SaleHandler) broadcastSaleUpdated(saleID string) {
	h.Broker.Broadcast(events.Event{
		Type:    eventSaleUpdated,
		Payload: map[string]string{"id": saleID},
	})
}

func NewSaleHandler(db *gorm.DB, broker *events.Broker) *SaleHandler {
	return &SaleHandler{
		DB:        db,
		Validator: validator.New(),
		Broker:    broker,
	}
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
	Note       *string

	ProgramLocator    string
	MainPassengerName *string
	IssuedAt          time.Time
	IssuerName        string
	Points            int

	ValueOfTheThousand int
	Taxes              *int
	// AdjustmentCents is what the ticket's alterations add to the price (sale.AdjustmentCentsJoin).
	AdjustmentCents int
	PaidCents       int
	CancelledAt     *time.Time
	// CancellationFee is the airline's fee on a cancelled ticket, which the client pays.
	CancellationFee *int
	// PriceAdjustmentCents is what the payments control set on top of the computed price.
	PriceAdjustmentCents int

	ClientPaymentCents  int
	ClientPaymentPaidOn *time.Time

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
			sales.note,
			sales.price_adjustment_cents,
			clients.name AS client_name,
			issuances.program_locator,
			issuances.main_passenger_name,
			issuances.created_at AS issued_at,
			issuances.points,
			issuances.value_of_the_thousand,
			issuances.taxes,
			COALESCE(adjustments.cents, 0) AS adjustment_cents,
			COALESCE(payments.paid_cents, 0) AS paid_cents,
			COALESCE(payments.client_payment_cents, 0) AS client_payment_cents,
			payments.client_payment_paid_on,
			issuances.cancelled_at,
			issuances.cancellation_fee,
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
		// What the ticket's alterations added to (or took off) the price.
		Joins("LEFT JOIN " + sale.AdjustmentCentsJoin + " AS adjustments ON adjustments.issuance_id = issuances.id").
		// Aggregated in the join rather than per row: the statement of every sale on the
		// screen in one pass, instead of one query per line.
		Joins(`LEFT JOIN (
			SELECT sale_id,
				SUM(amount_cents) AS paid_cents,
				SUM(amount_cents) FILTER (WHERE client_payment_id IS NOT NULL) AS client_payment_cents,
				MAX(paid_on) FILTER (WHERE client_payment_id IS NOT NULL) AS client_payment_paid_on
			FROM sale_payments
			GROUP BY sale_id
		) AS payments ON payments.sale_id = sales.id`).
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

func toSaleResponse(row saleRow) schemas.SaleResponse {
	amount := sale.PointsValueCents(row.Points, row.ValueOfTheThousand) + row.AdjustmentCents
	if row.Taxes != nil {
		amount += *row.Taxes
	}

	var clientPaymentPaidOn *string
	if row.ClientPaymentPaidOn != nil {
		day := row.ClientPaymentPaidOn.Format("2006-01-02")
		clientPaymentPaidOn = &day
	}

	computed := amount
	if row.CancelledAt == nil {
		amount += row.PriceAdjustmentCents
	}
	saleAmount := sale.Amount{Cents: amount, Cancelled: row.CancelledAt != nil}
	if saleAmount.Cancelled && row.CancellationFee != nil {
		saleAmount.CancellationFeeCents = *row.CancellationFee
	}

	return schemas.SaleResponse{
		ID:                   row.ID,
		PaidCents:            row.PaidCents,
		ClientPaymentCents:   row.ClientPaymentCents,
		ClientPaymentPaidOn:  clientPaymentPaidOn,
		AdjustmentCents:      row.AdjustmentCents,
		BalanceCents:         sale.BalanceCents(saleAmount, row.PaidCents),
		CancellationFeeCents: saleAmount.CancellationFeeCents,
		PriceAdjustmentCents: row.PriceAdjustmentCents,
		ComputedAmountCents:  computed,
		IssuanceID:           row.IssuanceID,
		ClientID:             row.ClientID,
		ClientName:           row.ClientName,
		Paid:                 row.Paid,
		Note:                 row.Note,
		ProgramLocator:       row.ProgramLocator,
		MainPassengerName:    row.MainPassengerName,
		IssuedAt:             row.IssuedAt,
		IssuerName:           row.IssuerName,
		Points:               row.Points,
		ValueOfTheThousand:   row.ValueOfTheThousand,
		Taxes:                row.Taxes,
		AmountCents:          amount,
		AccountID:            row.AccountID,
		AccountLabel:         saleAccountLabel(row),
		LoyaltyProgramName:   row.LoyaltyProgramName,
		LoyaltyProgramColor:  row.LoyaltyProgramColor,
		CancelledAt:          row.CancelledAt,
	}
}

// errAgencyOutOfScope is an agency_id the caller asked for but may not see. It answers like
// an agency that does not exist.
var errAgencyOutOfScope = errors.New("agency outside the caller's scope")

// resolveAgencyFilter reads the agency_id query param, falling back to the caller's default
// agency: the main one when they may see it, else their first (access.ResolveDefaultAgency).
// The fallback is what makes the operator land on their own agency instead of a mix of every
// partner's issuances, while still allowing them to switch - within their scope.
func resolveAgencyFilter(db *gorm.DB, r *http.Request) (string, error) {
	scope := middleware.AgencyScopeFrom(r.Context())
	if value := r.URL.Query().Get("agency_id"); value != "" {
		if !scope.Allows(value) {
			return "", errAgencyOutOfScope
		}
		return value, nil
	}
	return access.ResolveDefaultAgency(db, scope)
}

// sendAgencyFilterError answers a failed resolveAgencyFilter. The two scope outcomes are
// kept apart from a real failure: a user with no agency is a 403 the panel can explain, and
// an out-of-scope agency is a plain 404.
func sendAgencyFilterError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, access.ErrNoAgencyAccess):
		utils.SendError(w, r, http.StatusForbidden, "Você não tem acesso a nenhuma agência")
	case errors.Is(err, errAgencyOutOfScope):
		utils.SendError(w, r, http.StatusNotFound, "Agency not found")
	default:
		utils.SendError(w, r, http.StatusInternalServerError, "No default agency configured")
	}
}

// allAgenciesToken is the explicit "every agency" choice on the suppliers view. It exists
// because an empty query param cannot mean it: the panel drops empty params from the URL,
// so "nothing selected" and "the param was never sent" arrive identical - and the second
// one has to keep meaning the main agency, or an old bookmark would silently widen into
// every partner's issuances.
const allAgenciesToken = "all"

// splitIDs reads one of the comma-separated id params the suppliers view sends. Blank
// entries are dropped rather than passed through: a trailing comma would otherwise reach
// the query as an empty string and, in a NOT IN, match nothing at all.
func splitIDs(raw string) []string {
	ids := make([]string, 0, 4)
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// resolveSupplierAgencyFilter reads the suppliers view's multi-valued agency choice. It
// returns nil when the caller asked for every agency, and otherwise the ids to restrict to.
//
// Precedence is deliberate: agency_ids wins when present, and its absence falls all the way
// back to resolveAgencyFilter - the single agency_id, then the main agency. That keeps every
// existing caller, bookmark and sticky-filter cookie behaving exactly as it did.
func resolveSupplierAgencyFilter(db *gorm.DB, r *http.Request) ([]string, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("agency_ids"))
	if raw == "" {
		agencyID, err := resolveAgencyFilter(db, r)
		if err != nil {
			return nil, err
		}
		return []string{agencyID}, nil
	}

	// "Every agency" is every agency the caller may see - nil (no restriction) only for dev.
	scope := middleware.AgencyScopeFrom(r.Context())
	if raw == allAgenciesToken {
		return scope.Intersect(nil), nil
	}

	ids := scope.Intersect(splitIDs(raw))
	// A list that parsed down to nothing is a malformed param, not a request for
	// everything - fall back rather than quietly widening the query.
	if len(ids) == 0 {
		agencyID, err := resolveAgencyFilter(db, r)
		if err != nil {
			return nil, err
		}
		return []string{agencyID}, nil
	}

	return ids, nil
}

// applySaleListFilters applies the listing filters to a query built by saleListQuery. It is
// shared by GetSales and GetSalesExport so the spreadsheet can never contain a different set
// of sales than the screen it was downloaded from. agency_id is deliberately not handled
// here: it needs the DB to resolve the main agency, and resolveAgencyFilter already owns it.
func applySaleListFilters(query *gorm.DB, r *http.Request) *gorm.DB {
	q := r.URL.Query()

	for column, param := range map[string]string{
		"issuances.loyalty_program_id": "loyalty_program_id",
		"issuances.account_id":         "account_id",
		"issuances.issuer_id":          "issuer_id",
	} {
		if value := q.Get(param); value != "" {
			query = query.Where(column+" = ?", value)
		}
	}

	query = applySaleClientFilter(query, r)

	if value := q.Get("paid"); value != "" {
		if paid, err := strconvParseBool(value); err == nil {
			query = query.Where("sales.paid = ?", paid)
		}
	}

	// created_at is UTC in the DB; parseDateRange resolves the local calendar days into UTC
	// bounds so "today" matches the operator's day.
	//
	// A sale also belongs to the day its ticket was altered, when the alteration changed what
	// the client owes: that is the day somebody has to go and charge (or pay back) the
	// difference. An alteration the agency absorbed changes nothing to collect, so it does
	// not bring an old sale back.
	from, to := parseDateRange(r)
	if from != nil || to != nil {
		created := []string{}
		createdArgs := []any{}
		if from != nil {
			created = append(created, "issuances.created_at >= ?")
			createdArgs = append(createdArgs, *from)
		}
		if to != nil {
			created = append(created, "issuances.created_at < ?")
			createdArgs = append(createdArgs, *to)
		}
		altered := []string{
			"issuance_adjustments.issuance_id = issuances.id",
			"issuance_adjustments.deleted_at IS NULL",
			"(issuance_adjustments.value_of_the_thousand > 0 OR issuance_adjustments.taxes_passed_to_client)",
		}
		altered, alteredArgs := appendAdjustmentDayBounds(altered, nil, from, to)

		// And to the day its ticket was cancelled, when the cancellation left something to
		// do: a fee to collect, or money to give back.
		cancelled := []string{
			"issuances.cancelled_at IS NOT NULL",
			"(COALESCE(issuances.cancellation_fee, 0) > 0 OR COALESCE(payments.paid_cents, 0) <> 0)",
		}
		cancelledArgs := []any{}
		if from != nil {
			cancelled = append(cancelled, "issuances.cancelled_at >= ?")
			cancelledArgs = append(cancelledArgs, *from)
		}
		if to != nil {
			cancelled = append(cancelled, "issuances.cancelled_at < ?")
			cancelledArgs = append(cancelledArgs, *to)
		}

		args := append(append(createdArgs, alteredArgs...), cancelledArgs...)
		query = query.Where(
			"("+strings.Join(created, " AND ")+")"+
				" OR EXISTS (SELECT 1 FROM issuance_adjustments WHERE "+strings.Join(altered, " AND ")+")"+
				" OR ("+strings.Join(cancelled, " AND ")+")",
			args...,
		)
	}

	return query
}

// alterationCentsInWindow sums, per issuance, what the alterations inside the period charge
// or give back to the client - for the sales that are listed only because of them (issued
// outside the period). Empty when the listing has no period, or no such sale.
func alterationCentsInWindow(db *gorm.DB, rows []saleRow, from, to *time.Time) map[string]int {
	totals := map[string]int{}
	if from == nil && to == nil {
		return totals
	}
	issuanceIDs := make([]string, 0)
	for _, row := range rows {
		issuedAt := row.IssuedAt
		if !inRange(&issuedAt, from, to) {
			issuanceIDs = append(issuanceIDs, row.IssuanceID)
		}
	}
	if len(issuanceIDs) == 0 {
		return totals
	}

	clauses, args := appendAdjustmentDayBounds([]string{"issuance_id IN ?"}, []any{issuanceIDs}, from, to)
	var adjustments []data.IssuanceAdjustment
	if err := db.Where(strings.Join(clauses, " AND "), args...).Find(&adjustments).Error; err != nil {
		slog.Error("failed to read the alterations of the listed sales", "err", err)
		return totals
	}
	for _, adjustment := range adjustments {
		totals[adjustment.IssuanceID] += sale.AdjustmentClientCents(
			adjustment.Points, adjustment.ValueOfTheThousand, adjustment.Taxes, adjustment.TaxesPassedToClient)
	}
	return totals
}

// asAlterationDay reshapes a sale listed on the day its ticket was altered. If everything
// owed before those alterations had been paid, the row shows only the alteration: its value,
// and what has come in towards it. Otherwise the client still owes the booking too, and the
// row shows the whole sale. The balance is the same either way - what is still missing - so
// marking the row paid settles exactly that.
//
// A cancelled sale is left whole: its balance is money to give back, not an alteration to
// collect.
func asAlterationDay(response schemas.SaleResponse, alterations map[string]int, from, to *time.Time) schemas.SaleResponse {
	issuedAt := response.IssuedAt
	if (from == nil && to == nil) || inRange(&issuedAt, from, to) {
		return response
	}
	// Listed outside its issue day for another reason - its cancellation day.
	inWindow, altered := alterations[response.IssuanceID]
	if !altered {
		return response
	}
	response.AlterationDay = true
	if response.CancelledAt != nil {
		return response
	}

	owedBefore := response.AmountCents - inWindow
	if response.PaidCents >= owedBefore {
		response.AmountCents = inWindow
		response.PaidCents -= owedBefore
		response.ClientPaymentCents = max(0, response.ClientPaymentCents-owedBefore)
	}
	return response
}

// noClientToken is the "Sem cliente registrado" entry of the clients view's filter. It is
// not a uuid on purpose, so it can never collide with a client. Has to match NO_CLIENT in
// frontend/src/app/dashboard/sales/period-params.ts.
const noClientToken = "none"

// applySaleClientFilter narrows the listing to the chosen clients. client_ids is the
// multi-valued choice and may carry noClientToken for the sales nobody was linked to;
// client_id is the single choice an older panel tab (or a remembered filter cookie) still
// sends, and only applies when client_ids is absent.
//
// Entries that are not uuids are dropped here: compared against the uuid column they would
// fail the whole query instead of matching nothing. A list that parses down to nothing
// matches nothing, rather than silently widening into every client.
func applySaleClientFilter(query *gorm.DB, r *http.Request) *gorm.DB {
	q := r.URL.Query()
	raw := strings.TrimSpace(q.Get("client_ids"))
	if raw == "" {
		if value := q.Get("client_id"); value != "" {
			return query.Where("sales.client_id = ?", value)
		}
		return query
	}

	withoutClient := false
	ids := make([]string, 0, 4)
	for _, id := range splitIDs(raw) {
		if id == noClientToken {
			withoutClient = true
			continue
		}
		if _, err := uuid.Parse(id); err == nil {
			ids = append(ids, id)
		}
	}

	switch {
	case withoutClient && len(ids) > 0:
		return query.Where("(sales.client_id IN ? OR sales.client_id IS NULL)", ids)
	case withoutClient:
		return query.Where("sales.client_id IS NULL")
	case len(ids) > 0:
		return query.Where("sales.client_id IN ?", ids)
	default:
		return query.Where("1 = 0")
	}
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
// @Param        client_id           query  string  false  "UUID do cliente vinculado (ignorado quando client_ids vem)"
// @Param        client_ids          query  string  false  "UUIDs de clientes separados por vírgula; 'none' inclui as vendas sem cliente"
// @Param        paid                query  boolean false  "Filtra por vendas pagas (true) ou em aberto (false)"
// @Success      200  {object}  schemas.SaleListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Nenhuma agência principal configurada ou erro ao buscar vendas"
// @Router       /sales [get]
func (h *SaleHandler) GetSales(w http.ResponseWriter, r *http.Request) {
	agencyID, err := resolveAgencyFilter(h.DB, r)
	if err != nil {
		sendAgencyFilterError(w, r, err)
		return
	}

	query := applySaleListFilters(
		saleListQuery(h.DB).Where("issuances.agency_id = ?", agencyID),
		r,
	)

	// Oldest first, like the spreadsheet the screen replaced: it is read top to bottom in
	// the order the tickets were issued, which is also the order a client's payments are
	// spread in - the partly paid row sits right below the settled ones.
	var rows []saleRow
	if err := query.Order("issuances.created_at asc, sales.id").Scan(&rows).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch sales")
		return
	}

	from, to := parseDateRange(r)
	alterations := alterationCentsInWindow(h.DB, rows, from, to)

	resp := make([]schemas.SaleResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, asAlterationDay(toSaleResponse(row), alterations, from, to))
	}

	utils.SendJSON(w, r, http.StatusOK, "Sales fetched successfully", resp)
}

// GetSalesExport handles the GET /sales/export route.
// @Summary      Exportar vendas (.xlsx)
// @Description  Gera uma planilha .xlsx das vendas filtradas, com as colunas DATA, CLIENTE, LOCALIZADOR, EMISSÃO e STATUS, da mais antiga para a mais recente. Aceita os mesmos filtros da listagem. STATUS é PG (paga), APG (em aberto) ou CANC (emissão cancelada, tenha sido paga ou não).
// @Tags         Vendas
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security     BearerAuth
// @Param        date_from           query  string  false  "Data inicial da emissão (YYYY-MM-DD), inclusiva"
// @Param        date_to             query  string  false  "Data final da emissão (YYYY-MM-DD), inclusiva"
// @Param        agency_id           query  string  false  "UUID da agência (padrão: a agência principal)"
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        account_id          query  string  false  "UUID da conta"
// @Param        issuer_id           query  string  false  "UUID do emissor (usuário)"
// @Param        client_id           query  string  false  "UUID do cliente vinculado (ignorado quando client_ids vem)"
// @Param        client_ids          query  string  false  "UUIDs de clientes separados por vírgula; 'none' inclui as vendas sem cliente"
// @Param        paid                query  boolean false  "Filtra por vendas pagas (true) ou em aberto (false)"
// @Success      200  {file}    binary  "Planilha .xlsx das vendas filtradas"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Nenhuma agência principal configurada ou erro ao gerar a planilha"
// @Router       /sales/export [get]
func (h *SaleHandler) GetSalesExport(w http.ResponseWriter, r *http.Request) {
	agencyID, err := resolveAgencyFilter(h.DB, r)
	if err != nil {
		sendAgencyFilterError(w, r, err)
		return
	}

	// Same filters and the same chronological order as the listing, so the spreadsheet
	// reads like the screen it was exported from.
	query := applySaleListFilters(
		saleListQuery(h.DB).Where("issuances.agency_id = ?", agencyID),
		r,
	)

	var rows []saleRow
	if err := query.Order("issuances.created_at asc").Scan(&rows).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch sales")
		return
	}

	from, to := parseDateRange(r)
	alterations := alterationCentsInWindow(h.DB, rows, from, to)

	sales := make([]export.SaleRow, 0, len(rows))
	for _, row := range rows {
		// Through toSaleResponse rather than recomputing the amount here: a second copy of
		// the formula is how the sheet and the screen start disagreeing by a cent.
		response := asAlterationDay(toSaleResponse(row), alterations, from, to)
		sales = append(sales, export.SaleRow{
			IssuedAt:       response.IssuedAt,
			ClientName:     response.ClientName,
			ProgramLocator: response.ProgramLocator,
			AmountCents:    response.AmountCents,
			Paid:           response.Paid,
			CancelledAt:    response.CancelledAt,
		})
	}

	content, err := export.BuildSalesXLSX(sales)
	if err != nil {
		slog.Error("failed to build sales export", "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to build sales export")
		return
	}

	// Binary response: the standard JSON envelope does not apply, mirroring the issuances
	// export and the PDF path.
	filename := fmt.Sprintf("vendas-%s.xlsx", exportFilenameDate(time.Now()))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	if _, err := w.Write(content); err != nil {
		slog.Error("failed to write sales export response", "err", err)
	}
}

// UpdateSale handles the PATCH /sales/{id} route.
// @Summary      Atualizar venda
// @Description  Vincula ou desvincula o cliente, grava a observação e/ou marca a venda como paga. Campos ausentes ficam inalterados; client_id ou note explicitamente null limpam o campo.
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
// @Failure      409  {object}  schemas.ErrorResponse "paid=false numa venda paga por pagamento do cliente, ou paid numa venda parcialmente paga"
// @Failure      422  {object}  schemas.ErrorResponse "Cliente não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar venda"
// @Router       /sales/{id} [patch]
func (h *SaleHandler) UpdateSale(w http.ResponseWriter, r *http.Request) {
	// Needed only by the legacy paid translation below, which records a payment and has to
	// credit somebody for it.
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

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
			// A sale and its client belong to the same agency. That is what keeps a client's
			// payments inside one agency - they are spread over the client's open sales - and
			// what lets a user see a client exactly when they can see the client's sales.
			var issuance data.Issuance
			if err := h.DB.Unscoped().Select("agency_id").First(&issuance, "id = ?", sale.IssuanceID).Error; err != nil {
				utils.SendError(w, r, http.StatusInternalServerError, "Failed to load the sale's issuance: "+err.Error())
				return
			}
			if client.AgencyID == nil || *client.AgencyID != issuance.AgencyID {
				utils.SendError(w, r, http.StatusUnprocessableEntity, "O cliente é de outra agência")
				return
			}
		}
		updates["client_id"] = req.ClientID
	}
	if _, ok := present["note"]; ok {
		// An emptied field and an explicit null are the same fact - nothing written - so
		// both are stored as NULL rather than leaving two spellings of "no note".
		if req.Note != nil && strings.TrimSpace(*req.Note) != "" {
			updates["note"] = strings.TrimSpace(*req.Note)
		} else {
			updates["note"] = nil
		}
	}

	// The amount to receive is typed as a total and kept as its difference from the
	// computed price, so a later correction to the points or the rate moves the total with
	// it. Typing the computed price back clears the difference.
	priceChanged := false
	if req.AmountCents != nil {
		amount, err := saleAmountOf(h.DB, sale.ID)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to update sale: "+err.Error())
			return
		}
		if amount.Cancelled {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "O valor de uma venda cancelada é a multa de cancelamento")
			return
		}
		computed := amount.Cents - amount.PriceAdjustmentCents
		updates["price_adjustment_cents"] = *req.AmountCents - computed
		priceChanged = true
	}

	if len(updates) == 0 && req.Paid == nil {
		utils.SendError(w, r, http.StatusBadRequest, "No fields to update")
		return
	}

	// A partly paid sale is where a client's lump sum ran out. Settling it by hand, or
	// clearing it, would move money the operator never received or wipe the trace of the
	// payment that did arrive - only another client payment is allowed to move it.
	if req.Paid != nil {
		partial, err := salePartlyPaid(h.DB, sale.ID)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to update sale: "+err.Error())
			return
		}
		if partial {
			utils.SendError(w, r, http.StatusConflict, "Essa venda está parcialmente paga; só um novo pagamento do cliente pode alterá-la")
			return
		}
	}

	// The switch only takes back what the switch (or the old per-sale statement) put on the
	// sale. Money a client's payment put here is taken back by undoing that payment:
	// clearing its lines would last until the next replay put them back. So a sale covered
	// only by client payments has nothing the switch can undo, and says so.
	if req.Paid != nil && !*req.Paid {
		var lines struct {
			Own    int64
			Spread int64
		}
		if err := h.DB.Model(&data.SalePayment{}).
			Select(`COUNT(*) FILTER (WHERE client_payment_id IS NULL) AS own,
				COUNT(*) FILTER (WHERE client_payment_id IS NOT NULL) AS spread`).
			Where("sale_id = ?", sale.ID).
			Scan(&lines).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to update sale: "+err.Error())
			return
		}
		if lines.Own == 0 && lines.Spread > 0 {
			utils.SendError(w, r, http.StatusConflict, "Essa venda foi paga por um pagamento do cliente; desfaça o pagamento no histórico")
			return
		}
	}

	// Who the sale belonged to before and after, for the replay below. Both, because the
	// money of a client's payments follows the client, not the sale: moving a paid sale
	// takes the old client's money off it (back to their other sales, or their credit)
	// and lets the new client's credit reach it.
	var clientsToRebalance []string
	if _, ok := present["client_id"]; ok {
		if sale.ClientID != nil {
			clientsToRebalance = append(clientsToRebalance, *sale.ClientID)
		}
		if req.ClientID != nil && (sale.ClientID == nil || *sale.ClientID != *req.ClientID) {
			clientsToRebalance = append(clientsToRebalance, *req.ClientID)
		}
	}

	// Sales the replay changed besides this one, so their rows refresh too.
	var rebalancedSaleIDs []string
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&sale).Updates(updates).Error; err != nil {
				return err
			}
		}
		for _, clientID := range clientsToRebalance {
			changed, err := rebalanceClient(tx, clientID)
			if err != nil {
				return err
			}
			rebalancedSaleIDs = append(rebalancedSaleIDs, changed...)
		}
		// What is owed moved: sales.paid follows, and the client's lump sums are replayed
		// over the new figure - more to cover, or money freed for the next sale.
		if priceChanged {
			if err := recalculateSale(tx, sale.IssuanceID); err != nil {
				return err
			}
		}
		// paid is no longer a column an operator sets: it is the sum of the sale's
		// payments. A client still sending it gets that intent translated into the
		// statement - see settleSaleFromLegacyFlag for why this exists and when it goes.
		if req.Paid == nil {
			return nil
		}
		if err := settleSaleFromLegacyFlag(tx, sale.ID, *req.Paid, userCtx.ID); err != nil {
			return err
		}
		// Clearing the sale's own lines leaves room the client's credit can fill, and
		// settling it by hand frees what a client payment had put there for the next sale.
		var owner data.Sale
		if err := tx.Select("client_id").First(&owner, "id = ?", sale.ID).Error; err != nil {
			return err
		}
		if owner.ClientID == nil {
			return nil
		}
		changed, err := rebalanceClient(tx, *owner.ClientID)
		rebalancedSaleIDs = append(rebalancedSaleIDs, changed...)
		return err
	})
	if err != nil {
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

	h.broadcastSaleUpdated(sale.ID)
	for _, saleID := range rebalancedSaleIDs {
		if saleID != sale.ID {
			h.broadcastSaleUpdated(saleID)
		}
	}

	utils.SendJSON(w, r, http.StatusOK, "Sale updated successfully", toSaleResponse(row))
}

// supplierIssuanceRow is one issuance's contribution to an account's balance. Rows are
// aggregated in Go rather than with SQL SUMs so the period rule - issued in it, refunded
// in it - lives in one testable function instead of inside a CASE expression.
type supplierIssuanceRow struct {
	IssuanceID     string
	AccountID      string
	Points         int
	RefundedPoints *int
	CreatedAt      time.Time
	CancelledAt    *time.Time
	// CancellationOnly says the row was invented by the spreadsheet importer purely to
	// carry an estorno - see data.Issuance.CancellationOnly. Its points were never miles
	// this agency issued, so only its refund side counts.
	CancellationOnly bool

	ProgramLocator      string
	AirlineLocator      *string
	OriginIATACode      *string
	DestinationIATACode *string
	Taxes               *int
	RefundedTaxes       *int
	CostOfTheThousand   *int
	TaxesIncludedInCost bool
	Reviewed            bool
	// OwnsPointInventory says the cost of this issuance is derived from the points
	// inventory rather than typed on this screen. On those accounts we bought the points
	// ourselves, so there is no transfer price to agree with an administrator - see
	// data.Account.OwnsPointInventory.
	OwnsPointInventory bool

	AccountLogin      *string
	AccountNickname   *string
	HolderName        *string
	AdministratorID   *string
	AdministratorName *string
	// AdministratorExportCounter is the administrator's supplier-PDF export count. Nil on an
	// account with no administrator (the LEFT JOIN produces NULL there).
	AdministratorExportCounter *int
	LoyaltyProgramName         string
	LoyaltyProgramColor        string
	// LoyaltyProgramCode and DefaultAirlineCode resolve the brand mark on the payment
	// summary, program first and its airline second - TudoAzul and AZUL share artwork,
	// Smiles and GOL do not. They stay off the JSON response: the panel has no use for
	// them, and the logos are a server-side asset.
	LoyaltyProgramCode string
	DefaultAirlineCode *string
}

// GetSupplierBalances handles the GET /sales/suppliers route.
// @Summary      Saldo por fornecedor (Visão Fornecedores)
// @Description  Agrega, por conta, as milhas emitidas no período menos as devolvidas por cancelamentos no mesmo período. Sem agency_id nem agency_ids, filtra pela agência principal (is_default=true); agency_ids aceita uma lista ou 'all'. Emissões excluídas não entram. Rota restrita a administradores.
// @Tags         Vendas
// @Produce      json
// @Security     BearerAuth
// @Param        date_from           query  string  false  "Data inicial do período (YYYY-MM-DD), inclusiva"
// @Param        date_to             query  string  false  "Data final do período (YYYY-MM-DD), inclusiva"
// @Param        agency_id           query  string  false  "UUID da agência (padrão: a agência principal)"
// @Param        agency_ids          query  string  false  "UUIDs de agências separados por vírgula, ou 'all' para todas. Tem precedência sobre agency_id"
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        account_id          query  string  false  "UUID da conta"
// @Param        exclude_account_ids query  string  false  "UUIDs de contas separados por vírgula a excluir do resultado"
// @Param        administrator_id    query  string  false  "UUID do administrador da conta (people.id)"
// @Param        group_by            query  string  false  "Agrupamento das linhas: account (padrão) ou administrator"
// @Success      200  {object}  schemas.SupplierBalanceListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Usuário não é administrador"
// @Failure      500  {object}  schemas.ErrorResponse "Nenhuma agência principal configurada ou erro ao calcular saldos"
// @Router       /sales/suppliers [get]
func (h *SaleHandler) GetSupplierBalances(w http.ResponseWriter, r *http.Request) {
	rows, from, to, err := h.fetchSupplierRows(r)
	if err != nil {
		utils.SendError(w, r, err.status, err.message)
		return
	}

	balances := aggregateSupplierBalances(h.DB, rows, from, to, resolveSupplierGrouping(r))

	utils.SendJSON(w, r, http.StatusOK, "Supplier balances fetched successfully", balances)
}

// supplierFetchError carries the status the caller should answer with, so the two handlers
// that fetch supplier rows report the same failure the same way.
type supplierFetchError struct {
	status  int
	message string
}

func (e *supplierFetchError) Error() string { return e.message }

// fetchSupplierRows reads every issuance that can contribute to the requested period, with
// the requested filters applied. It is shared by the balances endpoint and the payment
// summary PDF so a document can never cover a different set of issuances than the screen it
// was generated from.
func (h *SaleHandler) fetchSupplierRows(r *http.Request) ([]supplierIssuanceRow, *time.Time, *time.Time, *supplierFetchError) {
	agencyIDs, err := resolveSupplierAgencyFilter(h.DB, r)
	switch {
	case errors.Is(err, access.ErrNoAgencyAccess):
		return nil, nil, nil, &supplierFetchError{http.StatusForbidden, "Você não tem acesso a nenhuma agência"}
	case errors.Is(err, errAgencyOutOfScope):
		return nil, nil, nil, &supplierFetchError{http.StatusNotFound, "Agency not found"}
	case err != nil:
		return nil, nil, nil, &supplierFetchError{http.StatusInternalServerError, "No default agency configured"}
	}

	from, to := parseDateRange(r)

	query := h.DB.Table("issuances").
		Select(`issuances.id AS issuance_id,
			issuances.account_id,
			issuances.points,
			issuances.refunded_points,
			issuances.created_at,
			issuances.cancelled_at,
			issuances.program_locator,
			issuances.airline_locator,
			issuances.origin_iata_code,
			issuances.destination_iata_code,
			issuances.taxes,
			issuances.refunded_taxes,
			issuances.cost_of_the_thousand,
			issuances.taxes_included_in_cost,
		issuances.reviewed,
			issuances.cancellation_only,
			accounts.owns_point_inventory,
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
			accounts.administrator_id,
			administrators.name AS administrator_name,
			administrators.report_export_counter AS administrator_export_counter,
			loyalty_programs.name AS loyalty_program_name,
			loyalty_programs.color AS loyalty_program_color,
			loyalty_programs.code AS loyalty_program_code,
			program_airlines.code AS default_airline_code`).
		Joins("JOIN accounts ON accounts.id = issuances.account_id").
		Joins("JOIN loyalty_programs ON loyalty_programs.id = issuances.loyalty_program_id").
		Joins("LEFT JOIN airlines AS program_airlines ON program_airlines.id = loyalty_programs.default_airline_id").
		Joins("LEFT JOIN people AS holders ON holders.id = accounts.holder_id").
		Joins("LEFT JOIN people AS administrators ON administrators.id = accounts.administrator_id").
		// A deleted issuance is a typo being erased, not a debt with the account's
		// administrator - unlike a cancellation, which really did consume and give back
		// miles. The join is hand-written, so this scope has to be explicit.
		Where("issuances.deleted_at IS NULL")

	// nil means every agency was asked for by a dev; IN () on an empty slice would match
	// nothing, which is the opposite. A restricted "all" is never nil - see
	// resolveSupplierAgencyFilter.
	if agencyIDs != nil {
		query = query.Where("issuances.agency_id IN ?", agencyIDs)
	}

	q := r.URL.Query()
	for column, param := range map[string]string{
		"issuances.loyalty_program_id": "loyalty_program_id",
		"issuances.account_id":         "account_id",
		"accounts.administrator_id":    "administrator_id",
	} {
		if value := q.Get(param); value != "" {
			query = query.Where(column+" = ?", value)
		}
	}

	// Accounts the operator parked on the screen while reconciling. It is the one filter
	// that subtracts rather than narrows, so it is a separate clause: the loop above reads
	// a single value per key, and this one is a list. Empty entries are dropped so a
	// trailing comma cannot turn into a NOT IN ('') that matches nothing.
	if excluded := splitIDs(q.Get("exclude_account_ids")); len(excluded) > 0 {
		query = query.Where("issuances.account_id NOT IN ?", excluded)
	}

	// Narrow to rows that can contribute to the period at all: issued in it, cancelled in
	// it, or altered in it. Without this the aggregation would read the whole table.
	if from != nil || to != nil {
		created := h.DB.Session(&gorm.Session{NewDB: true})
		cancelled := h.DB.Session(&gorm.Session{NewDB: true})
		altered := h.DB.Session(&gorm.Session{NewDB: true}).Table("issuance_adjustments").
			Select("1").
			Where("issuance_adjustments.issuance_id = issuances.id AND issuance_adjustments.deleted_at IS NULL")
		if from != nil {
			created = created.Where("issuances.created_at >= ?", *from)
			cancelled = cancelled.Where("issuances.cancelled_at >= ?", *from)
			altered = altered.Where("issuance_adjustments.occurred_on >= ?", from.Format(time.DateOnly))
		}
		if to != nil {
			created = created.Where("issuances.created_at < ?", *to)
			cancelled = cancelled.Where("issuances.cancelled_at < ?", *to)
			altered = altered.Where("issuance_adjustments.occurred_on < ?", to.Format(time.DateOnly))
		}
		query = query.Where(created.Or(cancelled).Or("EXISTS (?)", altered))
	}

	var rows []supplierIssuanceRow
	if scanErr := query.Scan(&rows).Error; scanErr != nil {
		slog.Error("failed to fetch supplier balance rows", "err", scanErr)
		return nil, nil, nil, &supplierFetchError{http.StatusInternalServerError, "Failed to fetch supplier balances"}
	}

	return rows, from, to, nil
}

// supplierGrouping is what a row of the suppliers view stands for. The screen offers both
// because they answer different questions: an account is who the miles came out of, an
// administrator is who gets paid, and one administrator can run several accounts.
type supplierGrouping string

const (
	groupByAccount       supplierGrouping = "account"
	groupByAdministrator supplierGrouping = "administrator"
)

// resolveSupplierGrouping defaults to account, so an old bookmark, an empty param and a
// panel that predates the choice all keep the behaviour they had.
func resolveSupplierGrouping(r *http.Request) supplierGrouping {
	if r.URL.Query().Get("group_by") == string(groupByAdministrator) {
		return groupByAdministrator
	}
	return groupByAccount
}

// noAdministratorKey buckets the accounts nobody administers. It has to be a key of its own
// and not the empty administrator id, because those accounts still owe miles and dropping
// them would make the totals quietly disagree with the account view.
const noAdministratorKey = "administrator:none"

// supplierGroupOf answers which row of the screen an issuance belongs to, and what that row
// is called.
func supplierGroupOf(row supplierIssuanceRow, grouping supplierGrouping) (key, label string) {
	if grouping == groupByAdministrator {
		if row.AdministratorID == nil || *row.AdministratorID == "" {
			return noAdministratorKey, "Sem administrador"
		}
		if row.AdministratorName != nil && *row.AdministratorName != "" {
			return *row.AdministratorID, *row.AdministratorName
		}
		return *row.AdministratorID, *row.AdministratorID
	}
	return row.AccountID, supplierAccountLabel(row)
}

// aggregateSupplierBalances folds per-issuance rows into one balance per account: miles
// issued inside the period minus miles refunded by cancellations inside the same period.
// An issuance created and cancelled in the same period contributes to both sides, which
// is what makes the two cancel out to zero without any same-day special case - as long as
// it was really issued here. The one exception is a cancellation-only row, which the
// spreadsheet importer invents so an estorno for a booking the panel never had has
// something to hang off; that one contributes its refund and nothing else, so the day
// reads minus the refund the way the tab it came from does.
//
// The same two branches also emit the account's statement, so the lines the panel shows can
// never describe a different set of issuances than the totals beside them.
func aggregateSupplierBalances(
	db *gorm.DB,
	rows []supplierIssuanceRow,
	from, to *time.Time,
	grouping supplierGrouping,
) []schemas.SupplierBalanceResponse {
	type accumulator struct {
		row            supplierIssuanceRow
		key            string
		label          string
		accountCount   map[string]struct{}
		issuedPoints   int
		refundedPoints int
		statement      []schemas.SupplierIssuanceResponse
	}

	// One query for the whole statement rather than one per line. Empty on a set with no
	// inventory account in it, which is the ordinary case.
	inventoryCosts := supplierInventoryCosts(db, rows)
	adjustments, adjustmentCosts := supplierAdjustments(db, rows)

	byGroup := map[string]*accumulator{}
	for _, row := range rows {
		key, label := supplierGroupOf(row, grouping)
		acc, ok := byGroup[key]
		if !ok {
			acc = &accumulator{row: row, key: key, label: label, accountCount: map[string]struct{}{}}
			byGroup[key] = acc
		}
		acc.accountCount[row.AccountID] = struct{}{}

		// A cancellation-only row never issued anything: it exists so an estorno for a
		// booking the panel never had has something to hang off. Counting its points here
		// would pair a phantom emission with its own refund and net the day to zero, when
		// the spreadsheet tab being reconciled reads minus the refund.
		if !row.CancellationOnly && inRange(&row.CreatedAt, from, to) {
			acc.issuedPoints += row.Points
			acc.statement = append(acc.statement, supplierStatementLine(
				row, "issued", row.CreatedAt, row.Points, row.Taxes, chargedCents(inventoryCosts, row),
			))
		}
		if inRange(row.CancelledAt, from, to) && row.RefundedPoints != nil {
			acc.refundedPoints += *row.RefundedPoints
			acc.statement = append(acc.statement, supplierStatementLine(
				row, "refund", *row.CancelledAt, -*row.RefundedPoints, negateCents(row.RefundedTaxes),
				refundedCents(inventoryCosts, row),
			))
		}

		// An alteration is more miles out of the account, or miles back into it, on its
		// own day. It reads as one more issued or refund line rather than a kind of its
		// own, so a panel tab still on the previous bundle - whose schema knows only those
		// two - keeps parsing the statement. adjustment_id is what tells the two apart.
		for _, adjustment := range adjustments[row.IssuanceID] {
			// occurred_on is a calendar day, stored as UTC midnight - which is the evening
			// before in São Paulo, so it would fall out of its own month at the boundary
			// and show the previous day on the line. Noon UTC is the same day everywhere
			// the panel runs.
			day := adjustment.OccurredOn
			occurredOn := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.UTC)
			if !inRange(&occurredOn, from, to) {
				continue
			}
			kind := "issued"
			if adjustment.Points < 0 {
				kind = "refund"
				acc.refundedPoints -= adjustment.Points
			} else {
				acc.issuedPoints += adjustment.Points
			}
			taxes := adjustment.Taxes
			line := supplierStatementLine(
				row, kind, occurredOn, adjustment.Points, &taxes,
				adjustmentCostCents(adjustmentCosts, row, adjustment.ID),
			)
			adjustmentID, reason := adjustment.ID, adjustment.Reason
			line.AdjustmentID = &adjustmentID
			line.AdjustmentReason = &reason
			acc.statement = append(acc.statement, line)
		}
	}

	balances := make([]schemas.SupplierBalanceResponse, 0, len(byGroup))
	for _, acc := range byGroup {
		// Chronological, so a statement reads like one: what was issued, then what came
		// back. Ranging over a map randomises the input order, so this is also what makes
		// the response stable across identical requests.
		sort.SliceStable(acc.statement, func(i, j int) bool {
			return acc.statement[i].Date.Before(acc.statement[j].Date)
		})

		balance := schemas.SupplierBalanceResponse{
			GroupKey:            acc.key,
			GroupLabel:          acc.label,
			AccountCount:        len(acc.accountCount),
			AccountID:           acc.row.AccountID,
			AccountLabel:        supplierAccountLabel(acc.row),
			LoyaltyProgramName:  acc.row.LoyaltyProgramName,
			LoyaltyProgramColor: acc.row.LoyaltyProgramColor,
			AdministratorID:     acc.row.AdministratorID,
			AdministratorName:   acc.row.AdministratorName,
			IssuedPoints:        acc.issuedPoints,
			RefundedPoints:      acc.refundedPoints,
			NetPoints:           acc.issuedPoints - acc.refundedPoints,
			Issuances:           acc.statement,
		}
		// The export counter belongs to the administrator, so it is only meaningful on an
		// administrator-grouped row that actually has one (never the "Sem administrador" bucket).
		if grouping == groupByAdministrator && acc.row.AdministratorID != nil {
			balance.ExportCounter = acc.row.AdministratorExportCounter
		}
		balances = append(balances, balance)
	}

	// Biggest debt first; the label breaks ties so the order is stable across requests
	// (ranging over a map is deliberately randomised in Go).
	sort.Slice(balances, func(i, j int) bool {
		if balances[i].NetPoints != balances[j].NetPoints {
			return balances[i].NetPoints > balances[j].NetPoints
		}
		return balances[i].GroupLabel < balances[j].GroupLabel
	})

	return balances
}

// UpdateSupplierIssuances handles the POST /sales/suppliers/issuances route.
// @Summary      Definir os termos de pagamento ao fornecedor
// @Description  Grava o milheiro de custo (em centavos) e/ou se as taxas entram no valor, nas emissões informadas. Recebe a lista explícita de ids, nunca um filtro: um id é a edição de uma linha, vários são a aplicação em massa. Campo omitido é preservado; cost_of_the_thousand explicitamente nulo limpa o valor. Rota restrita a administradores.
// @Tags         Vendas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  schemas.UpdateSupplierIssuancesRequest  true  "Emissões e os termos de pagamento"
// @Success      200  {object}  schemas.UpdateSupplierIssuancesSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Usuário não é administrador"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao gravar os termos de pagamento"
// @Router       /sales/suppliers/issuances [post]
func (h *SaleHandler) UpdateSupplierIssuances(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Decoded twice, the same way UpdateSale is and for the same reason: the struct
	// carries the values, the raw map tells an omitted cost_of_the_thousand (leave the
	// rate alone) from an explicit null (clear it back to "not agreed"). A single *int
	// cannot express that difference, and here it is the difference between the tax
	// buttons preserving the rates and wiping every one of them.
	var present map[string]json.RawMessage
	if err := json.Unmarshal(body, &present); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	var req schemas.UpdateSupplierIssuancesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// The validator carries the real guard: required + min=1 is what makes an UPDATE
	// without a WHERE unreachable, and max=1000 keeps a runaway request from rewriting the
	// table in one statement.
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// The ids arrive loose in the body, so no route middleware saw them. One issuance outside
	// the caller's agencies refuses the whole batch - writing the rest would make the answer
	// depend on which rows happened to be in scope.
	if scope := middleware.AgencyScopeFrom(r.Context()); !scope.All {
		var inScope int64
		query := scope.Restrict(h.DB.Model(&data.Issuance{}), "issuances.agency_id")
		if err := query.Where("issuances.id IN ?", req.IssuanceIDs).Count(&inScope).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the issuances: "+err.Error())
			return
		}
		if inScope != int64(len(slices.Compact(slices.Sorted(slices.Values(req.IssuanceIDs))))) {
			utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
			return
		}
	}

	updates := map[string]any{}
	if _, ok := present["cost_of_the_thousand"]; ok {
		// Once the batches have paid for a booking its rate is derived from them, and
		// accepting a typed one here would leave two answers to the same question. A
		// booking the FIFO has not covered yet has no derived rate, so it is typed like any
		// other. The screen only offers the field on those; this is the guard that does not
		// depend on the screen.
		owned, err := h.issuancesCostedByInventory(req.IssuanceIDs)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the accounts: "+err.Error())
			return
		}
		if owned > 0 {
			utils.SendError(w, r, http.StatusUnprocessableEntity,
				"O custo destas emissões vem do estoque de pontos da conta e não pode ser digitado")
			return
		}
		updates["cost_of_the_thousand"] = req.CostOfTheThousand
	}
	if req.TaxesIncludedInCost != nil {
		updates["taxes_included_in_cost"] = *req.TaxesIncludedInCost
	}
	if req.Reviewed != nil {
		updates["reviewed"] = *req.Reviewed
	}
	if len(updates) == 0 {
		utils.SendError(w, r, http.StatusBadRequest, "No fields to update")
		return
	}

	result := h.DB.Model(&data.Issuance{}).
		Where("id IN ?", req.IssuanceIDs).
		Updates(updates)
	if result.Error != nil {
		slog.Error("failed to update the supplier payment terms",
			"issuance_count", len(req.IssuanceIDs), "err", result.Error)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update the payment terms")
		return
	}

	// One event for the batch rather than one per issuance: the panels that care refetch
	// with their own filters anyway, and a bulk apply would otherwise put hundreds of
	// messages on every open stream to say the same thing.
	h.Broker.Broadcast(events.Event{
		Type:    eventIssuanceUpdated,
		Payload: map[string]string{"id": req.IssuanceIDs[0]},
	})

	utils.SendJSON(w, r, http.StatusOK, "Payment terms updated successfully",
		schemas.UpdateSupplierIssuancesResponse{Updated: int(result.RowsAffected)})
}

// IncrementSupplierExportCounter handles the POST /sales/suppliers/export-counter route.
// @Summary      Incrementar o contador de exportação do administrador
// @Description  Soma 1 ao contador de exportação do PDF de pagamento do administrador (people.report_export_counter) e devolve o novo valor, que o painel usa no nome do arquivo. Chamado a cada exportação. Rota restrita a administradores.
// @Tags         Vendas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  schemas.IncrementSupplierExportCounterRequest  true  "Administrador"
// @Success      200  {object}  schemas.SupplierExportCounterSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Usuário não é administrador"
// @Failure      404  {object}  schemas.ErrorResponse "Administrador não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao incrementar o contador"
// @Router       /sales/suppliers/export-counter [post]
func (h *SaleHandler) IncrementSupplierExportCounter(w http.ResponseWriter, r *http.Request) {
	var req schemas.IncrementSupplierExportCounterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// One atomic statement with RETURNING: the increment and the value read back are the same
	// row-level operation, so two concurrent exports never hand back the same number.
	person := data.Person{ID: req.AdministratorID}
	result := h.DB.Model(&person).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "report_export_counter"}}}).
		Where("id = ?", req.AdministratorID).
		UpdateColumn("report_export_counter", gorm.Expr("report_export_counter + 1"))
	if result.Error != nil {
		slog.Error("failed to increment supplier export counter", "administrator_id", req.AdministratorID, "err", result.Error)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to increment the export counter")
		return
	}
	if result.RowsAffected == 0 {
		utils.SendError(w, r, http.StatusNotFound, "Administrator not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Export counter incremented successfully",
		schemas.SupplierExportCounterResponse{Counter: person.ReportExportCounter})
}

// SetSupplierExportCounter handles the PUT /sales/suppliers/export-counter route.
// @Summary      Definir o contador de exportação do administrador
// @Description  Grava um valor exato no contador de exportação do administrador (people.report_export_counter), para corrigi-lo ou reiniciar a sequência à mão. Rota restrita a administradores.
// @Tags         Vendas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  schemas.SetSupplierExportCounterRequest  true  "Administrador e valor"
// @Success      200  {object}  schemas.SupplierExportCounterSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Usuário não é administrador"
// @Failure      404  {object}  schemas.ErrorResponse "Administrador não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao gravar o contador"
// @Router       /sales/suppliers/export-counter [put]
func (h *SaleHandler) SetSupplierExportCounter(w http.ResponseWriter, r *http.Request) {
	var req schemas.SetSupplierExportCounterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	result := h.DB.Model(&data.Person{}).
		Where("id = ?", req.AdministratorID).
		UpdateColumn("report_export_counter", req.Counter)
	if result.Error != nil {
		slog.Error("failed to set supplier export counter", "administrator_id", req.AdministratorID, "err", result.Error)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to set the export counter")
		return
	}
	if result.RowsAffected == 0 {
		utils.SendError(w, r, http.StatusNotFound, "Administrator not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Export counter updated successfully",
		schemas.SupplierExportCounterResponse{Counter: req.Counter})
}

// BulkIncrementSupplierExportCounter handles the POST /sales/suppliers/export-counter/bulk route.
// @Summary      Incrementar o contador de vários administradores
// @Description  Soma 1 ao contador de exportação de cada administrador informado, numa única operação. Usado pelo botão "Gerar PDF de todas", que cobre vários administradores de uma vez. Rota restrita a administradores.
// @Tags         Vendas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  schemas.BulkIncrementSupplierExportCounterRequest  true  "Administradores"
// @Success      200  {object}  schemas.BulkSupplierExportCounterSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Usuário não é administrador"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao incrementar os contadores"
// @Router       /sales/suppliers/export-counter/bulk [post]
func (h *SaleHandler) BulkIncrementSupplierExportCounter(w http.ResponseWriter, r *http.Request) {
	var req schemas.BulkIncrementSupplierExportCounterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// One statement for the whole set, so the "all suppliers" export never bumps half the
	// administrators and then fails.
	result := h.DB.Model(&data.Person{}).
		Where("id IN ?", req.AdministratorIDs).
		UpdateColumn("report_export_counter", gorm.Expr("report_export_counter + 1"))
	if result.Error != nil {
		slog.Error("failed to bulk-increment supplier export counters",
			"administrator_count", len(req.AdministratorIDs), "err", result.Error)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to increment the export counters")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Export counters incremented successfully",
		schemas.BulkSupplierExportCounterResponse{Updated: int(result.RowsAffected)})
}

// supplierStatementLine builds one line of an account's statement. Points and taxes are
// passed in already signed rather than derived here, because only the caller knows which of
// the two branches it is in.
//
// costCents is the exact figure the points inventory charged this line, and is nil on an
// ordinary account, where the cost is the rate somebody typed. When present it wins: it is
// the sum of the batch slices that actually paid for the booking, whereas the rate beside
// it is that same figure rounded to a whole centavo per thousand for display.
func supplierStatementLine(row supplierIssuanceRow, kind string, date time.Time, points int, taxes *int, costCents *int) schemas.SupplierIssuanceResponse {
	rate := row.CostOfTheThousand
	if costCents != nil {
		derived := inventory.RateOfTheThousand(*costCents, points)
		rate = &derived
	}

	return schemas.SupplierIssuanceResponse{
		IssuanceID:          row.IssuanceID,
		AccountID:           row.AccountID,
		AccountLabel:        supplierAccountLabel(row),
		Kind:                kind,
		Date:                date,
		ProgramLocator:      row.ProgramLocator,
		AirlineLocator:      row.AirlineLocator,
		OriginIATACode:      row.OriginIATACode,
		DestinationIATACode: row.DestinationIATACode,
		Points:              points,
		Taxes:               taxes,
		CostOfTheThousand:   rate,
		CostCents:           costCents,
		CostFromInventory:   costCents != nil,
		OwnsPointInventory:  row.OwnsPointInventory,
		TaxesIncludedInCost: row.TaxesIncludedInCost,
		Reviewed:            row.Reviewed,
	}
}

// negateCents flips a nullable amount, preserving nil. A cancellation with no tax refund
// recorded is not a refund of zero: the line simply has no tax side.
func negateCents(cents *int) *int {
	if cents == nil {
		return nil
	}
	negated := -*cents
	return &negated
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

// supplierInventoryCosts reads the FIFO cost of every inventory-account issuance in the
// statement, in one query.
//
// Each inventory account in the set is recomputed first. The allocations are derived data
// that only the closing screen used to refresh, so a booking made since then had none and
// showed no cost here at all - and with the field locked, there was nothing to do about it.
//
// A failure is logged and swallowed rather than propagated: without these figures the
// statement falls back to the typed rate, which is what it did before the inventory
// existed. Failing the whole request instead would take down a screen that ordinary
// accounts - the overwhelming majority - do not need this for at all.
func supplierInventoryCosts(db *gorm.DB, rows []supplierIssuanceRow) map[string]inventory.IssuanceCost {
	issuanceIDs := make([]string, 0)
	accountIDs := make([]string, 0)
	seenAccount := map[string]struct{}{}
	for _, row := range rows {
		if !row.OwnsPointInventory {
			continue
		}
		issuanceIDs = append(issuanceIDs, row.IssuanceID)
		if _, seen := seenAccount[row.AccountID]; !seen {
			seenAccount[row.AccountID] = struct{}{}
			accountIDs = append(accountIDs, row.AccountID)
		}
	}
	if len(issuanceIDs) == 0 {
		return nil
	}

	for _, accountID := range accountIDs {
		if err := inventory.Recompute(db, accountID); err != nil {
			slog.Error("failed to recompute the points inventory for the supplier statement",
				"account_id", accountID, "err", err)
		}
	}

	costs, err := inventory.CostByIssuance(db, issuanceIDs)
	if err != nil {
		slog.Error("failed to read the points inventory cost for the supplier statement",
			"issuance_count", len(issuanceIDs), "err", err)
		return nil
	}
	return costs
}

// chargedCents is what the inventory charged an issuance, or nil when this row is not
// costed by it.
//
// Nil unless every point of the booking was charged to a batch. A booking still pending -
// its batch not entered yet - falls back to the typed rate, which the screen then lets the
// operator fill in, instead of claiming a cost of zero. A batch that genuinely cost nothing
// is covered, and shows its zero.
func chargedCents(costs map[string]inventory.IssuanceCost, row supplierIssuanceRow) *int {
	if !row.OwnsPointInventory {
		return nil
	}
	cost, ok := costs[row.IssuanceID]
	if !ok || !cost.BookingCovered {
		return nil
	}
	return &cost.ChargedCents
}

// refundedCents is what the inventory gave back on a cancellation, as a negative amount to
// match the negative points on the refund line. Nil under the same rule as chargedCents.
// supplierAdjustments reads the alterations of every issuance on the statement, keyed by
// issuance, and on an inventory account what each of them cost. The period is applied by
// the caller, as it is for the issued and refund sides.
func supplierAdjustments(db *gorm.DB, rows []supplierIssuanceRow) (map[string][]data.IssuanceAdjustment, map[string]inventory.AdjustmentCost) {
	issuanceIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		issuanceIDs = append(issuanceIDs, row.IssuanceID)
	}
	if len(issuanceIDs) == 0 {
		return nil, nil
	}

	var adjustments []data.IssuanceAdjustment
	if err := db.Where("issuance_id IN ?", issuanceIDs).
		Order("occurred_on ASC").Order("created_at ASC").
		Find(&adjustments).Error; err != nil {
		slog.Error("failed to read the alterations for the supplier statement",
			"issuance_count", len(issuanceIDs), "err", err)
		return nil, nil
	}

	byIssuance := make(map[string][]data.IssuanceAdjustment, len(adjustments))
	adjustmentIDs := make([]string, 0, len(adjustments))
	for _, adjustment := range adjustments {
		byIssuance[adjustment.IssuanceID] = append(byIssuance[adjustment.IssuanceID], adjustment)
		adjustmentIDs = append(adjustmentIDs, adjustment.ID)
	}

	// The recompute already ran in supplierInventoryCosts, so the slices are current.
	costs, err := inventory.CostByAdjustment(db, adjustmentIDs)
	if err != nil {
		slog.Error("failed to read the points inventory cost of the alterations",
			"adjustment_count", len(adjustmentIDs), "err", err)
		return byIssuance, nil
	}
	return byIssuance, costs
}

// adjustmentCostCents is chargedCents for one alteration's line.
func adjustmentCostCents(costs map[string]inventory.AdjustmentCost, row supplierIssuanceRow, adjustmentID string) *int {
	if !row.OwnsPointInventory {
		return nil
	}
	cost, ok := costs[adjustmentID]
	if !ok || !cost.Covered {
		return nil
	}
	return &cost.CostCents
}

func refundedCents(costs map[string]inventory.IssuanceCost, row supplierIssuanceRow) *int {
	if !row.OwnsPointInventory {
		return nil
	}
	cost, ok := costs[row.IssuanceID]
	if !ok || !cost.RefundCovered {
		return nil
	}
	return &cost.RefundedCents
}

// issuancesCostedByInventory counts how many of the given issuances already have their cost
// from the points inventory - fully charged to batches, on the booking or on the refund.
// Those are the only ones a typed rate may not touch; an inventory booking the FIFO has not
// covered is priced by hand like any other until its batch is entered.
func (h *SaleHandler) issuancesCostedByInventory(issuanceIDs []string) (int, error) {
	var owned []string
	err := h.DB.Model(&data.Issuance{}).
		Joins("JOIN accounts ON accounts.id = issuances.account_id").
		Where("issuances.id IN ? AND accounts.owns_point_inventory", issuanceIDs).
		Pluck("issuances.id", &owned).Error
	if err != nil || len(owned) == 0 {
		return 0, err
	}

	costs, err := inventory.CostByIssuance(h.DB, owned)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, cost := range costs {
		if cost.BookingCovered || cost.RefundCovered {
			count++
		}
	}
	return count, nil
}
