package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/mcp"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// MCPProposalHandler is where a dev decides on what the agent proposed, and reads what it did.
type MCPProposalHandler struct {
	DB *gorm.DB
	// V1 is the /api/v1 router this handler is mounted on. An approved proposal runs through
	// it again, with the approver's own session, exactly as if the approver had clicked the
	// operation's button.
	V1 http.Handler
}

func NewMCPProposalHandler(db *gorm.DB, v1 http.Handler) *MCPProposalHandler {
	return &MCPProposalHandler{DB: db, V1: v1}
}

func rawOrNil(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	return json.RawMessage(value)
}

func toMCPProposalResponse(p data.MCPProposal, names map[string]string) schemas.MCPProposalResponse {
	resp := schemas.MCPProposalResponse{
		ID:          p.ID,
		UserID:      p.UserID,
		Tool:        p.Tool,
		Summary:     p.Summary,
		Operations:  rawOrNil(p.Operations),
		Preview:     rawOrNil(p.Preview),
		Status:      p.Status,
		Result:      rawOrNil(p.Result),
		CreatedAt:   p.CreatedAt,
		ExpiresAt:   p.CreatedAt.Add(data.MCPProposalLifetime),
		DecidedAt:   p.DecidedAt,
		DecidedByID: p.DecidedByID,
	}
	if p.DecidedByID != nil {
		if name, ok := names[*p.DecidedByID]; ok {
			resp.DecidedByName = &name
		}
	}
	return resp
}

// GetMCPProposals handles the GET /mcp-proposals route
// @Summary      Listar propostas MCP
// @Description  Lista as propostas do agente MCP, das mais recentes para as mais antigas. Propostas pendentes há mais de 48h viram expired antes da leitura.
// @Tags         MCP
// @Produce      json
// @Security     BearerAuth
// @Param        status  query  string  false  "pending (padrão), applied, rejected, failed, expired ou all"
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Router       /mcp-proposals [get]
func (h *MCPProposalHandler) GetMCPProposals(w http.ResponseWriter, r *http.Request) {
	if err := data.ExpireStaleMCPProposals(h.DB, time.Now()); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar propostas MCP")
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = data.MCPProposalPending
	}
	query := h.DB.Order("created_at desc").Limit(200)
	if status != "all" {
		query = query.Where("status = ?", status)
	}

	var proposals []data.MCPProposal
	if err := query.Find(&proposals).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar propostas MCP")
		return
	}

	names, err := h.deciderNames(proposals)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar propostas MCP")
		return
	}
	resp := make([]schemas.MCPProposalResponse, 0, len(proposals))
	for _, p := range proposals {
		resp = append(resp, toMCPProposalResponse(p, names))
	}
	utils.SendJSON(w, r, http.StatusOK, "Propostas MCP listadas com sucesso", resp)
}

func (h *MCPProposalHandler) deciderNames(proposals []data.MCPProposal) (map[string]string, error) {
	var ids []string
	for _, p := range proposals {
		if p.DecidedByID != nil {
			ids = append(ids, *p.DecidedByID)
		}
	}
	names := map[string]string{}
	if len(ids) == 0 {
		return names, nil
	}
	var users []data.User
	if err := h.DB.Unscoped().Select("id", "name").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		names[u.ID] = u.Name
	}
	return names, nil
}

func (h *MCPProposalHandler) sendDecision(w http.ResponseWriter, r *http.Request, proposal data.MCPProposal, err error, message string) {
	switch {
	case errors.Is(err, mcp.ErrProposalNotFound):
		utils.SendError(w, r, http.StatusNotFound, "Proposta não encontrada")
	case errors.Is(err, mcp.ErrProposalNotPending):
		utils.SendError(w, r, http.StatusUnprocessableEntity, "Proposta já decidida ou expirada")
	case err != nil:
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao decidir a proposta")
	default:
		names, nameErr := h.deciderNames([]data.MCPProposal{proposal})
		if nameErr != nil {
			names = map[string]string{}
		}
		utils.SendJSON(w, r, http.StatusOK, message, toMCPProposalResponse(proposal, names))
	}
}

// ApproveMCPProposal handles the POST /mcp-proposals/{id}/approve route
// @Summary      Aprovar proposta MCP
// @Description  Executa a operação proposta pelo agente com a sessão de quem aprova (o perfil e o escopo checados são os do aprovador). Se a API recusar a operação, a proposta vira failed com a mensagem da recusa.
// @Tags         MCP
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "ID da proposta"
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      404  {object}  schemas.ErrorResponse "Proposta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Proposta já decidida ou expirada"
// @Router       /mcp-proposals/{id}/approve [post]
func (h *MCPProposalHandler) ApproveMCPProposal(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}

	// The approver's own Authorization header, which RequireAuth has just validated. It is a
	// session JWT by construction: an MCP token is never accepted on this route.
	proposal, err := mcp.ApplyProposal(r.Context(), h.DB, h.V1, chi.URLParam(r, "id"), user.ID, r.Header.Get("Authorization"))
	message := "Proposta aprovada e aplicada"
	if err == nil && proposal.Status == data.MCPProposalFailed {
		message = "Proposta aprovada, mas a operação foi recusada pela API"
	}
	h.sendDecision(w, r, proposal, err, message)
}

// RejectMCPProposal handles the POST /mcp-proposals/{id}/reject route
// @Summary      Rejeitar proposta MCP
// @Description  Encerra a proposta sem executar nada.
// @Tags         MCP
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "ID da proposta"
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      404  {object}  schemas.ErrorResponse "Proposta não encontrada"
// @Failure      422  {object}  schemas.ErrorResponse "Proposta já decidida ou expirada"
// @Router       /mcp-proposals/{id}/reject [post]
func (h *MCPProposalHandler) RejectMCPProposal(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}
	proposal, err := mcp.RejectProposal(h.DB, chi.URLParam(r, "id"), user.ID)
	h.sendDecision(w, r, proposal, err, "Proposta rejeitada")
}

// GetMCPCalls handles the GET /mcp-calls route
// @Summary      Listar chamadas MCP
// @Description  Trilha das chamadas de ferramenta feitas pelo agente MCP, das mais recentes para as mais antigas.
// @Tags         MCP
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query  int  false  "Quantidade (padrão 100, máximo 500)"
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Router       /mcp-calls [get]
func (h *MCPProposalHandler) GetMCPCalls(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = min(parsed, 500)
		}
	}

	var calls []data.MCPCall
	if err := h.DB.Order("created_at desc").Limit(limit).Find(&calls).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar chamadas MCP")
		return
	}

	tokenIDs := make([]string, 0, len(calls))
	for _, c := range calls {
		tokenIDs = append(tokenIDs, c.TokenID)
	}
	tokenNames := map[string]string{}
	if len(tokenIDs) > 0 {
		var tokens []data.MCPToken
		if err := h.DB.Select("id", "name").Where("id IN ?", tokenIDs).Find(&tokens).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar chamadas MCP")
			return
		}
		for _, t := range tokens {
			tokenNames[t.ID] = t.Name
		}
	}

	resp := make([]schemas.MCPCallResponse, 0, len(calls))
	for _, c := range calls {
		item := schemas.MCPCallResponse{
			ID:         c.ID,
			TokenID:    c.TokenID,
			Tool:       c.Tool,
			Args:       rawOrNil(c.Args),
			OK:         c.OK,
			Error:      c.Error,
			DurationMs: c.DurationMs,
			CreatedAt:  c.CreatedAt,
		}
		if name, ok := tokenNames[c.TokenID]; ok {
			item.TokenName = &name
		}
		resp = append(resp, item)
	}
	utils.SendJSON(w, r, http.StatusOK, "Chamadas MCP listadas com sucesso", resp)
}
