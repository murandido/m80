package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
)

// The panel no longer shows people and accounts as two screens. To the operator "Fulana's
// account" is one thing that happens to have a Smiles, an Azul and a LATAM login, so the
// accounts screen lists one row per holder, with every loyalty account of theirs inside it.
//
// People stay what they are in the schema - the bot, the sales screens and the
// administrator's payment report all read them - so this is only a different way of paging
// the same two tables. A row is a live person with whatever accounts they hold, possibly none:
// an administrator who holds nothing still needs somewhere to be edited. Every account has a
// holder (accounts.holder_id is NOT NULL since migration 75), so no account is left outside a
// row.

// holderRowOrder is the whitelist of sort keys over the rows below; the client's string only
// ever indexes it.
var holderRowOrder = map[string]string{
	"name_asc":     `label COLLATE "pt-BR-x-icu" ASC NULLS LAST, created_at ASC, id`,
	"name_desc":    `label COLLATE "pt-BR-x-icu" DESC NULLS LAST, created_at ASC, id`,
	"created_desc": "created_at DESC, id",
	"created_asc":  "created_at ASC, id",
}

// holderAccountFilter renders the conditions an account has to meet for the listing's
// program, category and search filters, over whatever alias the caller gave the accounts
// table. Every identifier is a compile-time constant; only the values are bound.
type holderAccountFilter struct {
	programID string
	category  string
	like      string
}

func (f holderAccountFilter) conditions(alias string) (string, []any) {
	clauses := []string{alias + ".deleted_at IS NULL"}
	var args []any

	if f.programID != "" {
		clauses = append(clauses, alias+".loyalty_program_id = ?")
		args = append(args, f.programID)
	}
	if f.category != "" {
		clauses = append(clauses, alias+".category = ?")
		args = append(args, f.category)
	}

	return strings.Join(clauses, " AND "), args
}

// search is the part of the account search the holder's own columns do not already cover:
// login, administrator, program and the account's aliases.
func (f holderAccountFilter) search(alias string) (string, []any) {
	clause := "(" + unaccentedLike(alias+".login") +
		" OR EXISTS (SELECT 1 FROM people AS administrators WHERE administrators.id = " + alias + ".administrator_id AND " +
		unaccentedLike("administrators.name") + ")" +
		" OR EXISTS (SELECT 1 FROM loyalty_programs WHERE loyalty_programs.id = " + alias + ".loyalty_program_id AND (" +
		unaccentedLike("loyalty_programs.name") + " OR " + unaccentedLike("loyalty_programs.code") + "))" +
		" OR " + nicknameMatch("account_nicknames", "account_id", alias, true) + ")"
	return clause, []any{f.like, f.like, f.like, f.like, f.like}
}

// With a search, a row is ranked by how it matched before the chosen order applies. The
// search reaches through a person's accounts - login, program, alias and administrator - so
// searching an administrator's name returns every holder whose account they run, and sorted
// by name alone the administrator herself could land at the bottom of her own search. The
// person whose own name is the term comes first, then names that start with it, then names
// with a word that starts with it, then names or aliases that merely contain it, and last
// the rows that matched only through something else.
const (
	holderRankExact = iota
	holderRankPrefix
	holderRankWordPrefix
	holderRankContains
	holderRankOther
)

// equalsUnaccented compares a column with the bound term ignoring case and accents - the
// exact-match counterpart of unaccentedLike.
func equalsUnaccented(column string) string {
	return "unaccent(lower(" + column + ")) = unaccent(lower(?))"
}

// personRank renders the rank of a person row and its bound arguments. fingerprint is the
// CPF the term normalizes to, when it is one: a whole CPF names exactly one person.
func personRank(q, fingerprint string) (string, []any) {
	exact := equalsUnaccented("people.name") +
		" OR EXISTS (SELECT 1 FROM person_nicknames WHERE person_nicknames.person_id = people.id AND " +
		equalsUnaccented("person_nicknames.nickname") + ")"
	args := []any{q, q}
	if fingerprint != "" {
		exact += " OR people.cpf_fingerprint = ?"
		args = append(args, fingerprint)
	}

	sql := fmt.Sprintf(`CASE
		WHEN %s THEN %d
		WHEN %s THEN %d
		WHEN %s THEN %d
		WHEN %s OR %s THEN %d
		ELSE %d END`,
		exact, holderRankExact,
		unaccentedLike("people.name"), holderRankPrefix,
		unaccentedLike("people.name"), holderRankWordPrefix,
		unaccentedLike("people.name"), nicknameMatch("person_nicknames", "person_id", "people", false), holderRankContains,
		holderRankOther)
	like := likePattern(q)
	args = append(args, q+"%", "% "+q+"%", like, like)
	return sql, args
}

// holderRowsQuery builds the rows the listing pages over - one per live person - and their
// bound arguments.
func holderRowsQuery(params listQuery, filter holderAccountFilter, complete *bool) (string, []any) {
	var personWhere []string
	var personArgs []any
	var searchedFingerprint string

	// Person rows. With a program or category chosen, only somebody holding a matching
	// account qualifies - a person with no accounts has neither.
	if filter.programID != "" || filter.category != "" {
		cond, args := filter.conditions("held")
		personWhere = append(personWhere, "EXISTS (SELECT 1 FROM accounts AS held WHERE held.holder_id = people.id AND "+cond+")")
		personArgs = append(personArgs, args...)
	}

	if params.Q != "" {
		// The person's own columns, as the old owners screen searched them, or any of their
		// accounts as the old accounts screen did.
		personSearch := "(" + unaccentedLike("people.name") +
			" OR " + unaccentedLike("people.email") +
			" OR " + unaccentedLike("people.phone") +
			" OR " + unaccentedLike("people.city") +
			" OR " + nicknameMatch("person_nicknames", "person_id", "people", false)
		personSearchArgs := []any{filter.like, filter.like, filter.like, filter.like, filter.like}

		if fingerprint, ok := searchFingerprint(params.Q); ok {
			personSearch += " OR people.cpf_fingerprint = ?"
			personSearchArgs = append(personSearchArgs, fingerprint)
			searchedFingerprint = fingerprint
		}

		accountSearch, accountSearchArgs := filter.search("held")
		personSearch += " OR EXISTS (SELECT 1 FROM accounts AS held WHERE held.holder_id = people.id AND held.deleted_at IS NULL AND " + accountSearch + "))"
		personSearchArgs = append(personSearchArgs, accountSearchArgs...)

		personWhere = append(personWhere, personSearch)
		personArgs = append(personArgs, personSearchArgs...)
	}

	// Complete means the whole row is: the person and every account under them. Incomplete
	// is anything short of that, which is what somebody filtering for work to do is after.
	incompleteAccount := "EXISTS (SELECT 1 FROM accounts AS held WHERE held.holder_id = people.id AND held.deleted_at IS NULL AND held.complete = false)"
	if complete != nil {
		if *complete {
			personWhere = append(personWhere, "people.complete = true AND NOT "+incompleteAccount)
		} else {
			personWhere = append(personWhere, "(people.complete = false OR "+incompleteAccount+")")
		}
	}

	// Without a search every row ranks the same, and the chosen order alone applies.
	rankSQL := "0"
	var rankArgs []any
	if params.Q != "" {
		rankSQL, rankArgs = personRank(params.Q, searchedFingerprint)
	}

	// Archived people are gone from the panel (DeletePerson); their archived accounts too.
	personWhere = append([]string{"people.deleted_at IS NULL"}, personWhere...)
	// The rank's arguments sit in the select list, so they come before the WHERE's.
	return "SELECT people.id, people.name AS label, people.created_at, " + rankSQL +
			" AS rank FROM people WHERE " + strings.Join(personWhere, " AND "),
		append(rankArgs, personArgs...)
}

// ListAccountHolders handles the GET /accounts/holders route.
// @Summary      Listar contas agrupadas por titular (paginado)
// @Description  Uma linha por titular vivo, com todas as contas de programa dele; pessoas sem conta também aparecem. Os filtros escolhem linhas: a linha traz todas as contas não arquivadas do titular, não só as que casaram. Com busca, quem casa pelo próprio nome vem primeiro (nome igual, depois começando pelo termo, depois uma palavra começando pelo termo, depois contendo), e só então quem casou por outro campo ou pelas contas; a ordem escolhida desempata dentro de cada faixa.
// @Tags         Contas
// @Produce      json
// @Security     BearerAuth
// @Param        q                   query  string  false  "Busca por nome, e-mail, telefone, cidade, apelido, CPF completo, login, administrador ou programa"
// @Param        order               query  string  false  "Ordenação"  Enums(name_asc, name_desc, created_desc, created_asc)
// @Param        loyalty_program_id  query  string  false  "Somente titulares com conta neste programa"
// @Param        category            query  string  false  "Somente titulares com conta nesta categoria"  Enums(unidentified, standard, injunction)
// @Param        complete            query  bool    false  "true: titular e todas as contas completos; false: algo incompleto"
// @Param        page                query  int     false  "Página (padrão 1)"
// @Param        limit               query  int     false  "Itens por página (padrão 20, máximo 100)"
// @Success      200  {object}  schemas.AccountHolderPaginatedResponse
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      500  {object}  schemas.ErrorResponse "Erro ao buscar contas"
// @Router       /accounts/holders [get]
func (h *AccountHandler) ListAccountHolders(w http.ResponseWriter, r *http.Request) {
	params := parseListQuery(r)
	filter := holderAccountFilter{
		programID: r.URL.Query().Get("loyalty_program_id"),
		category:  r.URL.Query().Get("category"),
		like:      likePattern(params.Q),
	}
	rowsSQL, args := holderRowsQuery(params, filter, parseOptionalBool(r.URL.Query().Get("complete")))

	var total int64
	if err := h.DB.Raw("SELECT COUNT(*) FROM ("+rowsSQL+") AS holder_rows", args...).
		Scan(&total).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to count accounts")
		return
	}

	var personIDs []string
	// rank first: with a search, how a row matched outweighs the chosen order, which then
	// settles the rows that matched the same way (see holderRankExact).
	pageSQL := "SELECT id FROM (" + rowsSQL + ") AS holder_rows ORDER BY rank ASC, " +
		orderClause(params.Order, holderRowOrder, holderRowOrder["name_asc"]) + " LIMIT ? OFFSET ?"
	if err := h.DB.Raw(pageSQL, append(args, params.Limit, (params.Page-1)*params.Limit)...).
		Scan(&personIDs).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch accounts")
		return
	}

	rows, err := h.loadHolderRows(personIDs)
	if err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch accounts")
		return
	}

	utils.SendJSON(w, r, http.StatusOK, "Accounts fetched successfully", schemas.AccountHolderPaginatedData{
		Rows:  rows,
		Total: total,
		Page:  params.Page,
		Limit: params.Limit,
	})
}

// loadHolderRows hydrates a page of people, in the order given: each with their aliases, and
// every live account under them in one query, so the CPF counter and the balances are
// computed once for the page.
func (h *AccountHandler) loadHolderRows(personIDs []string) ([]schemas.AccountHolderRow, error) {
	rows := make([]schemas.AccountHolderRow, 0, len(personIDs))
	if len(personIDs) == 0 {
		return rows, nil
	}

	var found []data.Person
	if err := h.DB.Preload("Nicknames").Where("id IN ?", personIDs).Find(&found).Error; err != nil {
		return nil, err
	}
	people := make(map[string]data.Person, len(found))
	for _, person := range found {
		people[person.ID] = person
	}

	// Oldest first inside a row, so a holder's accounts do not reshuffle between loads.
	var accounts []data.Account
	if err := h.DB.Preload("Nicknames").Preload("LoyaltyProgram").
		Where("holder_id IN ?", personIDs).Order("created_at ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}

	usage, err := h.cpfUsage(accounts)
	if err != nil {
		return nil, err
	}
	responses := make([]schemas.AccountResponse, 0, len(accounts))
	for _, account := range accounts {
		responses = append(responses, toAccountResponseWithUsage(account, usage[account.ID]))
	}
	fillExpectedBalances(h.DB, responses)
	fillLastSynced(h.DB, responses)

	byHolder := map[string][]schemas.AccountResponse{}
	for _, response := range responses {
		byHolder[*response.HolderID] = append(byHolder[*response.HolderID], response)
	}

	for _, id := range personIDs {
		person, ok := people[id]
		if !ok {
			// Deleted between the page query and this one - drop the row rather than render
			// a holder with no name.
			continue
		}
		held := byHolder[id]
		if held == nil {
			held = []schemas.AccountResponse{}
		}
		rows = append(rows, schemas.AccountHolderRow{Holder: toPersonResponse(person), Accounts: held})
	}

	return rows, nil
}
