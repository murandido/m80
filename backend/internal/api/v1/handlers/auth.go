package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"

	"github.com/murandido/m-80-backend/internal/auth"
	"github.com/murandido/m-80-backend/internal/data"
)

type AuthHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

// constructor
func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{DB: db, Validator: validator.New()}
}

// Login handles the http req to login an user.
// @Summary      Autenticação de usuário
// @Description  Recebe telefone e senha, valida no banco e retorna um token JWT.
// @Tags         Autenticação
// @Accept       json
// @Produce      json
// @Param        payload body schemas.LoginRequest true "Credenciais de login"
// @Success      200  {object}  schemas.AuthLoginResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido ou telefone inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Telefone ou senha incorretos"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao gerar token"
// @Router       /auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req schemas.LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// Normalized the same way users.phone is on create/update, so a login
	// typed without Brazil's optional mobile "nono dígito" still matches.
	req.Phone = utils.NormalizeBRPhone(req.Phone)
	if !utils.ValidateBRPhone(req.Phone) {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid phone number")
		return
	}

	var user data.User
	result := h.DB.Where("phone = ?", req.Phone).First(&user)

	if result.Error != nil {
		utils.SendError(w, r, http.StatusUnauthorized, "Phone or password incorrect")
		return
	}

	// checks the password
	if !user.CheckPassword(req.Password) {
		utils.SendError(w, r, http.StatusUnauthorized, "Phone or password incorrect")
		return
	}

	// generates token
	token, err := auth.GenerateToken(user.ID, user.Phone, user.Role)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Error generating token")
		return
	}

	// success
	responseData := map[string]string{
		"token": token,
	}
	utils.SendJSON(w, r, http.StatusOK, "Login successful", responseData)
}

// Me return the data of the authenticated user in the moment
// @Summary      Dados do usuário autenticado
// @Description  Retorna os dados do usuário cujo token JWT foi enviado no header Authorization.
// @Tags         Autenticação
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.AuthMeResponse
// @Failure      401  {object}  schemas.ErrorResponse "Token ausente ou inválido"
// @Failure      404  {object}  schemas.ErrorResponse "Usuário não encontrado"
// @Router       /auth/me [get]
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	// gets the data.User struct injected by the middleware
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	// get the user
	var fullUser data.User
	if err := h.DB.First(&fullUser, "id = ?", userCtx.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "User not found")
		return
	}

	// clear the password hash before send to the frontend
	fullUser.Password = ""

	grants, err := loadUserAgencyIDs(h.DB, []string{fullUser.ID})
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the user's agencies")
		return
	}
	agencyIDs := grants[fullUser.ID]
	if agencyIDs == nil {
		agencyIDs = []string{}
	}

	// Embedding keeps every field the panel already reads exactly where it was; agency_ids
	// is added alongside rather than by switching to schemas.UserResponse, whose shape differs.
	utils.SendJSON(w, r, http.StatusOK, "Data successfully recovered", struct {
		data.User
		AgencyIDs []string `json:"agency_ids"`
	}{fullUser, agencyIDs})
}

// UpdateMe lets the authenticated user change their own name, phone and password.
// @Summary      Atualizar os próprios dados
// @Description  Atualiza nome, telefone e/ou senha do usuário autenticado. Trocar telefone ou senha exige a senha atual. O perfil (role) não é alterável por esta rota.
// @Tags         Autenticação
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.UpdateMeRequest true "Campos a atualizar"
// @Success      200  {object}  schemas.UserSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload ou telefone inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Token ausente ou inválido"
// @Failure      403  {object}  schemas.ErrorResponse "Senha atual ausente ou incorreta"
// @Failure      422  {object}  schemas.ErrorResponse "Telefone já está em uso"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar usuário"
// @Router       /auth/me [put]
func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusUnauthorized, "Unauthorized or invalid context")
		return
	}

	var user data.User
	if err := h.DB.First(&user, "id = ?", userCtx.ID).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "User not found")
		return
	}

	var req schemas.UpdateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// Compared in canonical form, so retyping the same number with or without the
	// "nono dígito" is not mistaken for a phone change that demands the password.
	phoneChanged := false
	if req.Phone != "" {
		req.Phone = utils.NormalizeBRPhone(req.Phone)
		if !utils.ValidateBRPhone(req.Phone) {
			utils.SendError(w, r, http.StatusBadRequest, "Invalid phone number")
			return
		}
		phoneChanged = req.Phone != user.Phone
	}

	// The phone is both the login and how the bot recognizes the issuer, and a JWT
	// lives seven days with no revocation - so anything that could take the account
	// over from a session left open asks for the current password. 403, never 401:
	// the panel drops the session cookie on 401, and a typo is not a logout.
	if (phoneChanged || req.NewPassword != "") && !user.CheckPassword(req.CurrentPassword) {
		utils.SendError(w, r, http.StatusForbidden, "Senha atual incorreta")
		return
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if phoneChanged {
		user.Phone = req.Phone
	}
	if req.NewPassword != "" {
		// Hashed by data.User's BeforeUpdate hook.
		user.Password = req.NewPassword
	}

	if err := h.DB.Save(&user).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Telefone já está em uso")
			return
		}
		slog.Error("auth: failed to update own user", "user_id", user.ID, "err", err)
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update user")
		return
	}

	resp := schemas.UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Phone:     user.Phone,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}

	utils.SendJSON(w, r, http.StatusOK, "User updated successfully", resp)
}
