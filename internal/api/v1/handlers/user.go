package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type UserHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

// CreateUser handles the POST /users route
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateUserRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	user := data.User{
		Name:     req.Name,
		Phone:    req.Phone,
		Password: req.Password,
		Role:     req.Role,
	}

	if err := h.DB.Create(&user).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create user: "+err.Error())
		return
	}

	resp := schemas.UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Phone:     user.Phone,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}

	utils.SendJSON(w, r, http.StatusCreated, "User created successfully", resp)
}

// GetUsers handles the GET /users route.
func (h *UserHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	var users []data.User

	if err := h.DB.Find(&users).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch users")
		return
	}

	var resp []schemas.UserResponse
	for _, user := range users {
		resp = append(resp, schemas.UserResponse{
			ID:        user.ID,
			Name:      user.Name,
			Phone:     user.Phone,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
		})
	}

	utils.SendJSON(w, r, http.StatusOK, "Users fetched successfully", resp)
}
