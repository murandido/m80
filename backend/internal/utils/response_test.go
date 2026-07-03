package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
