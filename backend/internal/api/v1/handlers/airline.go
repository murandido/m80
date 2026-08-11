package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type AirlineHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewAirlineHandler(db *gorm.DB) *AirlineHandler {
	return &AirlineHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toAirlineResponse(airline data.Airline) schemas.AirlineResponse {
	return schemas.AirlineResponse{
		ID:                     airline.ID,
		Name:                   airline.Name,
		Code:                   airline.Code,
		TraveosSupported:       airline.TraveosSupported,
		RequiresLastName:       airline.RequiresLastName,
		RequiresPurchaseNumber: airline.RequiresPurchaseNumber,
		TraveosAirlineID:       airline.TraveosAirlineID,
		BrandColor:             airline.BrandColor,
		CreatedAt:              airline.CreatedAt,
		UpdatedAt:              airline.UpdatedAt,
	}
}

// CreateAirline handles the POST /airlines route
// @Summary      Criar companhia aérea
// @Description  Cria uma nova companhia aérea no sistema.
// @Tags         Companhias Aéreas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateAirlineRequest true "Dados da nova companhia aérea"
// @Success      201  {object}  schemas.AirlineSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma companhia aérea com este código"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar companhia aérea"
// @Router       /airlines [post]
func (h *AirlineHandler) CreateAirline(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateAirlineRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	airline := data.Airline{
		Name: req.Name,
		Code: req.Code,
	}

	if req.TraveosSupported != nil {
		airline.TraveosSupported = *req.TraveosSupported
	}
	if req.RequiresLastName != nil {
		airline.RequiresLastName = *req.RequiresLastName
	}
	if req.RequiresPurchaseNumber != nil {
		airline.RequiresPurchaseNumber = *req.RequiresPurchaseNumber
	}
	airline.TraveosAirlineID = req.TraveosAirlineID
	airline.BrandColor = req.BrandColor

	if err := h.DB.Create(&airline).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe uma companhia aérea com este código")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create airline: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Airline created successfully", toAirlineResponse(airline))
}

// airlineOrder is the whitelist of sort keys; the client's string only ever indexes it.
var airlineOrder = map[string]string{
	"name_asc":     textOrder("airlines.name", "ASC"),
	"name_desc":    textOrder("airlines.name", "DESC"),
	"created_desc": "airlines.created_at DESC",
	"created_asc":  "airlines.created_at ASC",
}

// airlineListQuery applies the optional search and the ordering. The ordering is not
// optional: without it Postgres was free to return rows in heap order, so the list
// reshuffled itself after any update.
func airlineListQuery(db *gorm.DB, params listQuery, traveosSupported *bool) *gorm.DB {
	query := db.Model(&data.Airline{})

	if params.Q != "" {
		like := likePattern(params.Q)
		search := db.Where("airlines.name ILIKE ?", like)
		search = search.Or("airlines.code ILIKE ?", like)
		query = query.Where(search)
	}

	if traveosSupported != nil {
		query = query.Where("airlines.traveos_supported = ?", *traveosSupported)
	}

	return query.Order(orderClause(params.Order, airlineOrder, airlineOrder["name_asc"]))
}

// GetAirlines handles the GET /airlines route.
// @Summary      Listar companhias aéreas
// @Description  Retorna todas as companhias aéreas cadastradas no sistema.
// @Tags         Companhias Aéreas
// @Produce      json
// @Security     BearerAuth
// @Param        q      query  string  false  "Busca por nome ou código"
// @Param        order  query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        traveos_supported  query  bool  false  "Somente companhias com (true) ou sem (false) suporte na Traveos"
// @Success      200  {object}  schemas.AirlineListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar companhias aéreas"
// @Router       /airlines [get]
func (h *AirlineHandler) GetAirlines(w http.ResponseWriter, r *http.Request) {
	var airlines []data.Airline

	if err := airlineListQuery(h.DB, parseListQuery(r), parseOptionalBool(r.URL.Query().Get("traveos_supported"))).Find(&airlines).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch airlines")
		return
	}

	resp := make([]schemas.AirlineResponse, 0, len(airlines))
	for _, airline := range airlines {
		resp = append(resp, toAirlineResponse(airline))
	}

	utils.SendJSON(w, r, http.StatusOK, "Airlines fetched successfully", resp)
}

// GetAirline handles the GET /airlines/{id} route
// @Summary      Buscar companhia aérea por ID
// @Description  Retorna os dados de uma companhia aérea específica identificada pelo UUID.
// @Tags         Companhias Aéreas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da companhia aérea"
// @Success      200  {object}  schemas.AirlineSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Companhia aérea não encontrada"
// @Router       /airlines/{id} [get]
func (h *AirlineHandler) GetAirline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var airline data.Airline
	if err := h.DB.First(&airline, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Airline not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Airline fetched successfully", toAirlineResponse(airline))
}

// UpdateAirline handles the PUT /airlines/{id} route
// @Summary      Atualizar companhia aérea
// @Description  Atualiza os dados de uma companhia aérea existente. Os campos são opcionais (atualização parcial), com duas exceções: traveos_airline_id e brand_color são apagados quando enviados como null.
// @Tags         Companhias Aéreas
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                      true  "UUID da companhia aérea"
// @Param        payload body  schemas.UpdateAirlineRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.AirlineSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      404  {object}  schemas.ErrorResponse "Companhia aérea não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma companhia aérea com este código"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar companhia aérea"
// @Router       /airlines/{id} [put]
func (h *AirlineHandler) UpdateAirline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var airline data.Airline
	if err := h.DB.First(&airline, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Airline not found")
		return
	}

	var req schemas.UpdateAirlineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// This handler did not validate before brand_color existed, because every
	// field was a free-form optional. brand_color is different: it reaches the
	// ticket's stylesheet, so a malformed value has to be refused here rather
	// than relying on the template's escaping alone.
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	if req.Name != nil {
		airline.Name = *req.Name
	}
	if req.Code != nil {
		airline.Code = *req.Code
	}
	if req.TraveosSupported != nil {
		airline.TraveosSupported = *req.TraveosSupported
	}
	if req.RequiresLastName != nil {
		airline.RequiresLastName = *req.RequiresLastName
	}
	if req.RequiresPurchaseNumber != nil {
		airline.RequiresPurchaseNumber = *req.RequiresPurchaseNumber
	}
	// Assigned unconditionally, unlike the fields above: these two are the only
	// nullable ones, and the panel - the sole client of this route - always sends
	// both. Reading nil as "leave alone" here left no value at all capable of
	// undoing a Traveos id that was typed wrong, since 0 is refused by validation.
	airline.TraveosAirlineID = req.TraveosAirlineID
	airline.BrandColor = req.BrandColor

	if err := h.DB.Save(&airline).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe uma companhia aérea com este código")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update airline: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Airline updated successfully", toAirlineResponse(airline))
}

// DeleteAirline handles the DELETE /airlines/{id} route.
// @Summary      Deletar companhia aérea
// @Description  Remove uma companhia aérea do sistema. Falha com 422 se estiver vinculada a um programa de fidelidade ou emissão.
// @Tags         Companhias Aéreas
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da companhia aérea"
// @Success      200  {object}  schemas.ErrorResponse "Companhia aérea deletada com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      422  {object}  schemas.ErrorResponse "Companhia aérea vinculada a um programa de fidelidade ou emissão"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar companhia aérea"
// @Router       /airlines/{id} [delete]
func (h *AirlineHandler) DeleteAirline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.Delete(&data.Airline{}, "id = ?", id).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não é possível excluir companhia aérea vinculada a um programa de fidelidade ou emissão")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete airline: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Airline deleted successfully", nil)
}
