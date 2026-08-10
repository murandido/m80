package middleware

import (
	"context"
	"errors"
	"log/slog"
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

			// "The user is gone" and "I could not ask" are different answers and
			// must not share a status. Every error used to become a 401, so a
			// dropped connection, a statement timeout or a lock held by someone
			// working in psql read to the panel as an expired session - and the
			// panel logs out on 401. A signed, unexpired token is still valid
			// when the database hiccups; say the truth (503) and let it retry.
			var user data.User
			err = db.Select("id", "role").First(&user, "id = ?", userId).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				utils.SendError(w, r, http.StatusUnauthorized, "Access denied: User no longer exists or was deactivated")
				return
			case err != nil:
				slog.Error("auth: failed to load the authenticated user", "user_id", userId, "err", err)
				utils.SendError(w, r, http.StatusServiceUnavailable, "Serviço temporariamente indisponível")
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

			// Anything that is not admin is denied, dev included: adding the dev role was
			// deliberately kept free of authorization changes, so it inherits the plain
			// user's access until permissions are redistributed.
			if user.Role != data.RoleAdmin {
				utils.SendError(w, r, http.StatusForbidden, "Access denied: Role not allowed")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
