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

type PointBatchOriginHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewPointBatchOriginHandler(db *gorm.DB) *PointBatchOriginHandler {
	return &PointBatchOriginHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toPointBatchOriginResponse(origin data.PointBatchOrigin) schemas.PointBatchOriginResponse {
	return schemas.PointBatchOriginResponse{
		ID:        origin.ID,
		Name:      origin.Name,
		Active:    origin.Active,
		CreatedAt: origin.CreatedAt,
		UpdatedAt: origin.UpdatedAt,
	}
}

// CreatePointBatchOrigin handles the POST /point-batch-origins route
// @Summary      Criar origem de lote
// @Description  Cadastra uma nova origem de pontos (parceiro, banco, clube, compra direta).
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreatePointBatchOriginRequest true "Dados da nova origem"
// @Success      201  {object}  schemas.PointBatchOriginSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma origem com este nome"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar origem"
// @Router       /point-batch-origins [post]
func (h *PointBatchOriginHandler) CreatePointBatchOrigin(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreatePointBatchOriginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	origin := data.PointBatchOrigin{Name: req.Name, Active: true}

	if err := h.DB.Create(&origin).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe uma origem com este nome")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create point batch origin: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Origem criada com sucesso", toPointBatchOriginResponse(origin))
}

// GetPointBatchOrigins handles the GET /point-batch-origins route.
// @Summary      Listar origens de lote
// @Description  Retorna as origens cadastradas, as inativas por último. Use ?active=true para trazer só as ativas, que é o que o combobox de lote precisa.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        active query bool false "Somente origens ativas"
// @Success      200  {object}  schemas.PointBatchOriginListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao listar origens"
// @Router       /point-batch-origins [get]
func (h *PointBatchOriginHandler) GetPointBatchOrigins(w http.ResponseWriter, r *http.Request) {
	query := h.DB.Model(&data.PointBatchOrigin{})
	if r.URL.Query().Get("active") == "true" {
		query = query.Where("active = ?", true)
	}

	var origins []data.PointBatchOrigin
	// Inactive last, then by name: a retired origin still has to be findable on the
	// management screen, but it should never sit above a live one in the picker.
	if err := query.Order("active DESC").Order("name ASC").Find(&origins).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch point batch origins: "+err.Error())
		return
	}

	response := make([]schemas.PointBatchOriginResponse, 0, len(origins))
	for _, origin := range origins {
		response = append(response, toPointBatchOriginResponse(origin))
	}

	utils.SendJSON(w, r, http.StatusOK, "Origens listadas com sucesso", response)
}

// UpdatePointBatchOrigin handles the PUT /point-batch-origins/{id} route.
// @Summary      Atualizar origem de lote
// @Description  Renomeia uma origem ou a ativa/desativa. Desativar é o caminho para tirar do combobox uma origem já usada por algum lote.
// @Tags         Estoque de pontos
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                                  true  "ID da origem"
// @Param        payload body  schemas.UpdatePointBatchOriginRequest    true  "Campos a atualizar"
// @Success      200  {object}  schemas.PointBatchOriginSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Origem não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma origem com este nome"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar origem"
// @Router       /point-batch-origins/{id} [put]
func (h *PointBatchOriginHandler) UpdatePointBatchOrigin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var origin data.PointBatchOrigin
	if err := h.DB.First(&origin, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Origem não encontrada")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch point batch origin: "+err.Error())
		return
	}

	var req schemas.UpdatePointBatchOriginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Active != nil {
		updates["active"] = *req.Active
	}

	if len(updates) > 0 {
		if err := h.DB.Model(&origin).Updates(updates).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
				utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe uma origem com este nome")
				return
			}
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to update point batch origin: "+err.Error())
			return
		}
	}

	utils.SendJSON(w, r, http.StatusOK, "Origem atualizada com sucesso", toPointBatchOriginResponse(origin))
}

// DeletePointBatchOrigin handles the DELETE /point-batch-origins/{id} route.
//
// The origin has no soft delete, so this is a real delete and Postgres refuses it the
// moment any batch points at the row. That refusal is the useful answer rather than an
// obstacle: an origin that priced real batches has to stay readable on them, and the
// way to get it out of the picker is to deactivate it.
// @Summary      Excluir origem de lote
// @Description  Exclui uma origem nunca usada. Origem já referenciada por um lote não pode ser excluída — desative-a.
// @Tags         Estoque de pontos
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "ID da origem"
// @Success      200  {object}  schemas.ErrorResponse "Origem excluída com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Requer admin"
// @Failure      404  {object}  schemas.ErrorResponse "Origem não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Origem em uso por algum lote"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao excluir origem"
// @Router       /point-batch-origins/{id} [delete]
func (h *PointBatchOriginHandler) DeletePointBatchOrigin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var origin data.PointBatchOrigin
	if err := h.DB.First(&origin, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Origem não encontrada")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch point batch origin: "+err.Error())
		return
	}

	if err := h.DB.Delete(&origin).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity,
				"Esta origem já foi usada por algum lote. Desative-a para tirá-la da lista.")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete point batch origin: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Origem excluída com sucesso", nil)
}
