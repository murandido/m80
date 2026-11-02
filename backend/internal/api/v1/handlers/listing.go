package handlers

import (
	"net/http"
	"strings"

	"gorm.io/gorm"
)

// Shared plumbing for the management listings behind the panel's cadastro screens.
// Every one of them was a bare Find() with no ORDER BY, which meant Postgres was free
// to return rows in heap order - so a list could reshuffle itself after any UPDATE.
// A default ordering is therefore not a feature here, it is the fix.

const (
	listDefaultLimit = 20
	listMaxLimit     = 100
)

// listQuery is the slice of the request every listing reads the same way.
type listQuery struct {
	Q     string
	Order string
	Page  int
	Limit int
}

func parseListQuery(r *http.Request) listQuery {
	limit := parsePositiveInt(r.URL.Query().Get("limit"), listDefaultLimit)
	if limit > listMaxLimit {
		limit = listMaxLimit
	}

	return listQuery{
		Q:     strings.TrimSpace(r.URL.Query().Get("q")),
		Order: strings.TrimSpace(r.URL.Query().Get("order")),
		Page:  parsePositiveInt(r.URL.Query().Get("page"), 1),
		Limit: limit,
	}
}

// paginate applies the window the request asked for.
//
// The arithmetic is one line, but it is the line an off-by-one hides in, and there
// are now five listings doing it. Callers run their Count on the same query first
// and then open a fresh Session, so the Offset/Limit applied here does not leak
// back into the count - see ListPeople for the shape.
func paginate(query *gorm.DB, params listQuery) *gorm.DB {
	return query.Offset((params.Page - 1) * params.Limit).Limit(params.Limit)
}

// textOrder renders an ORDER BY over a text column the way a Portuguese reader
// expects. The database collates by byte value, so "aleleli" sorts after "Wilmara"
// and "Ágata" after "Zeca" - which reads as a list that is not sorted at all.
//
// pt-BR-x-icu handles both case and accents. It ships with the postgres:16-alpine
// image every environment pins, so it is not an availability gamble; LOWER() would
// have been portable but still puts accented names at the end.
func textOrder(column, direction string) string {
	return column + ` COLLATE "pt-BR-x-icu" ` + direction
}

// orderClause resolves a client-supplied sort key against a whitelist.
//
// The client's string is never interpolated into SQL - it is only ever a map key, and
// an unrecognised one falls back to the caller's default. That is what keeps `order`
// from being an injection vector, and it also means a stale link with a sort key we no
// longer support renders a sensible page instead of a 500.
func orderClause(order string, allowed map[string]string, fallback string) string {
	if clause, ok := allowed[order]; ok {
		return clause
	}
	return fallback
}

// likeWildcardEscaper neutralizes the characters LIKE treats as special. Postgres uses
// backslash as the default escape character, so no ESCAPE clause is needed - and the
// backslash itself must be escaped too, or a term ending in "\" would swallow the
// closing "%" and silently turn the contains-match into a suffix match.
var likeWildcardEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// escapeLike makes a search term match literally inside a LIKE pattern. Without it a
// search for "50%" matched every row and "a_b" matched "axb", because the operator's
// text was spliced in as pattern syntax.
func escapeLike(q string) string {
	return likeWildcardEscaper.Replace(q)
}

// likePattern wraps a search term for a case-insensitive contains match. Returned
// separately from the query so callers can bind it once per column.
func likePattern(q string) string {
	return "%" + escapeLike(q) + "%"
}

// unaccentedLike renders a contains-match that ignores case and accents. ILIKE alone
// folds case but not diacritics, so an operator searching "joao" was told that "João
// Silva" does not exist - and registered the same client a second time.
//
// The column is always a compile-time constant supplied by the handler; nothing from
// the request reaches this string. The pattern stays a bound argument.
func unaccentedLike(column string) string {
	return "unaccent(" + column + ") ILIKE unaccent(?)"
}

// parseOptionalBool reads a tri-state filter. Absent leaves the listing unfiltered, and
// anything that is not exactly "true" or "false" counts as absent rather than as false -
// so a stray ?complete=maybe shows everything instead of silently hiding half the rows.
func parseOptionalBool(raw string) *bool {
	switch raw {
	case "true":
		value := true
		return &value
	case "false":
		value := false
		return &value
	default:
		return nil
	}
}

// nicknameMatch builds an EXISTS clause matching a listing's rows against their
// nicknames. It takes one bound argument: the LIKE pattern.
//
// A subquery rather than a join: an entity can carry several nicknames, and joining
// would return the same row once per nickname, which the callers - who Preload the
// nicknames anyway - would then have to de-duplicate.
//
// Every identifier here is a compile-time constant supplied by the handler; nothing
// from the request reaches the string.
func nicknameMatch(nicknameTable, foreignKey, parentTable string, softDeleted bool) string {
	clause := "EXISTS (SELECT 1 FROM " + nicknameTable +
		" WHERE " + nicknameTable + "." + foreignKey + " = " + parentTable + ".id"
	if softDeleted {
		// Only account_nicknames is archived rather than removed; without this an
		// archived alias would still surface the account it used to name.
		clause += " AND " + nicknameTable + ".deleted_at IS NULL"
	}
	return clause + " AND " + unaccentedLike(nicknameTable+".nickname") + ")"
}
