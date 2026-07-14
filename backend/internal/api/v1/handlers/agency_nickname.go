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

type AgencyNicknameHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewAgencyNicknameHandler(db *gorm.DB) *AgencyNicknameHandler {
	return &AgencyNicknameHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toAgencyNicknameResponse(nickname data.AgencyNickname) schemas.AgencyNicknameResponse {
	return schemas.AgencyNicknameResponse{
		ID:        nickname.ID,
		AgencyID:  nickname.AgencyID,
		Nickname:  nickname.Nickname,
		CreatedAt: nickname.CreatedAt,
	}
}

// agencyExists reports whether an agency with the given ID exists.
func (h *AgencyNicknameHandler) agencyExists(agencyID string) bool {
	return h.DB.First(&data.Agency{}, "id = ?", agencyID).Error == nil
}

// CreateAgencyNickname handles the POST /agencies/{id}/nicknames route
// @Summary      Criar apelido de agência
// @Description  Cria um novo apelido para uma agência existente.
// @Tags         Agências
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                             true  "UUID da agência"
// @Param        payload  body  schemas.CreateAgencyNicknameRequest true  "Dados do novo apelido"
// @Success      201  {object}  schemas.AgencyNicknameResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Agência não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Apelido já está em uso"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar apelido"
// @Router       /agencies/{id}/nicknames [post]
func (h *AgencyNicknameHandler) CreateAgencyNickname(w http.ResponseWriter, r *http.Request) {
	agencyID := chi.URLParam(r, "id")

	if !h.agencyExists(agencyID) {
		utils.SendError(w, r, http.StatusNotFound, "Agency not found")
		return
	}

	var req schemas.CreateAgencyNicknameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	nickname := data.AgencyNickname{
		AgencyID: agencyID,
		Nickname: req.Nickname,
	}

	if err := h.DB.Create(&nickname).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Apelido já está em uso")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create nickname: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Nickname created successfully", toAgencyNicknameResponse(nickname))
}

// CheckAgencyNicknameAvailability handles the GET /agencies/nicknames/availability route.
// @Summary      Verificar disponibilidade de apelido de agência
// @Description  Verifica se um apelido de agência já está em uso.
// @Tags         Agências
// @Produce      json
// @Security     BearerAuth
// @Param        nickname  query  string  true  "Apelido a verificar"
// @Success      200  {object}  schemas.NicknameAvailabilityResponse
// @Failure      400  {object}  schemas.ErrorResponse "nickname query param ausente"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao verificar disponibilidade"
// @Router       /agencies/nicknames/availability [get]
func (h *AgencyNicknameHandler) CheckAgencyNicknameAvailability(w http.ResponseWriter, r *http.Request) {
	nickname := r.URL.Query().Get("nickname")
	if nickname == "" {
		utils.SendError(w, r, http.StatusBadRequest, "nickname query param is required")
		return
	}

	var count int64
	if err := h.DB.Model(&data.AgencyNickname{}).Where("nickname = ?", nickname).Count(&count).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to check nickname availability")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Disponibilidade verificada", schemas.NicknameAvailabilityResponse{Available: count == 0})
}

// GetAgencyNicknames handles the GET /agencies/{id}/nicknames route.
// @Summary      Listar apelidos de uma agência
// @Description  Retorna todos os apelidos cadastrados para uma agência.
// @Tags         Agências
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da agência"
// @Success      200  {array}   schemas.AgencyNicknameResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Agência não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar apelidos"
// @Router       /agencies/{id}/nicknames [get]
func (h *AgencyNicknameHandler) GetAgencyNicknames(w http.ResponseWriter, r *http.Request) {
	agencyID := chi.URLParam(r, "id")

	if !h.agencyExists(agencyID) {
		utils.SendError(w, r, http.StatusNotFound, "Agency not found")
		return
	}

	var nicknames []data.AgencyNickname
	if err := h.DB.Where("agency_id = ?", agencyID).Find(&nicknames).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch nicknames")
		return
	}

	resp := make([]schemas.AgencyNicknameResponse, 0, len(nicknames))
	for _, nickname := range nicknames {
		resp = append(resp, toAgencyNicknameResponse(nickname))
	}

	utils.SendJSON(w, r, http.StatusOK, "Nicknames fetched successfully", resp)
}

// GetAgencyNickname handles the GET /agencies/{id}/nicknames/{nicknameID} route
// @Summary      Buscar apelido de agência por ID
// @Description  Retorna os dados de um apelido específico de uma agência.
// @Tags         Agências
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  string  true  "UUID da agência"
// @Param        nicknameID  path  string  true  "UUID do apelido"
// @Success      200  {object}  schemas.AgencyNicknameResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Apelido não encontrado"
// @Router       /agencies/{id}/nicknames/{nicknameID} [get]
func (h *AgencyNicknameHandler) GetAgencyNickname(w http.ResponseWriter, r *http.Request) {
	agencyID := chi.URLParam(r, "id")
	nicknameID := chi.URLParam(r, "nicknameID")

	var nickname data.AgencyNickname
	if err := h.DB.First(&nickname, "id = ? AND agency_id = ?", nicknameID, agencyID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Nickname not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Nickname fetched successfully", toAgencyNicknameResponse(nickname))
}

// UpdateAgencyNickname handles the PUT /agencies/{id}/nicknames/{nicknameID} route
// @Summary      Atualizar apelido de agência
// @Description  Atualiza o apelido de uma agência existente.
// @Tags         Agências
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  string                             true  "UUID da agência"
// @Param        nicknameID  path  string                             true  "UUID do apelido"
// @Param        payload     body  schemas.UpdateAgencyNicknameRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.AgencyNicknameResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Apelido não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Apelido já está em uso"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar apelido"
// @Router       /agencies/{id}/nicknames/{nicknameID} [put]
func (h *AgencyNicknameHandler) UpdateAgencyNickname(w http.ResponseWriter, r *http.Request) {
	agencyID := chi.URLParam(r, "id")
	nicknameID := chi.URLParam(r, "nicknameID")

	var nickname data.AgencyNickname
	if err := h.DB.First(&nickname, "id = ? AND agency_id = ?", nicknameID, agencyID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Nickname not found")
		return
	}

	var req schemas.UpdateAgencyNicknameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Nickname != nil {
		nickname.Nickname = *req.Nickname
	}

	if err := h.DB.Save(&nickname).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Apelido já está em uso")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update nickname: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Nickname updated successfully", toAgencyNicknameResponse(nickname))
}

// DeleteAgencyNickname handles the DELETE /agencies/{id}/nicknames/{nicknameID} route.
// @Summary      Deletar apelido de agência
// @Description  Remove um apelido de uma agência.
// @Tags         Agências
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  string  true  "UUID da agência"
// @Param        nicknameID  path  string  true  "UUID do apelido"
// @Success      200  {object}  schemas.ErrorResponse "Apelido deletado com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Apelido não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar apelido"
// @Router       /agencies/{id}/nicknames/{nicknameID} [delete]
func (h *AgencyNicknameHandler) DeleteAgencyNickname(w http.ResponseWriter, r *http.Request) {
	agencyID := chi.URLParam(r, "id")
	nicknameID := chi.URLParam(r, "nicknameID")

	var nickname data.AgencyNickname
	if err := h.DB.First(&nickname, "id = ? AND agency_id = ?", nicknameID, agencyID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Nickname not found")
		return
	}

	if err := h.DB.Delete(&nickname).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete nickname: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Nickname deleted successfully", nil)
}
