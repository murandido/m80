package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// MCPReportHandler serves the agents' daily reports to the panel.
type MCPReportHandler struct {
	DB *gorm.DB
}

func NewMCPReportHandler(db *gorm.DB) *MCPReportHandler {
	return &MCPReportHandler{DB: db}
}

// GetMCPReports handles the GET /mcp-reports route
// @Summary      Listar relatórios MCP
// @Description  Relatórios diários gravados pelos agentes MCP (ex.: conferência das liminares), dos mais recentes para os mais antigos.
// @Tags         MCP
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query  int  false  "Quantidade (padrão 30, máximo 120)"
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Router       /mcp-reports [get]
func (h *MCPReportHandler) GetMCPReports(w http.ResponseWriter, r *http.Request) {
	limit := 30
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = min(parsed, 120)
		}
	}

	var reports []data.MCPReport
	if err := h.DB.Order("report_date desc").Order("updated_at desc").Limit(limit).Find(&reports).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar relatórios MCP")
		return
	}

	tokenNames := map[string]string{}
	if len(reports) > 0 {
		ids := make([]string, 0, len(reports))
		for _, report := range reports {
			ids = append(ids, report.TokenID)
		}
		var tokens []data.MCPToken
		if err := h.DB.Select("id", "name").Where("id IN ?", ids).Find(&tokens).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar relatórios MCP")
			return
		}
		for _, token := range tokens {
			tokenNames[token.ID] = token.Name
		}
	}

	resp := make([]schemas.MCPReportResponse, 0, len(reports))
	for _, report := range reports {
		item := schemas.MCPReportResponse{
			ID:         report.ID,
			TokenID:    report.TokenID,
			ReportDate: report.ReportDate.Format(time.DateOnly),
			Title:      report.Title,
			Summary:    report.Summary,
			Findings:   rawOrNil(report.Findings),
			Actions:    rawOrNil(report.Actions),
			CreatedAt:  report.CreatedAt,
			UpdatedAt:  report.UpdatedAt,
		}
		if name, ok := tokenNames[report.TokenID]; ok {
			item.TokenName = &name
		}
		resp = append(resp, item)
	}
	utils.SendJSON(w, r, http.StatusOK, "Relatórios MCP listados com sucesso", resp)
}
