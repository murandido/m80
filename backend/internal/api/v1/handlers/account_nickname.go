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

type AccountNicknameHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewAccountNicknameHandler(db *gorm.DB) *AccountNicknameHandler {
	return &AccountNicknameHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toAccountNicknameResponse(nickname data.AccountNickname) schemas.AccountNicknameResponse {
	return schemas.AccountNicknameResponse{
		ID:               nickname.ID,
		AccountID:        nickname.AccountID,
		LoyaltyProgramID: nickname.LoyaltyProgramID,
		Nickname:         nickname.Nickname,
		CreatedAt:        nickname.CreatedAt,
	}
}

// accountExists reports whether an account with the given ID exists.
func (h *AccountNicknameHandler) accountExists(accountID string) bool {
	return h.DB.First(&data.Account{}, "id = ?", accountID).Error == nil
}

// CreateAccountNickname handles the POST /accounts/{id}/nicknames route
// @Summary      Criar apelido de conta
// @Description  Cria um novo apelido para a conta identificada pelo UUID.
// @Tags         Apelidos de Conta
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                              true  "UUID da conta"
// @Param        payload  body  schemas.CreateAccountNicknameRequest true  "Dados do novo apelido"
// @Success      201  {object}  schemas.AccountNicknameSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Apelido já está em uso neste programa"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar apelido"
// @Router       /accounts/{id}/nicknames [post]
func (h *AccountNicknameHandler) CreateAccountNickname(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")

	if !h.accountExists(accountID) {
		utils.SendError(w, r, http.StatusNotFound, "Account not found")
		return
	}

	var req schemas.CreateAccountNicknameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	nickname := data.AccountNickname{
		AccountID:        accountID,
		LoyaltyProgramID: req.LoyaltyProgramID,
		Nickname:         req.Nickname,
	}

	if err := h.DB.Create(&nickname).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Apelido já está em uso neste programa")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create nickname: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Nickname created successfully", toAccountNicknameResponse(nickname))
}

// CheckAccountNicknameAvailability handles the GET /accounts/nicknames/availability route.
// @Summary      Verificar disponibilidade de apelido de conta
// @Description  Verifica se um apelido já está em uso dentro de um programa de fidelidade.
// @Tags         Apelidos de Conta
// @Produce      json
// @Security     BearerAuth
// @Param        nickname            query  string  true  "Apelido a verificar"
// @Param        loyalty_program_id  query  string  true  "UUID do programa de fidelidade"
// @Success      200  {object}  schemas.NicknameAvailabilityCheckResponse
// @Failure      400  {object}  schemas.ErrorResponse "Parâmetro ausente"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao verificar disponibilidade"
// @Router       /accounts/nicknames/availability [get]
func (h *AccountNicknameHandler) CheckAccountNicknameAvailability(w http.ResponseWriter, r *http.Request) {
	nickname := r.URL.Query().Get("nickname")
	loyaltyProgramID := r.URL.Query().Get("loyalty_program_id")

	if nickname == "" || loyaltyProgramID == "" {
		utils.SendError(w, r, http.StatusBadRequest, "nickname and loyalty_program_id query params are required")
		return
	}

	var count int64
	if err := h.DB.Model(&data.AccountNickname{}).
		Where("loyalty_program_id = ? AND nickname = ?", loyaltyProgramID, nickname).
		Count(&count).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to check nickname availability")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Disponibilidade verificada", schemas.NicknameAvailabilityResponse{Available: count == 0})
}

// GetAccountNicknames handles the GET /accounts/{id}/nicknames route.
// @Summary      Listar apelidos de uma conta
// @Description  Retorna todos os apelidos cadastrados para a conta identificada pelo UUID.
// @Tags         Apelidos de Conta
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID da conta"
// @Success      200  {object}  schemas.AccountNicknameListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Conta não encontrada"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar apelidos"
// @Router       /accounts/{id}/nicknames [get]
func (h *AccountNicknameHandler) GetAccountNicknames(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")

	if !h.accountExists(accountID) {
		utils.SendError(w, r, http.StatusNotFound, "Account not found")
		return
	}

	var nicknames []data.AccountNickname
	if err := h.DB.Where("account_id = ?", accountID).Find(&nicknames).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch nicknames")
		return
	}

	resp := make([]schemas.AccountNicknameResponse, 0, len(nicknames))
	for _, nickname := range nicknames {
		resp = append(resp, toAccountNicknameResponse(nickname))
	}

	utils.SendJSON(w, r, http.StatusOK, "Nicknames fetched successfully", resp)
}

// GetAccountNickname handles the GET /accounts/{id}/nicknames/{nicknameID} route
// @Summary      Buscar apelido por ID
// @Description  Retorna os dados de um apelido específico de uma conta.
// @Tags         Apelidos de Conta
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  string  true  "UUID da conta"
// @Param        nicknameID  path  string  true  "UUID do apelido"
// @Success      200  {object}  schemas.AccountNicknameSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Apelido não encontrado"
// @Router       /accounts/{id}/nicknames/{nicknameID} [get]
func (h *AccountNicknameHandler) GetAccountNickname(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	nicknameID := chi.URLParam(r, "nicknameID")

	var nickname data.AccountNickname
	if err := h.DB.First(&nickname, "id = ? AND account_id = ?", nicknameID, accountID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Nickname not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Nickname fetched successfully", toAccountNicknameResponse(nickname))
}

// UpdateAccountNickname handles the PUT /accounts/{id}/nicknames/{nicknameID} route
// @Summary      Atualizar apelido de conta
// @Description  Atualiza o apelido de uma conta existente.
// @Tags         Apelidos de Conta
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  string                              true  "UUID da conta"
// @Param        nicknameID  path  string                              true  "UUID do apelido"
// @Param        payload     body  schemas.UpdateAccountNicknameRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.AccountNicknameSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Apelido não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Apelido já está em uso neste programa"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar apelido"
// @Router       /accounts/{id}/nicknames/{nicknameID} [put]
func (h *AccountNicknameHandler) UpdateAccountNickname(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	nicknameID := chi.URLParam(r, "nicknameID")

	var nickname data.AccountNickname
	if err := h.DB.First(&nickname, "id = ? AND account_id = ?", nicknameID, accountID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Nickname not found")
		return
	}

	var req schemas.UpdateAccountNicknameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.LoyaltyProgramID != nil {
		nickname.LoyaltyProgramID = *req.LoyaltyProgramID
	}
	if req.Nickname != nil {
		nickname.Nickname = *req.Nickname
	}

	if err := h.DB.Save(&nickname).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Apelido já está em uso neste programa")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update nickname: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Nickname updated successfully", toAccountNicknameResponse(nickname))
}

// DeleteAccountNickname handles the DELETE /accounts/{id}/nicknames/{nicknameID} route.
// @Summary      Deletar apelido de conta
// @Description  Remove um apelido de uma conta.
// @Tags         Apelidos de Conta
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  string  true  "UUID da conta"
// @Param        nicknameID  path  string  true  "UUID do apelido"
// @Success      200  {object}  schemas.ErrorResponse "Apelido deletado com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Apelido não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar apelido"
// @Router       /accounts/{id}/nicknames/{nicknameID} [delete]
func (h *AccountNicknameHandler) DeleteAccountNickname(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	nicknameID := chi.URLParam(r, "nicknameID")

	var nickname data.AccountNickname
	if err := h.DB.First(&nickname, "id = ? AND account_id = ?", nicknameID, accountID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Nickname not found")
		return
	}

	if err := h.DB.Delete(&nickname).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete nickname: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Nickname deleted successfully", nil)
}
