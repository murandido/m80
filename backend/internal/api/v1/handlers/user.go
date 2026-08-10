package handlers

import (
	"encoding/json"
	"net/http"
	"slices"

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

// userOrder is the whitelist of sort keys; the client's string only ever indexes it.
var userOrder = map[string]string{
	"name_asc":     textOrder("users.name", "ASC"),
	"name_desc":    textOrder("users.name", "DESC"),
	"created_desc": "users.created_at DESC",
	"created_asc":  "users.created_at ASC",
}

// userListQuery applies the optional search and the ordering. The ordering is not
// optional: without it Postgres was free to return rows in heap order, so the list
// reshuffled itself after any update.
func userListQuery(db *gorm.DB, params listQuery, role string) *gorm.DB {
	query := db.Model(&data.User{})

	if params.Q != "" {
		like := likePattern(params.Q)
		search := db.Where("users.name ILIKE ?", like)
		search = search.Or("users.phone ILIKE ?", like)
		query = query.Where(search)
	}

	// Only the roles the enum actually has; anything else leaves the list unfiltered
	// rather than returning nothing for a typo in the URL.
	if slices.Contains(data.UserRoles, role) {
		query = query.Where("users.role = ?", role)
	}

	return query.Order(orderClause(params.Order, userOrder, userOrder["name_asc"]))
}

// GetUsers handles the GET /users route.
// @Summary      Listar usuários
// @Description  Retorna todos os usuários cadastrados no sistema (sem senhas). Acessível a qualquer usuário autenticado (ex: para popular o combobox de "emissor" em outras telas).
// @Tags         Usuários
// @Produce      json
// @Security     BearerAuth
// @Param        q      query  string  false  "Busca por nome ou telefone"
// @Param        order  query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        role   query  string  false  "Perfil"  Enums(admin, user, dev)
// @Success      200  {object}  schemas.UserListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar usuários"
// @Router       /users [get]
func (h *UserHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	var users []data.User

	if err := userListQuery(h.DB, parseListQuery(r), r.URL.Query().Get("role")).Find(&users).Error; err != nil {
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

	// Without this the oneof/min tags on UpdateUserRequest were dead letters: an invalid
	// role reached the Postgres enum and came back as a 500 carrying the raw driver
	// message. Every tag here is omitempty, so this only rejects payloads that were
	// already broken.
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
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
