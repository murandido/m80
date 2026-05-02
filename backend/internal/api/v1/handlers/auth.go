package handlers

import (
	"encoding/json"
	"net/http"

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

	utils.SendJSON(w, r, http.StatusOK, "Data successfully recovered", fullUser)
}
