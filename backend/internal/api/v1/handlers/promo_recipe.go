package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/promotion"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type PromoRecipeHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
	Promo     *promotion.Client
}

func NewPromoRecipeHandler(db *gorm.DB) *PromoRecipeHandler {
	return &PromoRecipeHandler{
		DB:        db,
		Validator: validator.New(),
		Promo:     promotion.NewClient(),
	}
}

func toPromoRecipeResponse(recipe data.PromoRecipe) schemas.PromoRecipeResponse {
	response := schemas.PromoRecipeResponse{
		ID:               recipe.ID,
		Domain:           recipe.Domain,
		Name:             recipe.Name,
		LoyaltyProgramID: recipe.LoyaltyProgramID,
		CPFSelector:      recipe.CPFSelector,
		CheckboxSelector: recipe.CheckboxSelector,
		SubmitSelector:   recipe.SubmitSelector,
		SuccessPattern:   recipe.SuccessPattern,
		FailurePattern:   recipe.FailurePattern,
		WaitSeconds:      recipe.WaitSeconds,
		Active:           recipe.Active,
		Notes:            recipe.Notes,
		CreatedAt:        recipe.CreatedAt,
		UpdatedAt:        recipe.UpdatedAt,
	}
	if recipe.LoyaltyProgram != nil {
		response.LoyaltyProgramName = recipe.LoyaltyProgram.Name
	}
	return response
}

// normalizeDomain strips what people paste out of a browser bar. A recipe whose domain
// reads "https://www.voeazul.com.br/" would never match anything, and the failure would
// look like the site changing rather than a typo.
func normalizeDomain(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("empty domain")
	}
	if strings.Contains(trimmed, "://") || strings.Contains(trimmed, "/") {
		return promotion.DomainOf(trimmed)
	}
	return strings.TrimPrefix(strings.ToLower(trimmed), "www."), nil
}

// validatePatterns rejects an unparseable regular expression at the door. Accepting one
// would push the failure into the middle of a live campaign run, where it costs an
// enrolment window instead of a form submission.
func validatePatterns(patterns ...*string) error {
	for _, pattern := range patterns {
		if pattern == nil || strings.TrimSpace(*pattern) == "" {
			continue
		}
		if _, err := regexp.Compile(*pattern); err != nil {
			return err
		}
	}
	return nil
}

// CreatePromoRecipe handles the POST /promo-recipes route
// @Summary      Criar receita de promoção
// @Description  Registra como o formulário de cadastro de um site deve ser preenchido.
// @Tags         Promoções
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.CreatePromoRecipeRequest true "Dados da receita"
// @Success      201  {object}  schemas.PromoRecipeSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma receita para este domínio"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao criar receita"
// @Router       /promo-recipes [post]
func (h *PromoRecipeHandler) CreatePromoRecipe(w http.ResponseWriter, r *http.Request) {
	var req schemas.CreatePromoRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}
	if err := validatePatterns(req.SuccessPattern, req.FailurePattern); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Expressão regular inválida: "+err.Error())
		return
	}

	domain, err := normalizeDomain(req.Domain)
	if err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Domínio inválido: "+err.Error())
		return
	}

	recipe := data.PromoRecipe{
		Domain:           domain,
		Name:             req.Name,
		LoyaltyProgramID: req.LoyaltyProgramID,
		CPFSelector:      req.CPFSelector,
		CheckboxSelector: req.CheckboxSelector,
		SubmitSelector:   req.SubmitSelector,
		SuccessPattern:   req.SuccessPattern,
		FailurePattern:   req.FailurePattern,
		WaitSeconds:      5,
		Active:           true,
		Notes:            req.Notes,
	}
	if req.WaitSeconds != nil {
		recipe.WaitSeconds = *req.WaitSeconds
	}
	if req.Active != nil {
		recipe.Active = *req.Active
	}

	if err := h.DB.Create(&recipe).Error; err != nil {
		utils.SendError(w, r, promoWriteStatus(err), promoWriteMessage(err, "Erro ao criar receita"))
		return
	}
	h.respondWithRecipe(w, r, http.StatusCreated, "Receita criada com sucesso", recipe.ID)
}

// promoRecipeOrder is the whitelist of sort keys; the client's string only ever indexes
// it, so `order` can never reach the SQL.
var promoRecipeOrder = map[string]string{
	"name_asc":     textOrder("promo_recipes.name", "ASC"),
	"name_desc":    textOrder("promo_recipes.name", "DESC"),
	"domain_asc":   textOrder("promo_recipes.domain", "ASC"),
	"created_desc": "promo_recipes.created_at DESC",
	"created_asc":  "promo_recipes.created_at ASC",
}

// GetPromoRecipes handles the GET /promo-recipes route
// @Summary      Listar receitas de promoção
// @Description  Lista as receitas cadastradas, com busca por nome ou domínio.
// @Tags         Promoções
// @Produce      json
// @Security     BearerAuth
// @Param        q query string false "Busca por nome ou domínio"
// @Param        order query string false "Ordenação" Enums(name_asc, name_desc, domain_asc, created_desc, created_asc)
// @Param        active query bool false "Filtra por ativas ou inativas"
// @Success      200  {object}  schemas.PromoRecipeListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao listar receitas"
// @Router       /promo-recipes [get]
func (h *PromoRecipeHandler) GetPromoRecipes(w http.ResponseWriter, r *http.Request) {
	params := parseListQuery(r)
	query := h.DB.Model(&data.PromoRecipe{}).Preload("LoyaltyProgram")

	if params.Q != "" {
		pattern := likePattern(params.Q)
		query = query.Where(
			h.DB.Where(unaccentedLike("promo_recipes.name"), pattern).
				Or(unaccentedLike("promo_recipes.domain"), pattern),
		)
	}
	if active := parseOptionalBool(r.URL.Query().Get("active")); active != nil {
		query = query.Where("promo_recipes.active = ?", *active)
	}

	var recipes []data.PromoRecipe
	if err := query.
		Order(orderClause(params.Order, promoRecipeOrder, promoRecipeOrder["name_asc"])).
		Find(&recipes).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar receitas: "+err.Error())
		return
	}

	response := make([]schemas.PromoRecipeResponse, 0, len(recipes))
	for _, recipe := range recipes {
		response = append(response, toPromoRecipeResponse(recipe))
	}
	utils.SendJSON(w, r, http.StatusOK, "Receitas listadas com sucesso", response)
}

// GetPromoRecipe handles the GET /promo-recipes/{id} route
// @Summary      Buscar receita de promoção
// @Tags         Promoções
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "ID da receita"
// @Success      200  {object}  schemas.PromoRecipeSingleResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      404  {object}  schemas.ErrorResponse "Receita não encontrada"
// @Router       /promo-recipes/{id} [get]
func (h *PromoRecipeHandler) GetPromoRecipe(w http.ResponseWriter, r *http.Request) {
	h.respondWithRecipe(w, r, http.StatusOK, "Receita encontrada", chi.URLParam(r, "id"))
}

// UpdatePromoRecipe handles the PUT /promo-recipes/{id} route
// @Summary      Atualizar receita de promoção
// @Description  Atualiza uma receita. Campos ausentes não são alterados; seletores e expressões enviados como null são limpos, voltando à heurística.
// @Tags         Promoções
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "ID da receita"
// @Param        payload body schemas.UpdatePromoRecipeRequest true "Campos a atualizar"
// @Success      200  {object}  schemas.PromoRecipeSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      404  {object}  schemas.ErrorResponse "Receita não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Já existe uma receita para este domínio"
// @Router       /promo-recipes/{id} [put]
func (h *PromoRecipeHandler) UpdatePromoRecipe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req schemas.UpdatePromoRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}
	if err := validatePatterns(req.SuccessPattern, req.FailurePattern); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Expressão regular inválida: "+err.Error())
		return
	}

	var recipe data.PromoRecipe
	if err := h.DB.First(&recipe, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Receita não encontrada")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao buscar receita: "+err.Error())
		return
	}

	if req.Domain != nil {
		domain, err := normalizeDomain(*req.Domain)
		if err != nil {
			utils.SendError(w, r, http.StatusBadRequest, "Domínio inválido: "+err.Error())
			return
		}
		recipe.Domain = domain
	}
	if req.Name != nil {
		recipe.Name = *req.Name
	}
	if req.LoyaltyProgramID != nil {
		recipe.LoyaltyProgramID = *req.LoyaltyProgramID
	}
	if req.WaitSeconds != nil {
		recipe.WaitSeconds = *req.WaitSeconds
	}
	if req.Active != nil {
		recipe.Active = *req.Active
	}
	// The nullable columns take the payload as-is, including nil. That is what lets a
	// selector pinned by mistake be handed back to the heuristic - with "nil means
	// unchanged" there would be no way to un-pin one.
	recipe.CPFSelector = req.CPFSelector
	recipe.CheckboxSelector = req.CheckboxSelector
	recipe.SubmitSelector = req.SubmitSelector
	recipe.SuccessPattern = req.SuccessPattern
	recipe.FailurePattern = req.FailurePattern
	recipe.Notes = req.Notes

	if err := h.DB.Save(&recipe).Error; err != nil {
		utils.SendError(w, r, promoWriteStatus(err), promoWriteMessage(err, "Erro ao atualizar receita"))
		return
	}
	h.respondWithRecipe(w, r, http.StatusOK, "Receita atualizada com sucesso", recipe.ID)
}

// DeletePromoRecipe handles the DELETE /promo-recipes/{id} route
// @Summary      Excluir receita de promoção
// @Tags         Promoções
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "ID da receita"
// @Success      200  {object}  schemas.ErrorResponse "Receita excluída com sucesso"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      404  {object}  schemas.ErrorResponse "Receita não encontrada"
// @Router       /promo-recipes/{id} [delete]
func (h *PromoRecipeHandler) DeletePromoRecipe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.DB.Delete(&data.PromoRecipe{}, "id = ?", id)
	if result.Error != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao excluir receita: "+result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		utils.SendError(w, r, http.StatusNotFound, "Receita não encontrada")
		return
	}
	// promo_enrollments.recipe_id is ON DELETE SET NULL, so the history of what this
	// recipe did outlives it - deliberately.
	utils.SendJSON(w, r, http.StatusOK, "Receita excluída com sucesso", nil)
}

func (h *PromoRecipeHandler) respondWithRecipe(w http.ResponseWriter, r *http.Request, status int, message, id string) {
	var recipe data.PromoRecipe
	if err := h.DB.Preload("LoyaltyProgram").First(&recipe, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "Receita não encontrada")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao buscar receita: "+err.Error())
		return
	}
	utils.SendJSON(w, r, status, message, toPromoRecipeResponse(recipe))
}

// promoWriteStatus maps a write failure to a status the panel can act on.
func promoWriteStatus(err error) int {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case postgresUniqueViolation:
			return http.StatusUnprocessableEntity
		case postgresForeignKeyViolation:
			return http.StatusUnprocessableEntity
		}
	}
	return http.StatusInternalServerError
}

func promoWriteMessage(err error, fallback string) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case postgresUniqueViolation:
			return "Já existe um registro com este valor"
		case postgresForeignKeyViolation:
			return "Programa de fidelidade não encontrado"
		}
	}
	return fallback + ": " + err.Error()
}
