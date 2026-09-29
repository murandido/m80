package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// invalidTextRepresentation is Postgres' answer to an id that is not a UUID at all.
const invalidTextRepresentation = "22P02"

// RequireIssuanceInScope guards every /issuances/{id}/... route in one place: an issuance
// whose agency is outside the caller's scope answers 404, exactly like one that does not
// exist, so a restricted user cannot even learn which ids are real.
//
// It lives on the route group rather than in each handler because that group keeps growing
// (seats, baggage, ticket, passengers...), and a check that has to be remembered per handler
// is one that eventually is not.
//
// Unscoped on purpose: a soft-deleted issuance still belongs to an agency, and the handler
// decides what "deleted" answers; this only decides whose it is.
func RequireIssuanceInScope(db *gorm.DB) func(http.Handler) http.Handler {
	return requireRecordInScope(db, "Issuance not found",
		"SELECT agency_id FROM issuances WHERE id = ?")
}

// RequireSaleInScope is RequireIssuanceInScope for /sales/{id}/...: a sale has no agency of
// its own, it belongs to its issuance's.
func RequireSaleInScope(db *gorm.DB) func(http.Handler) http.Handler {
	return requireRecordInScope(db, "Sale not found",
		"SELECT issuances.agency_id FROM sales JOIN issuances ON issuances.id = sales.issuance_id WHERE sales.id = ?")
}

// RequireClientInScope is RequireIssuanceInScope for /clients/{id}/.... A client still
// waiting for an agency (NULL, see migration 000072) belongs to nobody's scope but a dev's -
// the COALESCE turns it into an id no grant can match.
func RequireClientInScope(db *gorm.DB) func(http.Handler) http.Handler {
	return requireRecordInScope(db, "Client not found",
		"SELECT COALESCE(agency_id::text, '') FROM clients WHERE id = ?")
}

// requireRecordInScope runs agencyQuery with the {id} URL param and refuses the request when
// the agency it returns is outside the caller's scope. An id that matches nothing is passed
// through untouched: the handler already owns that 404 - and the 400 for a malformed id -
// with its own message.
func requireRecordInScope(db *gorm.DB, notFound, agencyQuery string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope := AgencyScopeFrom(r.Context())
			if scope.All {
				next.ServeHTTP(w, r)
				return
			}

			var agencyIDs []string
			err := db.Raw(agencyQuery, chi.URLParam(r, "id")).Scan(&agencyIDs).Error
			var pgErr *pgconn.PgError
			switch {
			case errors.As(err, &pgErr) && pgErr.Code == invalidTextRepresentation,
				err == nil && len(agencyIDs) == 0:
				next.ServeHTTP(w, r)
				return
			case err != nil:
				slog.Error("scope: failed to load the record's agency", "err", err)
				utils.SendError(w, r, http.StatusServiceUnavailable, "Serviço temporariamente indisponível")
				return
			}

			if !scope.Allows(agencyIDs[0]) {
				utils.SendError(w, r, http.StatusNotFound, notFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
