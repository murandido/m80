// Package access decides which agencies a panel user may see and act on. It is shared by
// the API (loaded once per request in middleware.RequireAuth) and by the bot, which acts on
// behalf of the same users - a rule enforced in only one of the two would be a rule anybody
// could walk around by switching channel.
package access

import (
	"errors"
	"slices"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// ErrNoAgencyAccess means the user may not see any agency at all, so there is nothing to
// fall back to. Callers turn it into an empty result or a refusal, never into "every agency".
var ErrNoAgencyAccess = errors.New("user has no agency access")

// AgencyScope is the set of agencies a user is allowed to touch. All is true for dev only:
// the role itself means "every agency", including ones created after the grant was made.
// An empty IDs with All false means "none" - never "everything".
type AgencyScope struct {
	All bool
	IDs []string
}

// FullScope is what dev gets, and what internal callers without a user (jobs, migrations of
// data) use explicitly instead of passing a zero value that would mean "no access".
func FullScope() AgencyScope {
	return AgencyScope{All: true}
}

// LoadAgencyScope reads the grants of a user from user_agencies. dev never reads the table.
func LoadAgencyScope(db *gorm.DB, user data.User) (AgencyScope, error) {
	if user.Role == data.RoleDev {
		return FullScope(), nil
	}

	ids := make([]string, 0, 4)
	if err := db.Model(&data.UserAgency{}).Where("user_id = ?", user.ID).Pluck("agency_id", &ids).Error; err != nil {
		return AgencyScope{}, err
	}
	return AgencyScope{IDs: ids}, nil
}

// Allows reports whether the agency is inside the scope.
func (s AgencyScope) Allows(agencyID string) bool {
	return s.All || slices.Contains(s.IDs, agencyID)
}

// Restrict narrows a query to rows whose column is an allowed agency. An empty scope turns
// into a predicate that matches nothing rather than skipping the WHERE: a skipped WHERE
// would hand an unassigned user every agency, which is exactly what this exists to prevent.
func (s AgencyScope) Restrict(query *gorm.DB, column string) *gorm.DB {
	if s.All {
		return query
	}
	if len(s.IDs) == 0 {
		return query.Where("1 = 0")
	}
	return query.Where(column+" IN ?", s.IDs)
}

// Intersect keeps only the requested ids the scope allows. A nil request means "every agency
// the user can see" and comes back as the scope itself (nil when the scope is All).
func (s AgencyScope) Intersect(requested []string) []string {
	if requested == nil {
		if s.All {
			return nil
		}
		// Not slices.Clone: it returns nil for an empty scope, and nil here means
		// "every agency" - the one answer an unassigned user must never get.
		return append(make([]string, 0, len(s.IDs)), s.IDs...)
	}
	kept := make([]string, 0, len(requested))
	for _, id := range requested {
		if s.Allows(id) {
			kept = append(kept, id)
		}
	}
	return kept
}

// ResolveDefaultAgency picks the agency a view lands on, and the one an issuance goes to,
// when none was named. It is the "agência principal" whenever the user may see it; a user
// restricted away from it lands on their first agency in name order - the same order the
// agencies list shows - so the panel's own fallback to the first item agrees with this one.
func ResolveDefaultAgency(db *gorm.DB, scope AgencyScope) (string, error) {
	var agency data.Agency
	err := scope.Restrict(db.Model(&data.Agency{}), "agencies.id").
		Select("agencies.id").
		Order("agencies.is_default DESC").
		Order(`agencies.name COLLATE "pt-BR-x-icu" ASC`).
		First(&agency).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNoAgencyAccess
	}
	if err != nil {
		return "", err
	}
	return agency.ID, nil
}
