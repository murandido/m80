package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// MCPTokenPrefix marks a string as an MCP token, so a leaked one is recognisable at a glance
// (and by secret scanners) and can never be mistaken for a panel session JWT.
const MCPTokenPrefix = "m80mcp_"

type MCPTokenHandler struct {
	DB        *gorm.DB
	Validator *validator.Validate
}

func NewMCPTokenHandler(db *gorm.DB) *MCPTokenHandler {
	return &MCPTokenHandler{DB: db, Validator: validator.New()}
}

func toMCPTokenResponse(token data.MCPToken) schemas.MCPTokenResponse {
	resp := schemas.MCPTokenResponse{
		ID:         token.ID,
		UserID:     token.UserID,
		Name:       token.Name,
		LastUsedAt: token.LastUsedAt,
		RevokedAt:  token.RevokedAt,
		CreatedAt:  token.CreatedAt,
	}
	if token.User != nil {
		resp.UserName = &token.User.Name
	}
	return resp
}

// GetMCPTokens handles the GET /mcp-tokens route
// @Summary      Listar tokens MCP
// @Description  Lista os tokens do servidor MCP, inclusive os revogados. O valor do token nunca é devolvido.
// @Tags         MCP
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Router       /mcp-tokens [get]
func (h *MCPTokenHandler) GetMCPTokens(w http.ResponseWriter, r *http.Request) {
	var tokens []data.MCPToken
	if err := h.DB.Preload("User").Order("created_at desc").Find(&tokens).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao listar tokens MCP")
		return
	}

	resp := make([]schemas.MCPTokenResponse, 0, len(tokens))
	for _, token := range tokens {
		resp = append(resp, toMCPTokenResponse(token))
	}
	utils.SendJSON(w, r, http.StatusOK, "Tokens MCP listados com sucesso", resp)
}

// CreateMCPToken handles the POST /mcp-tokens route
// @Summary      Criar token MCP
// @Description  Cria um token MCP do próprio usuário. O valor em claro é devolvido só nesta resposta.
// @Tags         MCP
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      schemas.CreateMCPTokenRequest  true  "Nome do token"
// @Success      201  {object}  object
// @Failure      400  {object}  schemas.ErrorResponse "Dados inválidos"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Router       /mcp-tokens [post]
func (h *MCPTokenHandler) CreateMCPToken(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
		return
	}

	var req schemas.CreateMCPTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "JSON inválido")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Nome do token é obrigatório (até 80 caracteres)")
		return
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao gerar token")
		return
	}
	plain := MCPTokenPrefix + base64.RawURLEncoding.EncodeToString(secret)

	token := data.MCPToken{UserID: user.ID, Name: req.Name, TokenHash: data.HashMCPToken(plain)}
	if err := h.DB.Create(&token).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao criar token MCP")
		return
	}

	utils.SendJSON(w, r, http.StatusCreated, "Token MCP criado: copie agora, ele não será mostrado de novo", schemas.CreatedMCPTokenResponse{
		MCPTokenResponse: toMCPTokenResponse(token),
		Token:            plain,
	})
}

// RevokeMCPToken handles the DELETE /mcp-tokens/{id} route
// @Summary      Revogar token MCP
// @Description  Revoga um token MCP. A linha fica, para a trilha de chamadas continuar apontando para ela.
// @Tags         MCP
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "ID do token"
// @Success      200  {object}  object
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      404  {object}  schemas.ErrorResponse "Token não encontrado"
// @Router       /mcp-tokens/{id} [delete]
func (h *MCPTokenHandler) RevokeMCPToken(w http.ResponseWriter, r *http.Request) {
	var token data.MCPToken
	err := h.DB.First(&token, "id = ?", chi.URLParam(r, "id")).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.SendError(w, r, http.StatusNotFound, "Token MCP não encontrado")
		return
	}
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Erro ao buscar token MCP")
		return
	}

	if token.RevokedAt == nil {
		now := time.Now()
		if err := h.DB.Model(&token).Update("revoked_at", now).Error; err != nil {
			utils.SendError(w, r, http.StatusInternalServerError, "Erro ao revogar token MCP")
			return
		}
		token.RevokedAt = &now
	}

	utils.SendJSON(w, r, http.StatusOK, "Token MCP revogado", toMCPTokenResponse(token))
}
