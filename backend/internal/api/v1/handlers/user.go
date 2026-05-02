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
// @Summary      Criar usuário
// @Description  Cria um novo usuário no sistema. Apenas administradores podem acessar.
// @Tags         Usuários
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateUserRequest true "Dados do novo usuário"
// @Success      201  {object}  schemas.UserSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar usuário"
// @Router       /users [post]
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
// @Summary      Listar usuários
// @Description  Retorna todos os usuários cadastrados no sistema (sem senhas).
// @Tags         Usuários
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  schemas.UserListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar usuários"
// @Router       /users [get]
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

// GetUser handles the GET /users/{id} route
// @Summary      Buscar usuário por ID
// @Description  Retorna os dados de um usuário específico identificado pelo UUID.
// @Tags         Usuários
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do usuário"
// @Success      200  {object}  schemas.UserSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      404  {object}  schemas.ErrorResponse "Usuário não encontrado"
// @Router       /users/{id} [get]
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var user data.User
	if err := h.DB.First(&user, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "User not found")
		return
	}

	resp := schemas.UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Phone:     user.Phone,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}

	utils.SendJSON(w, r, http.StatusOK, "User fetched successfully", resp)
}

// UpdateUser handles the PUT /users/{id} route
// @Summary      Atualizar usuário
// @Description  Atualiza os dados de um usuário existente. Todos os campos são opcionais (atualização parcial).
// @Tags         Usuários
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id      path  string                    true  "UUID do usuário"
// @Param        payload body  schemas.UpdateUserRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.UserSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      404  {object}  schemas.ErrorResponse "Usuário não encontrado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar usuário"
// @Router       /users/{id} [put]
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
// @Summary      Deletar usuário
// @Description  Remove um usuário do sistema via soft delete (marca deleted_at, não apaga fisicamente).
// @Tags         Usuários
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do usuário"
// @Success      200  {object}  schemas.ErrorResponse "Usuário deletado com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar usuário"
// @Router       /users/{id} [delete]
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
