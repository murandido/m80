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

type AgencyHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewAgencyHandler(db *gorm.DB) *AgencyHandler {
	return &AgencyHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toAgencyResponse(agency data.Agency) schemas.AgencyResponse {
	nicknames := make([]schemas.AgencyNicknameResponse, 0, len(agency.Nicknames))
	for _, nickname := range agency.Nicknames {
		nicknames = append(nicknames, toAgencyNicknameResponse(nickname))
	}

	return schemas.AgencyResponse{
		ID:        agency.ID,
		Name:      agency.Name,
		IsDefault: agency.IsDefault,
		Nicknames: nicknames,
		CreatedAt: agency.CreatedAt,
		UpdatedAt: agency.UpdatedAt,
	}
}

// CreateAgency handles the POST /agencies route
// @Summary      Criar agência
// @Description  Cria uma nova agência no sistema. Se is_default=true, a agência padrão atual é automaticamente desmarcada.
// @Tags         Agências
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateAgencyRequest true "Dados da nova agência"
// @Success      201  {object}  schemas.AgencySingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma agência padrão definida"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar agência"
// @Router       /agencies [post]
func (h *AgencyHandler) CreateAgency(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateAgencyRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	agency := data.Agency{
		Name: req.Name,
	}

	if req.IsDefault != nil {
		agency.IsDefault = *req.IsDefault
	}

	var err error
	if agency.IsDefault {
		err = h.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&data.Agency{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
			return tx.Create(&agency).Error
		})
	} else {
		err = h.DB.Create(&agency).Error
	}

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe uma agência padrão definida")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create agency: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Agency created successfully", toAgencyResponse(agency))
}

// agencyOrder is the whitelist of sort keys; the client's string only ever indexes it.
var agencyOrder = map[string]string{
	"name_asc":     "agencies.name ASC",
	"name_desc":    "agencies.name DESC",
	"created_desc": "agencies.created_at DESC",
	"created_asc":  "agencies.created_at ASC",
}

// agencyListQuery applies the optional search and the ordering. The ordering is not
// optional: without it Postgres was free to return rows in heap order, so the list
// reshuffled itself after any update.
func agencyListQuery(db *gorm.DB, params listQuery) *gorm.DB {
	query := db.Model(&data.Agency{})

	if params.Q != "" {
		like := likePattern(params.Q)
		search := db.Where("agencies.name ILIKE ?", like)
		search = search.Or(nicknameMatch("agency_nicknames", "agency_id", "agencies", false), like)
		query = query.Where(search)
	}

	return query.Order(orderClause(params.Order, agencyOrder, agencyOrder["name_asc"]))
}

// GetAgencies handles the GET /agencies route.
// @Summary      Listar agências
// @Description  Retorna todas as agências cadastradas no sistema.
// @Tags         Agências
// @Produce      json
// @Security     BearerAuth
// @Param        q      query  string  false  "Busca por nome ou apelido"
// @Param        order  query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Success      200  {object}  schemas.AgencyListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar agências"
// @Router       /agencies [get]
func (h *AgencyHandler) GetAgencies(w http.ResponseWriter, r *http.Request) {
	var agencies []data.Agency

	if err := agencyListQuery(h.DB, parseListQuery(r)).Preload("Nicknames").Find(&agencies).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch agencies")
		return
	}

	resp := make([]schemas.AgencyResponse, 0, len(agencies))
	for _, agency := range agencies {
		resp = append(resp, toAgencyResponse(agency))
	}

	utils.SendJSON(w, r, http.StatusOK, "Agencies fetched successfully", resp)
}

// GetAgency handles the GET /agencies/{id} route
// @Summary      Buscar agência por ID
// @Description  Retorna os dados de uma agência específica identificada pelo UUID.
// @Tags         Agências
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da agência"
// @Success      200  {object}  schemas.AgencySingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Agência não encontrada"
// @Router       /agencies/{id} [get]
func (h *AgencyHandler) GetAgency(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var agency data.Agency
	if err := h.DB.Preload("Nicknames").First(&agency, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Agency not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Agency fetched successfully", toAgencyResponse(agency))
}

// UpdateAgency handles the PUT /agencies/{id} route
// @Summary      Atualizar agência
// @Description  Atualiza os dados de uma agência existente. Todos os campos são opcionais (atualização parcial). Se is_default=true, a agência padrão atual é automaticamente desmarcada.
// @Tags         Agências
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                     true  "UUID da agência"
// @Param        payload body  schemas.UpdateAgencyRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.AgencySingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Agência não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma agência padrão definida"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar agência"
// @Router       /agencies/{id} [put]
func (h *AgencyHandler) UpdateAgency(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var agency data.Agency
	if err := h.DB.Preload("Nicknames").First(&agency, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Agency not found")
		return
	}

	var req schemas.UpdateAgencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name != nil {
		agency.Name = *req.Name
	}

	makeDefault := req.IsDefault != nil && *req.IsDefault && !agency.IsDefault
	if req.IsDefault != nil {
		agency.IsDefault = *req.IsDefault
	}

	var err error
	if makeDefault {
		err = h.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&data.Agency{}).Where("is_default = ? AND id != ?", true, agency.ID).Update("is_default", false).Error; err != nil {
				return err
			}
			return tx.Save(&agency).Error
		})
	} else {
		err = h.DB.Save(&agency).Error
	}

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe uma agência padrão definida")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update agency: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Agency updated successfully", toAgencyResponse(agency))
}

// DeleteAgency handles the DELETE /agencies/{id} route.
// @Summary      Deletar agência
// @Description  Remove uma agência do sistema. Falha com 422 se estiver vinculada a uma emissão.
// @Tags         Agências
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da agência"
// @Success      200  {object}  schemas.ErrorResponse "Agência deletada com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Agência vinculada a uma emissão"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar agência"
// @Router       /agencies/{id} [delete]
func (h *AgencyHandler) DeleteAgency(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.Delete(&data.Agency{}, "id = ?", id).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não é possível excluir agência vinculada a uma emissão")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete agency: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Agency deleted successfully", nil)
}
