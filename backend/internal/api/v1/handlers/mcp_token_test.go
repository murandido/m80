package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func TestCreateMCPToken_ReturnsTheTokenOnceAndStoresOnlyItsHash(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "MCP user")
		handler := NewMCPTokenHandler(tx)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/mcp-tokens", strings.NewReader(`{"name":"  notebook  "}`))
		handler.CreateMCPToken(rec, req.WithContext(asUser(req.Context(), user)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}

		var created struct {
			Data schemas.CreatedMCPTokenResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("failed to decode the response: %v", err)
		}
		if !strings.HasPrefix(created.Data.Token, "m80mcp_") || created.Data.Name != "notebook" {
			t.Fatalf("unexpected created token: %+v", created.Data)
		}

		var stored data.MCPToken
		if err := tx.First(&stored, "id = ?", created.Data.ID).Error; err != nil {
			t.Fatalf("the token was not stored: %v", err)
		}
		if stored.TokenHash != data.HashMCPToken(created.Data.Token) || stored.TokenHash == created.Data.Token {
			t.Fatalf("the stored hash does not match the token")
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/mcp-tokens", nil)
		handler.GetMCPTokens(rec, req.WithContext(asUser(req.Context(), user)))
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), created.Data.Token) {
			t.Fatalf("the list must answer 200 without the token: %d %s", rec.Code, rec.Body.String())
		}
	})
}

func TestRevokeMCPToken_OnlyTheOwnerCanRevoke(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		owner := createUser(t, tx, "MCP owner")
		stranger := createUser(t, tx, "MCP stranger")
		_, hash, err := data.NewMCPTokenSecret()
		if err != nil {
			t.Fatalf("failed to generate a token: %v", err)
		}
		token := data.MCPToken{UserID: owner.ID, Name: "notebook", TokenHash: hash}
		create(t, tx, &token)

		revoke := func(user data.User) int {
			req := httptest.NewRequest(http.MethodDelete, "/mcp-tokens/"+token.ID, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", token.ID)
			ctx := context.WithValue(asUser(req.Context(), user), chi.RouteCtxKey, rctx)
			rec := httptest.NewRecorder()
			NewMCPTokenHandler(tx).RevokeMCPToken(rec, req.WithContext(ctx))
			return rec.Code
		}

		if code := revoke(stranger); code != http.StatusNotFound {
			t.Fatalf("someone else's token must answer 404, got %d", code)
		}
		if code := revoke(owner); code != http.StatusOK {
			t.Fatalf("the owner must be able to revoke, got %d", code)
		}

		var stored data.MCPToken
		if err := tx.First(&stored, "id = ?", token.ID).Error; err != nil || stored.RevokedAt == nil {
			t.Fatalf("the token was not revoked: %v %+v", err, stored)
		}
	})
}
