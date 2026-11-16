// Package mcp serves the personal MCP server: the tools an agent (Claude Code, a scheduled
// routine) uses to look after the points inventory and the issuances.
//
// Every tool is a call into the /api/v1 router, in process, authenticated as the token's owner
// with a JWT that lives a few minutes (see loopback.go). That is the whole design: the role
// gates, the agency scope, the handlers' validation and the closed-period guard all apply to
// the agent exactly as they apply to the panel, and no business rule is written twice.
//
// The tools come in three kinds, by what the agent may do on its own:
//   - reads, which answer directly;
//   - free writes (creating and editing issuances, adjustments and point batches), which run
//     directly and leave a row in mcp_calls;
//   - destructive writes (cancelling, deleting, closing a period), which only record a proposal
//     that a dev approves in the panel.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httprate"
	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// tokenPrefix must match handlers.MCPTokenPrefix. A string without it is rejected before the
// database is asked, so a panel JWT pasted by mistake never even reaches the lookup.
const tokenPrefix = "m80mcp_"

// maxBodyBytes caps a JSON-RPC request. The largest legitimate one is a create_issuance call,
// a few KB.
const maxBodyBytes = 1 << 20

// callsPerMinute is per token. The panel-wide limiter cannot tell MCP traffic apart (it keys
// on the JWT subject and falls back to the IP, which is the panel container), so the agent gets
// a budget of its own: generous for a routine walking every account, tight for a loop.
const callsPerMinute = 120

// Server holds what every tool needs.
type Server struct {
	db *gorm.DB
	// v1 is the /api/v1 router, which every tool calls into.
	v1 http.Handler
	// now is swappable for tests.
	now func() time.Time
}

// NewHandler builds the /mcp endpoint. v1 must be the same router mounted at /api/v1.
func NewHandler(db *gorm.DB, v1 http.Handler) http.Handler {
	s := &Server{db: db, v1: v1, now: time.Now}

	server := sdk.NewServer(&sdk.Implementation{Name: "m80", Version: "1.0.0"}, &sdk.ServerOptions{
		Instructions: serverInstructions,
	})
	s.registerSystemTools(server)

	// Stateless + JSON responses: every POST stands alone, so nothing depends on session
	// affinity or a long-lived SSE stream surviving the hop through the panel's relay.
	streamable := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})

	limited := httprate.LimitBy(callsPerMinute, time.Minute, func(r *http.Request) (string, error) {
		// Runs after RequireBearerToken, so the token info is always there.
		return auth.TokenInfoFromContext(r.Context()).Extra[extraTokenID].(string), nil
	})(streamable)

	authed := auth.RequireBearerToken(s.verifyToken, nil)(limited)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		authed.ServeHTTP(w, r)
	})
}

const serverInstructions = `M80: estoque de pontos (milhas) e emissões de passagens.
Unidades: pontos são inteiros (50 mil = 50000); dinheiro é sempre em centavos (R$ 12,34 = 1234); value_of_the_thousand e cost_of_the_thousand são centavos por mil pontos. Datas em AAAA-MM-DD, fuso America/Sao_Paulo.
Leituras respondem direto. Criar e editar emissões, ajustes e lotes de pontos grava direto. Cancelar, descancelar, excluir, fechar ou reabrir período e marcar acerto viram proposta (propose_*): só um dev aprova no painel; nenhuma ferramenta aprova.
Toda chamada fica registrada. Na dúvida sobre um id, busque antes (search_issuances, stock_overview, list_point_batches) em vez de adivinhar.`

// Keys of auth.TokenInfo.Extra set by verifyToken.
const (
	extraTokenID = "token_id"
	extraPhone   = "phone"
	extraRole    = "role"
)

// errUnavailable is what verifyToken returns when the database cannot answer. It is not an
// auth.ErrInvalidToken on purpose: the SDK turns that into a 401, and a database hiccup is not
// a verdict on the token.
var errUnavailable = errors.New("serviço temporariamente indisponível")

func (s *Server) verifyToken(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	if !strings.HasPrefix(token, tokenPrefix) {
		return nil, auth.ErrInvalidToken
	}

	var record data.MCPToken
	err := s.db.Preload("User").
		Where("token_hash = ? AND revoked_at IS NULL", data.HashMCPToken(token)).
		First(&record).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, auth.ErrInvalidToken
	case err != nil:
		slog.Error("mcp: failed to look up token", "err", err)
		return nil, errUnavailable
	}
	// Preload honours the users soft delete: a deactivated user's tokens die with them.
	if record.User == nil {
		return nil, auth.ErrInvalidToken
	}

	now := s.now()
	// One write a minute at most: a routine walking every account would otherwise update the
	// same row on every single call.
	if record.LastUsedAt == nil || now.Sub(*record.LastUsedAt) > time.Minute {
		if err := s.db.Model(&record).Update("last_used_at", now).Error; err != nil {
			slog.Warn("mcp: failed to record token use", "token_id", record.ID, "err", err)
		}
	}

	return &auth.TokenInfo{
		UserID: record.UserID,
		// The SDK refuses a token without an expiration. This one is re-verified on every
		// request, so the expiration only has to outlive the request itself.
		Expiration: now.Add(5 * time.Minute),
		Extra: map[string]any{
			extraTokenID: record.ID,
			extraPhone:   record.User.Phone,
			extraRole:    record.User.Role,
		},
	}, nil
}

// caller is who a tool call runs as.
type caller struct {
	TokenID string
	UserID  string
	Phone   string
	Role    string
}

func callerFrom(req *sdk.CallToolRequest) (caller, error) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return caller{}, fmt.Errorf("chamada sem token")
	}
	info := req.Extra.TokenInfo
	tokenID, _ := info.Extra[extraTokenID].(string)
	phone, _ := info.Extra[extraPhone].(string)
	role, _ := info.Extra[extraRole].(string)
	if tokenID == "" || info.UserID == "" {
		return caller{}, fmt.Errorf("chamada sem token")
	}
	return caller{TokenID: tokenID, UserID: info.UserID, Phone: phone, Role: role}, nil
}
