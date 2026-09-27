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

var clientOrder = map[string]string{
	"name_asc":     textOrder("clients.name", "ASC"),
	"name_desc":    textOrder("clients.name", "DESC"),
	"created_desc": "clients.created_at DESC",
	"created_asc":  "clients.created_at ASC",
}

// clientListQuery applies the optional search and the ordering.
//
// This listing was the one cadastro that never got an ORDER BY, so Postgres returned
// it in heap order and the screen reshuffled itself after any edit. That matters more
// now than it did: a page window over an unordered query is not just untidy, it can
// show the same client twice and skip another entirely.
//
// The phone is searched alongside the name because those are the only two fields a
// client has, and "who is this number" is half of why anyone opens this screen.
func clientListQuery(db *gorm.DB, params listQuery) *gorm.DB {
	query := db.Model(&data.Client{})

	if params.Q != "" {
		like := likePattern(params.Q)
		query = query.Where(
			db.Where(unaccentedLike("clients.name"), like).Or("clients.phone LIKE ?", like),
		)
	}

	return query.Order(orderClause(params.Order, clientOrder, clientOrder["name_asc"]))
}

// ListClients handles the GET /clients route.
// @Summary      Listar clientes
// @Description  Retorna todos os clientes cadastrados no sistema. Sem paginação — é a rota que alimenta o seletor de cliente da tela de vendas. Para a tela de gestão, use /clients/manage.
// @Tags         Clientes
// @Produce      json
// @Security     BearerAuth
// @Param        q      query  string  false  "Busca por nome ou telefone"
// @Param        order  query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Success      200  {object}  schemas.ClientListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar clientes"
// @Router       /clients [get]
func (h *ClientHandler) ListClients(w http.ResponseWriter, r *http.Request) {
	var clients []data.Client
	if err := clientListQuery(h.DB, parseListQuery(r)).Find(&clients).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch clients")
		return
	}

	resp := make([]schemas.ClientResponse, 0, len(clients))
	for _, client := range clients {
		resp = append(resp, toClientResponse(client))
	}

	utils.SendJSON(w, r, http.StatusOK, "Clients fetched successfully", resp)
}

// ManageClients handles the GET /clients/manage route.
// @Summary      Listar clientes (paginado)
// @Description  Listagem paginada para a tela de gestão. A rota /clients continua devolvendo tudo, porque é ela que alimenta o seletor de cliente da tela de vendas.
// @Tags         Clientes
// @Produce      json
// @Security     BearerAuth
// @Param        q      query  string  false  "Busca por nome ou telefone"
// @Param        order  query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        page   query  int     false  "Página (padrão 1)"
// @Param        limit  query  int     false  "Itens por página (padrão 20, máximo 100)"
// @Success      200  {object}  schemas.ClientPaginatedResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar clientes"
// @Router       /clients/manage [get]
func (h *ClientHandler) ManageClients(w http.ResponseWriter, r *http.Request) {
	params := parseListQuery(r)
	query := clientListQuery(h.DB, params)

	// The count runs on the same filtered query as the page, so the total the
	// pagination control shows can never disagree with what it is paginating.
	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to count clients")
		return
	}

	var clients []data.Client
	if err := paginate(query.Session(&gorm.Session{}), params).Find(&clients).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch clients")
		return
	}

	items := make([]schemas.ClientResponse, 0, len(clients))
	for _, client := range clients {
		items = append(items, toClientResponse(client))
	}

	utils.SendJSON(w, r, http.StatusOK, "Clients fetched successfully", schemas.ClientPaginatedData{
		Clients: items,
		Total:   total,
		Page:    params.Page,
		Limit:   params.Limit,
	})
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
// @Failure      422  {object}  schemas.ErrorResponse "Cliente tem pagamentos registrados"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao deletar cliente"
// @Router       /clients/{id} [delete]
func (h *ClientHandler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.Delete(&data.Client{}, "id = ?", id).Error; err != nil {
		// client_payments holds on to the client: deleting it would take the payments'
		// spread lines with it and leave sales marked paid for money nobody received.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			utils.SendError(w, r, http.StatusUnprocessableEntity, "Não é possível excluir cliente com pagamentos registrados")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to delete client: "+err.Error())
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Client deleted successfully", nil)
}
