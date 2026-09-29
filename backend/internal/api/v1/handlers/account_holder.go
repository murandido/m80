package handlers

import (
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
// the same two tables. A row is either a person (with whatever accounts they hold, possibly
// none: an administrator who holds nothing still needs somewhere to be edited) or an
// account that has no holder, which the API no longer lets anybody create but which older
// data still carries.

const (
	holderRowPerson  = "person"
	holderRowAccount = "account"
)

// holderRowOrphanLabel is how an orphan account is named in the listing: login, then its oldest
// alias - the same chain the panel falls back on when there is no holder to show.
const holderRowOrphanLabel = `COALESCE(
	NULLIF(accounts.login, ''),
	(SELECT nickname FROM account_nicknames
		WHERE account_nicknames.account_id = accounts.id
		  AND account_nicknames.deleted_at IS NULL
		ORDER BY account_nicknames.created_at ASC
		LIMIT 1)
)`

// holderRowOrder is the whitelist of sort keys over the union below; the client's string
// only ever indexes it.
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

// holderRowsQuery builds the union the listing pages over and its bound arguments.
func holderRowsQuery(params listQuery, filter holderAccountFilter, complete *bool) (string, []any) {
	var personWhere, orphanWhere []string
	var personArgs, orphanArgs []any

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

	// Orphan rows: one account each - archived ones included. They are invisible everywhere
	// else in the panel, but a NOT NULL on holder_id counts every row in the table, so they
	// need a holder as much as the living ones do.
	orphanWhere = append(orphanWhere, "accounts.holder_id IS NULL")
	if filter.programID != "" {
		orphanWhere = append(orphanWhere, "accounts.loyalty_program_id = ?")
		orphanArgs = append(orphanArgs, filter.programID)
	}
	if filter.category != "" {
		orphanWhere = append(orphanWhere, "accounts.category = ?")
		orphanArgs = append(orphanArgs, filter.category)
	}
	if params.Q != "" {
		search, searchArgs := filter.search("accounts")
		orphanWhere = append(orphanWhere, search)
		orphanArgs = append(orphanArgs, searchArgs...)
	}
	if complete != nil {
		// An account with no holder is incomplete by definition (Account.ComputeComplete
		// requires one), so "complete" can never match it.
		if *complete {
			orphanWhere = append(orphanWhere, "false")
		}
	}

	// Archived people are gone from the panel (DeletePerson); their archived accounts too.
	personWhere = append([]string{"people.deleted_at IS NULL"}, personWhere...)
	personSQL := "SELECT '" + holderRowPerson + "' AS kind, people.id, people.name AS label, people.created_at FROM people" +
		" WHERE " + strings.Join(personWhere, " AND ")
	orphanSQL := "SELECT '" + holderRowAccount + "' AS kind, accounts.id, " + holderRowOrphanLabel +
		" AS label, accounts.created_at FROM accounts WHERE " + strings.Join(orphanWhere, " AND ")

	return personSQL + " UNION ALL " + orphanSQL, append(personArgs, orphanArgs...)
}

type holderRowKey struct {
	Kind string
	ID   string
}

// ListAccountHolders handles the GET /accounts/holders route.
// @Summary      Listar contas agrupadas por titular (paginado)
// @Description  Uma linha por titular, com todas as contas de programa dele; pessoas sem conta e contas sem titular aparecem em linha própria. Os filtros escolhem linhas: a linha traz todas as contas não arquivadas do titular, não só as que casaram.
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

	var keys []holderRowKey
	pageSQL := "SELECT kind, id FROM (" + rowsSQL + ") AS holder_rows ORDER BY " +
		orderClause(params.Order, holderRowOrder, holderRowOrder["name_asc"]) + " LIMIT ? OFFSET ?"
	if err := h.DB.Raw(pageSQL, append(args, params.Limit, (params.Page-1)*params.Limit)...).
		Scan(&keys).Error; err != nil {
		utils.SendError(w, r, http.StatusInternalServerError, "Failed to fetch accounts")
		return
	}

	rows, err := h.loadHolderRows(keys)
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

// loadHolderRows hydrates a page of row keys, in the order given: the people with their
// aliases, and every live account under them (or the orphan itself) in one query, so the
// CPF counter and the balances are computed once for the page as ListAccounts does.
func (h *AccountHandler) loadHolderRows(keys []holderRowKey) ([]schemas.AccountHolderRow, error) {
	rows := make([]schemas.AccountHolderRow, 0, len(keys))
	if len(keys) == 0 {
		return rows, nil
	}

	var personIDs, orphanIDs []string
	for _, key := range keys {
		if key.Kind == holderRowPerson {
			personIDs = append(personIDs, key.ID)
		} else {
			orphanIDs = append(orphanIDs, key.ID)
		}
	}

	people := map[string]data.Person{}
	if len(personIDs) > 0 {
		var found []data.Person
		if err := h.DB.Preload("Nicknames").Where("id IN ?", personIDs).Find(&found).Error; err != nil {
			return nil, err
		}
		for _, person := range found {
			people[person.ID] = person
		}
	}

	// Oldest first inside a row, so a holder's accounts do not reshuffle between loads.
	var accounts []data.Account
	if len(personIDs) > 0 {
		if err := h.DB.Preload("Nicknames").Preload("LoyaltyProgram").
			Where("holder_id IN ?", personIDs).Order("created_at ASC").Find(&accounts).Error; err != nil {
			return nil, err
		}
	}
	// Unscoped: an orphan row may be an archived account (see holderRowsQuery).
	if len(orphanIDs) > 0 {
		var orphans []data.Account
		if err := h.DB.Unscoped().Preload("Nicknames").Preload("LoyaltyProgram").
			Where("id IN ?", orphanIDs).Find(&orphans).Error; err != nil {
			return nil, err
		}
		accounts = append(accounts, orphans...)
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
	byID := map[string]schemas.AccountResponse{}
	for _, response := range responses {
		byID[response.ID] = response
		if response.HolderID != nil {
			byHolder[*response.HolderID] = append(byHolder[*response.HolderID], response)
		}
	}

	for _, key := range keys {
		if key.Kind == holderRowPerson {
			person, ok := people[key.ID]
			if !ok {
				// Deleted between the page query and this one - drop the row rather than
				// render a holder with no name.
				continue
			}
			holder := toPersonResponse(person)
			held := byHolder[key.ID]
			if held == nil {
				held = []schemas.AccountResponse{}
			}
			rows = append(rows, schemas.AccountHolderRow{Holder: &holder, Accounts: held})
			continue
		}

		if account, ok := byID[key.ID]; ok {
			rows = append(rows, schemas.AccountHolderRow{Accounts: []schemas.AccountResponse{account}})
		}
	}

	return rows, nil
}
