package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/murandido/m-80-backend/internal/auth"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type ContextKey string

const UserContextKey ContextKey = "user_claims"

// RequireAuth validates the JWT and ensures the user still exists in the database
func RequireAuth(db *gorm.DB) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				utils.SendError(w, r, http.StatusUnauthorized, "Access denied: Token not provided")
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				utils.SendError(w, r, http.StatusUnauthorized, "Access denied: Malformatted token")
				return
			}

			tokenString := parts[1]
			claims, err := auth.ValidateToken(tokenString)
			if err != nil {
				utils.SendError(w, r, http.StatusUnauthorized, "Access denied: Token not valid or expired")
				return
			}

			userId, ok := claims["user_id"].(string)
			if !ok {
				utils.SendError(w, r, http.StatusUnauthorized, "Access denied: Invalid token payload")
				return
			}

			var user data.User
			if err := db.Select("id", "role").First(&user, "id = ?", userId).Error; err != nil {
				utils.SendError(w, r, http.StatusUnauthorized, "Access denied: User no longer exists or was deactivated")
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin checks the fresh database data put in the context
func RequireAdmin() func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(UserContextKey).(data.User)
			if !ok {
				utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
				return
			}

			if user.Role != "admin" {
				utils.SendError(w, r, http.StatusForbidden, "Access denied: Role not allowed")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
