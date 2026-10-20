package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// MCPTokenHandler manages the caller's own MCP tokens. Nobody manages anyone else's: a token
// acts as its owner, so handing one out on someone's behalf would be impersonation.
type MCPTokenHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewMCPTokenHandler(db *gorm.DB) *MCPTokenHandler {
	return &MCPTokenHandler{DB: db, Validator: validator.New()}
}

func toMCPTokenResponse(token data.MCPToken) schemas.MCPTokenResponse {
	return schemas.MCPTokenResponse{
		ID:         token.ID,
		Name:       token.Name,
		LastUsedAt: token.LastUsedAt,
		RevokedAt:  token.RevokedAt,
		CreatedAt:  token.CreatedAt,
	}
}

// GetMCPTokens handles the GET /mcp-tokens route.
// @Summary      Listar tokens MCP
// @Description  Lista os tokens MCP do próprio usuário, revogados inclusive. Nunca traz o token.
// @Tags         MCP
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   schemas.MCPTokenResponse
// @Failure      401  {object}  utils.StandardResponse
// @Router       /mcp-tokens [get]
func (h *MCPTokenHandler) GetMCPTokens(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}

	var tokens []data.MCPToken
	if err := h.DB.Where("user_id = ?", user.ID).Order("created_at DESC").Find(&tokens).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the MCP tokens")
		return
	}

	response := make([]schemas.MCPTokenResponse, 0, len(tokens))
	for _, token := range tokens {
		response = append(response, toMCPTokenResponse(token))
	}
	utils.SendJSON(w, r, http.StatusOK, "MCP tokens fetched successfully", response)
}

// CreateMCPToken handles the POST /mcp-tokens route.
// @Summary      Criar token MCP
// @Description  Cria um token MCP para o próprio usuário. O token só aparece nesta resposta.
// @Tags         MCP
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body schemas.CreateMCPTokenRequest true "Nome do token"
// @Success      201  {object}  schemas.CreatedMCPTokenResponse
// @Failure      400  {object}  utils.StandardResponse
// @Router       /mcp-tokens [post]
func (h *MCPTokenHandler) CreateMCPToken(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}

	var req schemas.CreateMCPTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Informe um nome de até 100 caracteres")
		return
	}

	secret, hash, err := data.NewMCPTokenSecret()
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to generate the MCP token")
		return
	}

	token := data.MCPToken{UserID: user.ID, Name: req.Name, TokenHash: hash}
	if err := h.DB.Create(&token).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to create the MCP token")
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "MCP token created successfully", schemas.CreatedMCPTokenResponse{
		MCPTokenResponse: toMCPTokenResponse(token),
		Token:            secret,
	})
}

// RevokeMCPToken handles the DELETE /mcp-tokens/{id} route.
// @Summary      Revogar token MCP
// @Description  Revoga um token MCP do próprio usuário. A linha fica, para o histórico.
// @Tags         MCP
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Token ID"
// @Success      200  {object}  schemas.MCPTokenResponse
// @Failure      404  {object}  utils.StandardResponse
// @Router       /mcp-tokens/{id} [delete]
func (h *MCPTokenHandler) RevokeMCPToken(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}

	tokenID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(tokenID); err != nil {
		utils.SendError(w, r, http.StatusNotFound, "MCP token not found")
		return
	}

	// Someone else's token answers 404, the same as one that does not exist.
	var token data.MCPToken
	if err := h.DB.First(&token, "id = ? AND user_id = ?", tokenID, user.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(w, r, http.StatusNotFound, "MCP token not found")
			return
		}
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch the MCP token")
		return
	}

	// Revoking twice keeps the first date: that is when it stopped working.
	if token.RevokedAt == nil {
		now := time.Now()
		if err := h.DB.Model(&token).Update("revoked_at", now).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Failed to revoke the MCP token")
			return
		}
		token.RevokedAt = &now
	}

	utils.SendJSON(w, r, http.StatusOK, "MCP token revoked successfully", toMCPTokenResponse(token))
}
