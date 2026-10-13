package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/export"
	"github.com/murandido/m-80-backend/internal/sale"
	"github.com/murandido/m-80-backend/internal/traveos"

	// Aliased because "issuance" is the local variable name for a data.Issuance
	// throughout this file - importing it unaliased would shadow the package in
	// every handler that holds one.
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// The "created", "cancelled" and "deleted" actions live in internal/issuance
// instead - they are written by the Create/Cancel/Delete rules the bot shares
// with this handler. auditActionCancelled survives here only because
// UncancelIssuance still queries it back out to decide what to restore.
const (
	auditActionCancelled = "cancelled"
	auditActionRestored  = "restored"
)

// SSE event types published by this handler. The payload is always just the
// affected issuance id: every connected panel refetches the list with its own
// filters anyway, and the filter rules (the creation/cancellation OR branch,
// the São Paulo timezone) only exist here on the server.
const (
	eventIssuanceCreated   = "issuance.created"
	eventIssuanceUpdated   = "issuance.updated"
	eventIssuanceCancelled = "issuance.cancelled"
	eventIssuanceDeleted   = "issuance.deleted"
)

type IssuanceHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
	Broker    *events.Broker
	// Bookings is where the itinerary comes from: which provider serves which
	// airline, and what catches the ones that could not be reached. It is nil or
	// empty when the process started without vendor credentials and no direct
	// integration is wired - a supported state, not a fatal one, in which the
	// routes that need an itinerary answer 503 and everything else works untouched.
	Bookings *booking.Registry
}

func NewIssuanceHandler(db *gorm.DB, broker *events.Broker, bookings *booking.Registry) *IssuanceHandler {
	return &IssuanceHandler{
		DB:        db,
		Validator: validator.New(),
		Broker:    broker,
		Bookings:  bookings,
	}
}

// broadcastIssuance announces a persisted mutation on the SSE stream. It is only
// ever called after the transaction committed, so no client can be told about a
// change that later rolls back.
func (h *IssuanceHandler) broadcastIssuance(eventType, issuanceID string) {
	h.Broker.Broadcast(events.Event{
		Type:    eventType,
		Payload: map[string]string{"id": issuanceID},
	})
}

func toIssuanceFlightResponse(flight data.IssuanceFlight) schemas.IssuanceFlightResponse {
	return schemas.IssuanceFlightResponse{
		ID:                  flight.ID,
		Direction:           flight.Direction,
		LegOrder:            flight.LegOrder,
		FlightNumber:        flight.FlightNumber,
		OriginIATACode:      flight.OriginIATACode,
		DestinationIATACode: flight.DestinationIATACode,
		DepartureAt:         flight.DepartureAt,
		ArrivalAt:           flight.ArrivalAt,
		DurationMin:         flight.DurationMin,
		Cabin:               flight.Cabin,
		CreatedAt:           flight.CreatedAt,
	}
}

func toIssuanceResponse(issuance data.Issuance) schemas.IssuanceResponse {
	resp := schemas.IssuanceResponse{
		ID:                  issuance.ID,
		LoyaltyProgramID:    issuance.LoyaltyProgramID,
		AirlineID:           issuance.AirlineID,
		ProgramLocator:      issuance.ProgramLocator,
		AirlineLocator:      issuance.AirlineLocator,
		LatamPurchaseNumber: issuance.LatamPurchaseNumber,
		MainPassengerName:   issuance.MainPassengerName,
		PassengerLastName:   issuance.PassengerLastName,
		TripType:            issuance.TripType,
		OriginIATACode:      issuance.OriginIATACode,
		DestinationIATACode: issuance.DestinationIATACode,
		Points:              issuance.Points,
		ValueOfTheThousand:  issuance.ValueOfTheThousand,
		Taxes:               issuance.Taxes,

		RedemptionFeeSavedCents: issuance.RedemptionFeeSavedCents,
		RedemptionFeeManual:     issuance.RedemptionFeeManual,

		AccountID:          issuance.AccountID,
		IssuerID:           issuance.IssuerID,
		AgencyID:           issuance.AgencyID,
		Complete:           issuance.Complete,
		TraveosSyncedAt:    issuance.TraveosSyncedAt,
		TraveosSyncingAt:   freshTraveosSyncing(issuance.TraveosSyncingAt),
		TraveosAttemptedAt: issuance.TraveosAttemptedAt,
		Imported:           issuance.Imported,
		CancellationOnly:   issuance.CancellationOnly,
		Status:             deriveIssuanceStatus(issuance.Complete, issuance.CancelledAt),
		AdditionalServices: issuance.AdditionalServices,
		Observation:        issuance.Observation,
		Active:             issuance.Active,
		CancelledAt:        issuance.CancelledAt,
		CancelledByID:      issuance.CancelledByID,
		RefundedPoints:     issuance.RefundedPoints,
		RefundedTaxes:      issuance.RefundedTaxes,
		CancellationFee:    issuance.CancellationFee,
		CreatedAt:          issuance.CreatedAt,
		IssuedOn:           localDayOf(issuance.CreatedAt),
		UpdatedAt:          issuance.UpdatedAt,
	}

	if issuance.LoyaltyProgram != nil {
		lp := toLoyaltyProgramResponse(*issuance.LoyaltyProgram)
		resp.LoyaltyProgram = &lp
	}
	if issuance.Airline != nil {
		airline := toAirlineResponse(*issuance.Airline)
		resp.Airline = &airline
	}
	if issuance.OriginAirport != nil {
		airport := toAirportResponse(*issuance.OriginAirport)
		resp.OriginAirport = &airport
	}
	if issuance.DestinationAirport != nil {
		airport := toAirportResponse(*issuance.DestinationAirport)
		resp.DestinationAirport = &airport
	}
	if issuance.Account != nil {
		account := toAccountResponse(*issuance.Account)
		resp.Account = &account
	}
	resp.AccountNicknameID = issuance.AccountNicknameID
	if issuance.AccountNickname != nil {
		nickname := toAccountNicknameResponse(*issuance.AccountNickname)
		resp.AccountNickname = &nickname
	}
	if issuance.Issuer != nil {
		// The issuer's grants are not loaded here - nothing on an issuance reads them - but
		// the field still has to be a list: a nil slice went out as null and the panel's
		// schema rejected the whole issuance list over it.
		issuer := toUserResponse(*issuance.Issuer, nil)
		resp.Issuer = &issuer
	}
	if issuance.Agency != nil {
		agency := toAgencyResponse(*issuance.Agency)
		resp.Agency = &agency
	}
	if len(issuance.Flights) > 0 {
		flights := make([]schemas.IssuanceFlightResponse, 0, len(issuance.Flights))
		for _, flight := range issuance.Flights {
			flights = append(flights, toIssuanceFlightResponse(flight))
		}
		resp.Flights = flights
	}
	// Always a list, never null, so the panel can map over it without a guard.
	resp.Adjustments = make([]schemas.IssuanceAdjustmentResponse, 0, len(issuance.Adjustments))
	for _, adjustment := range issuance.Adjustments {
		resp.Adjustments = append(resp.Adjustments, toIssuanceAdjustmentResponse(adjustment))
	}
	if len(issuance.Passengers) > 0 {
		passengers := make([]schemas.IssuancePassengerResponse, 0, len(issuance.Passengers))
		for _, passenger := range issuance.Passengers {
			passengers = append(passengers, toIssuancePassengerResponse(passenger))
		}
		resp.Passengers = passengers
	}

	return resp
}

// freshTraveosSyncing hides a marker older than the sync's own timeout. Past that
// point the pull cannot still be running, so what is left is a marker from a
// process that died mid-pull - and reporting it would lock the row on every screen
// with no way out. Filtering here rather than in the client keeps the clients free
// of time arithmetic and immune to a skewed browser clock, and it means no cleanup
// job has to exist.
func freshTraveosSyncing(startedAt *time.Time) *time.Time {
	if startedAt == nil || time.Since(*startedAt) > traveosSyncTimeout {
		return nil
	}
	return startedAt
}

// deriveIssuanceStatus derives the client-facing status from the underlying
// complete/cancelled_at columns. Deleted issuances never reach here (soft-deleted
// rows are excluded by GORM's default scope in every query this is called from).
func deriveIssuanceStatus(complete bool, cancelledAt *time.Time) string {
	if cancelledAt != nil {
		return "cancelled"
	}
	if complete {
		return "complete"
	}
	return "incomplete"
}

func toIssuanceSummaryResponse(issuance data.Issuance) schemas.IssuanceSummaryResponse {
	status := deriveIssuanceStatus(issuance.Complete, issuance.CancelledAt)

	resp := schemas.IssuanceSummaryResponse{
		ID:                  issuance.ID,
		ProgramLocator:      issuance.ProgramLocator,
		AirlineLocator:      issuance.AirlineLocator,
		IssuedAt:            issuance.CreatedAt,
		MainPassengerName:   issuance.MainPassengerName,
		OriginIATACode:      issuance.OriginIATACode,
		DestinationIATACode: issuance.DestinationIATACode,
		Points:              issuance.Points,
		ValueOfTheThousand:  issuance.ValueOfTheThousand,
		Taxes:               issuance.Taxes,
		TraveosSyncedAt:     issuance.TraveosSyncedAt,
		TraveosSyncingAt:    freshTraveosSyncing(issuance.TraveosSyncingAt),
		TraveosAttemptedAt:  issuance.TraveosAttemptedAt,
		Imported:            issuance.Imported,
		CancellationOnly:    issuance.CancellationOnly,
		Status:              status,
	}

	if issuance.LoyaltyProgram != nil {
		resp.LoyaltyProgramName = issuance.LoyaltyProgram.Name
	}
	if issuance.Airline != nil {
		resp.AirlineName = issuance.Airline.Name
	}
	if issuance.Account != nil {
		resp.AccountLogin = issuance.Account.Login
	}
	if issuance.Issuer != nil {
		resp.IssuerName = issuance.Issuer.Name
	}
	if issuance.Agency != nil {
		resp.AgencyName = issuance.Agency.Name
	}
	if len(issuance.Flights) > 0 {
		resp.FlightDate = &issuance.Flights[0].DepartureAt
	}

	return resp
}

// issuanceErrorResponse maps the issuance service's typed errors onto the exact
// status and message this API has always returned for them, so extracting the
// service out of these handlers changed no response the panel can observe. The
// capitalized messages are spelled out here rather than taken from err.Error()
// because Go error strings are conventionally uncapitalized. Returns (0, "") for
// an error the service doesn't classify - the caller then falls back to its own
// 500.
func issuanceErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, issuancesvc.ErrLoyaltyProgramNotFound):
		return http.StatusBadRequest, "Loyalty program not found"
	case errors.Is(err, issuancesvc.ErrIssuerNotFound):
		return http.StatusBadRequest, "Issuer not found"
	case errors.Is(err, issuancesvc.ErrNoDefaultAgency):
		return http.StatusInternalServerError, "No default agency configured"
	case errors.Is(err, issuancesvc.ErrAccountNicknameNotFound),
		errors.Is(err, issuancesvc.ErrAccountNicknameMismatch),
		errors.Is(err, issuancesvc.ErrAirportNotFound):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, issuancesvc.ErrLastNameRequired),
		errors.Is(err, issuancesvc.ErrPurchaseNumberRequired):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, issuancesvc.ErrProgramLocatorTaken):
		return http.StatusUnprocessableEntity, "Program locator já está em uso por uma emissão ativa"
	case errors.As(err, new(*account.ArchivedError)):
		return http.StatusUnprocessableEntity, err.Error()
	default:
		return 0, ""
	}
}

// preloadIssuanceAssociations applies the standard set of preloads used by the
// list and single-issuance responses.
//
// The account, its aliases and the loyalty program are loaded Unscoped on purpose.
// All three are soft-deleted (migration 29), and the GORM scope applies to preloads
// too - so without this, archiving an account would blank the CONTA column on every
// issuance it ever made. These associations exist to say what a past issuance was,
// not to offer a choice: the caller decides whether an archived one may be picked,
// which is why deleted_at is on the wire.
func preloadIssuanceAssociations(query *gorm.DB) *gorm.DB {
	unscoped := func(db *gorm.DB) *gorm.DB { return db.Unscoped() }

	return query.
		Preload("LoyaltyProgram", unscoped).
		Preload("Airline").
		Preload("Account", unscoped).
		Preload("Account.Nicknames", unscoped).
		Preload("AccountNickname", unscoped).
		Preload("Issuer").
		Preload("Agency").
		Preload("Adjustments", func(db *gorm.DB) *gorm.DB {
			return db.Order("occurred_on ASC").Order("created_at ASC")
		}).
		Preload("Adjustments.CreatedBy")
}

// applyIssuanceListFilters narrows an issuance list query by the optional query
// params (issuer_id, account_id, loyalty_program_id, airline_id, agency_id,
// date_from, date_to). Every param is optional: an absent or empty one leaves
// that dimension unfiltered.
//
// account_id/loyalty_program_id/airline_id/agency_id are plain ANDed filters that
// apply to every row. issuer_id + date_from/date_to are instead coupled into an OR of two
// branches so a cancelled issuance surfaces in the flow of whoever cancelled it,
// on the day they cancelled it - not only in the creator's creation-day flow:
//
//	(issuer_id = ? AND created_at in [from,to))            -- creation branch
//	OR
//	(active = false AND cancelled_by_id = ?                -- cancellation branch
//	     AND cancelled_at in [from,to))
//	OR
//	EXISTS (alteration created_by_id = ?                  -- alteration branch
//	        AND occurred_on in [from,to))
//
// The alteration branch follows the cancellation's reasoning: a ticket changed today
// belongs to today's flow, in the flow of whoever recorded the change, however old the
// booking. It is a subquery rather than a join so an issuance altered twice in a day is
// still one row.
//
// Each present dimension feeds both branches (issuer -> issuer_id/cancelled_by_id,
// dates -> created_at/cancelled_at). When neither issuer nor dates are given the
// OR group is skipped entirely and the query stays unfiltered on those dimensions.
//
// q is the free-text search and it deliberately suspends the issuer/date group:
// somebody typing a locator is looking for one issuance in the whole history, and
// the screen opens filtered on "today + me". Searching inside that window would
// answer "does not exist" for every issuance older than this morning. The panel
// greys the two controls out while a search is active so the suspension is visible
// rather than surprising; the other dimensions keep applying as plain ANDs.
// The two non-uuid values the issuer_id filter accepts, mirroring the panel's multi-select:
// "all" restricts to nothing (every issuer), "none" matches nothing (no issuer selected).
const (
	issuerFilterAll  = "all"
	issuerFilterNone = "none"
)

func applyIssuanceListFilters(db *gorm.DB, query *gorm.DB, r *http.Request) *gorm.DB {
	q := r.URL.Query()

	// First, and unconditionally: every early return below - the search, "none" - must
	// already be inside the caller's agencies.
	query = middleware.AgencyScopeFrom(r.Context()).Restrict(query, "issuances.agency_id")

	for column, param := range map[string]string{
		"account_id":         "account_id",
		"loyalty_program_id": "loyalty_program_id",
		"airline_id":         "airline_id",
		"agency_id":          "agency_id",
	} {
		if value := q.Get(param); value != "" {
			query = query.Where(column+" = ?", value)
		}
	}

	if search := strings.TrimSpace(q.Get("q")); search != "" {
		query = query.Where(issuanceSearchGroup(db, search))
		// The issuer/date group is skipped on purpose while searching - see the
		// function comment.
		return query
	}

	// issuer_id carries the multi-select's choice: a comma-separated list of ids, or one of two
	// tokens. "none" is the explicit "no issuers selected" state - the panel sends it so that
	// clearing every issuer shows nothing rather than everything, so short-circuit to an empty
	// result. "all" (like an absent param) means no issuer restriction. A single id has no comma,
	// so a panel bundle that still sends one keeps working.
	rawIssuer := q.Get("issuer_id")
	if rawIssuer == issuerFilterNone {
		return query.Where("1 = 0")
	}
	var issuers []string
	if rawIssuer != "" && rawIssuer != issuerFilterAll {
		issuers = splitIDs(rawIssuer)
	}

	// created_at/cancelled_at are UTC in the DB; parseDateRange resolves the local
	// calendar days into UTC bounds so "today" matches the operator's day.
	from, to := parseDateRange(r)

	if len(issuers) == 0 && from == nil && to == nil {
		return query
	}

	creation := []string{}
	cancellation := []string{"active = ?"}
	creationArgs := []any{}
	cancellationArgs := []any{false}

	if len(issuers) > 0 {
		creation = append(creation, "issuer_id IN ?")
		creationArgs = append(creationArgs, issuers)
		cancellation = append(cancellation, "cancelled_by_id IN ?")
		cancellationArgs = append(cancellationArgs, issuers)
	}
	if from != nil {
		creation = append(creation, "created_at >= ?")
		creationArgs = append(creationArgs, *from)
		cancellation = append(cancellation, "cancelled_at >= ?")
		cancellationArgs = append(cancellationArgs, *from)
	}
	if to != nil {
		creation = append(creation, "created_at < ?")
		creationArgs = append(creationArgs, *to)
		cancellation = append(cancellation, "cancelled_at < ?")
		cancellationArgs = append(cancellationArgs, *to)
	}

	alteration := []string{"issuance_adjustments.issuance_id = issuances.id", "issuance_adjustments.deleted_at IS NULL"}
	alterationArgs := []any{}
	if len(issuers) > 0 {
		alteration = append(alteration, "issuance_adjustments.created_by_id IN ?")
		alterationArgs = append(alterationArgs, issuers)
	}
	alteration, alterationArgs = appendAdjustmentDayBounds(alteration, alterationArgs, from, to)

	condition := "(" + strings.Join(creation, " AND ") + ") OR (" + strings.Join(cancellation, " AND ") + ")" +
		" OR EXISTS (SELECT 1 FROM issuance_adjustments WHERE " + strings.Join(alteration, " AND ") + ")"
	args := append(append(creationArgs, cancellationArgs...), alterationArgs...)
	return query.Where(condition, args...)
}

// appendAdjustmentDayBounds narrows issuance_adjustments.occurred_on to the requested days.
// occurred_on is a calendar day, not an instant, so it is compared with the days the bounds
// stand for. parseDateRange puts each bound at local midnight in UTC, 03:00 the same day,
// which is why the UTC date of the bound is the operator's day.
func appendAdjustmentDayBounds(clauses []string, args []any, from, to *time.Time) ([]string, []any) {
	if from != nil {
		clauses = append(clauses, "issuance_adjustments.occurred_on >= ?")
		args = append(args, from.UTC().Format(time.DateOnly))
	}
	if to != nil {
		clauses = append(clauses, "issuance_adjustments.occurred_on < ?")
		args = append(args, to.UTC().Format(time.DateOnly))
	}
	return clauses, args
}

// issuanceSearchGroup builds the free-text predicate over the fields an operator
// actually has in hand when hunting for an issuance: the three locators, the
// passenger, and the route. Every branch is accent- and case-insensitive, because
// "joao" not finding "João" is how the same booking gets registered twice.
//
// Returned as a single grouped *gorm.DB rather than chained onto the caller's
// query: the caller ANDs other dimensions on top, and an ungrouped Or() would
// escape those parentheses and widen the whole filter.
//
// The child tables are reached with EXISTS rather than a join - the list only
// Preloads, and joining would return the same issuance once per passenger and per
// flight leg.
//
// issuance_passengers.document is deliberately absent: it is encrypted at rest
// (data.IssuancePassenger.BeforeSave), so no SQL predicate can match it.
func issuanceSearchGroup(db *gorm.DB, search string) *gorm.DB {
	like := likePattern(search)

	return db.Where(unaccentedLike("program_locator"), like).
		Or(unaccentedLike("airline_locator"), like).
		Or(unaccentedLike("latam_purchase_number"), like).
		Or(unaccentedLike("main_passenger_name"), like).
		Or(unaccentedLike("origin_iata_code"), like).
		Or(unaccentedLike("destination_iata_code"), like).
		Or("EXISTS (SELECT 1 FROM issuance_passengers"+
			" WHERE issuance_passengers.issuance_id = issuances.id AND "+
			unaccentedLike("issuance_passengers.first_name || ' ' || issuance_passengers.last_name")+")", like).
		Or("EXISTS (SELECT 1 FROM issuance_flights"+
			" WHERE issuance_flights.issuance_id = issuances.id AND ("+
			unaccentedLike("issuance_flights.origin_iata_code")+" OR "+
			unaccentedLike("issuance_flights.destination_iata_code")+"))", like, like)
}

// GetIssuances handles the GET /issuances route.
// @Summary      Listar emissões
// @Description  Retorna as emissões cadastradas no sistema (exclui as removidas), da mais antiga para a mais recente. Aceita filtros opcionais por data, programa de fidelidade, cia aérea, conta, agência e emissor. As emissões canceladas também entram no filtro de data/usuário pela data e pelo usuário do cancelamento (cancelled_at/cancelled_by_id), não só pela criação.
// @Tags         Emissões
// @Produce      json
// @Security     BearerAuth
// @Param        date_from           query  string  false  "Data inicial da emissão (YYYY-MM-DD), inclusiva"
// @Param        date_to             query  string  false  "Data final da emissão (YYYY-MM-DD), inclusiva"
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        airline_id          query  string  false  "UUID da cia aérea"
// @Param        account_id          query  string  false  "UUID da conta"
// @Param        agency_id           query  string  false  "UUID da agência"
// @Param        q                   query  string  false  "Busca por localizador, passageiro, origem ou destino. Quando presente, os filtros de data e emissor são ignorados."
// @Param        issuer_id           query  string  false  "UUID do emissor (usuário). Aceita vários separados por vírgula; o token 'all' não filtra (todos os emissores) e 'none' não retorna nenhum."
// @Success      200  {object}  schemas.IssuanceListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar emissões"
// @Router       /issuances [get]
func (h *IssuanceHandler) GetIssuances(w http.ResponseWriter, r *http.Request) {
	var issuances []data.Issuance

	query := preloadIssuanceAssociations(h.DB).
		Preload("Flights", func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", "outbound").Order("leg_order asc")
		})
	// Oldest first: the grid is read top to bottom in the order the issuances actually
	// happened, and its draft row lives at the bottom - so a row created there stays
	// where it was typed instead of jumping to the top on the next refresh.
	query = applyIssuanceListFilters(h.DB, query, r).Order("created_at asc")

	if err := query.Find(&issuances).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch issuances")
		return
	}

	resp := make([]schemas.IssuanceResponse, 0, len(issuances))
	for _, issuance := range issuances {
		resp = append(resp, toIssuanceResponse(issuance))
	}

	utils.SendJSON(w, r, http.StatusOK, "Issuances fetched successfully", resp)
}

// The spreadsheets GET /issuances/export can build. The control sheet is the default because
// it is what the panel exported before the choice existed.
const (
	issuanceExportLayoutControl = "control"
	issuanceExportLayoutLegacy  = "legacy"
)

// GetIssuancesExport handles the GET /issuances/export route.
// @Summary      Exportar emissões (.xlsx)
// @Description  Gera uma planilha .xlsx das emissões filtradas. O layout é escolhido por layout: control (padrão) ou legacy. Layout control: planilha manual de controle numa aba única (a emissão cancelada é atribuída a quem cancelou, que é onde a listagem também a mostra), colunas A:O (Nº, Companhia, Localizador, Data do Voo, Passageiro Principal, Origem, Destino, Pontos, Milheiro, Taxa, Total, Conta, Observação, Taxa Resgate, Emissor), as linhas agrupadas em blocos por companhia, o bloco TOTAL DAS CONTAS ao final e, ao lado da tabela, um quadro por conta e companhia com os localizadores e o total de pontos. A coluna Emissor identifica quem fez cada linha, já que todos os emissores ficam na mesma aba. Dentro de cada bloco a ordem é a da listagem (da mais antiga para a mais recente). Aceita os mesmos filtros da listagem (data, programa de fidelidade, cia aérea, conta, agência e emissor). Emissão cancelada no mesmo dia em que foi criada entra pelo líquido (pontos e taxas menos o reembolso); cancelada em outro dia entra só pelo reembolso, negativo. A linha cancelada é destacada em vermelho claro. Layout legacy: a antiga planilha EMISSÕES GERAIS, uma tabela só (B:P) com os totais do modelo, mais a coluna TAXA RESGATE em Q. Nos dois layouts a Taxa Resgate é a taxa que a conta isenta (Azul Diamante Unique) deixou de pagar, em branco nas contas comuns, com o mesmo sinal dos pontos.
// @Tags         Emissões
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security     BearerAuth
// @Param        date_from           query  string  false  "Data inicial da emissão (YYYY-MM-DD), inclusiva"
// @Param        date_to             query  string  false  "Data final da emissão (YYYY-MM-DD), inclusiva"
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        airline_id          query  string  false  "UUID da cia aérea"
// @Param        account_id          query  string  false  "UUID da conta"
// @Param        agency_id           query  string  false  "UUID da agência"
// @Param        q                   query  string  false  "Busca por localizador, passageiro, origem ou destino. Quando presente, os filtros de data e emissor são ignorados."
// @Param        issuer_id           query  string  false  "UUID do emissor (usuário). Aceita vários separados por vírgula; o token 'all' não filtra (todos os emissores) e 'none' não retorna nenhum."
// @Param        layout              query  string  false  "Layout da planilha: control (padrão) ou legacy"  Enums(control, legacy)
// @Success      200  {file}    binary  "Planilha .xlsx das emissões filtradas"
// @Failure      400  {object}  schemas.ErrorResponse "Layout desconhecido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao gerar a planilha"
// @Router       /issuances/export [get]
func (h *IssuanceHandler) GetIssuancesExport(w http.ResponseWriter, r *http.Request) {
	// Checked before the query so a typo in the link fails fast instead of silently handing
	// back the other spreadsheet.
	layout := r.URL.Query().Get("layout")
	if layout != "" && layout != issuanceExportLayoutControl && layout != issuanceExportLayoutLegacy {
		utils.SendError(w, r, http.StatusBadRequest, "Unknown export layout")
		return
	}

	var issuances []data.Issuance

	// Same query as GetIssuances, plus the account holder needed to resolve the CONTA label
	// (aliasless accounts fall back to the holder's name) and the canceller, whose name decides
	// which tab a cancelled issuance lands on. Neither is preloaded for the listing, which shows
	// no such column.
	query := preloadIssuanceAssociations(h.DB).
		// Unscoped: an archived holder still names the issuances made on their accounts.
		Preload("Account.Holder", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Preload("CancelledBy").
		Preload("Flights", func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", "outbound").Order("leg_order asc")
		})
	query = applyIssuanceListFilters(h.DB, query, r).Order("created_at asc")

	if err := query.Find(&issuances).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch issuances")
		return
	}

	// Whether a cancelled issuance enters the sheet as its booking, as its refund, or as both
	// netting out is decided by its own two dates. The window is for the alterations: an old
	// booking listed only because it was altered in the period gets the alteration's line and
	// not its booking again. A search lifts the date filter on the listing, so it lifts the
	// window too.
	window := export.ExportWindow{}
	if strings.TrimSpace(r.URL.Query().Get("q")) == "" {
		window.From, window.To = parseDateRange(r)
		if raw := r.URL.Query().Get("issuer_id"); raw != "" && raw != issuerFilterAll {
			window.Issuers = splitIDs(raw)
		}
	}
	var content []byte
	var err error
	filenamePrefix := "emissoes"
	if layout == issuanceExportLayoutLegacy {
		content, err = export.BuildLegacyIssuancesXLSX(issuances)
		filenamePrefix = "emissoes-gerais"
	} else {
		content, err = export.BuildIssuancesXLSX(issuances, window)
	}
	if err != nil {
		slog.Error("failed to build issuances export", "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to build issuances export")
		return
	}

	// Binary response: the standard JSON envelope does not apply, mirroring the PDF path.
	filename := fmt.Sprintf("%s-%s.xlsx", filenamePrefix, exportFilenameDate(time.Now()))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	// gosec follows the date filters from the query string into this write and calls the
	// response reflected input. What actually reaches the browser is a .xlsx built by
	// excelize from database rows, served as a binary attachment - the two dates only ever
	// decide the sign of a number inside a cell, and never reach the bytes as text.
	//nolint:gosec // G705: no request text is echoed; see above.
	if _, err := w.Write(content); err != nil {
		slog.Error("failed to write issuances export response", "err", err)
	}
}

// GetIssuancesSummary handles the GET /issuances/summary route.
// @Summary      Listar emissões (resumo)
// @Description  Retorna uma versão enxuta das emissões ativas, com apenas os campos usados em telas de listagem.
// @Tags         Emissões
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.IssuanceSummaryListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar emissões"
// @Router       /issuances/summary [get]
func (h *IssuanceHandler) GetIssuancesSummary(w http.ResponseWriter, r *http.Request) {
	var issuances []data.Issuance

	query := preloadIssuanceAssociations(h.DB).
		Preload("Flights", func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", "outbound").Order("leg_order asc")
		})
	query = middleware.AgencyScopeFrom(r.Context()).Restrict(query, "issuances.agency_id")

	if err := query.Find(&issuances).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch issuances")
		return
	}

	resp := make([]schemas.IssuanceSummaryResponse, 0, len(issuances))
	for _, issuance := range issuances {
		resp = append(resp, toIssuanceSummaryResponse(issuance))
	}

	utils.SendJSON(w, r, http.StatusOK, "Issuances fetched successfully", resp)
}

// GetIssuance handles the GET /issuances/{id} route
// @Summary      Buscar emissão por ID
// @Description  Retorna os dados completos de uma emissão específica, incluindo os voos.
// @Tags         Emissões
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da emissão"
// @Success      200  {object}  schemas.IssuanceSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Router       /issuances/{id} [get]
func (h *IssuanceHandler) GetIssuance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var issuance data.Issuance
	query := preloadIssuanceAssociations(h.DB).
		Preload("Flights").
		Preload("Passengers").
		Preload("OriginAirport").
		Preload("DestinationAirport")

	if err := query.First(&issuance, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Issuance fetched successfully", toIssuanceResponse(issuance))
}

// CreateIssuance handles the POST /issuances route
// @Summary      Criar emissão
// @Description  Cria uma nova emissão. issuer_id usa o usuário autenticado (JWT) se omitido, agency_id usa a agência padrão se omitido, e a emissão sempre nasce com complete=false. Uma venda em aberto (client_id=null, paid=false) é criada na mesma transação. flight_date (YYYY-MM-DD) cria a data de partida da perna de ida principal. account_nickname_id é opcional e, quando informado, precisa pertencer à conta em account_id. issued_on (YYYY-MM-DD) registra a emissão em outro dia — uma que faltava e foi lançada depois; omitido, vale agora.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateIssuanceRequest true "Dados da nova emissão"
// @Success      201  {object}  schemas.IssuanceSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido, flight_date com formato inválido, código IATA inexistente ou apelido que não pertence à conta"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Usuário sem acesso a nenhuma agência"
// @Failure      404  {object}  schemas.ErrorResponse "Agência fora do escopo do usuário"
// @Failure      422  {object}  schemas.ErrorResponse "Program locator já está em uso por uma emissão ativa"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar emissão"
// @Router       /issuances [post]
func (h *IssuanceHandler) CreateIssuance(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	var req schemas.CreateIssuanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	var flightDate *time.Time
	if req.FlightDate != nil {
		parsed, err := time.Parse("2006-01-02", *req.FlightDate)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Invalid flight_date")
			return
		}
		flightDate = &parsed
	}

	// The day the issuance happened, when the operator says it was not today. The clock
	// stays now()'s, so several rows backdated to the same day keep the order they were
	// typed in - which is the order the FIFO queue reads them in.
	var issuedAt *time.Time
	if req.IssuedOn != nil {
		day, err := parseIssuanceDay(*req.IssuedOn)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Invalid issued_on")
			return
		}
		frozenUntil, err := refuseClosedPeriod(h.DB, req.AccountID, day)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the inventory period: "+err.Error())
			return
		}
		if !frozenUntil.IsZero() {
			utils.SendError(w, r, http.StatusUnprocessableEntity, closedPeriodMessage(frozenUntil))
			return
		}
		moment := rebaseToDay(time.Now(), day)
		issuedAt = &moment
	}

	// The agency is settled here rather than left to issuancesvc.Create's fallback: that
	// fallback is the main agency, which a restricted user may not be allowed to see - their
	// issuance would vanish from their own grid the moment it was saved.
	scope := middleware.AgencyScopeFrom(r.Context())
	if req.AgencyID != nil && !scope.Allows(*req.AgencyID) {
		utils.SendError(w, r, http.StatusNotFound, "Agency not found")
		return
	}
	if req.AgencyID == nil {
		agencyID, err := access.ResolveDefaultAgency(h.DB, scope)
		if errors.Is(err, access.ErrNoAgencyAccess) {
			utils.SendError(w, r, http.StatusForbidden, "Você não tem acesso a nenhuma agência")
			return
		}
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to resolve the agency: "+err.Error())
			return
		}
		req.AgencyID = &agencyID
	}

	// Complete is deliberately not part of the input: an issuance created from
	// the panel always starts incomplete and is finished later from the grid.
	created, err := issuancesvc.Create(h.DB, issuancesvc.CreateInput{
		LoyaltyProgramID:    req.LoyaltyProgramID,
		ProgramLocator:      req.ProgramLocator,
		AirlineLocator:      req.AirlineLocator,
		LatamPurchaseNumber: req.LatamPurchaseNumber,
		MainPassengerName:   req.MainPassengerName,
		PassengerLastName:   req.PassengerLastName,
		TripType:            req.TripType,
		OriginIATACode:      req.OriginIATACode,
		DestinationIATACode: req.DestinationIATACode,
		Points:              req.Points,
		ValueOfTheThousand:  req.ValueOfTheThousand,
		Taxes:               req.Taxes,
		AccountID:           req.AccountID,
		AccountNicknameID:   req.AccountNicknameID,
		Observation:         req.Observation,
		AdditionalServices:  req.AdditionalServices,
		AirlineID:           req.AirlineID,
		AgencyID:            req.AgencyID,
		IssuerID:            req.IssuerID,
		FlightDate:          flightDate,
		CreatedAt:           issuedAt,
	}, userCtx.ID)

	if err != nil {
		if status, message := issuanceErrorResponse(err); status != 0 {
			utils.SendError(w, r, status, message)
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create issuance: "+err.Error())
		return
	}

	if err := preloadIssuanceAssociations(h.DB).
		Preload("Flights", func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", "outbound").Order("leg_order asc")
		}).
		First(&created, "id = ?", created.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch created issuance")
		return
	}

	h.broadcastIssuance(eventIssuanceCreated, created.ID)

	utils.SendJSON(w, r, http.StatusCreated, "Issuance created successfully", toIssuanceResponse(created))
}

// UpdateIssuance handles the PATCH /issuances/{id} route
// @Summary      Atualizar emissão (autosave)
// @Description  Atualiza um subconjunto de campos de uma emissão existente. complete não é aceito diretamente - use status, que é traduzido para complete/cancelled_at. flight_date (YYYY-MM-DD) cria ou atualiza a data de partida da perna de ida principal. issued_on (YYYY-MM-DD) move a emissão para outro dia, mantendo a hora; numa conta com estoque próprio, uma data dentro de período fechado é recusada com 422.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                       true  "UUID da emissão"
// @Param        payload body  schemas.UpdateIssuanceRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.IssuanceSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido, flight_date com formato inválido ou código IATA inexistente"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Program locator já está em uso, ou a emissão foi marcada como completa sem um campo que a cia aérea exige"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar emissão"
// @Router       /issuances/{id} [patch]
func (h *IssuanceHandler) UpdateIssuance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var issuance data.Issuance
	if err := h.DB.First(&issuance, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return
	}

	// Captured before the patch is merged in below, so the beneficiary bookkeeping at the
	// end of the transaction can tell whether the passenger moved at all.
	previousPassengerName := issuance.MainPassengerName
	previousAccountID := issuance.AccountID
	previousCreatedAt := issuance.CreatedAt

	body, err := io.ReadAll(r.Body)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	var req schemas.UpdateIssuanceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Decoded a second time, the same way UpdateSupplierIssuances is: the struct carries
	// the values, the raw map tells an omitted redemption fee (leave the correction alone)
	// from an explicit null (drop it and let the rule recompute). Every other field here
	// only needs "absent means unchanged", which a *int already says.
	var present map[string]json.RawMessage
	if err := json.Unmarshal(body, &present); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	if req.IssuerID != nil {
		var user data.User
		if err := h.DB.First(&user, "id = ?", *req.IssuerID).Error; err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Issuer not found")
			return
		}
		issuance.IssuerID = *req.IssuerID
	}

	if req.ProgramLocator != nil {
		issuance.ProgramLocator = *req.ProgramLocator
	}
	// An empty string is how the client says "cleared": JSON null and an absent key both
	// decode to nil here, so the empty string is the only value left to express it.
	if req.AirlineLocator != nil {
		if *req.AirlineLocator == "" {
			issuance.AirlineLocator = nil
		} else {
			issuance.AirlineLocator = req.AirlineLocator
		}
	}

	if req.LoyaltyProgramID != nil {
		issuance.LoyaltyProgramID = *req.LoyaltyProgramID
	}
	if req.AirlineID != nil {
		issuance.AirlineID = *req.AirlineID
	}
	// account_id and account_nickname_id are updated as a unit: the grid always sends
	// both when the account cell changes, so an omitted nickname alongside a present
	// account means "this account has no alias", not "keep the previous alias". A plain
	// *string field cannot otherwise express clearing the alias, since JSON null and an
	// absent key both decode to nil.
	if req.AccountID != nil {
		// Moving an issuance onto an archived account is a new reference to it: refused.
		if *req.AccountID != issuance.AccountID {
			if err := account.RequireLiveAccount(h.DB, *req.AccountID); err != nil {
				utils.SendError(w, r, http.StatusUnprocessableEntity, err.Error())
				return
			}
		}
		if err := issuancesvc.ValidateAccountNickname(h.DB, *req.AccountID, req.AccountNicknameID); err != nil {
			utils.SendError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		issuance.AccountID = *req.AccountID
		issuance.AccountNicknameID = req.AccountNicknameID
	}
	if req.AgencyID != nil {
		// The route already proved the issuance is in scope; moving it has to land in
		// scope too, or it would be a way to hand an issuance to an agency the user
		// cannot see - and lose it from their own grid.
		if !middleware.AgencyScopeFrom(r.Context()).Allows(*req.AgencyID) {
			utils.SendError(w, r, http.StatusNotFound, "Agency not found")
			return
		}
		// A sale's client belongs to the sale's agency, so moving an issuance whose sale
		// already has a client would break that pairing. Unlinking the client comes first.
		if *req.AgencyID != issuance.AgencyID {
			var linked int64
			if err := h.DB.Model(&data.Sale{}).
				Where("issuance_id = ? AND client_id IS NOT NULL", issuance.ID).
				Count(&linked).Error; err != nil {
				utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the sale: "+err.Error())
				return
			}
			if linked > 0 {
				utils.SendError(w, r, http.StatusUnprocessableEntity,
					"A venda desta emissão tem cliente de outra agência: desvincule o cliente antes de trocar a agência")
				return
			}
		}
		issuance.AgencyID = *req.AgencyID
	}
	// Same empty-string-means-cleared convention as AirlineLocator above, and for the
	// same reason: the Localizador cell sends all three of its fields on every save,
	// so it needs a value that says "the operator removed this".
	if req.LatamPurchaseNumber != nil {
		if *req.LatamPurchaseNumber == "" {
			issuance.LatamPurchaseNumber = nil
		} else {
			issuance.LatamPurchaseNumber = req.LatamPurchaseNumber
		}
	}
	if req.MainPassengerName != nil {
		issuance.MainPassengerName = req.MainPassengerName
	}
	if req.PassengerLastName != nil {
		issuance.PassengerLastName = req.PassengerLastName
	}
	if req.TripType != nil {
		issuance.TripType = req.TripType
	}
	if req.OriginIATACode != nil {
		issuance.OriginIATACode = req.OriginIATACode
	}
	if req.DestinationIATACode != nil {
		issuance.DestinationIATACode = req.DestinationIATACode
	}
	if req.Points != nil {
		issuance.Points = *req.Points
	}
	if req.ValueOfTheThousand != nil {
		issuance.ValueOfTheThousand = *req.ValueOfTheThousand
	}
	if req.Taxes != nil {
		issuance.Taxes = req.Taxes
	}

	// A value pins the correction; an explicit null releases the issuance and lets the
	// recalculation at the end of the transaction fill the amount back in.
	if _, present := present["redemption_fee_saved_cents"]; present {
		issuance.RedemptionFeeManual = req.RedemptionFeeSavedCents != nil
		issuance.RedemptionFeeSavedCents = req.RedemptionFeeSavedCents
	}
	if req.Observation != nil {
		issuance.Observation = req.Observation
	}
	if req.AdditionalServices != nil {
		issuance.AdditionalServices = req.AdditionalServices
	}

	if req.Status != nil {
		switch *req.Status {
		case "incomplete":
			issuance.Complete = false
			issuance.CancelledAt = nil
		case "complete":
			issuance.Complete = true
			issuance.CancelledAt = nil
		case "cancelled":
			now := time.Now()
			issuance.CancelledAt = &now
		}
	}

	var flightDate *time.Time
	if req.FlightDate != nil {
		parsed, err := time.Parse("2006-01-02", *req.FlightDate)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Invalid flight_date")
			return
		}
		flightDate = &parsed
	}

	// Moving the issuance to another day. Both days are checked, not just the new one: a
	// booking already costed inside a closed period cannot leave it either, or the period
	// keeps a slice of a row that is no longer in it.
	if req.IssuedOn != nil {
		day, err := parseIssuanceDay(*req.IssuedOn)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Invalid issued_on")
			return
		}
		frozenUntil, err := refuseClosedPeriod(h.DB, issuance.AccountID, day, previousCreatedAt)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the inventory period: "+err.Error())
			return
		}
		if !frozenUntil.IsZero() {
			utils.SendError(w, r, http.StatusUnprocessableEntity, closedPeriodMessage(frozenUntil))
			return
		}
		issuance.CreatedAt = rebaseToDay(previousCreatedAt, day)
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// The airline's requires_last_name/requires_purchase_number flags are
		// enforced here rather than on create so the grid's autosave keeps
		// working: a draft row is POSTed the moment its five mandatory fields
		// are filled, well before a passenger name exists. Completing the
		// issuance is the operator's explicit "this is finished" action, and
		// what gets checked is the merged state above - not the incoming patch,
		// which may not carry the passenger name at all.
		if issuance.Complete {
			var airline data.Airline
			if err := tx.First(&airline, "id = ?", issuance.AirlineID).Error; err != nil {
				return err
			}
			if err := issuancesvc.ValidateAirlineRequirements(airline, issuance.BookingLastName(), issuance.LatamPurchaseNumber); err != nil {
				return err
			}
		}

		if err := issuancesvc.ValidateAirportCodes(tx, issuance.OriginIATACode, issuance.DestinationIATACode); err != nil {
			return err
		}

		if err := tx.Save(&issuance).Error; err != nil {
			return err
		}

		// Written as a column of its own because created_at is the one field GORM manages
		// rather than carries: leaving it to Save would make the correction depend on a
		// convention that exists to stop exactly this from being overwritten by accident.
		if !issuance.CreatedAt.Equal(previousCreatedAt) {
			if err := tx.Model(&issuance).UpdateColumn("created_at", issuance.CreatedAt).Error; err != nil {
				return err
			}
			if err := recordIssuanceDateChange(tx, r, issuance.ID, previousCreatedAt, issuance.CreatedAt); err != nil {
				return err
			}
		}

		if err := syncIssuanceBeneficiary(tx, issuance, previousPassengerName, previousAccountID); err != nil {
			return err
		}

		if flightDate != nil {
			if err := issuancesvc.UpsertPrimaryOutboundFlight(tx, issuance.ID, *flightDate); err != nil {
				return err
			}
		}

		// After the flight upsert, never before: the Save above runs while the departure
		// date is still the old one, so a fee computed any earlier would be derived from
		// the itinerary this very PATCH is replacing. Moving the issuance to another
		// account matters just as much - the account is what decides whether the amount
		// is owed or saved.
		return issuancesvc.RecalculateRedemptionFee(tx, issuance.ID)
	})

	if err != nil {
		err = issuancesvc.TranslateWriteError(err)
		if status, message := issuanceErrorResponse(err); status != 0 {
			utils.SendError(w, r, status, message)
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update issuance: "+err.Error())
		return
	}

	if err := preloadIssuanceAssociations(h.DB).
		Preload("Flights", func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", "outbound").Order("leg_order asc")
		}).
		First(&issuance, "id = ?", issuance.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch updated issuance")
		return
	}

	h.broadcastIssuance(eventIssuanceUpdated, issuance.ID)

	utils.SendJSON(w, r, http.StatusOK, "Issuance updated successfully", toIssuanceResponse(issuance))
}

// syncIssuanceBeneficiary keeps the account's CPF consumption in step with a patched
// issuance. It only acts when the passenger or the account actually moved, because the grid
// PATCHes on every cell edit and re-registering on each one would be pure noise.
//
// Two corrections it has to handle, and both are why this replaces rather than adds:
//
//   - the passenger name was fixed. The slot follows the new name; the old beneficiary is
//     dropped if this issuance was the only thing keeping it alive.
//   - the issuance was moved to another account. The slot has to leave the account that
//     never issued it, or that account carries a phantom forever.
//
// An issuance whose passengers came from Traveos is deliberately left alone: the vendor
// knows the whole booking (a family of four, not one name), and overwriting that with the
// single main_passenger_name would throw the other three away.
func syncIssuanceBeneficiary(
	tx *gorm.DB,
	issuance data.Issuance,
	previousPassengerName *string,
	previousAccountID string,
) error {
	if issuance.TraveosSyncedAt != nil {
		return nil
	}

	nameChanged := !equalStringPointers(previousPassengerName, issuance.MainPassengerName)
	if !nameChanged && issuance.AccountID == previousAccountID {
		return nil
	}

	var passengers []account.Passenger
	if issuance.MainPassengerName != nil {
		passengers = append(passengers, account.Passenger{Name: *issuance.MainPassengerName})
	}

	return account.ReplaceIssuanceUses(
		tx, issuance.AccountID, issuance.ID, passengers, issuance.CreatedAt,
	)
}

func equalStringPointers(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// DeleteIssuance handles the DELETE /issuances/{id} route.
// @Summary      Deletar emissão
// @Description  Remove (soft delete) uma emissão, registra o audit log e apaga a venda associada, se houver. Diferente do cancelamento, uma emissão deletada não deixa venda em aberto - ela some da aba de Vendas.
// @Tags         Emissões
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da emissão"
// @Success      200  {object}  schemas.ErrorResponse "Emissão deletada com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar emissão"
// @Router       /issuances/{id} [delete]
func (h *IssuanceHandler) DeleteIssuance(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	id := chi.URLParam(r, "id")

	var issuance data.Issuance
	if err := h.DB.First(&issuance, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		return issuancesvc.Delete(tx, issuance, userCtx.ID)
	})

	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete issuance: "+err.Error())
		return
	}

	h.broadcastIssuance(eventIssuanceDeleted, issuance.ID)

	utils.SendJSON(w, r, http.StatusOK, "Issuance deleted successfully", nil)
}

// CancelIssuance handles the POST /issuances/{id}/cancel route.
// @Summary      Cancelar emissão
// @Description  Cancela uma emissão (active=false, cancelled_at=now, ou a data em cancelled_on), registra os valores de reembolso, reseta o pagamento da venda associada (se paga) e grava um audit log. Tudo numa única transação. Com cancellation_only=true, a emissão passa a valer apenas como estorno: o saldo do fornecedor conta o reembolso e não os pontos, para o caso da reserva ter sido feita fora do painel.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                        true  "UUID da emissão"
// @Param        payload body  schemas.CancelIssuanceRequest false "Valores de reembolso (todos opcionais)"
// @Success      200  {object}  schemas.IssuanceSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Emissão já está cancelada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao cancelar emissão"
// @Router       /issuances/{id}/cancel [post]
func (h *IssuanceHandler) CancelIssuance(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	id := chi.URLParam(r, "id")

	// GORM's soft-delete scope excludes deleted_at IS NOT NULL rows, so a deleted
	// issuance is indistinguishable from a missing one here - both are 404.
	var issuance data.Issuance
	if err := h.DB.First(&issuance, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return
	}

	var req schemas.CancelIssuanceRequest
	// An empty body is valid (cancel without refund), so treat EOF as no payload.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// The day the points came back, when the cancellation is being recorded after the
	// fact. Same shape as issued_on, and the same guard: the refund is an event on the
	// costing's timeline, and one dated inside a closed period would never be replayed.
	var cancelledAt *time.Time
	if req.CancelledOn != nil {
		day, err := parseIssuanceDay(*req.CancelledOn)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Invalid cancelled_on")
			return
		}
		frozenUntil, err := refuseClosedPeriod(h.DB, issuance.AccountID, day)
		if err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to check the inventory period: "+err.Error())
			return
		}
		if !frozenUntil.IsZero() {
			utils.SendError(w, r, http.StatusUnprocessableEntity, closedPeriodMessage(frozenUntil))
			return
		}
		moment := rebaseToDay(time.Now(), day)
		cancelledAt = &moment
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		issuance, txErr = issuancesvc.Cancel(tx, issuance, issuancesvc.CancelInput{
			RefundedPoints:   req.RefundedPoints,
			RefundedTaxes:    req.RefundedTaxes,
			CancellationFee:  req.CancellationFee,
			CancellationOnly: req.CancellationOnly,
			CancelledAt:      cancelledAt,
		}, userCtx.ID)
		return txErr
	})

	if errors.Is(err, issuancesvc.ErrIssuanceAlreadyCancelled) {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "issuance already cancelled")
		return
	}
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to cancel issuance: "+err.Error())
		return
	}

	if err := preloadIssuanceAssociations(h.DB).
		Preload("Flights", func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", "outbound").Order("leg_order asc")
		}).
		First(&issuance, "id = ?", issuance.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch cancelled issuance")
		return
	}

	h.broadcastIssuance(eventIssuanceCancelled, issuance.ID)

	utils.SendJSON(w, r, http.StatusOK, "Issuance cancelled successfully", toIssuanceResponse(issuance))
}

// UncancelIssuance handles the POST /issuances/{id}/uncancel route.
// @Summary      Reverter cancelamento de emissão
// @Description  Reverte o cancelamento de uma emissão (active=true, limpa cancelled_at/cancelled_by e os valores de reembolso). O status volta a ser derivado de complete. Se o cancelamento havia resetado o pagamento da venda associada, restaura paid=true, voltando ao estado original. Grava um audit log. Tudo numa única transação.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path  string  true  "UUID da emissão"
// @Success      200  {object}  schemas.IssuanceSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Emissão não está cancelada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao reverter cancelamento"
// @Router       /issuances/{id}/uncancel [post]
func (h *IssuanceHandler) UncancelIssuance(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	id := chi.URLParam(r, "id")

	// GORM's soft-delete scope excludes deleted_at IS NOT NULL rows, so a deleted
	// issuance is indistinguishable from a missing one here - both are 404.
	var issuance data.Issuance
	if err := h.DB.First(&issuance, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return
	}

	if issuance.Active {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "issuance is not cancelled")
		return
	}

	// Reviving the booking makes it count on its account again, which an archived account
	// must not start doing (internal/account/live.go).
	if err := account.RequireLiveAccount(h.DB, issuance.AccountID); err != nil {
		utils.SendError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// The sale's ledger is not touched here either - what the client paid is the sum of its
	// payments, and reviving a booking does not move money. What does change is what is
	// owed: the ticket is a debt again, so the balance goes back to the price minus what
	// came in. The recalculation happens with the write, below.
	//
	// Older cancellations recorded a sale_paid_reset flag for this branch to undo; those
	// entries stay in audit_logs as history and are simply no longer acted on.
	now := time.Now()
	// Clearing cancelled_at makes deriveIssuanceStatus fall back to complete/incomplete;
	// cancel never touched Complete, so the pre-cancel status is recovered for free.
	issuance.Active = true
	issuance.CancelledAt = nil
	issuance.CancelledByID = nil
	issuance.RefundedPoints = nil
	issuance.RefundedTaxes = nil
	issuance.CancellationFee = nil
	// Reverting a cancellation asserts the booking is live, which is the opposite of what
	// cancellation_only claims. Left set, the row would count neither as issued (the flag
	// suppresses it) nor as refunded (cancelled_at is nil now), and it would vanish from
	// the supplier balance on both sides.
	issuance.CancellationOnly = false

	details, err := json.Marshal(map[string]any{
		"restored_at": now,
	})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to build audit details: "+err.Error())
		return
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// Save (not Updates) so the nil pointers above persist as NULL - Updates would skip
		// them as zero values, leaving the stale cancellation data behind.
		if err := tx.Save(&issuance).Error; err != nil {
			return err
		}

		if err := sale.RecalculateByIssuance(tx, issuance.ID); err != nil {
			return err
		}

		return tx.Create(&data.AuditLog{
			IssuanceID:  issuance.ID,
			ChangedByID: userCtx.ID,
			Action:      auditActionRestored,
			Details:     datatypes.JSON(details),
		}).Error
	})

	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to revert issuance cancellation: "+err.Error())
		return
	}

	if err := preloadIssuanceAssociations(h.DB).
		Preload("Flights", func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", "outbound").Order("leg_order asc")
		}).
		First(&issuance, "id = ?", issuance.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch restored issuance")
		return
	}

	// An uncancel is an update from the panel's point of view: the row stays where
	// it is and only its status and cancellation metadata change.
	h.broadcastIssuance(eventIssuanceUpdated, issuance.ID)

	utils.SendJSON(w, r, http.StatusOK, "Issuance cancellation reverted successfully", toIssuanceResponse(issuance))
}

// traveosSyncTimeout bounds the whole sync, not one HTTP call. Issuing a ticket
// alone can take up to 120s because Traveos drives the airline's own website, and
// the sync may issue *and* then read - so the budget has to cover both plus the
// database work, with room to spare rather than cutting a real emission short.
const traveosSyncTimeout = 3 * time.Minute

// SyncIssuanceWithTraveos handles the POST /issuances/{id}/traveos-sync route.
// @Summary      Puxar dados da Traveos
// @Description  Busca o bilhete na Traveos (emitindo lá primeiro, se ainda não existir) e grava passageiros, voos, assentos, bagagens, rota e tipo de viagem na emissão, marcando-a como completa. Sempre força: uma emissão já sincronizada é repuxada de propósito, porque é assim que uma alteração de voo entra no sistema.
// @Tags         Emissões
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da emissão"
// @Success      200  {object}  schemas.IssuanceSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "A cia aérea não usa Traveos, não tem o id da Traveos configurado, a emissão não tem um campo que a cia exige, ou nenhum provedor encontrou a reserva"
// @Failure      502  {object}  schemas.ErrorResponse "A Traveos recusou ou falhou"
// @Failure      503  {object}  schemas.ErrorResponse "Integração com a Traveos não configurada neste servidor"
// @Router       /issuances/{id}/traveos-sync [post]
func (h *IssuanceHandler) SyncIssuanceWithTraveos(w http.ResponseWriter, r *http.Request) {
	if h.Bookings == nil || h.Bookings.Empty() {
		// 503 and not 401: the panel deletes the session cookie on a 401, so a
		// misconfigured integration would log every operator out (see
		// middleware.RequireAuth for the same reasoning about database hiccups).
		utils.SendError(w, r, http.StatusServiceUnavailable,
			"A integração com a Traveos não está configurada neste servidor")
		return
	}

	id := chi.URLParam(r, "id")

	// force is always true here. This route is only ever reached by an operator
	// pressing the button, and a deliberate press on an already-synced issuance
	// means "the flight changed, fetch it again" - not "retry a failure". That is
	// also why nothing here refuses a complete issuance: the silent-ignore rule
	// that protects origin/last-name/purchase-number from accidental edits is
	// about editing fields, and this is a resync. Different intent, different rule.
	ctx, cancel := context.WithTimeout(r.Context(), traveosSyncTimeout)
	defer cancel()

	// Announced before the pull, not after: Sync is synchronous and can take minutes,
	// so waiting for it to return would tell the other operators the row was busy
	// only once it no longer is. Sync marks the row itself as well - it has to, for
	// the bot's path - and marking twice only refreshes the timestamp.
	if err := issuancesvc.MarkTraveosSyncing(h.DB, id); err != nil {
		slog.Error("failed to mark the issuance as syncing", "issuance_id", id, "err", err)
	} else {
		h.broadcastIssuance(eventIssuanceUpdated, id)
	}

	if err := issuancesvc.Sync(ctx, h.DB, h.Bookings, id, true); err != nil {
		// Sync only unmarks what it marked, and it marks late - after the airline
		// gates. A refusal before that (airline not integrated, no locator) would
		// otherwise leave the row locked on every screen until the marker went stale.
		if clearErr := issuancesvc.ClearTraveosSyncing(h.DB, id); clearErr != nil {
			slog.Error("failed to clear the traveos syncing marker",
				"issuance_id", id, "err", clearErr)
		}
		h.broadcastIssuance(eventIssuanceUpdated, id)

		h.respondToTraveosSyncError(w, r, id, err)
		return
	}

	var issuance data.Issuance
	if err := preloadIssuanceAssociations(h.DB).
		Preload("Flights", func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", "outbound").Order("leg_order asc")
		}).
		First(&issuance, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch synced issuance")
		return
	}

	h.broadcastIssuance(eventIssuanceUpdated, issuance.ID)

	utils.SendJSON(w, r, http.StatusOK, "Issuance synced with Traveos successfully", toIssuanceResponse(issuance))
}

// respondToTraveosSyncError turns a sync failure into a response an operator can
// act on - or honestly cannot.
//
// The important line is the last one. Every Traveos emission failure except an
// unknown airlineId arrives as the same opaque 500: a wrong locator, a given name
// where the surname belongs, an airline with no scraper, and the airline's site
// being down are indistinguishable. So the message says what happened and stops
// there, and the real error goes to the log. Inventing a cause would send someone
// looking in the wrong place.
func (h *IssuanceHandler) respondToTraveosSyncError(w http.ResponseWriter, r *http.Request, id string, err error) {
	switch {
	case errors.Is(err, issuancesvc.ErrIssuanceNotFound):
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")

	case errors.Is(err, issuancesvc.ErrAirlineNotSupported):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Essa companhia aérea não está integrada com a Traveos")

	case errors.Is(err, issuancesvc.ErrAirlineNotMapped):
		// Nameable and fixable, unlike everything below: someone has to go set the
		// airline's Traveos id.
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Essa companhia aérea não tem o ID da Traveos configurado")

	case errors.Is(err, issuancesvc.ErrLocatorMissing):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"A emissão não tem localizador para consultar")

	// Nameable, and a dead end rather than something to retry: the booking was made
	// outside the system, so if Traveos does not already have it, the only way to put
	// it there would be to issue the ticket again - for a flight that already flew.
	case errors.Is(err, issuancesvc.ErrImportedNotAtVendor):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Essa emissão foi importada da planilha e a Traveos não tem essa reserva. "+
				"Emitir de novo não é opção, então os dados do voo têm que ser preenchidos à mão")

	// Also nameable, and the reason the sync refused before spending anything: the
	// vendor's emission is not idempotent, so asking it with one of these fields
	// empty would create a real ticket and still answer the opaque 500 below.
	case errors.Is(err, issuancesvc.ErrLastNameRequired):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Essa companhia aérea exige o sobrenome do passageiro para consultar o bilhete")

	case errors.Is(err, issuancesvc.ErrPurchaseNumberRequired):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"Essa companhia aérea exige o número da compra para consultar o bilhete")

	case errors.Is(err, traveos.ErrAirlineNotFound):
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"A Traveos não reconhece o ID configurado para essa companhia aérea")

	case errors.Is(err, issuancesvc.ErrTicketNotFoundAfterEmission):
		slog.Error("traveos reported a successful emission but the ticket is not listed",
			"issuance_id", id, "err", err)
		utils.SendError(w, r, http.StatusBadGateway,
			"A Traveos confirmou a emissão mas não devolveu o bilhete. Confira lá antes de tentar de novo")

	// An answer, not an outage: the airline looked and found nothing, and so did the
	// vendor when it was asked. For a carrier read directly that almost always means
	// the origin or the surname on the row does not match the booking, which the
	// operator can fix - the generic 502 below read as "the system is down" instead.
	case errors.Is(err, booking.ErrNotFound):
		slog.Warn("no provider found the booking", "issuance_id", id, "err", err)
		utils.SendError(w, r, http.StatusUnprocessableEntity,
			"A companhia aérea não encontrou essa reserva. Confira o localizador, a origem e o sobrenome do passageiro")

	default:
		slog.Error("failed to sync an issuance with traveos", "issuance_id", id, "err", err)
		utils.SendError(w, r, http.StatusBadGateway,
			"Não consegui puxar os dados da Traveos para essa emissão")
	}
}
