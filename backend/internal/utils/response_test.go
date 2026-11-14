package utils

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	SendJSON(w, r, http.StatusOK, "success message", map[string]string{"key": "value"})

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("status code: got %d, want %d", res.StatusCode, http.StatusOK)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}

	var body StandardResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if !body.Success {
		t.Error("expected success:true")
	}
	if body.Message != "success message" {
		t.Errorf("message: got %q, want %q", body.Message, "success message")
	}
	if body.Data == nil {
		t.Error("expected data to be present")
	}
}

func TestSendJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	SendJSON(w, r, http.StatusCreated, "created", nil)

	res := w.Result()
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status code: got %d, want %d", res.StatusCode, http.StatusCreated)
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if _, exists := raw["data"]; exists {
		t.Error("expected 'data' field to be omitted when nil (omitempty)")
	}
}

func TestSendError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	SendError(w, r, http.StatusUnauthorized, "unauthorized")

	res := w.Result()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status code: got %d, want %d", res.StatusCode, http.StatusUnauthorized)
	}

	var body StandardResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Success {
		t.Error("expected success:false")
	}
	if body.Message != "unauthorized" {
		t.Errorf("message: got %q, want %q", body.Message, "unauthorized")
	}
	if body.Data != nil {
		t.Errorf("expected data:null, got %v", body.Data)
	}
}

func TestSendError_StatusCodes(t *testing.T) {
	cases := []int{
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}

	for _, code := range cases {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		SendError(w, r, code, "error")

		if w.Result().StatusCode != code {
			t.Errorf("status %d: got %d", code, w.Result().StatusCode)
		}
	}
}

// captureDefaultLogger points slog's default logger at a buffer for the duration of the test.
func captureDefaultLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buffer
}

func TestSendError_LogsServerErrors(t *testing.T) {
	buffer := captureDefaultLogger(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/issuances/abc?q=secret-search", nil)
	SendError(w, r, http.StatusInternalServerError, "Failed to update issuance: boom")

	logged := buffer.String()
	for _, want := range []string{
		`level=ERROR`,
		`msg="http 5xx"`,
		`method=PATCH`,
		`path=/api/v1/issuances/abc`,
		`status=500`,
		`message="Failed to update issuance: boom"`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("log line missing %q: %s", want, logged)
		}
	}
	if strings.Contains(logged, "secret-search") {
		t.Errorf("log line must not carry the query string: %s", logged)
	}
	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Errorf("status code: got %d, want 500", w.Result().StatusCode)
	}
}

func TestSendError_DoesNotLogClientErrors(t *testing.T) {
	buffer := captureDefaultLogger(t)

	for _, code := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusTooManyRequests} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		SendError(w, r, code, "client error")
	}

	if buffer.Len() != 0 {
		t.Errorf("expected no log for 4xx, got: %s", buffer.String())
	}
}
