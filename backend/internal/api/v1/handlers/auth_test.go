package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"
)

type loginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Token string `json:"token"`
	} `json:"data"`
}

func callLogin(t *testing.T, tx *gorm.DB, phone, password string) *httptest.ResponseRecorder {
	t.Helper()
	// A successful login ends in auth.GenerateToken, which refuses to sign
	// without a secret; TestMain supplies one when the environment does not.

	handler := NewAuthHandler(tx)

	body, err := json.Marshal(map[string]string{"phone": phone, "password": password})
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Login(rec, req)
	return rec
}

// Regression test: users.phone is normalized on create/update to always
// include Brazil's optional mobile "nono dígito", but WhatsApp (and some
// people, out of habit) don't always include it - login must still resolve
// the same user regardless of which form was typed.
func TestLogin_AcceptsPhoneWithoutNinthDigit(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Tester")
		withoutNinthDigit := user.Phone[:2] + user.Phone[3:]

		rec := callLogin(t, tx, withoutNinthDigit, "irrelevant")
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var resp loginResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Data.Token == "" {
			t.Fatal("expected a non-empty token")
		}
	})
}

func TestLogin_WithNinthDigit_StillWorks(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Tester")

		rec := callLogin(t, tx, user.Phone, "irrelevant")
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}
	})
}
