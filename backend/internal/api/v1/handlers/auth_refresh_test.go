package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/auth"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// callRefresh drives POST /auth/refresh behind the real RequireAuth, because the property
// that matters most - an expired token cannot be renewed - is enforced there, not in the
// handler.
func callRefresh(t *testing.T, tx *gorm.DB, token string) *httptest.ResponseRecorder {
	t.Helper()
	handler := middleware.RequireAuth(tx)(http.HandlerFunc(NewAuthHandler(tx).Refresh))

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRefresh_IssuesAFreshTokenFromTheCurrentUserRow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Refresher")
		// UpdateColumn skips the model hooks, which re-validate the phone on every update.
		if err := tx.Model(&data.User{}).Where("id = ?", user.ID).UpdateColumn("role", data.RoleAdmin).Error; err != nil {
			t.Fatalf("promote user: %v", err)
		}

		// Signed with a stale phone and role, as if both changed after login: the renewed
		// token must carry what the database says now.
		old, err := auth.GenerateToken(user.ID, "11900000000", data.RoleUser)
		if err != nil {
			t.Fatalf("sign old token: %v", err)
		}

		rec := callRefresh(t, tx, old)
		assertStatus(t, rec, http.StatusOK)

		var resp loginResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		claims, err := auth.ValidateToken(resp.Data.Token)
		if err != nil {
			t.Fatalf("the refreshed token does not validate: %v", err)
		}
		if claims["user_id"] != user.ID {
			t.Errorf("user_id: got %v, want %s", claims["user_id"], user.ID)
		}
		if claims["phone"] != user.Phone {
			t.Errorf("phone: got %v, want %s", claims["phone"], user.Phone)
		}
		if claims["role"] != data.RoleAdmin {
			t.Errorf("role: got %v, want %s", claims["role"], data.RoleAdmin)
		}
		exp := time.Unix(int64(claims["exp"].(float64)), 0)
		if remaining := time.Until(exp); remaining < auth.TokenLifetime-time.Minute {
			t.Errorf("the refreshed token should have a full lifetime, has %v left", remaining)
		}
	})
}

// Renewal extends a live session; it must never revive one that already ended.
func TestRefresh_RejectsAnExpiredToken(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Expired")

		expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": user.ID,
			"exp":     time.Now().Add(-time.Minute).Unix(),
		}).SignedString([]byte(os.Getenv("JWT_SECRET")))
		if err != nil {
			t.Fatalf("sign expired token: %v", err)
		}

		assertStatus(t, callRefresh(t, tx, expired), http.StatusUnauthorized)
	})
}

// A token for a user that was deleted since login is not renewable either.
func TestRefresh_RejectsATokenForAMissingUser(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		token, err := auth.GenerateToken("00000000-0000-0000-0000-000000000000", "11999999999", data.RoleUser)
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}

		assertStatus(t, callRefresh(t, tx, token), http.StatusUnauthorized)
	})
}
