package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/auth"
)

// loopbackTokenLifetime bounds the JWT minted for one call. A call that takes longer than this
// is already broken; a token that outlives it is only a liability.
const loopbackTokenLifetime = 5 * time.Minute

// apiError is a non-2xx answer from a /api/v1 handler. Its message is the handler's own, in
// Portuguese, which is exactly what the agent needs to understand a refusal.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
}

// envelope is utils.StandardResponse as it arrives on the wire.
type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// call runs one request through the /api/v1 router as who, and returns the envelope's data,
// with credentials and documents removed (see redact).
// path is relative to /api/v1 (e.g. "/issuances").
func (s *Server) call(ctx context.Context, who caller, method, path string, query url.Values, body any) (json.RawMessage, error) {
	token, err := auth.GenerateTokenWithLifetime(who.UserID, who.Phone, who.Role, loopbackTokenLifetime)
	if err != nil {
		return nil, fmt.Errorf("mcp: sign loopback token: %w", err)
	}

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("mcp: encode body: %w", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	target := path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	// The tool's context may descend from the /mcp request, which carries the outer router's
	// chi.Context. chi reuses a route context it finds instead of starting its own, so the
	// v1 router would route against the wrong path; shadowing it with a nil one makes the
	// loopback request route from scratch.
	ctx = context.WithValue(ctx, chi.RouteCtxKey, (*chi.Context)(nil))

	req, err := http.NewRequestWithContext(ctx, method, "http://loopback"+target, reader)
	if err != nil {
		return nil, fmt.Errorf("mcp: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	s.v1.ServeHTTP(rec, req)

	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		if rec.Code >= http.StatusBadRequest {
			return nil, &apiError{Status: rec.Code, Message: http.StatusText(rec.Code)}
		}
		return nil, fmt.Errorf("mcp: %s %s answered something other than JSON (status %d)", method, path, rec.Code)
	}
	if rec.Code >= http.StatusBadRequest || !env.Success {
		return nil, &apiError{Status: rec.Code, Message: env.Message}
	}
	if len(env.Data) == 0 {
		return json.RawMessage(`null`), nil
	}
	// Every answer goes through redact, not just the ones known to carry secrets: an
	// endpoint that starts embedding an account tomorrow must not leak its password today.
	return redact(env.Data), nil
}

// get is call for a GET that decodes data into out.
func (s *Server) get(ctx context.Context, who caller, path string, query url.Values, out any) error {
	raw, err := s.call(ctx, who, http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("mcp: decode %s: %w", path, err)
	}
	return nil
}
