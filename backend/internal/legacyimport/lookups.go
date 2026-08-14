package legacyimport

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// resolvedAccount is an account the alias reached, plus which alias reached it - the
// nickname id is recorded on the issuance so renaming an alias later propagates.
type resolvedAccount struct {
	id         string
	name       string
	nicknameID *string
}

// lookups holds everything a day's rows need, read once. A day has a few dozen rows over
// at most a couple of dozen distinct aliases, so resolving row by row would be dozens of
// round trips for the same handful of answers.
type lookups struct {
	// accounts is keyed by program id + the folded alias.
	accounts map[string]resolvedAccount
	// issuers is keyed by the folded block title.
	issuers map[string]string
	// airports holds the IATA codes that exist, keyed folded, valued as stored.
	airports map[string]string
	// alreadyImported holds the upper-cased locators an active issuance already has.
	alreadyImported map[string]bool

	pendingAccounts []PendingAccount
	pendingIssuers  []PendingIssuer
}

func newLookups(
	db *gorm.DB,
	day Day,
	programs map[string]data.LoyaltyProgram,
	mappings Mappings,
) (*lookups, error) {
	out := &lookups{
		accounts:        map[string]resolvedAccount{},
		issuers:         map[string]string{},
		airports:        map[string]string{},
		alreadyImported: map[string]bool{},
	}

	if err := out.loadAirports(db, day); err != nil {
		return nil, err
	}
	if err := out.loadExistingLocators(db, day); err != nil {
		return nil, err
	}
	if err := out.loadAccounts(db, day, programs, mappings); err != nil {
		return nil, err
	}
	if err := out.loadIssuers(db, day, mappings); err != nil {
		return nil, err
	}

	return out, nil
}

// fold is the comparison this package uses for anything a human typed: case and
// surrounding space do not distinguish two spellings of the same name. Accents are folded
// in the database instead, by unaccent(), because Go has no equivalent in the standard
// library and the queries have to fold the stored side anyway.
func fold(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func accountKey(programID, label string) string {
	return programID + "\x00" + fold(label)
}

func (l *lookups) issuerID(name string) string {
	return l.issuers[fold(name)]
}

func (l *lookups) account(program data.LoyaltyProgram, label string) (resolvedAccount, bool) {
	account, ok := l.accounts[accountKey(program.ID, label)]
	return account, ok
}

// airport returns the stored IATA code for a label, or "" plus a note when the airports
// table has never heard of it. The route is not what makes an issuance, so an unknown
// code costs the cell and not the row.
func (l *lookups) airport(label, field string, notes []string) (string, []string) {
	if strings.TrimSpace(label) == "" {
		return "", notes
	}
	if code, ok := l.airports[fold(label)]; ok {
		return code, notes
	}
	return "", append(notes, fmt.Sprintf("Aeroporto de %s %q não existe no cadastro — a emissão entra sem ele", field, label))
}

func (l *lookups) loadAirports(db *gorm.DB, day Day) error {
	labels := map[string]bool{}
	forEachRow(day, func(_ Block, row Row) {
		for _, label := range []string{row.OriginLabel, row.DestinationLabel} {
			if strings.TrimSpace(label) != "" {
				labels[strings.ToUpper(strings.TrimSpace(label))] = true
			}
		}
	})
	if len(labels) == 0 {
		return nil
	}

	var airports []data.Airport
	if err := db.Where("UPPER(iata_code) IN ?", keys(labels)).Find(&airports).Error; err != nil {
		return fmt.Errorf("legacyimport: loading airports: %w", err)
	}
	for _, airport := range airports {
		l.airports[fold(airport.IATACode)] = airport.IATACode
	}
	return nil
}

// loadExistingLocators finds which of the day's locators the database already carries on
// an active issuance. It is what makes re-running a day safe, and what quietly excludes
// the bookings the operators already registered through the panel.
func (l *lookups) loadExistingLocators(db *gorm.DB, day Day) error {
	locators := map[string]bool{}
	forEachRow(day, func(_ Block, row Row) {
		locators[strings.ToUpper(row.ProgramLocator)] = true
	})
	if len(locators) == 0 {
		return nil
	}

	var existing []string
	err := db.Model(&data.Issuance{}).
		Where("UPPER(program_locator) IN ?", keys(locators)).
		Where("active = ?", true).
		Pluck("program_locator", &existing).Error
	if err != nil {
		return fmt.Errorf("legacyimport: loading existing locators: %w", err)
	}
	for _, locator := range existing {
		l.alreadyImported[strings.ToUpper(locator)] = true
	}
	return nil
}

// loadAccounts resolves each (program, alias) pair the day uses, in this order:
// what the operator mapped on the screen, then a registered alias of the program, then
// the name of an account holder in that program. Whatever is left over is reported as
// pending, for the screen to ask about.
func (l *lookups) loadAccounts(
	db *gorm.DB,
	day Day,
	programs map[string]data.LoyaltyProgram,
	mappings Mappings,
) error {
	// Distinct (program, alias) pairs, with how many rows ride on each - the count is
	// what tells the operator whether a mapping is worth their attention.
	type pair struct {
		program data.LoyaltyProgram
		label   string
	}
	pairs := map[string]pair{}
	counts := map[string]int{}
	forEachRow(day, func(_ Block, row Row) {
		program, ok := programs[programKey(row.ProgramLabel)]
		if !ok || strings.TrimSpace(row.AccountLabel) == "" {
			return
		}
		key := accountKey(program.ID, row.AccountLabel)
		pairs[key] = pair{program: program, label: row.AccountLabel}
		counts[key]++
	})
	if len(pairs) == 0 {
		return nil
	}

	// The operator's own choices win over anything looked up: they are the answer to
	// this exact question, asked because the lookup had already failed.
	chosen := map[string]string{}
	for _, mapping := range mappings.Accounts {
		program, ok := programs[programKey(mapping.ProgramCode)]
		if !ok || mapping.AccountID == "" {
			continue
		}
		chosen[accountKey(program.ID, mapping.AccountLabel)] = mapping.AccountID
	}

	for key, p := range pairs {
		if accountID, ok := chosen[key]; ok {
			account, err := l.loadAccountByID(db, accountID)
			if err != nil {
				return err
			}
			if account.id != "" {
				l.accounts[key] = account
				continue
			}
		}

		account, err := l.findAccountByLabel(db, p.program, p.label)
		if err != nil {
			return err
		}
		if account.id != "" {
			l.accounts[key] = account
			continue
		}

		l.pendingAccounts = append(l.pendingAccounts, PendingAccount{
			ProgramCode:  p.program.Code,
			ProgramName:  p.program.Name,
			ProgramID:    p.program.ID,
			AccountLabel: p.label,
			RowCount:     counts[key],
		})
	}

	sort.Slice(l.pendingAccounts, func(i, j int) bool {
		if l.pendingAccounts[i].ProgramName != l.pendingAccounts[j].ProgramName {
			return l.pendingAccounts[i].ProgramName < l.pendingAccounts[j].ProgramName
		}
		return l.pendingAccounts[i].AccountLabel < l.pendingAccounts[j].AccountLabel
	})

	return nil
}

func (l *lookups) loadAccountByID(db *gorm.DB, accountID string) (resolvedAccount, error) {
	var account data.Account
	err := db.Preload("Holder").Preload("Nicknames").First(&account, "id = ?", accountID).Error
	if err != nil {
		// A mapping pointing at an account that is gone is treated as no mapping at
		// all: the row falls back to the lookup and, failing that, is asked about
		// again. Better than refusing the whole preview over a stale id.
		return resolvedAccount{}, nil
	}
	return resolvedAccount{id: account.ID, name: accountDisplayName(account)}, nil
}

// findAccountByLabel looks the alias up the two ways it can mean something: as a
// registered alias of an account in that program, or as the name of the person who holds
// one. Both fold case and accents, so "wilmara" reaches "Wilmara" and "jose" reaches
// "José" - the same comparison the panel's own search uses since the unaccent migration.
func (l *lookups) findAccountByLabel(
	db *gorm.DB,
	program data.LoyaltyProgram,
	label string,
) (resolvedAccount, error) {
	var nickname data.AccountNickname
	err := db.Preload("Account.Holder").Preload("Account.Nicknames").
		Where("loyalty_program_id = ?", program.ID).
		Where("unaccent(nickname) ILIKE unaccent(?)", label).
		First(&nickname).Error
	if err == nil && nickname.Account != nil {
		return resolvedAccount{
			id:         nickname.AccountID,
			name:       accountDisplayName(*nickname.Account),
			nicknameID: &nickname.ID,
		}, nil
	}
	if err != nil && !isNotFound(err) {
		return resolvedAccount{}, fmt.Errorf("legacyimport: searching account aliases: %w", err)
	}

	var account data.Account
	err = db.Preload("Holder").Preload("Nicknames").
		Joins("JOIN people ON people.id = accounts.holder_id").
		Where("accounts.loyalty_program_id = ?", program.ID).
		Where("unaccent(people.name) ILIKE unaccent(?)", label).
		First(&account).Error
	if err == nil {
		return resolvedAccount{id: account.ID, name: accountDisplayName(account)}, nil
	}
	if !isNotFound(err) {
		return resolvedAccount{}, fmt.Errorf("legacyimport: searching account holders: %w", err)
	}

	return resolvedAccount{}, nil
}

// loadIssuers resolves each block title to a user. Only an exact name match is taken as
// an answer; anything looser is offered as a suggestion for the operator to confirm,
// because a block title can name two people ("WILMARA/CLARA") and picking one of them is
// not something a query gets to decide.
func (l *lookups) loadIssuers(db *gorm.DB, day Day, mappings Mappings) error {
	counts := map[string]int{}
	names := map[string]string{}
	for _, block := range day.Blocks {
		// A block with no rows needs no issuer. The template ships six of them per day
		// and a quiet day fills two, so asking about the other four would be four
		// questions whose answer changes nothing.
		if strings.TrimSpace(block.IssuerName) == "" || len(block.Rows) == 0 {
			continue
		}
		names[fold(block.IssuerName)] = block.IssuerName
		counts[fold(block.IssuerName)] += len(block.Rows)
	}
	if len(names) == 0 {
		return nil
	}

	chosen := map[string]string{}
	for _, mapping := range mappings.Issuers {
		if mapping.UserID != "" {
			chosen[fold(mapping.IssuerName)] = mapping.UserID
		}
	}

	var users []data.User
	if err := db.Find(&users).Error; err != nil {
		return fmt.Errorf("legacyimport: loading users: %w", err)
	}

	for key, name := range names {
		if userID, ok := chosen[key]; ok && userExists(users, userID) {
			l.issuers[key] = userID
			continue
		}

		exact, suggestion := matchUserByName(users, name)
		if exact != "" {
			l.issuers[key] = exact
			continue
		}

		l.pendingIssuers = append(l.pendingIssuers, PendingIssuer{
			IssuerName:      name,
			SuggestedUserID: suggestion,
			RowCount:        counts[key],
		})
	}

	sort.Slice(l.pendingIssuers, func(i, j int) bool {
		return l.pendingIssuers[i].IssuerName < l.pendingIssuers[j].IssuerName
	})

	return nil
}

// matchUserByName returns an id to use outright and, failing that, one to suggest.
// The suggestion is a unique first-name match: the sheet writes "MARCUS" where the user
// is registered as "Marcus Andrade". Unique, because two Marcuses make the guess a
// coin flip, and the operator is the one who knows.
func matchUserByName(users []data.User, name string) (exact, suggestion string) {
	needle := fold(name)

	var prefixed []string
	for _, user := range users {
		stored := fold(user.Name)
		if stored == needle {
			return user.ID, ""
		}
		if firstToken(stored) == firstToken(needle) {
			prefixed = append(prefixed, user.ID)
		}
	}

	if len(prefixed) == 1 {
		return "", prefixed[0]
	}
	return "", ""
}

func firstToken(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func userExists(users []data.User, id string) bool {
	for _, user := range users {
		if user.ID == id {
			return true
		}
	}
	return false
}

// accountDisplayName mirrors how the panel labels an account, so the preview shows the
// operator the same words the grid will: holder, then login, then a registered alias,
// then a short id. The alias step matters here - an account created from an EMISSAO has
// no login, and one whose holder was never filled in would otherwise show up in the
// preview as a raw uuid, which tells the operator nothing about what they are confirming.
func accountDisplayName(account data.Account) string {
	if account.Holder != nil && account.Holder.Name != "" {
		return account.Holder.Name
	}
	if account.Login != nil && *account.Login != "" {
		return *account.Login
	}
	if len(account.Nicknames) > 0 && account.Nicknames[0].Nickname != "" {
		return account.Nicknames[0].Nickname
	}
	if len(account.ID) > 8 {
		return account.ID[:8]
	}
	return account.ID
}

func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func forEachRow(day Day, visit func(Block, Row)) {
	for _, block := range day.Blocks {
		for _, row := range block.Rows {
			visit(block, row)
		}
	}
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
