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

type LoyaltyProgramHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewLoyaltyProgramHandler(db *gorm.DB) *LoyaltyProgramHandler {
	return &LoyaltyProgramHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toLoyaltyProgramResponse(loyaltyProgram data.LoyaltyProgram) schemas.LoyaltyProgramResponse {
	var defaultAirline *schemas.AirlineResponse
	if loyaltyProgram.DefaultAirline != nil {
		resp := toAirlineResponse(*loyaltyProgram.DefaultAirline)
		defaultAirline = &resp
	}

	return schemas.LoyaltyProgramResponse{
		ID:               loyaltyProgram.ID,
		Name:             loyaltyProgram.Name,
		Code:             loyaltyProgram.Code,
		Color:            loyaltyProgram.Color,
		DefaultAirlineID: loyaltyProgram.DefaultAirlineID,
		DefaultAirline:   defaultAirline,
		CreatedAt:        loyaltyProgram.CreatedAt,
		UpdatedAt:        loyaltyProgram.UpdatedAt,
		DeletedAt:        archivedAt(loyaltyProgram.DeletedAt),
	}
}

// CreateLoyaltyProgram handles the POST /loyalty-programs route
// @Summary      Criar programa de fidelidade
// @Description  Cria um novo programa de fidelidade no sistema.
// @Tags         Programas de Fidelidade
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateLoyaltyProgramRequest true "Dados do novo programa de fidelidade"
// @Success      201  {object}  schemas.LoyaltyProgramSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Código duplicado ou companhia aérea padrão inexistente"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar programa de fidelidade"
// @Router       /loyalty-programs [post]
func (h *LoyaltyProgramHandler) CreateLoyaltyProgram(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateLoyaltyProgramRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	loyaltyProgram := data.LoyaltyProgram{
		Name:             req.Name,
		Code:             req.Code,
		DefaultAirlineID: req.DefaultAirlineID,
	}

	if req.Color != nil {
		loyaltyProgram.Color = *req.Color
	}

	if err := h.DB.Create(&loyaltyProgram).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == postgresUniqueViolation {
				utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe um programa de fidelidade com este código")
				return
			}
			if pgErr.Code == postgresForeignKeyViolation {
				utils.SendError(w, r, http.StatusUnprocessableEntity, "Companhia aérea padrão informada não existe")
				return
			}
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create loyalty program: "+err.Error())
		return
	}

	if err := h.DB.Preload("DefaultAirline").First(&loyaltyProgram, "id = ?", loyaltyProgram.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load created loyalty program: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Loyalty program created successfully", toLoyaltyProgramResponse(loyaltyProgram))
}

// GetLoyaltyPrograms handles the GET /loyalty-programs route.
// @Summary      Listar programas de fidelidade
// @Description  Retorna todos os programas de fidelidade cadastrados no sistema.
// @Tags         Programas de Fidelidade
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.LoyaltyProgramListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar programas de fidelidade"
// @Router       /loyalty-programs [get]
func (h *LoyaltyProgramHandler) GetLoyaltyPrograms(w http.ResponseWriter, r *http.Request) {
	var loyaltyPrograms []data.LoyaltyProgram

	if err := h.DB.Preload("DefaultAirline").Find(&loyaltyPrograms).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch loyalty programs")
		return
	}

	resp := make([]schemas.LoyaltyProgramResponse, 0, len(loyaltyPrograms))
	for _, loyaltyProgram := range loyaltyPrograms {
		resp = append(resp, toLoyaltyProgramResponse(loyaltyProgram))
	}

	utils.SendJSON(w, r, http.StatusOK, "Loyalty programs fetched successfully", resp)
}

// GetLoyaltyProgram handles the GET /loyalty-programs/{id} route
// @Summary      Buscar programa de fidelidade por ID
// @Description  Retorna os dados de um programa de fidelidade específico identificado pelo UUID.
// @Tags         Programas de Fidelidade
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do programa de fidelidade"
// @Success      200  {object}  schemas.LoyaltyProgramSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Programa de fidelidade não encontrado"
// @Router       /loyalty-programs/{id} [get]
func (h *LoyaltyProgramHandler) GetLoyaltyProgram(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var loyaltyProgram data.LoyaltyProgram
	if err := h.DB.Preload("DefaultAirline").First(&loyaltyProgram, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Loyalty program not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Loyalty program fetched successfully", toLoyaltyProgramResponse(loyaltyProgram))
}

// UpdateLoyaltyProgram handles the PUT /loyalty-programs/{id} route
// @Summary      Atualizar programa de fidelidade
// @Description  Atualiza os dados de um programa de fidelidade existente. Todos os campos são opcionais (atualização parcial).
// @Tags         Programas de Fidelidade
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                             true  "UUID do programa de fidelidade"
// @Param        payload body  schemas.UpdateLoyaltyProgramRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.LoyaltyProgramSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Programa de fidelidade não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Código duplicado ou companhia aérea padrão inexistente"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar programa de fidelidade"
// @Router       /loyalty-programs/{id} [put]
func (h *LoyaltyProgramHandler) UpdateLoyaltyProgram(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var loyaltyProgram data.LoyaltyProgram
	if err := h.DB.First(&loyaltyProgram, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Loyalty program not found")
		return
	}

	var req schemas.UpdateLoyaltyProgramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name != nil {
		loyaltyProgram.Name = *req.Name
	}
	if req.Code != nil {
		loyaltyProgram.Code = *req.Code
	}
	if req.Color != nil {
		loyaltyProgram.Color = *req.Color
	}
	if req.DefaultAirlineID != nil {
		loyaltyProgram.DefaultAirlineID = *req.DefaultAirlineID
	}

	if err := h.DB.Save(&loyaltyProgram).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == postgresUniqueViolation {
				utils.SendError(w, r, http.StatusUnprocessableEntity, "Já existe um programa de fidelidade com este código")
				return
			}
			if pgErr.Code == postgresForeignKeyViolation {
				utils.SendError(w, r, http.StatusUnprocessableEntity, "Companhia aérea padrão informada não existe")
				return
			}
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update loyalty program: "+err.Error())
		return
	}

	if err := h.DB.Preload("DefaultAirline").First(&loyaltyProgram, "id = ?", loyaltyProgram.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to load updated loyalty program: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Loyalty program updated successfully", toLoyaltyProgramResponse(loyaltyProgram))
}

// DeleteLoyaltyProgram handles the DELETE /loyalty-programs/{id} route.
// @Summary      Deletar programa de fidelidade
// @Description  Arquiva um programa de fidelidade: ele some das listagens e dos seletores, o código volta a ficar disponível, e as contas e emissões antigas continuam mostrando a que programa pertencem.
// @Tags         Programas de Fidelidade
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do programa de fidelidade"
// @Success      200  {object}  schemas.ErrorResponse "Programa de fidelidade arquivado com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Programa de fidelidade ainda referenciado por outro registro"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao arquivar programa de fidelidade"
// @Router       /loyalty-programs/{id} [delete]
func (h *LoyaltyProgramHandler) DeleteLoyaltyProgram(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.Delete(&data.LoyaltyProgram{}, "id = ?", id).Error; err != nil {
		var pgErr *pgconn.PgError
		// Kept even though archiving no longer trips the issuance/account foreign
		// keys: default_airline_id and any future reference still can.
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não é possível excluir programa de fidelidade ainda referenciado por outro registro")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete loyalty program: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Loyalty program deleted successfully", nil)
}
