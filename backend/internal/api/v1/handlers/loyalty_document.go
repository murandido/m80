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

type LoyaltyDocumentHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewLoyaltyDocumentHandler(db *gorm.DB) *LoyaltyDocumentHandler {
	return &LoyaltyDocumentHandler{DB: db, Validator: validator.New()}
}

func toLoyaltyDocumentResponse(document data.LoyaltyDocument) schemas.LoyaltyDocumentResponse {
	response := schemas.LoyaltyDocumentResponse{
		ID:               document.ID,
		Name:             document.Name,
		CPF:              document.CPF,
		LoyaltyProgramID: document.LoyaltyProgramID,
		Active:           document.Active,
		CreatedAt:        document.CreatedAt,
		UpdatedAt:        document.UpdatedAt,
	}
	if document.LoyaltyProgram != nil {
		response.LoyaltyProgramName = document.LoyaltyProgram.Name
	}
	return response
}

// CreateLoyaltyDocument handles the POST /loyalty-documents route
// @Summary      Cadastrar CPF das promoções
// @Description  Registra um CPF no grupo de um programa de fidelidade. O número é guardado criptografado.
// @Tags         Promoções
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreateLoyaltyDocumentRequest true "Dados do CPF"
// @Success      201  {object}  schemas.LoyaltyDocumentSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido ou CPF inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      422  {object}  schemas.ErrorResponse "CPF já cadastrado neste programa"
// @Router       /loyalty-documents [post]
func (h *LoyaltyDocumentHandler) CreateLoyaltyDocument(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreateLoyaltyDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	// Strict validation here, where a 400 can be returned - the model's save hook is
	// deliberately lenient so that a document keeps whatever uniqueness it had. A CPF
	// one digit off would enrol a stranger in the agency's promotion and consume the
	// bonus cap of a real account, so this is the wrong place to be forgiving.
	cpf, ok := utils.NormalizeCPF(req.CPF)
	if !ok {
		utils.SendError(w, r, http.StatusBadRequest, "CPF inválido")
		return
	}

	document := data.LoyaltyDocument{
		Name:             req.Name,
		CPF:              cpf,
		LoyaltyProgramID: req.LoyaltyProgramID,
		Active:           true,
	}
	if req.Active != nil {
		document.Active = *req.Active
	}

	if err := h.DB.Create(&document).Error; err != nil {
		utils.SendError(w, r, promoWriteStatus(err), loyaltyDocumentWriteMessage(err, "Erro ao cadastrar CPF"))
		return
	}
	// Reload so AfterFind decrypts: right after Create the struct still holds the
	// ciphertext BeforeSave put there, and responding with it would show "enc:..." on
	// screen. Same reload AccountHandler does for the account password.
	h.respondWithDocument(w, r, http.StatusCreated, "CPF cadastrado com sucesso", document.ID)
}

var loyaltyDocumentOrder = map[string]string{
	"name_asc":     textOrder("loyalty_documents.name", "ASC"),
	"name_desc":    textOrder("loyalty_documents.name", "DESC"),
	"created_desc": "loyalty_documents.created_at DESC",
	"created_asc":  "loyalty_documents.created_at ASC",
}

// loyaltyDocumentListQuery applies the optional search, the filters and the ordering.
//
// Searching by name only. The CPF column holds ciphertext with a random nonce, so a
// LIKE over it would match nothing - looking one up means searching by the name it
// was registered under.
func loyaltyDocumentListQuery(db *gorm.DB, params listQuery, active *bool, programID string) *gorm.DB {
	query := db.Model(&data.LoyaltyDocument{})

	if params.Q != "" {
		query = query.Where(unaccentedLike("loyalty_documents.name"), likePattern(params.Q))
	}
	if active != nil {
		query = query.Where("loyalty_documents.active = ?", *active)
	}
	if programID != "" {
		query = query.Where("loyalty_documents.loyalty_program_id = ?", programID)
	}

	return query.Order(orderClause(params.Order, loyaltyDocumentOrder, loyaltyDocumentOrder["name_asc"]))
}

// GetLoyaltyDocuments handles the GET /loyalty-documents route
// @Summary      Listar CPFs das promoções
// @Description  Retorna todos os CPFs cadastrados. Sem paginação — é a rota que a ação de cadastro relê para conferir o resultado. Para a tela de gestão, use /loyalty-documents/manage.
// @Tags         Promoções
// @Produce      json
// @Security     BearerAuth
// @Param        q query string false "Busca por nome"
// @Param        order query string false "Ordenação" Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        active query bool false "Filtra por ativos ou inativos"
// @Param        loyalty_program_id query string false "Filtra por programa de fidelidade"
// @Success      200  {object}  schemas.LoyaltyDocumentListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Router       /loyalty-documents [get]
func (h *LoyaltyDocumentHandler) GetLoyaltyDocuments(w http.ResponseWriter, r *http.Request) {
	query := loyaltyDocumentListQuery(
		h.DB,
		parseListQuery(r),
		parseOptionalBool(r.URL.Query().Get("active")),
		r.URL.Query().Get("loyalty_program_id"),
	)

	var documents []data.LoyaltyDocument
	if err := query.Preload("LoyaltyProgram").Find(&documents).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar CPFs: "+err.Error())
		return
	}

	response := make([]schemas.LoyaltyDocumentResponse, 0, len(documents))
	for _, document := range documents {
		response = append(response, toLoyaltyDocumentResponse(document))
	}
	utils.SendJSON(w, r, http.StatusOK, "CPFs listados com sucesso", response)
}

// ListLoyaltyDocuments handles the GET /loyalty-documents/manage route
// @Summary      Listar CPFs das promoções (paginado)
// @Description  Listagem paginada para a tela de gestão. Esta tabela cresce um registro por CPF por programa, que é a taxa de crescimento de uma agenda de clientes.
// @Tags         Promoções
// @Produce      json
// @Security     BearerAuth
// @Param        q query string false "Busca por nome"
// @Param        order query string false "Ordenação" Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        active query bool false "Filtra por ativos ou inativos"
// @Param        loyalty_program_id query string false "Filtra por programa de fidelidade"
// @Param        page query int false "Página (padrão 1)"
// @Param        limit query int false "Itens por página (padrão 20, máximo 100)"
// @Success      200  {object}  schemas.LoyaltyDocumentPaginatedResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Router       /loyalty-documents/manage [get]
func (h *LoyaltyDocumentHandler) ListLoyaltyDocuments(w http.ResponseWriter, r *http.Request) {
	params := parseListQuery(r)
	query := loyaltyDocumentListQuery(
		h.DB,
		params,
		parseOptionalBool(r.URL.Query().Get("active")),
		r.URL.Query().Get("loyalty_program_id"),
	)

	// The count runs on the same filtered query as the page, so the total the
	// pagination control shows can never disagree with what it is paginating.
	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao contar CPFs")
		return
	}

	var documents []data.LoyaltyDocument
	if err := paginate(query.Session(&gorm.Session{}), params).
		Preload("LoyaltyProgram").
		Find(&documents).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar CPFs: "+err.Error())
		return
	}

	items := make([]schemas.LoyaltyDocumentResponse, 0, len(documents))
	for _, document := range documents {
		items = append(items, toLoyaltyDocumentResponse(document))
	}

	utils.SendJSON(w, r, http.StatusOK, "CPFs listados com sucesso", schemas.LoyaltyDocumentPaginatedData{
		Documents: items,
		Total:     total,
		Page:      params.Page,
		Limit:     params.Limit,
	})
}

// UpdateLoyaltyDocument handles the PUT /loyalty-documents/{id} route
// @Summary      Atualizar CPF das promoções
// @Tags         Promoções
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "ID do CPF"
// @Param        payload body schemas.UpdateLoyaltyDocumentRequest true "Campos a atualizar"
// @Success      200  {object}  schemas.LoyaltyDocumentSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido ou CPF inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      404  {object}  schemas.ErrorResponse "CPF não encontrado"
// @Failure      422  {object}  schemas.ErrorResponse "CPF já cadastrado neste programa"
// @Router       /loyalty-documents/{id} [put]
func (h *LoyaltyDocumentHandler) UpdateLoyaltyDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req schemas.UpdateLoyaltyDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	var document data.LoyaltyDocument
	if err := h.DB.First(&document, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "CPF não encontrado")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao buscar CPF: "+err.Error())
		return
	}

	if req.Name != nil {
		document.Name = *req.Name
	}
	if req.CPF != nil {
		cpf, ok := utils.NormalizeCPF(*req.CPF)
		if !ok {
			utils.SendError(w, r, http.StatusBadRequest, "CPF inválido")
			return
		}
		document.CPF = cpf
	}
	if req.LoyaltyProgramID != nil {
		document.LoyaltyProgramID = *req.LoyaltyProgramID
	}
	if req.Active != nil {
		document.Active = *req.Active
	}

	if err := h.DB.Save(&document).Error; err != nil {
		utils.SendError(w, r, promoWriteStatus(err), loyaltyDocumentWriteMessage(err, "Erro ao atualizar CPF"))
		return
	}
	h.respondWithDocument(w, r, http.StatusOK, "CPF atualizado com sucesso", document.ID)
}

// DeleteLoyaltyDocument handles the DELETE /loyalty-documents/{id} route
// @Summary      Excluir CPF das promoções
// @Description  Remove um CPF e, junto com ele, todo o histórico de cadastro em promoções desse CPF. Para preservar o histórico, desative em vez de excluir.
// @Tags         Promoções
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "ID do CPF"
// @Success      200  {object}  schemas.ErrorResponse "CPF excluído com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      404  {object}  schemas.ErrorResponse "CPF não encontrado"
// @Router       /loyalty-documents/{id} [delete]
func (h *LoyaltyDocumentHandler) DeleteLoyaltyDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Since migration 45 promo_enrollments cascades from here, so a document with
	// history deletes cleanly and takes that history with it. There is no foreign key
	// left to trip over on this path.
	result := h.DB.Delete(&data.LoyaltyDocument{}, "id = ?", id)
	if result.Error != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao excluir CPF: "+result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		utils.SendError(w, r, http.StatusNotFound, "CPF não encontrado")
		return
	}
	utils.SendJSON(w, r, http.StatusOK, "CPF excluído com sucesso", nil)
}

func (h *LoyaltyDocumentHandler) respondWithDocument(w http.ResponseWriter, r *http.Request, status int, message, id string) {
	var document data.LoyaltyDocument
	if err := h.DB.Preload("LoyaltyProgram").First(&document, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "CPF não encontrado")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao buscar CPF: "+err.Error())
		return
	}
	utils.SendJSON(w, r, status, message, toLoyaltyDocumentResponse(document))
}

func loyaltyDocumentWriteMessage(err error, fallback string) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case postgresUniqueViolation:
			return "Este CPF já está cadastrado neste programa de fidelidade"
		case postgresForeignKeyViolation:
			return "Programa de fidelidade não encontrado"
		}
	}
	return fallback + ": " + err.Error()
}
