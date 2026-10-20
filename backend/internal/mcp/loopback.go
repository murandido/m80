package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/auth"
)

const (
	// The JWT minted for each loopback call. It never leaves the process; it only has to
	// outlive the one request it is attached to.
	loopbackTokenTTL = 5 * time.Minute
	loopbackTimeout  = 60 * time.Second
)

// apiError is a non-2xx answer from the v1 router, kept with its status so the tool result can
// tell "not found" and "not allowed" apart from "broken".
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("API respondeu %d: %s", e.Status, e.Message)
}

// loopback calls the API's own router in-process, as the caller. Going through the router
// rather than the database is the point of the design: RequireAuth reloads the role,
// agency scope narrows every listing, RequireAdmin gates the inventory, and the handlers
// validate input - none of it reimplemented here, so none of it can drift.
type loopback struct {
	api http.Handler
}

func (l loopback) call(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
	who, ok := callerFrom(ctx)
	if !ok {
		return nil, errors.New("requisição sem token MCP")
	}

	token, err := auth.GenerateTokenWithTTL(who.user.ID, who.user.Phone, who.user.Role, loopbackTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("gerar token interno: %w", err)
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("serializar corpo: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	target := "/api/v1" + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	ctx, cancel := context.WithTimeout(ctx, loopbackTimeout)
	defer cancel()
	// The /mcp request already went through chi, and chi reuses a route context it finds on
	// the request as if it were routing a sub-router - the loopback would resolve against the
	// /mcp match and answer 405. A fresh call starts with no routing state.
	ctx = context.WithValue(ctx, chi.RouteCtxKey, (*chi.Context)(nil))
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("montar requisição: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.RemoteAddr = who.remoteAddr

	rec := httptest.NewRecorder()
	l.api.ServeHTTP(rec, req)

	// Exports and PDFs answer with a file; an MCP tool has nowhere to put one.
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		return nil, &apiError{Status: rec.Code, Message: "resposta não é JSON (" + contentType + "); use a tela do painel para arquivos"}
	}

	var envelope struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		return nil, &apiError{Status: rec.Code, Message: "resposta ilegível"}
	}
	if rec.Code < 200 || rec.Code >= 300 || !envelope.Success {
		return nil, &apiError{Status: rec.Code, Message: envelope.Message}
	}
	return envelope.Data, nil
}
