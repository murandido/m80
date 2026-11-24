// Package injunction cross-checks a Tropa: the workbook its people keep, the issuances the
// panel holds and the airline statement of its injunction accounts. Some issuances live only
// in the workbook, which is why it is read every day.
//
// The workbook is parsed and resolved by internal/legacyimport, the same code the import
// screen uses. Two things are a Tropa's own: only rows of the accounts it maps are read (the
// same workbook carries accounts that are not injunctions), and the "Taxa de Resgate" column
// is ignored - Azul stopped charging it, and the importer would otherwise record it as the
// issuance's taxes.
package injunction

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/legacyimport"
	"github.com/murandido/m-80-backend/internal/statement"
	"gorm.io/gorm"
)

// Item is one workbook row or statement line that needs a decision.
type Item struct {
	Day                string  `json:"day,omitempty"`
	BlockIndex         int     `json:"block_index"`
	Number             int     `json:"number"`
	Locator            string  `json:"locator"`
	Program            string  `json:"program"`
	AccountID          string  `json:"account_id,omitempty"`
	AccountLabel       string  `json:"account_label,omitempty"`
	Points             int     `json:"points"`
	ValueOfTheThousand int     `json:"value_of_the_thousand"`
	Issuer             string  `json:"issuer,omitempty"`
	Passenger          string  `json:"passenger,omitempty"`
	StatementPoints    *int    `json:"statement_points,omitempty"`
	PanelIssuanceID    string  `json:"panel_issuance_id,omitempty"`
	Detail             string  `json:"detail"`
	Observation        *string `json:"observation,omitempty"`
}

// AccountStatement is how far the statement of one account could be read.
type AccountStatement struct {
	AccountID string `json:"account_id"`
	Program   string `json:"program"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Error     string `json:"error,omitempty"`
}

// CheckResult is the cross-check of a Tropa over a range of workbook days, in buckets by
// what each item needs.
type CheckResult struct {
	From       string             `json:"from"`
	To         string             `json:"to"`
	SheetRows  int                `json:"sheet_rows"`
	Statements []AccountStatement `json:"statements"`
	// In the workbook and the statement with the same points, not in the panel: two sources
	// agree, so it can be imported without asking anybody.
	MissingInPanelConfirmed []Item `json:"missing_in_panel_confirmed"`
	// In the workbook, not in the panel, and the statement does not (yet) back it up.
	MissingInPanelUnconfirmed []Item `json:"missing_in_panel_unconfirmed"`
	// In the workbook and the panel, with different points, rate or account.
	DiffersFromPanel []Item `json:"differs_from_panel"`
	// On the statement, missing from both the panel and the workbook.
	MissingInSheetAndPanel []Item `json:"missing_in_sheet_and_panel"`
	// Negative workbook rows (refunds) not yet applied in the panel.
	PendingCancellations []Item `json:"pending_cancellations"`
	// Rows that cannot be read: a locator without points or rate, a locator twice in a day.
	Invalid []Item `json:"invalid"`
	// Rows whose issuer or account has no mapping on the Tropa.
	NeedsMapping []Item `json:"needs_mapping"`
}

func fold(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// Mappings turns the Tropa's setup into the importer's. The importer wants a day agency, but
// it never decides anything here: every row the Tropa imports carries its issuer's own (see
// rowAgencies), and a row whose issuer has none is held back before the commit. Any issuer's
// agency fills it, so a Tropa never depends on the installation having a default agency.
func Mappings(group data.InjunctionGroup) legacyimport.Mappings {
	mappings := legacyimport.Mappings{}
	if agencyIDs := IssuerAgencyIDs(group); len(agencyIDs) > 0 {
		mappings.AgencyID = agencyIDs[0]
	}
	for _, account := range group.Mappings.Accounts {
		mappings.Accounts = append(mappings.Accounts, legacyimport.AccountMapping{
			ProgramCode: account.ProgramCode, AccountLabel: account.AccountLabel, AccountID: account.AccountID,
		})
	}
	for _, issuer := range group.Mappings.Issuers {
		mappings.Issuers = append(mappings.Issuers, legacyimport.IssuerMapping{IssuerName: issuer.IssuerName, UserID: issuer.UserID})
	}
	return mappings
}

// scope is the set of (program, account label) pairs a Tropa reads, and the agency of each
// issuer it maps.
type scope struct {
	programs map[string]string // loyalty program id -> code
	labels   map[string]bool   // code + "\x00" + fold(label)
	agencies map[string]string // fold(issuer name) -> agency id
}

func loadScope(db *gorm.DB, group data.InjunctionGroup) (scope, error) {
	var programs []data.LoyaltyProgram
	if err := db.Unscoped().Select("id", "code").Find(&programs).Error; err != nil {
		return scope{}, fmt.Errorf("injunction: loading programs: %w", err)
	}
	s := scope{programs: map[string]string{}, labels: map[string]bool{}, agencies: map[string]string{}}
	for _, program := range programs {
		s.programs[program.ID] = program.Code
	}
	for _, account := range group.Mappings.Accounts {
		s.labels[account.ProgramCode+"\x00"+fold(account.AccountLabel)] = true
	}
	for _, issuer := range group.Mappings.Issuers {
		if issuer.AgencyID != "" {
			s.agencies[fold(issuer.IssuerName)] = issuer.AgencyID
		}
	}
	return s, nil
}

// requireIssuerAgency turns a row that would be written into "needs mapping" when its issuer
// has no agency on the Tropa. The importer would otherwise file it under the default agency,
// and a wrong agency is not something anybody notices before the month closes. It also
// catches an issuer the importer matched to a user by name alone, without a Tropa mapping.
func (s scope) requireIssuerAgency(row legacyimport.ResolvedRow) legacyimport.ResolvedRow {
	if row.Verdict != legacyimport.VerdictOk && row.Verdict != legacyimport.VerdictCancellation {
		return row
	}
	if s.agencies[fold(row.IssuerName)] != "" {
		return row
	}
	row.Verdict = legacyimport.VerdictNeedsMapping
	row.Reason = fmt.Sprintf("o emissor \"%s\" não tem agência definida neste grupo", strings.TrimSpace(row.IssuerName))
	row.Cancellation = nil
	return row
}

// rowAgencies is the agency of every selected row, from its issuer.
func (s scope) rowAgencies(rows []legacyimport.ResolvedRow, selection []legacyimport.RowSelection) []legacyimport.RowAgencyMapping {
	selected := map[legacyimport.RowSelection]bool{}
	for _, row := range selection {
		selected[row] = true
	}
	var mappings []legacyimport.RowAgencyMapping
	for _, row := range rows {
		if !selected[legacyimport.RowSelection{BlockIndex: row.BlockIndex, Number: row.Number}] {
			continue
		}
		mappings = append(mappings, legacyimport.RowAgencyMapping{
			BlockIndex: row.BlockIndex, Number: row.Number, AgencyID: s.agencies[fold(row.IssuerName)],
		})
	}
	return mappings
}

// includes says whether a resolved row belongs to the Tropa. A row too broken to name its
// account (no label) is kept: it was typed into this Tropa's workbook, and somebody has to
// fix it.
func (s scope) includes(row legacyimport.ResolvedRow) bool {
	if strings.TrimSpace(row.AccountLabel) == "" {
		return row.Verdict == legacyimport.VerdictInvalid
	}
	return s.labels[s.programs[row.LoyaltyProgramID]+"\x00"+fold(row.AccountLabel)]
}

// withoutRedemptionFee drops the "Taxa de Resgate" column from every row of a day.
func withoutRedemptionFee(day legacyimport.Day) legacyimport.Day {
	for b := range day.Blocks {
		for r := range day.Blocks[b].Rows {
			day.Blocks[b].Rows[r].Taxes = nil
		}
	}
	return day
}

// ResolveDay resolves one workbook day for the Tropa: every row of its accounts, nothing else.
func ResolveDay(db *gorm.DB, group data.InjunctionGroup, day legacyimport.Day) ([]legacyimport.ResolvedRow, error) {
	s, err := loadScope(db, group)
	if err != nil {
		return nil, err
	}
	return resolveDay(db, group, s, day)
}

func resolveDay(db *gorm.DB, group data.InjunctionGroup, s scope, day legacyimport.Day) ([]legacyimport.ResolvedRow, error) {
	preview, err := legacyimport.Resolve(db, withoutRedemptionFee(day), Mappings(group))
	if err != nil {
		return nil, err
	}
	var rows []legacyimport.ResolvedRow
	for _, block := range preview.Blocks {
		for _, row := range block.Rows {
			if s.includes(row) {
				rows = append(rows, s.requireIssuerAgency(row))
			}
		}
	}
	return rows, nil
}

func itemOf(day time.Time, s scope, row legacyimport.ResolvedRow, detail string) Item {
	item := Item{
		Day:                day.Format(time.DateOnly),
		BlockIndex:         row.BlockIndex,
		Number:             row.Number,
		Locator:            strings.ToUpper(row.ProgramLocator),
		Program:            s.programs[row.LoyaltyProgramID],
		AccountID:          row.AccountID,
		AccountLabel:       strings.TrimSpace(row.AccountLabel),
		Points:             row.Points,
		ValueOfTheThousand: row.ValueOfTheThousand,
		Issuer:             row.IssuerName,
		Passenger:          row.MainPassengerName,
		Detail:             detail,
	}
	if strings.TrimSpace(row.Observation) != "" && row.Observation != "-" {
		observation := row.Observation
		item.Observation = &observation
	}
	return item
}

// statementBooking is one statement booking the panel does not have.
type statementBooking struct {
	accountID string
	program   string
	points    int
}

// statementGaps reconciles every account of the Tropa and returns the bookings missing in the
// panel, by locator.
func statementGaps(db *gorm.DB, group data.InjunctionGroup) (map[string]statementBooking, []AccountStatement) {
	gaps := map[string]statementBooking{}
	var reports []AccountStatement
	seen := map[string]bool{}
	for _, mapping := range group.Mappings.Accounts {
		if seen[mapping.AccountID] {
			continue
		}
		seen[mapping.AccountID] = true
		report := AccountStatement{AccountID: mapping.AccountID, Program: mapping.ProgramCode}

		var parsed statement.Statement
		var err error
		switch mapping.ProgramCode {
		case "AZUL":
			parsed, err = statement.LoadAzulStatement(db, mapping.AccountID)
		case "SMILES":
			parsed, err = statement.LoadSmilesStatement(db, mapping.AccountID)
		default:
			report.Error = "programa sem extrato"
			reports = append(reports, report)
			continue
		}
		if err != nil {
			report.Error = err.Error()
			reports = append(reports, report)
			continue
		}
		from, to, ok := statement.PeriodOf(parsed)
		if !ok {
			report.Error = "extrato ainda não sincronizado"
			reports = append(reports, report)
			continue
		}
		report.From, report.To = from.Format(time.DateOnly), to.Format(time.DateOnly)
		bookings, batches, err := statement.LoadPanel(db, mapping.AccountID, from, to)
		if err != nil {
			report.Error = err.Error()
			reports = append(reports, report)
			continue
		}
		for _, booking := range statement.Reconcile(parsed.Movements, bookings, batches).Bookings {
			if booking.Verdict == statement.VerdictMissingInPanel && booking.Locator != "" {
				gaps[strings.ToUpper(booking.Locator)] = statementBooking{accountID: mapping.AccountID, program: mapping.ProgramCode, points: booking.StatementPoints}
			}
		}
		reports = append(reports, report)
	}
	return gaps, reports
}

// Check cross-checks the workbook days in [from, to] against the panel and the statement.
func Check(db *gorm.DB, group data.InjunctionGroup, workbook []byte, from, to time.Time) (CheckResult, error) {
	result := CheckResult{
		From: from.Format(time.DateOnly), To: to.Format(time.DateOnly),
		MissingInPanelConfirmed: []Item{}, MissingInPanelUnconfirmed: []Item{}, DiffersFromPanel: []Item{},
		MissingInSheetAndPanel: []Item{}, PendingCancellations: []Item{}, Invalid: []Item{}, NeedsMapping: []Item{},
	}
	days, err := legacyimport.Parse(workbook)
	if err != nil {
		return result, fmt.Errorf("a planilha não pôde ser lida: %w", err)
	}
	s, err := loadScope(db, group)
	if err != nil {
		return result, err
	}
	gaps, statements := statementGaps(db, group)
	result.Statements = statements

	// Every locator the workbook carries for these accounts, in any day: a statement
	// booking from last month is not "missing in the workbook" just because this week's
	// window does not reach it.
	inSheet := map[string]bool{}
	var alreadyImported []struct {
		day time.Time
		row legacyimport.ResolvedRow
	}

	for _, day := range days {
		inRange := !day.Date.Before(from) && !day.Date.After(to)
		if !inRange {
			for _, block := range day.Blocks {
				for _, row := range block.Rows {
					inSheet[strings.ToUpper(strings.TrimSpace(row.ProgramLocator))] = true
				}
			}
			continue
		}
		rows, err := resolveDay(db, group, s, day)
		if err != nil {
			return result, fmt.Errorf("dia %s: %w", day.SheetName, err)
		}
		for _, row := range rows {
			locator := strings.ToUpper(strings.TrimSpace(row.ProgramLocator))
			inSheet[locator] = true
			result.SheetRows++
			switch row.Verdict {
			case legacyimport.VerdictOk:
				gap, onStatement := gaps[locator]
				switch {
				case onStatement && gap.points == row.Points:
					item := itemOf(day.Date, s, row, "na planilha e no extrato com os mesmos pontos; falta no painel")
					item.StatementPoints = &gap.points
					result.MissingInPanelConfirmed = append(result.MissingInPanelConfirmed, item)
				case onStatement:
					item := itemOf(day.Date, s, row, fmt.Sprintf("na planilha com %d pontos, no extrato com %d; falta no painel", row.Points, gap.points))
					item.StatementPoints = &gap.points
					result.MissingInPanelUnconfirmed = append(result.MissingInPanelUnconfirmed, item)
				default:
					result.MissingInPanelUnconfirmed = append(result.MissingInPanelUnconfirmed,
						itemOf(day.Date, s, row, "na planilha, mas não no painel nem no extrato lido"))
				}
			case legacyimport.VerdictAlreadyImported:
				alreadyImported = append(alreadyImported, struct {
					day time.Time
					row legacyimport.ResolvedRow
				}{day.Date, row})
			case legacyimport.VerdictCancellation:
				result.PendingCancellations = append(result.PendingCancellations, itemOf(day.Date, s, row, row.Reason))
			case legacyimport.VerdictNeedsMapping:
				result.NeedsMapping = append(result.NeedsMapping, itemOf(day.Date, s, row, row.Reason))
			default:
				result.Invalid = append(result.Invalid, itemOf(day.Date, s, row, row.Reason))
			}
		}
	}

	if err := compareWithPanel(db, s, alreadyImported, &result); err != nil {
		return result, err
	}

	for locator, gap := range gaps {
		if inSheet[locator] {
			continue
		}
		points := gap.points
		result.MissingInSheetAndPanel = append(result.MissingInSheetAndPanel, Item{
			Locator: locator, Program: gap.program, AccountID: gap.accountID, Points: gap.points, StatementPoints: &points,
			Detail: "no extrato, mas nem na planilha nem no painel",
		})
	}
	sort.Slice(result.MissingInSheetAndPanel, func(i, j int) bool {
		return result.MissingInSheetAndPanel[i].Locator < result.MissingInSheetAndPanel[j].Locator
	})
	return result, nil
}

// compareWithPanel checks the rows the panel already has, field by field.
func compareWithPanel(db *gorm.DB, s scope, rows []struct {
	day time.Time
	row legacyimport.ResolvedRow
}, result *CheckResult) error {
	if len(rows) == 0 {
		return nil
	}
	locators := make([]string, 0, len(rows))
	for _, r := range rows {
		locators = append(locators, strings.ToUpper(strings.TrimSpace(r.row.ProgramLocator)))
	}
	var issuances []data.Issuance
	if err := db.Where("UPPER(program_locator) IN ? AND active", locators).Find(&issuances).Error; err != nil {
		return fmt.Errorf("injunction: loading panel issuances: %w", err)
	}
	panel := map[string]data.Issuance{}
	for _, issuance := range issuances {
		panel[strings.ToUpper(issuance.ProgramLocator)] = issuance
	}
	for _, r := range rows {
		issuance, ok := panel[strings.ToUpper(strings.TrimSpace(r.row.ProgramLocator))]
		if !ok {
			continue
		}
		var differences []string
		if issuance.Points != r.row.Points {
			differences = append(differences, fmt.Sprintf("pontos %d no painel × %d na planilha", issuance.Points, r.row.Points))
		}
		if issuance.ValueOfTheThousand != r.row.ValueOfTheThousand {
			differences = append(differences, fmt.Sprintf("milheiro %d × %d centavos", issuance.ValueOfTheThousand, r.row.ValueOfTheThousand))
		}
		if r.row.AccountID != "" && issuance.AccountID != r.row.AccountID {
			differences = append(differences, "conta diferente")
		}
		if len(differences) == 0 {
			continue
		}
		item := itemOf(r.day, s, r.row, strings.Join(differences, "; "))
		item.PanelIssuanceID = issuance.ID
		result.DiffersFromPanel = append(result.DiffersFromPanel, item)
	}
	return nil
}

// Importable is what Import may write from one day: the Tropa's rows that are ready (ok) or
// are refunds to apply (cancellation).
func Importable(rows []legacyimport.ResolvedRow) []legacyimport.RowSelection {
	var selection []legacyimport.RowSelection
	for _, row := range rows {
		if row.Verdict == legacyimport.VerdictOk || row.Verdict == legacyimport.VerdictCancellation {
			selection = append(selection, legacyimport.RowSelection{BlockIndex: row.BlockIndex, Number: row.Number})
		}
	}
	return selection
}

// Import writes rows of one workbook day as issuances, through the importer. rows nil means
// every importable row of the Tropa; otherwise only those of them that are importable - a row
// of another account cannot be smuggled in by number.
func Import(db *gorm.DB, group data.InjunctionGroup, workbook []byte, date time.Time, rows []legacyimport.RowSelection, actorID string) (legacyimport.CommitResult, error) {
	day, found, err := legacyimport.ParseDay(workbook, date)
	if err != nil {
		return legacyimport.CommitResult{}, fmt.Errorf("a planilha não pôde ser lida: %w", err)
	}
	if !found {
		return legacyimport.CommitResult{}, fmt.Errorf("a planilha não tem a aba do dia %s", date.Format("02-01-06"))
	}
	day = withoutRedemptionFee(day)

	s, err := loadScope(db, group)
	if err != nil {
		return legacyimport.CommitResult{}, err
	}
	resolved, err := resolveDay(db, group, s, day)
	if err != nil {
		return legacyimport.CommitResult{}, err
	}
	allowed := Importable(resolved)
	selection := allowed
	if rows != nil {
		permitted := map[legacyimport.RowSelection]bool{}
		for _, row := range allowed {
			permitted[row] = true
		}
		selection = nil
		for _, row := range rows {
			if permitted[row] {
				selection = append(selection, row)
			}
		}
	}
	if len(selection) == 0 {
		return legacyimport.CommitResult{}, nil
	}
	mappings := Mappings(group)
	mappings.RowAgencies = s.rowAgencies(resolved, selection)
	return legacyimport.Commit(db, day, mappings, selection, actorID)
}

// IssuerAgencyIDs is every agency the Tropa's issuers write into, without repeats.
func IssuerAgencyIDs(group data.InjunctionGroup) []string {
	seen := map[string]bool{}
	var ids []string
	for _, issuer := range group.Mappings.Issuers {
		if issuer.AgencyID != "" && !seen[issuer.AgencyID] {
			seen[issuer.AgencyID] = true
			ids = append(ids, issuer.AgencyID)
		}
	}
	return ids
}
