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

type ClientHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewClientHandler(db *gorm.DB) *ClientHandler {
	return &ClientHandler{
		DB:        db,
		Validator: validator.New(),
	}
}

func toClientResponse(client data.Client) schemas.ClientResponse {
	return schemas.ClientResponse{
		ID:        client.ID,
		Name:      client.Name,
		Phone:     client.Phone,
		CreatedAt: client.CreatedAt,
		UpdatedAt: client.UpdatedAt,
	}
}

// CreateClient handles the POST /clients route
// @Summary      Criar cliente
// @Description  Cria um novo cliente no sistema.
// @Tags         Clientes
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateClientRequest true "Dados do novo cliente"
// @Success      201  {object}  schemas.ClientSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      422  {object}  schemas.ErrorResponse "Nome já está em uso"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar cliente"
// @Router       /clients [post]
func (h *ClientHandler) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateClientRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	client := data.Client{
		Name:  req.Name,
		Phone: req.Phone,
	}

	if err := h.DB.Create(&client).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Client name already in use")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create client: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Client created successfully", toClientResponse(client))
}

// ListClients handles the GET /clients route.
// @Summary      Listar clientes
// @Description  Retorna todos os clientes cadastrados no sistema. Aceita filtro opcional por nome.
// @Tags         Clientes
// @Produce      json
// @Security     BearerAuth
// @Param        search  query  string  false  "Filtra clientes cujo nome contém o texto informado"
// @Success      200  {object}  schemas.ClientListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar clientes"
// @Router       /clients [get]
func (h *ClientHandler) ListClients(w http.ResponseWriter, r *http.Request) {
	query := h.DB

	if search := r.URL.Query().Get("search"); search != "" {
		query = query.Where(unaccentedLike("name"), likePattern(search))
	}

	var clients []data.Client
	if err := query.Find(&clients).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch clients")
		return
	}

	resp := make([]schemas.ClientResponse, 0, len(clients))
	for _, client := range clients {
		resp = append(resp, toClientResponse(client))
	}

	utils.SendJSON(w, r, http.StatusOK, "Clients fetched successfully", resp)
}

// GetClient handles the GET /clients/{id} route
// @Summary      Buscar cliente por ID
// @Description  Retorna os dados de um cliente específico identificado pelo UUID.
// @Tags         Clientes
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do cliente"
// @Success      200  {object}  schemas.ClientSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Cliente não encontrado"
// @Router       /clients/{id} [get]
func (h *ClientHandler) GetClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var client data.Client
	if err := h.DB.First(&client, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Client not found")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Client fetched successfully", toClientResponse(client))
}

// UpdateClient handles the PUT /clients/{id} route
// @Summary      Atualizar cliente
// @Description  Atualiza os dados de um cliente existente. Todos os campos são opcionais (atualização parcial).
// @Tags         Clientes
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string                      true  "UUID do cliente"
// @Param        payload  body  schemas.UpdateClientRequest true  "Campos a atualizar"
// @Success      200  {object}  schemas.ClientSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      404  {object}  schemas.ErrorResponse "Cliente não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "Nome já está em uso"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao atualizar cliente"
// @Router       /clients/{id} [put]
func (h *ClientHandler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var client data.Client
	if err := h.DB.First(&client, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusNotFound, "Client not found")
		return
	}

	var req schemas.UpdateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name != "" {
		client.Name = req.Name
	}

	if req.Phone != nil {
		client.Phone = req.Phone
	}

	if err := h.DB.Save(&client).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Client name already in use")
			return
		}

		utils.SendError(w, r, http.StatusInternalServerError, "Failed to update client: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Client updated successfully", toClientResponse(client))
}

// DeleteClient handles the DELETE /clients/{id} route.
// @Summary      Deletar cliente
// @Description  Remove um cliente do sistema (hard delete).
// @Tags         Clientes
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  string  true  "UUID do cliente"
// @Success      200  {object}  schemas.ErrorResponse "Cliente deletado com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar cliente"
// @Router       /clients/{id} [delete]
func (h *ClientHandler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.Delete(&data.Client{}, "id = ?", id).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete client: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Client deleted successfully", nil)
}
