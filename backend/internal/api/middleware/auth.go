package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/auth"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type ContextKey string

const UserContextKey ContextKey = "user_claims"

// AgencyScopeContextKey holds the access.AgencyScope of the authenticated user. It lives next
// to the user rather than inside data.User because it is not a column of users, and loading it
// here - once per request - is what lets every handler filter without a query of its own.
const AgencyScopeContextKey ContextKey = "agency_scope"

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

			// Same reasoning as above: failing to read the grants is an outage, not a
			// verdict on the user, and treating it as "no agencies" would show an empty
			// panel that looks exactly like data loss.
			scope, err := access.LoadAgencyScope(db, user)
			if err != nil {
				slog.Error("auth: failed to load the user's agency scope", "user_id", userId, "err", err)
				utils.SendError(w, r, http.StatusServiceUnavailable, "Serviço temporariamente indisponível")
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			ctx = context.WithValue(ctx, AgencyScopeContextKey, scope)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole gates a route on the role the auth middleware loaded from the database.
// dev passes every gate - that is the whole definition of the role - so it is never named
// at a call site and therefore can never be forgotten at one.
//
// A missing context user is a 500, not a 403: it means the route was mounted outside
// RequireAuth, which is a wiring mistake in the router rather than a permission decision.
func RequireRole(allowed ...string) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(UserContextKey).(data.User)
			if !ok {
				utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
				return
			}

			if user.Role != data.RoleDev && !slices.Contains(allowed, user.Role) {
				utils.SendError(w, r, http.StatusForbidden, "Access denied: Role not allowed")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin allows admin - and dev, through RequireRole.
func RequireAdmin() func(handler http.Handler) http.Handler {
	return RequireRole(data.RoleAdmin)
}

// RequireDev allows nobody but dev, hence the empty list.
func RequireDev() func(handler http.Handler) http.Handler {
	return RequireRole()
}

// AgencyScopeFrom returns the agency scope RequireAuth stored. A request without one was
// never authenticated, so it gets an empty scope - "no agency" - instead of a full one: a
// wiring mistake has to fail closed.
func AgencyScopeFrom(ctx context.Context) access.AgencyScope {
	scope, _ := ctx.Value(AgencyScopeContextKey).(access.AgencyScope)
	return scope
}
