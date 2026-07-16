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

type ObservationPresetHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewObservationPresetHandler(db *gorm.DB) *ObservationPresetHandler {
	return &ObservationPresetHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toObservationPresetResponse(observationPreset data.ObservationPreset) schemas.ObservationPresetResponse {
	return schemas.ObservationPresetResponse{
		ID:        observationPreset.ID,
		Code:      observationPreset.Code,
		Text:      observationPreset.Text,
		CreatedAt: observationPreset.CreatedAt,
	}
}

// CreateObservationPreset handles the POST /observation-presets route
// @Summary      Criar observação
// @Description  Cria uma nova observação padrão no sistema.
// @Tags         Observações
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateObservationPresetRequest true "Dados da nova observação"
// @Success      201  {object}  schemas.ObservationPresetSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma observação com este código"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar observação"
// @Router       /observation-presets [post]
func (h *ObservationPresetHandler) CreateObservationPreset(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateObservationPresetRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	observationPreset := data.ObservationPreset{
		Code: req.Code,
		Text: req.Text,
	}

	if err := h.DB.Create(&observationPreset).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe uma observação com este código")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create observation preset: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Observation preset created successfully", toObservationPresetResponse(observationPreset))
}

// GetObservationPresets handles the GET /observation-presets route.
// @Summary      Listar observações
// @Description  Retorna todas as observações padrão cadastradas no sistema.
// @Tags         Observações
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.ObservationPresetListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar observações"
// @Router       /observation-presets [get]
func (h *ObservationPresetHandler) GetObservationPresets(w http.ResponseWriter, r *http.Request) {
	var observationPresets []data.ObservationPreset

	if err := h.DB.Find(&observationPresets).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch observation presets")
		return
	}

	resp := make([]schemas.ObservationPresetResponse, 0, len(observationPresets))
	for _, observationPreset := range observationPresets {
		resp = append(resp, toObservationPresetResponse(observationPreset))
	}

	utils.SendJSON(w, r, http.StatusOK, "Observation presets fetched successfully", resp)
}

// GetObservationPreset handles the GET /observation-presets/{id} route
// @Summary      Buscar observação por ID
// @Description  Retorna os dados de uma observação padrão específica identificada pelo UUID.
// @Tags         Observações
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da observação"
// @Success      200  {object}  schemas.ObservationPresetSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Observação não encontrada"
// @Router       /observation-presets/{id} [get]
func (h *ObservationPresetHandler) GetObservationPreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var observationPreset data.ObservationPreset
	if err := h.DB.First(&observationPreset, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Observation preset not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Observation preset fetched successfully", toObservationPresetResponse(observationPreset))
}

// UpdateObservationPreset handles the PUT /observation-presets/{id} route
// @Summary      Atualizar observação
// @Description  Atualiza os dados de uma observação padrão existente. Todos os campos são opcionais (atualização parcial).
// @Tags         Observações
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                                 true  "UUID da observação"
// @Param        payload body  schemas.UpdateObservationPresetRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.ObservationPresetSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Observação não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma observação com este código"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar observação"
// @Router       /observation-presets/{id} [put]
func (h *ObservationPresetHandler) UpdateObservationPreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var observationPreset data.ObservationPreset
	if err := h.DB.First(&observationPreset, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Observation preset not found")
		return
	}

	var req schemas.UpdateObservationPresetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Code != nil {
		observationPreset.Code = *req.Code
	}
	if req.Text != nil {
		observationPreset.Text = *req.Text
	}

	if err := h.DB.Save(&observationPreset).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe uma observação com este código")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update observation preset: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Observation preset updated successfully", toObservationPresetResponse(observationPreset))
}

// DeleteObservationPreset handles the DELETE /observation-presets/{id} route.
// @Summary      Deletar observação
// @Description  Remove uma observação padrão do sistema. Falha com 422 se estiver vinculada a outro recurso.
// @Tags         Observações
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da observação"
// @Success      200  {object}  schemas.ErrorResponse "Observação deletada com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Observação vinculada a outro recurso"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar observação"
// @Router       /observation-presets/{id} [delete]
func (h *ObservationPresetHandler) DeleteObservationPreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.Delete(&data.ObservationPreset{}, "id = ?", id).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não é possível excluir observação vinculada a outro recurso")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete observation preset: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Observation preset deleted successfully", nil)
}
