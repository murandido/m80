package mcp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type callerContextKey struct{}

// caller is who an MCP request acts as: the owner of the token, plus where the request came
// from so the loopback calls carry the same address into the v1 router's logs and limits.
type caller struct {
	user       data.User
	tokenID    string
	remoteAddr string
}

func callerFrom(ctx context.Context) (caller, bool) {
	c, ok := ctx.Value(callerContextKey{}).(caller)
	return c, ok
}

// requireToken accepts only an MCP token - never a login JWT. Keeping the two apart is what
// lets a leaked MCP token be revoked on its own, and what keeps an MCP token from reaching
// any /api/v1 route directly.
func requireToken(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !found || token == "" {
				utils.SendError(w, r, http.StatusUnauthorized, "Access denied: MCP token not provided")
				return
			}

			var stored data.MCPToken
			err := db.Where("token_hash = ? AND revoked_at IS NULL", data.HashMCPToken(token)).First(&stored).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				utils.SendError(w, r, http.StatusUnauthorized, "Access denied: MCP token not valid or revoked")
				return
			case err != nil:
				slog.Error("mcp: failed to load the token", "err", err)
				utils.SendError(w, r, http.StatusServiceUnavailable, "Serviço temporariamente indisponível")
				return
			}

			// The owner is loaded on every request, like RequireAuth does: a deactivated user's
			// token stops working the moment they are deactivated, not when someone remembers it.
			var user data.User
			err = db.Select("id", "phone", "role").First(&user, "id = ?", stored.UserID).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				utils.SendError(w, r, http.StatusUnauthorized, "Access denied: User no longer exists or was deactivated")
				return
			case err != nil:
				slog.Error("mcp: failed to load the token owner", "token_id", stored.ID, "err", err)
				utils.SendError(w, r, http.StatusServiceUnavailable, "Serviço temporariamente indisponível")
				return
			}

			// Best effort: a failed touch must not cost the call, it only blurs "last used".
			if err := db.Model(&stored).UpdateColumn("last_used_at", time.Now()).Error; err != nil {
				slog.Warn("mcp: failed to record the token use", "token_id", stored.ID, "err", err)
			}

			ctx := context.WithValue(r.Context(), callerContextKey{}, caller{
				user:       user,
				tokenID:    stored.ID,
				remoteAddr: r.RemoteAddr,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
