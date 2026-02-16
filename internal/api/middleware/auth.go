package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/murandido/m-80-backend/internal/auth"
	"github.com/murandido/m-80-backend/internal/utils"
)

type ContextKey string

const UserContextKey ContextKey = "user_claims"

func RequireAuth() func(handler http.Handler) http.Handler {
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

			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAdmin() func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(UserContextKey).(jwt.MapClaims)
			if !ok {
				utils.SendError(w, r, http.StatusInternalServerError, "Access denied: Server error reading credentials")
				return
			}

			role, ok := claims["role"].(string)
			if !ok || role != "admin" {
				utils.SendError(w, r, http.StatusForbidden, "Access denied: Role not allowed")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
