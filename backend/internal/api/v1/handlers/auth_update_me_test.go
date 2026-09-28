package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// createUser seeds every user with this password.
const seededPassword = "irrelevant"

// callUpdateMe drives UpdateMe with the user injected the way RequireAuth does.
func callUpdateMe(t *testing.T, tx *gorm.DB, user data.User, payload map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/auth/me", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, data.User{ID: user.ID, Role: user.Role}))
	rec := httptest.NewRecorder()
	NewAuthHandler(tx).UpdateMe(rec, req)
	return rec
}

func reloadUser(t *testing.T, tx *gorm.DB, id string) data.User {
	t.Helper()
	var user data.User
	if err := tx.First(&user, "id = ?", id).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	return user
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, want, rec.Body.String())
	}
}

func TestUpdateMe_NameAloneNeedsNoPassword(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Before")

		assertStatus(t, callUpdateMe(t, tx, user, map[string]string{"name": "After"}), http.StatusOK)

		if got := reloadUser(t, tx, user.ID).Name; got != "After" {
			t.Fatalf("name: got %q, want %q", got, "After")
		}
	})
}

// Retyping the same number without the ninth digit is not a phone change.
func TestUpdateMe_SamePhoneInOtherFormNeedsNoPassword(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Tester")
		withoutNinthDigit := user.Phone[:2] + user.Phone[3:]

		assertStatus(t, callUpdateMe(t, tx, user, map[string]string{"name": "Tester", "phone": withoutNinthDigit}), http.StatusOK)
	})
}

func TestUpdateMe_NewPasswordRequiresCurrentPassword(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Tester")

		assertStatus(t, callUpdateMe(t, tx, user, map[string]string{"new_password": "brand-new"}), http.StatusForbidden)
		assertStatus(t, callUpdateMe(t, tx, user, map[string]string{"new_password": "brand-new", "current_password": "wrong-one"}), http.StatusForbidden)
		if reloaded := reloadUser(t, tx, user.ID); !reloaded.CheckPassword(seededPassword) {
			t.Fatal("password changed despite the rejected requests")
		}

		assertStatus(t, callUpdateMe(t, tx, user, map[string]string{"new_password": "brand-new", "current_password": seededPassword}), http.StatusOK)
		assertStatus(t, callLogin(t, tx, user.Phone, "brand-new"), http.StatusOK)
	})
}

func TestUpdateMe_PhoneChangeRequiresCurrentPassword(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Tester")
		spare := createUser(t, tx, "Spare")
		newPhone := spare.Phone
		// Free the spare's number so the only thing standing in the way is the password.
		if err := tx.Unscoped().Delete(&spare).Error; err != nil {
			t.Fatalf("delete spare: %v", err)
		}

		assertStatus(t, callUpdateMe(t, tx, user, map[string]string{"phone": newPhone}), http.StatusForbidden)
		assertStatus(t, callUpdateMe(t, tx, user, map[string]string{"phone": newPhone, "current_password": seededPassword}), http.StatusOK)

		if got := reloadUser(t, tx, user.ID).Phone; got != newPhone {
			t.Fatalf("phone: got %q, want %q", got, newPhone)
		}
	})
}

func TestUpdateMe_PhoneOfAnotherUserIsUnprocessable(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Tester")
		other := createUser(t, tx, "Other")

		// Last statement in the transaction: the unique violation aborts it.
		assertStatus(t, callUpdateMe(t, tx, user, map[string]string{"phone": other.Phone, "current_password": seededPassword}), http.StatusUnprocessableEntity)
	})
}

func TestUpdateMe_RoleIsIgnored(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Tester")

		assertStatus(t, callUpdateMe(t, tx, user, map[string]string{"name": "Tester", "role": "admin"}), http.StatusOK)

		if got := reloadUser(t, tx, user.ID).Role; got != "user" {
			t.Fatalf("role: got %q, want %q", got, "user")
		}
	})
}
