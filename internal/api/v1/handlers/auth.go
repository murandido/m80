package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
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
// @Summary      User Auth
// @Description  Receives phone and password, checks in database and returns a JWT token.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body schemas.LoginRequest true "Login Credentials"
// @Success      200  {object}  schemas.LoginResponse
// @Failure      400  {object}  map[string]string "Invalid data"
// @Failure      400  {object}  map[string]string "Invalid phone number"
// @Failure      401  {object}  map[string]string "Phone or password incorrect"
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
