package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// LinkedProgramContextKey holds the loyalty program code ("AZUL", "SMILES") of the account a
// /portal route is acting on, read once by RequireLinkedAccount so handlers can dispatch on it.
const LinkedProgramContextKey ContextKey = "linked_program_code"

// RequireLinkedAccount lets a statement client through only to an account granted to them in
// user_accounts that still exists. Anything else is a 404, never a 403: whether an account
// exists at all is not something an outsider gets to learn. There is no bypass for staff on
// purpose - they never use /portal, and a bypass is one more way for the gate to be wrong.
func RequireLinkedAccount(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(UserContextKey).(data.User)
			if !ok {
				utils.SendError(w, r, http.StatusInternalServerError, "Internal Server Error: Could not read user context")
				return
			}

			// A malformed id cannot be anyone's account; answering before the query also keeps
			// Postgres from erroring on the uuid cast.
			accountID := chi.URLParam(r, "id")
			if uuid.Validate(accountID) != nil {
				utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
				return
			}

			var linked struct{ Code string }
			err := db.Table("user_accounts").
				Select("loyalty_programs.code AS code").
				Joins("JOIN accounts ON accounts.id = user_accounts.account_id AND accounts.deleted_at IS NULL").
				Joins("JOIN loyalty_programs ON loyalty_programs.id = accounts.loyalty_program_id").
				Where("user_accounts.user_id = ? AND user_accounts.account_id = ?", user.ID, accountID).
				Take(&linked).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				utils.SendError(w, r, http.StatusNotFound, "Conta não encontrada")
				return
			case err != nil:
				slog.Error("portal: checking the account grant", "user_id", user.ID, "err", err)
				utils.SendError(w, r, http.StatusServiceUnavailable, "Serviço temporariamente indisponível")
				return
			}

			ctx := context.WithValue(r.Context(), LinkedProgramContextKey, linked.Code)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// LinkedProgramFrom returns the program code RequireLinkedAccount stored, or "" outside it.
func LinkedProgramFrom(ctx context.Context) string {
	code, _ := ctx.Value(LinkedProgramContextKey).(string)
	return code
}
