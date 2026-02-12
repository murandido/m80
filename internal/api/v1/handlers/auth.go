package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/render"
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

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req schemas.LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid data", "details": err.Error()})
		return
	}

	if !utils.ValidateBRPhone(req.Phone) {
		http.Error(w, "Invalid phone number", http.StatusBadRequest)
		return
	}

	var user data.User
	result := h.DB.Where("phone = ?", req.Phone).First(&user)

	if result.Error != nil {
		http.Error(w, "Phone or password incorrect", http.StatusUnauthorized)
		return
	}

	// checks the password
	if !user.CheckPassword(req.Password) {
		http.Error(w, "Phone or password incorrect", http.StatusUnauthorized)
		return
	}

	// generates token
	token, err := auth.GenerateToken(user.ID, user.Phone, user.Role)
	if err != nil {
		http.Error(w, "Error generating token", http.StatusInternalServerError)
		return
	}

	// success
	render.JSON(w, r, &schemas.LoginResponse{Token: token})
}
