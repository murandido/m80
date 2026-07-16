package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type IssuancePassengerHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewIssuancePassengerHandler(db *gorm.DB) *IssuancePassengerHandler {
	return &IssuancePassengerHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toIssuancePassengerResponse(passenger data.IssuancePassenger) schemas.IssuancePassengerResponse {
	return schemas.IssuancePassengerResponse{
		ID:         passenger.ID,
		IssuanceID: passenger.IssuanceID,
		FirstName:  passenger.FirstName,
		LastName:   passenger.LastName,
		Type:       passenger.Type,
		Document:   passenger.Document,
		Position:   passenger.Position,
		CreatedAt:  passenger.CreatedAt,
	}
}

// issuanceExists reports whether an issuance with the given ID exists.
func (h *IssuancePassengerHandler) issuanceExists(issuanceID string) bool {
	return h.DB.First(&data.Issuance{}, "id = ?", issuanceID).Error == nil
}

// CreateIssuancePassenger handles the POST /issuances/{id}/passengers route
// @Summary      Criar passageiro de emissão
// @Description  Cria um novo passageiro para a emissão identificada pelo UUID.
// @Tags         Passageiros de Emissão
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                                 true  "UUID da emissão"
// @Param        payload  body  schemas.CreateIssuancePassengerRequest true  "Dados do novo passageiro"
// @Success      201  {object}  schemas.IssuancePassengerSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar passageiro"
// @Router       /issuances/{id}/passengers [post]
func (h *IssuancePassengerHandler) CreateIssuancePassenger(w http.ResponseWriter, r *http.Request) {
	issuanceID := chi.URLParam(r, "id")

	if !h.issuanceExists(issuanceID) {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return
	}

	var req schemas.CreateIssuancePassengerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	passengerType := "ADT"
	if req.Type != nil {
		passengerType = *req.Type
	}

	passenger := data.IssuancePassenger{
		IssuanceID: issuanceID,
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		Type:       passengerType,
		Document:   req.Document,
		Position:   req.Position,
	}

	if err := h.DB.Create(&passenger).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create passenger: "+err.Error())
		return
	}

	// Reload: BeforeSave leaves Document encrypted in memory, and only a fresh
	// read triggers AfterFind to decrypt it back for the response.
	if err := h.DB.First(&passenger, "id = ?", passenger.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load created passenger: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Passenger created successfully", toIssuancePassengerResponse(passenger))
}

// GetIssuancePassengers handles the GET /issuances/{id}/passengers route.
// @Summary      Listar passageiros de uma emissão
// @Description  Retorna todos os passageiros cadastrados para a emissão identificada pelo UUID, ordenados pela posição.
// @Tags         Passageiros de Emissão
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da emissão"
// @Success      200  {object}  schemas.IssuancePassengerListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Emissão não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar passageiros"
// @Router       /issuances/{id}/passengers [get]
func (h *IssuancePassengerHandler) GetIssuancePassengers(w http.ResponseWriter, r *http.Request) {
	issuanceID := chi.URLParam(r, "id")

	if !h.issuanceExists(issuanceID) {
		utils.SendError(w, r, http.StatusNotFound, "Issuance not found")
		return
	}

	var passengers []data.IssuancePassenger
	if err := h.DB.Where("issuance_id = ?", issuanceID).Order("position").Find(&passengers).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch passengers")
		return
	}

	resp := make([]schemas.IssuancePassengerResponse, 0, len(passengers))
	for _, passenger := range passengers {
		resp = append(resp, toIssuancePassengerResponse(passenger))
	}

	utils.SendJSON(w, r, http.StatusOK, "Passengers fetched successfully", resp)
}

// GetIssuancePassenger handles the GET /issuances/{id}/passengers/{passengerID} route
// @Summary      Buscar passageiro por ID
// @Description  Retorna os dados de um passageiro específico de uma emissão.
// @Tags         Passageiros de Emissão
// @Produce      json
// @Security     BearerAuth
// @Param        id           path  string  true  "UUID da emissão"
// @Param        passengerID  path  string  true  "UUID do passageiro"
// @Success      200  {object}  schemas.IssuancePassengerSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Passageiro não encontrado"
// @Router       /issuances/{id}/passengers/{passengerID} [get]
func (h *IssuancePassengerHandler) GetIssuancePassenger(w http.ResponseWriter, r *http.Request) {
	issuanceID := chi.URLParam(r, "id")
	passengerID := chi.URLParam(r, "passengerID")

	var passenger data.IssuancePassenger
	if err := h.DB.First(&passenger, "id = ? AND issuance_id = ?", passengerID, issuanceID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Passenger not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Passenger fetched successfully", toIssuancePassengerResponse(passenger))
}

// UpdateIssuancePassenger handles the PUT /issuances/{id}/passengers/{passengerID} route
// @Summary      Atualizar passageiro de emissão
// @Description  Atualiza os dados de um passageiro existente de uma emissão.
// @Tags         Passageiros de Emissão
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id           path  string                                 true  "UUID da emissão"
// @Param        passengerID  path  string                                 true  "UUID do passageiro"
// @Param        payload      body  schemas.UpdateIssuancePassengerRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.IssuancePassengerSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Passageiro não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar passageiro"
// @Router       /issuances/{id}/passengers/{passengerID} [put]
func (h *IssuancePassengerHandler) UpdateIssuancePassenger(w http.ResponseWriter, r *http.Request) {
	issuanceID := chi.URLParam(r, "id")
	passengerID := chi.URLParam(r, "passengerID")

	var passenger data.IssuancePassenger
	if err := h.DB.First(&passenger, "id = ? AND issuance_id = ?", passengerID, issuanceID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Passenger not found")
		return
	}

	var req schemas.UpdateIssuancePassengerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	if req.FirstName != nil {
		passenger.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		passenger.LastName = *req.LastName
	}
	if req.Type != nil {
		passenger.Type = *req.Type
	}
	if req.Document != nil {
		passenger.Document = req.Document
	}
	if req.Position != nil {
		passenger.Position = *req.Position
	}

	if err := h.DB.Save(&passenger).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update passenger: "+err.Error())
		return
	}

	// Reload: BeforeSave leaves Document encrypted in memory, and only a fresh
	// read triggers AfterFind to decrypt it back for the response.
	if err := h.DB.First(&passenger, "id = ?", passenger.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load updated passenger: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Passenger updated successfully", toIssuancePassengerResponse(passenger))
}

// DeleteIssuancePassenger handles the DELETE /issuances/{id}/passengers/{passengerID} route.
// @Summary      Deletar passageiro de emissão
// @Description  Remove um passageiro de uma emissão.
// @Tags         Passageiros de Emissão
// @Produce      json
// @Security     BearerAuth
// @Param        id           path  string  true  "UUID da emissão"
// @Param        passengerID  path  string  true  "UUID do passageiro"
// @Success      200  {object}  schemas.ErrorResponse "Passageiro deletado com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Passageiro não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar passageiro"
// @Router       /issuances/{id}/passengers/{passengerID} [delete]
func (h *IssuancePassengerHandler) DeleteIssuancePassenger(w http.ResponseWriter, r *http.Request) {
	issuanceID := chi.URLParam(r, "id")
	passengerID := chi.URLParam(r, "passengerID")

	var passenger data.IssuancePassenger
	if err := h.DB.First(&passenger, "id = ? AND issuance_id = ?", passengerID, issuanceID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Passenger not found")
		return
	}

	if err := h.DB.Delete(&passenger).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete passenger: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Passenger deleted successfully", nil)
}
