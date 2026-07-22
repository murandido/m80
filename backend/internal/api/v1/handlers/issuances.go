package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	auditActionCreated   = "created"
	auditActionCancelled = "cancelled"
	auditActionRestored  = "restored"
	auditActionDeleted   = "deleted"
)

type IssuanceHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewIssuanceHandler(db *gorm.DB) *IssuanceHandler {
	return &IssuanceHandler{
		DB:        db,
		Validator: validator.New(),
	}
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
		TripType:            issuance.TripType,
		OriginIATACode:      issuance.OriginIATACode,
		DestinationIATACode: issuance.DestinationIATACode,
		Points:              issuance.Points,
		ValueOfTheThousand:  issuance.ValueOfTheThousand,
		Taxes:               issuance.Taxes,
		AccountID:           issuance.AccountID,
		IssuerID:            issuance.IssuerID,
		AgencyID:            issuance.AgencyID,
		Complete:            issuance.Complete,
		Status:              deriveIssuanceStatus(issuance.Complete, issuance.CancelledAt),
		AdditionalServices:  issuance.AdditionalServices,
		Observation:         issuance.Observation,
		Active:              issuance.Active,
		CancelledAt:         issuance.CancelledAt,
		CancelledByID:       issuance.CancelledByID,
		RefundedPoints:      issuance.RefundedPoints,
		RefundedTaxes:       issuance.RefundedTaxes,
		CancellationFee:     issuance.CancellationFee,
		CreatedAt:           issuance.CreatedAt,
		UpdatedAt:           issuance.UpdatedAt,
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
		resp.Issuer = &schemas.UserResponse{
			ID:        issuance.Issuer.ID,
			Name:      issuance.Issuer.Name,
			Phone:     issuance.Issuer.Phone,
			Role:      issuance.Issuer.Role,
			CreatedAt: issuance.Issuer.CreatedAt,
		}
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
	if len(issuance.Passengers) > 0 {
		passengers := make([]schemas.IssuancePassengerResponse, 0, len(issuance.Passengers))
		for _, passenger := range issuance.Passengers {
			passengers = append(passengers, toIssuancePassengerResponse(passenger))
		}
		resp.Passengers = passengers
	}

	return resp
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

// upsertPrimaryOutboundFlight creates or updates the leg_order=1 outbound leg with the
// only field this simplified UI collects - the departure date. flight_number/arrival_at/etc.
// are left nil rather than filled with placeholder data. Shared by create and update so
// a date typed before the issuance exists is persisted the same way as one edited after.
func upsertPrimaryOutboundFlight(tx *gorm.DB, issuanceID string, departureAt time.Time) error {
	var flight data.IssuanceFlight
	err := tx.Where("issuance_id = ? AND direction = ? AND leg_order = ?", issuanceID, "outbound", 1).
		First(&flight).Error

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		flight = data.IssuanceFlight{
			IssuanceID:  issuanceID,
			Direction:   "outbound",
			LegOrder:    1,
			DepartureAt: departureAt,
		}
		return tx.Create(&flight).Error
	case err != nil:
		return err
	default:
		flight.DepartureAt = departureAt
		return tx.Save(&flight).Error
	}
}

// validateAccountNickname checks that the chosen alias actually belongs to the chosen
// account. Both ids arrive from the client independently, so without this a typo (or a
// stale grid row) could persist an issuance labelled with another account's alias.
// A nil nicknameID is valid: accounts with no registered alias are still issuable.
func validateAccountNickname(db *gorm.DB, accountID string, nicknameID *string) error {
	if nicknameID == nil {
		return nil
	}

	var nickname data.AccountNickname
	if err := db.First(&nickname, "id = ?", *nicknameID).Error; err != nil {
		return errors.New("account nickname not found")
	}
	if nickname.AccountID != accountID {
		return errors.New("account nickname does not belong to the given account")
	}
	return nil
}

// preloadIssuanceAssociations applies the standard set of preloads used by the
// list and single-issuance responses.
func preloadIssuanceAssociations(query *gorm.DB) *gorm.DB {
	return query.
		Preload("LoyaltyProgram").
		Preload("Airline").
		Preload("Account").
		Preload("Account.Nicknames").
		Preload("AccountNickname").
		Preload("Issuer").
		Preload("Agency")
}

// issuanceListTimezone is the timezone used to interpret the date_from/date_to
// filters. created_at is stored in UTC, but operators think in local (Brazil)
// calendar days, so "today" must be resolved against this zone, not UTC.
const issuanceListTimezone = "America/Sao_Paulo"

// applyIssuanceListFilters narrows an issuance list query by the optional query
// params (issuer_id, account_id, loyalty_program_id, airline_id, date_from,
// date_to). Every param is optional: an absent or empty one leaves that
// dimension unfiltered.
//
// account_id/loyalty_program_id/airline_id are plain ANDed filters that apply to
// every row. issuer_id + date_from/date_to are instead coupled into an OR of two
// branches so a cancelled issuance surfaces in the flow of whoever cancelled it,
// on the day they cancelled it - not only in the creator's creation-day flow:
//
//	(issuer_id = ? AND created_at in [from,to))            -- creation branch
//	OR
//	(active = false AND cancelled_by_id = ?                -- cancellation branch
//	     AND cancelled_at in [from,to))
//
// Each present dimension feeds both branches (issuer -> issuer_id/cancelled_by_id,
// dates -> created_at/cancelled_at). When neither issuer nor dates are given the
// OR group is skipped entirely and the query stays unfiltered on those dimensions.
func applyIssuanceListFilters(query *gorm.DB, r *http.Request) *gorm.DB {
	q := r.URL.Query()

	for column, param := range map[string]string{
		"account_id":         "account_id",
		"loyalty_program_id": "loyalty_program_id",
		"airline_id":         "airline_id",
	} {
		if value := q.Get(param); value != "" {
			query = query.Where(column+" = ?", value)
		}
	}

	// created_at/cancelled_at are UTC in the DB; resolve local calendar days into UTC
	// bounds so "today" matches the operator's day. Fall back to UTC if tzdata is missing.
	loc, err := time.LoadLocation(issuanceListTimezone)
	if err != nil {
		loc = time.UTC
	}

	issuer := q.Get("issuer_id")
	var from, to *time.Time
	if v := q.Get("date_from"); v != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
			t := parsed.UTC()
			from = &t
		}
	}
	if v := q.Get("date_to"); v != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
			// Exclusive upper bound at the start of the next day so the whole
			// "to" day is included.
			t := parsed.AddDate(0, 0, 1).UTC()
			to = &t
		}
	}

	if issuer == "" && from == nil && to == nil {
		return query
	}

	creation := []string{}
	cancellation := []string{"active = ?"}
	creationArgs := []any{}
	cancellationArgs := []any{false}

	if issuer != "" {
		creation = append(creation, "issuer_id = ?")
		creationArgs = append(creationArgs, issuer)
		cancellation = append(cancellation, "cancelled_by_id = ?")
		cancellationArgs = append(cancellationArgs, issuer)
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

	condition := "(" + strings.Join(creation, " AND ") + ") OR (" + strings.Join(cancellation, " AND ") + ")"
	return query.Where(condition, append(creationArgs, cancellationArgs...)...)
}

// GetIssuances handles the GET /issuances route.
// @Summary      Listar emissões
// @Description  Retorna as emissões cadastradas no sistema (exclui as removidas). Aceita filtros opcionais por data, programa de fidelidade, cia aérea, conta e emissor. As emissões canceladas também entram no filtro de data/usuário pela data e pelo usuário do cancelamento (cancelled_at/cancelled_by_id), não só pela criação.
// @Tags         Emissões
// @Produce      json
// @Security     BearerAuth
// @Param        date_from           query  string  false  "Data inicial da emissão (YYYY-MM-DD), inclusiva"
// @Param        date_to             query  string  false  "Data final da emissão (YYYY-MM-DD), inclusiva"
// @Param        loyalty_program_id  query  string  false  "UUID do programa de fidelidade"
// @Param        airline_id          query  string  false  "UUID da cia aérea"
// @Param        account_id          query  string  false  "UUID da conta"
// @Param        issuer_id           query  string  false  "UUID do emissor (usuário)"
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
	query = applyIssuanceListFilters(query, r).Order("created_at desc")

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
// @Description  Cria uma nova emissão. issuer_id usa o usuário autenticado (JWT) se omitido, agency_id usa a agência padrão se omitido, e a emissão sempre nasce com complete=false. flight_date (YYYY-MM-DD) cria a data de partida da perna de ida principal. account_nickname_id é opcional e, quando informado, precisa pertencer à conta em account_id.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateIssuanceRequest true "Dados da nova emissão"
// @Success      201  {object}  schemas.IssuanceSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido, flight_date com formato inválido ou apelido que não pertence à conta"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
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

	if err := validateAccountNickname(h.DB, req.AccountID, req.AccountNicknameID); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	issuance := data.Issuance{
		LoyaltyProgramID:    req.LoyaltyProgramID,
		ProgramLocator:      req.ProgramLocator,
		AirlineLocator:      req.AirlineLocator,
		LatamPurchaseNumber: req.LatamPurchaseNumber,
		MainPassengerName:   req.MainPassengerName,
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
		IssuerID:            userCtx.ID,
		Complete:            false,
	}

	if req.AirlineID != nil {
		issuance.AirlineID = *req.AirlineID
	} else {
		var loyaltyProgram data.LoyaltyProgram
		if err := h.DB.First(&loyaltyProgram, "id = ?", req.LoyaltyProgramID).Error; err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Loyalty program not found")
			return
		}
		issuance.AirlineID = loyaltyProgram.DefaultAirlineID
	}

	if req.AgencyID != nil {
		issuance.AgencyID = *req.AgencyID
	} else {
		var defaultAgency data.Agency
		if err := h.DB.Where("is_default = ?", true).First(&defaultAgency).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "No default agency configured")
			return
		}
		issuance.AgencyID = defaultAgency.ID
	}

	if req.IssuerID != nil {
		var issuer data.User
		if err := h.DB.First(&issuer, "id = ?", *req.IssuerID).Error; err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Issuer not found")
			return
		}
		issuance.IssuerID = *req.IssuerID
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

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&issuance).Error; err != nil {
			return err
		}

		if flightDate != nil {
			if err := upsertPrimaryOutboundFlight(tx, issuance.ID, *flightDate); err != nil {
				return err
			}
		}

		return tx.Create(&data.AuditLog{
			IssuanceID:  issuance.ID,
			ChangedByID: userCtx.ID,
			Action:      auditActionCreated,
		}).Error
	})

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Program locator já está em uso por uma emissão ativa")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create issuance: "+err.Error())
		return
	}

	if err := preloadIssuanceAssociations(h.DB).
		Preload("Flights", func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", "outbound").Order("leg_order asc")
		}).
		First(&issuance, "id = ?", issuance.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch created issuance")
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Issuance created successfully", toIssuanceResponse(issuance))
}

// UpdateIssuance handles the PATCH /issuances/{id} route
// @Summary      Atualizar emissão (autosave)
// @Description  Atualiza um subconjunto de campos de uma emissão existente. complete não é aceito diretamente - use status, que é traduzido para complete/cancelled_at. flight_date (YYYY-MM-DD) cria ou atualiza a data de partida da perna de ida principal.
// @Tags         Emissões
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                       true  "UUID da emissão"
// @Param        payload body  schemas.UpdateIssuanceRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.IssuanceSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido ou flight_date com formato inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Program locator já está em uso por uma emissão ativa"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar emissão"
// @Router       /issuances/{id} [patch]
func (h *IssuanceHandler) UpdateIssuance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var issuance data.Issuance
	if err := h.DB.First(&issuance, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return
	}

	var req schemas.UpdateIssuanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
		if err := validateAccountNickname(h.DB, *req.AccountID, req.AccountNicknameID); err != nil {
			utils.SendError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		issuance.AccountID = *req.AccountID
		issuance.AccountNicknameID = req.AccountNicknameID
	}
	if req.AgencyID != nil {
		issuance.AgencyID = *req.AgencyID
	}
	if req.LatamPurchaseNumber != nil {
		issuance.LatamPurchaseNumber = req.LatamPurchaseNumber
	}
	if req.MainPassengerName != nil {
		issuance.MainPassengerName = req.MainPassengerName
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

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&issuance).Error; err != nil {
			return err
		}

		if flightDate == nil {
			return nil
		}

		return upsertPrimaryOutboundFlight(tx, issuance.ID, *flightDate)
	})

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Program locator já está em uso por uma emissão ativa")
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

	utils.SendJSON(w, r, http.StatusOK, "Issuance updated successfully", toIssuanceResponse(issuance))
}

// DeleteIssuance handles the DELETE /issuances/{id} route.
// @Summary      Deletar emissão
// @Description  Remove (soft delete) uma emissão, registra o audit log e reseta o pagamento da venda associada, se houver.
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
		if err := tx.Delete(&issuance).Error; err != nil {
			return err
		}

		if err := tx.Create(&data.AuditLog{
			IssuanceID:  issuance.ID,
			ChangedByID: userCtx.ID,
			Action:      auditActionDeleted,
		}).Error; err != nil {
			return err
		}

		return tx.Model(&data.Sale{}).Where("issuance_id = ?", issuance.ID).Update("paid", false).Error
	})

	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete issuance: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Issuance deleted successfully", nil)
}

// CancelIssuance handles the POST /issuances/{id}/cancel route.
// @Summary      Cancelar emissão
// @Description  Cancela uma emissão (active=false, cancelled_at=now), registra os valores de reembolso, reseta o pagamento da venda associada (se paga) e grava um audit log. Tudo numa única transação.
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

	if !issuance.Active {
		utils.SendError(w, r, http.StatusUnprocessableEntity, "issuance already cancelled")
		return
	}

	now := time.Now()
	issuance.Active = false
	issuance.CancelledAt = &now
	// Denormalize the canceller onto the row so the list filter can surface it in the
	// canceller's flow (audit_logs also records it, but isn't joined by the list query).
	cancelledBy := userCtx.ID
	issuance.CancelledByID = &cancelledBy
	issuance.RefundedPoints = req.RefundedPoints
	issuance.RefundedTaxes = req.RefundedTaxes
	issuance.CancellationFee = req.CancellationFee

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&issuance).Error; err != nil {
			return err
		}

		// Only touch a paid sale: scoping on paid=true keeps this a no-op when there is
		// no sale linked or it was already unpaid. RowsAffected records whether we actually
		// flipped it, so UncancelIssuance can restore the exact prior state on a revert.
		res := tx.Model(&data.Sale{}).
			Where("issuance_id = ? AND paid = ?", issuance.ID, true).
			Update("paid", false)
		if res.Error != nil {
			return res.Error
		}

		details, err := json.Marshal(map[string]any{
			"refunded_points":  req.RefundedPoints,
			"refunded_taxes":   req.RefundedTaxes,
			"cancellation_fee": req.CancellationFee,
			"cancelled_at":     now,
			"sale_paid_reset":  res.RowsAffected > 0,
		})
		if err != nil {
			return err
		}

		return tx.Create(&data.AuditLog{
			IssuanceID:  issuance.ID,
			ChangedByID: userCtx.ID,
			Action:      auditActionCancelled,
			Details:     datatypes.JSON(details),
		}).Error
	})

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

	// The linked sale's paid flag is only restored if the cancellation itself reset it -
	// blindly setting paid=true could wrongly mark a sale that was never paid. The cancel
	// audit log records that decision under sale_paid_reset; read the latest one.
	salePaidReset := false
	var lastCancel data.AuditLog
	if err := h.DB.
		Where("issuance_id = ? AND action = ?", id, auditActionCancelled).
		Order("created_at desc").
		First(&lastCancel).Error; err == nil {
		var details struct {
			SalePaidReset bool `json:"sale_paid_reset"`
		}
		_ = json.Unmarshal(lastCancel.Details, &details)
		salePaidReset = details.SalePaidReset
	}

	now := time.Now()
	// Clearing cancelled_at makes deriveIssuanceStatus fall back to complete/incomplete;
	// cancel never touched Complete, so the pre-cancel status is recovered for free.
	issuance.Active = true
	issuance.CancelledAt = nil
	issuance.CancelledByID = nil
	issuance.RefundedPoints = nil
	issuance.RefundedTaxes = nil
	issuance.CancellationFee = nil

	details, err := json.Marshal(map[string]any{
		"restored_sale_paid": salePaidReset,
		"restored_at":        now,
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

		if salePaidReset {
			if err := tx.Model(&data.Sale{}).
				Where("issuance_id = ?", issuance.ID).
				Update("paid", true).Error; err != nil {
				return err
			}
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

	utils.SendJSON(w, r, http.StatusOK, "Issuance cancellation reverted successfully", toIssuanceResponse(issuance))
}
