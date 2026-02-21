package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
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

// UpdateUser handles the PUT /users/{id} route
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Error parsing ID")
		return
	}
	fmt.Println(id)

	var user data.User
	if err := h.DB.First(&user, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "User not found")
		return
	}

	var req schemas.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name != "" {
		user.Name = req.Name
	}

	if req.Phone != "" {
		user.Phone = req.Phone
	}

	if req.Password != "" {
		user.Password = req.Password
	}

	if req.Role != "" {
		user.Role = req.Role
	}

	if err := h.DB.Save(&user).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update user: "+err.Error())
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

// DeleteUser handles the DELETE /users/{id} route.
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// soft delete
	// because data.User model embeds gorm.Model, gorm will not run a "DELETE FROM" query
	// instead, it runs an "UPDATE users SET deleted_at = NOW() WHERE id = ?"
	if err := h.DB.Delete(&data.User{}, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete user: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "User deleted successfully", nil)
}
