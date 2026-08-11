package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	airportSearchLimit  = 20
	airportDefaultLimit = 20
	airportMaxLimit     = 100
)

type AirportHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewAirportHandler(db *gorm.DB) *AirportHandler {
	return &AirportHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toAirportResponse(airport data.Airport) schemas.AirportResponse {
	return schemas.AirportResponse{
		ID:       airport.ID,
		IATACode: airport.IATACode,
		Name:     airport.Name,
		City:     airport.City,
		State:    airport.State,
		Country:  airport.Country,
	}
}

// GetAirports handles the GET /airports route.
// @Summary      Buscar aeroportos
// @Description  Busca aeroportos por código IATA, nome ou cidade (contains, case-insensitive), priorizando quem começa com o texto buscado. Sem "q", retorna uma lista vazia - a tabela tem milhares de linhas e não é servida inteira.
// @Tags         Aeroportos
// @Produce      json
// @Security     BearerAuth
// @Param        q  query  string  false  "Texto de busca (código IATA, nome ou cidade)"
// @Success      200  {object}  schemas.AirportListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar aeroportos"
// @Router       /airports [get]
func (h *AirportHandler) GetAirports(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	airports := make([]data.Airport, 0)
	if q != "" {
		like := "%" + q + "%"
		if err := h.DB.
			Where("iata_code ILIKE ? OR name ILIKE ? OR city ILIKE ?", like, like, like).
			Order("iata_code ASC").
			Limit(airportSearchLimit * 5).
			Find(&airports).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch airports")
			return
		}

		lowerQ := strings.ToLower(q)
		sort.SliceStable(airports, func(i, j int) bool {
			iStarts := strings.HasPrefix(strings.ToLower(airports[i].IATACode), lowerQ)
			jStarts := strings.HasPrefix(strings.ToLower(airports[j].IATACode), lowerQ)
			return iStarts && !jStarts
		})
		if len(airports) > airportSearchLimit {
			airports = airports[:airportSearchLimit]
		}
	}

	resp := make([]schemas.AirportResponse, 0, len(airports))
	for _, airport := range airports {
		resp = append(resp, toAirportResponse(airport))
	}

	utils.SendJSON(w, r, http.StatusOK, "Airports fetched successfully", resp)
}

// ListAllAirports handles the GET /airports/all route.
// @Summary      Listar todos os aeroportos
// @Description  Retorna todos os aeroportos cadastrados, ordenados por código IATA. Usado para alimentar comboboxes que filtram no cliente.
// @Tags         Aeroportos
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.AirportListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar aeroportos"
// @Router       /airports/all [get]
func (h *AirportHandler) ListAllAirports(w http.ResponseWriter, r *http.Request) {
	var airports []data.Airport

	if err := h.DB.Order("iata_code ASC").Find(&airports).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch airports")
		return
	}

	resp := make([]schemas.AirportResponse, 0, len(airports))
	for _, airport := range airports {
		resp = append(resp, toAirportResponse(airport))
	}

	utils.SendJSON(w, r, http.StatusOK, "Airports fetched successfully", resp)
}

// ListAirports handles the GET /airports/manage route.
// @Summary      Listar aeroportos (paginado)
// @Description  Retorna aeroportos de forma paginada. Com "q", filtra por código IATA, nome, cidade, estado ou país (contains, case-insensitive), priorizando a correspondência nessa ordem (IATA primeiro).
// @Tags         Aeroportos
// @Produce      json
// @Security     BearerAuth
// @Param        q      query  string  false  "Texto de busca"
// @Param        page   query  int     false  "Página (default 1)"
// @Param        limit  query  int     false  "Itens por página (default 20, máx 100)"
// @Success      200  {object}  schemas.AirportPaginatedResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar aeroportos"
// @Router       /airports/manage [get]
// Airports sort by their own vocabulary rather than the shared name_asc/name_desc:
// the row leads with the IATA code, so "A–Z" would be ambiguous between the two.
var airportOrder = map[string]string{
	"code_asc":  "iata_code ASC",
	"code_desc": "iata_code DESC",
	"name_asc":  `name COLLATE "pt-BR-x-icu" ASC`,
	"name_desc": `name COLLATE "pt-BR-x-icu" DESC`,
}

func (h *AirportHandler) ListAirports(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	limit := parsePositiveInt(r.URL.Query().Get("limit"), airportDefaultLimit)
	if limit > airportMaxLimit {
		limit = airportMaxLimit
	}

	// Base query is shared by the count and the paginated fetch so both apply the
	// same search filter.
	query := h.DB.Model(&data.Airport{})
	if q != "" {
		like := "%" + q + "%"
		query = query.Where(
			"iata_code ILIKE ? OR name ILIKE ? OR city ILIKE ? OR state ILIKE ? OR country ILIKE ?",
			like, like, like, like, like,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to count airports")
		return
	}

	// Relevance ordering, but only when the operator has not asked for a specific
	// sort: rows matching by IATA code come first, then name, city, state and
	// finally country — mirroring the user-facing fallback chain, with IATA code as
	// the tie-breaker. A raw string in .Order() cannot carry bound args, and
	// .Order() silently ignores a clause.Expr value, so the CASE goes through
	// clause.OrderBy{Expression:...} which does render bound placeholders.
	//
	// An explicit `order` wins over relevance: asking for Z–A and getting the
	// closest matches first would look like the control did nothing.
	order := strings.TrimSpace(r.URL.Query().Get("order"))
	_, explicitOrder := airportOrder[order]

	dbQuery := query.Session(&gorm.Session{})
	switch {
	case explicitOrder:
		dbQuery = dbQuery.Order(airportOrder[order])
	case q != "":
		like := "%" + q + "%"
		dbQuery = dbQuery.Clauses(clause.OrderBy{
			Expression: gorm.Expr(
				"CASE WHEN iata_code ILIKE ? THEN 1 WHEN name ILIKE ? THEN 2 WHEN city ILIKE ? THEN 3 WHEN state ILIKE ? THEN 4 WHEN country ILIKE ? THEN 5 ELSE 6 END, iata_code ASC",
				like, like, like, like, like,
			),
		})
	default:
		dbQuery = dbQuery.Order(airportOrder["code_asc"])
	}

	var airports []data.Airport
	if err := dbQuery.
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&airports).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch airports")
		return
	}

	items := make([]schemas.AirportResponse, 0, len(airports))
	for _, airport := range airports {
		items = append(items, toAirportResponse(airport))
	}

	utils.SendJSON(w, r, http.StatusOK, "Airports fetched successfully", schemas.AirportPaginatedData{
		Airports: items,
		Total:    total,
		Page:     page,
		Limit:    limit,
	})
}

// parsePositiveInt returns the parsed value when it is a positive integer,
// otherwise the provided fallback.
func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

// GetAirport handles the GET /airports/{id} route.
// @Summary      Buscar aeroporto por ID
// @Description  Retorna os dados de um aeroporto específico identificado pelo UUID.
// @Tags         Aeroportos
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do aeroporto"
// @Success      200  {object}  schemas.AirportSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      404  {object}  schemas.ErrorResponse "Aeroporto não encontrado"
// @Router       /airports/{id} [get]
func (h *AirportHandler) GetAirport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var airport data.Airport
	if err := h.DB.First(&airport, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Airport not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Airport fetched successfully", toAirportResponse(airport))
}

// CreateAirport handles the POST /airports route.
// @Summary      Criar aeroporto
// @Description  Cria um novo aeroporto no sistema.
// @Tags         Aeroportos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateAirportRequest true "Dados do novo aeroporto"
// @Success      201  {object}  schemas.AirportSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe um aeroporto com este código IATA"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar aeroporto"
// @Router       /airports [post]
func (h *AirportHandler) CreateAirport(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateAirportRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	airport := data.Airport{
		IATACode: req.IATACode,
		Name:     req.Name,
		City:     req.City,
		State:    req.State,
		Country:  "BR",
	}
	if req.Country != nil {
		airport.Country = *req.Country
	}

	if err := h.DB.Create(&airport).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe um aeroporto com este código IATA")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create airport: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Airport created successfully", toAirportResponse(airport))
}

// UpdateAirport handles the PUT /airports/{id} route.
// @Summary      Atualizar aeroporto
// @Description  Atualiza os dados de um aeroporto existente. Todos os campos são opcionais (atualização parcial).
// @Tags         Aeroportos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                      true  "UUID do aeroporto"
// @Param        payload body  schemas.UpdateAirportRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.AirportSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      404  {object}  schemas.ErrorResponse "Aeroporto não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe um aeroporto com este código IATA"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar aeroporto"
// @Router       /airports/{id} [put]
func (h *AirportHandler) UpdateAirport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var airport data.Airport
	if err := h.DB.First(&airport, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Airport not found")
		return
	}

	var req schemas.UpdateAirportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.IATACode != nil {
		airport.IATACode = *req.IATACode
	}
	if req.Name != nil {
		airport.Name = *req.Name
	}
	if req.City != nil {
		airport.City = req.City
	}
	if req.State != nil {
		airport.State = req.State
	}
	if req.Country != nil {
		airport.Country = *req.Country
	}

	if err := h.DB.Save(&airport).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe um aeroporto com este código IATA")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update airport: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Airport updated successfully", toAirportResponse(airport))
}

// DeleteAirport handles the DELETE /airports/{id} route.
// @Summary      Deletar aeroporto
// @Description  Remove um aeroporto do sistema. Falha com 422 se estiver vinculado a uma emissão.
// @Tags         Aeroportos
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do aeroporto"
// @Success      200  {object}  schemas.ErrorResponse "Aeroporto deletado com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      422  {object}  schemas.ErrorResponse "Aeroporto vinculado a uma emissão"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar aeroporto"
// @Router       /airports/{id} [delete]
func (h *AirportHandler) DeleteAirport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.Delete(&data.Airport{}, "id = ?", id).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não é possível excluir aeroporto vinculado a uma emissão")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete airport: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Airport deleted successfully", nil)
}
