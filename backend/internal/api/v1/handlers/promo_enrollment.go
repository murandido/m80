package handlers

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type PromoEnrollmentHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewPromoEnrollmentHandler(db *gorm.DB) *PromoEnrollmentHandler {
	return &PromoEnrollmentHandler{DB: db, Validator: validator.New()}
}

func toPromoEnrollmentResponse(enrollment data.PromoEnrollment) schemas.PromoEnrollmentResponse {
	response := schemas.PromoEnrollmentResponse{
		ID:          enrollment.ID,
		URL:         enrollment.URL,
		URLKey:      enrollment.URLKey,
		DocumentID:  enrollment.DocumentID,
		RecipeID:    enrollment.RecipeID,
		Status:      enrollment.Status,
		Message:     enrollment.Message,
		CreatedByID: enrollment.CreatedByID,
		ChatJID:     enrollment.ChatJID,
		CreatedAt:   enrollment.CreatedAt,
	}
	if enrollment.Document != nil {
		response.DocumentName = enrollment.Document.Name
	}
	if enrollment.Recipe != nil {
		response.RecipeName = enrollment.Recipe.Name
	}
	if enrollment.CreatedBy != nil {
		response.CreatedBy = enrollment.CreatedBy.Name
	}
	return response
}

var promoEnrollmentOrder = map[string]string{
	"created_desc": "promo_enrollments.created_at DESC",
	"created_asc":  "promo_enrollments.created_at ASC",
}

// GetPromoEnrollments handles the GET /promo-enrollments route
// @Summary      Listar histórico de cadastros em promoções
// @Description  Histórico paginado do que foi tentado, para quem e como terminou.
// @Tags         Promoções
// @Produce      json
// @Security     BearerAuth
// @Param        q query string false "Busca pelo link"
// @Param        status query string false "Filtra por resultado" Enums(success, failure, unknown, refused, error)
// @Param        document_id query string false "Filtra por CPF"
// @Param        order query string false "Ordenação" Enums(created_desc, created_asc)
// @Param        page query int false "Página" default(1)
// @Param        limit query int false "Itens por página" default(20)
// @Success      200  {object}  schemas.PromoEnrollmentListResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a administradores"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao listar histórico"
// @Router       /promo-enrollments [get]
//
// Paginated because this table only ever grows: it is the audit trail, so nothing is
// ever removed from it.
func (h *PromoEnrollmentHandler) GetPromoEnrollments(w http.ResponseWriter, r *http.Request) {
	params := parseListQuery(r)

	query := h.DB.Model(&data.PromoEnrollment{})
	if params.Q != "" {
		query = query.Where("promo_enrollments.url ILIKE ?", likePattern(params.Q))
	}
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("promo_enrollments.status = ?", status)
	}
	if documentID := r.URL.Query().Get("document_id"); documentID != "" {
		query = query.Where("promo_enrollments.document_id = ?", documentID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao contar histórico: "+err.Error())
		return
	}

	var enrollments []data.PromoEnrollment
	if err := query.Session(&gorm.Session{}).
		Preload("Document").
		Preload("Recipe").
		Preload("CreatedBy").
		Order(orderClause(params.Order, promoEnrollmentOrder, promoEnrollmentOrder["created_desc"])).
		Offset((params.Page - 1) * params.Limit).
		Limit(params.Limit).
		Find(&enrollments).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar histórico: "+err.Error())
		return
	}

	rows := make([]schemas.PromoEnrollmentResponse, 0, len(enrollments))
	for _, enrollment := range enrollments {
		rows = append(rows, toPromoEnrollmentResponse(enrollment))
	}

	utils.SendJSON(w, r, http.StatusOK, "Histórico listado com sucesso", schemas.PromoEnrollmentPaginatedData{
		Enrollments: rows,
		Total:       total,
		Page:        params.Page,
		Limit:       params.Limit,
	})
}
