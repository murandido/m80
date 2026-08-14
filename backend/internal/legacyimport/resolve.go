package legacyimport

import (
	"fmt"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// A Verdict is what the preview says about one row: whether it will be written, and if
// not, why not. Only rows that come back Ok are ticked on the screen by default.
type Verdict string

const (
	// VerdictOk: everything resolved, the row is ready to be written.
	VerdictOk Verdict = "ok"
	// VerdictAlreadyImported: an active issuance already carries this locator. Nearly
	// always a booking somebody registered through the panel directly - which is the
	// whole reason re-running a day is safe.
	VerdictAlreadyImported Verdict = "already_imported"
	// VerdictNeedsMapping: an account alias or an issuer name with no id behind it yet.
	// Resolvable on the screen, which is what the mapping controls are for.
	VerdictNeedsMapping Verdict = "needs_mapping"
	// VerdictDuplicateInFile: the same locator appears twice in the same day. Only the
	// first is offered - issuances.program_locator is unique among active rows, so
	// writing both is impossible whatever we decide here.
	VerdictDuplicateInFile Verdict = "duplicate_in_file"
	// VerdictInvalid: something the spreadsheet cannot supply and the screen cannot fix,
	// like an unknown loyalty program or a missing price.
	VerdictInvalid Verdict = "invalid"
)

// Mappings is what the operator decided on the screen. It travels with both the preview
// and the commit, so the preview shows the same resolution the commit will perform.
type Mappings struct {
	// Accounts resolves one (program, alias as typed) pair. Keyed by both because
	// account_nicknames is unique per loyalty program, not globally: "WESLEY" on Azul
	// and "WESLEY" on Smiles are two different accounts.
	Accounts []AccountMapping `json:"accounts"`
	// Issuers resolves a block title's name. "WILMARA/CLARA" is two people sharing a
	// block, so this is a decision and not a lookup.
	Issuers []IssuerMapping `json:"issuers"`
	// AgencyID is the day's default. Empty means the agency flagged is_default, which
	// is what issuance.Create does with a nil AgencyID - the spreadsheet has no agency
	// column at all, so every row starts from this.
	AgencyID string `json:"agency_id"`
	// RowAgencies overrides that default for individual rows. Which agency an issuance
	// belongs to is a per-issuance fact, and one spreadsheet day can hold both.
	//
	// Unlike the account and issuer mappings, this one is not resolved for the preview:
	// there is nothing to resolve. The operator picks an agency by id from a list the
	// screen already has, so what they see is already what will be written, and making
	// it a round trip would mean re-uploading the workbook on every pick.
	RowAgencies []RowAgencyMapping `json:"row_agencies"`
}

// RowAgencyMapping names one row of the sheet and the agency it belongs to.
type RowAgencyMapping struct {
	BlockIndex int    `json:"block_index"`
	Number     int    `json:"number"`
	AgencyID   string `json:"agency_id"`
}

type AccountMapping struct {
	ProgramCode  string `json:"program_code"`
	AccountLabel string `json:"account_label"`
	AccountID    string `json:"account_id"`
}

type IssuerMapping struct {
	IssuerName string `json:"issuer_name"`
	UserID     string `json:"user_id"`
}

// ResolvedRow is one spreadsheet row with every reference turned into an id - i.e. exactly
// what would be written, which is what the preview shows.
type ResolvedRow struct {
	BlockIndex int     `json:"block_index"`
	Number     int     `json:"number"`
	Verdict    Verdict `json:"verdict"`
	// Reason explains a verdict that is not Ok, in the operator's language.
	Reason string `json:"reason"`
	// Notes are things worth seeing that do not stop the row from being written -
	// today, an origin or destination the airports table does not know.
	Notes []string `json:"notes"`

	LoyaltyProgramID   string     `json:"loyalty_program_id"`
	LoyaltyProgramName string     `json:"loyalty_program_name"`
	AirlineName        string     `json:"airline_name"`
	ProgramLocator     string     `json:"program_locator"`
	AirlineLocator     string     `json:"airline_locator"`
	FlightDate         *time.Time `json:"flight_date"`
	MainPassengerName  string     `json:"main_passenger_name"`
	OriginIATACode     string     `json:"origin_iata_code"`
	DestIATACode       string     `json:"destination_iata_code"`
	Points             int        `json:"points"`
	ValueOfTheThousand int        `json:"value_of_the_thousand"`
	Taxes              *int       `json:"taxes"`
	// AccountLabel is the alias as the spreadsheet has it; AccountName is what the
	// panel will show for the account it resolved to. Both, because the operator is
	// checking one against the other.
	AccountLabel      string  `json:"account_label"`
	AccountID         string  `json:"account_id"`
	AccountName       string  `json:"account_name"`
	AccountNicknameID *string `json:"account_nickname_id"`
	IssuerName        string  `json:"issuer_name"`
	IssuerID          string  `json:"issuer_id"`
	Observation       string  `json:"observation"`
}

// ResolvedBlock keeps the sheet's own grouping, which is how the decision to bring a
// block in or leave it out is actually made.
type ResolvedBlock struct {
	Index      int           `json:"index"`
	IssuerName string        `json:"issuer_name"`
	IssuerID   string        `json:"issuer_id"`
	Rows       []ResolvedRow `json:"rows"`
}

// PendingAccount is an alias with no account behind it yet - one entry per (program,
// alias) pair, however many rows use it.
type PendingAccount struct {
	ProgramCode  string `json:"program_code"`
	ProgramName  string `json:"program_name"`
	ProgramID    string `json:"loyalty_program_id"`
	AccountLabel string `json:"account_label"`
	RowCount     int    `json:"row_count"`
}

// PendingIssuer is a block title with no user behind it. Suggested is a name match and
// nothing more - the operator confirms it, because a block can name two people.
type PendingIssuer struct {
	IssuerName      string `json:"issuer_name"`
	SuggestedUserID string `json:"suggested_user_id"`
	RowCount        int    `json:"row_count"`
}

// DayPreview is the whole answer for one day.
type DayPreview struct {
	Date            time.Time        `json:"date"`
	SheetName       string           `json:"sheet_name"`
	AgencyID        string           `json:"agency_id"`
	AgencyName      string           `json:"agency_name"`
	Blocks          []ResolvedBlock  `json:"blocks"`
	PendingAccounts []PendingAccount `json:"pending_accounts"`
	PendingIssuers  []PendingIssuer  `json:"pending_issuers"`
}

// Resolve turns a parsed day into the preview: every row with its ids filled in and its
// verdict decided, plus the two lists of things the screen has to resolve.
//
// Read-only. Nothing here creates an account, a person or an alias - unlike the bot's
// path for an unknown nickname, which does. The spreadsheet has 77 spellings of maybe
// half as many accounts, and an account that has issued can never be deleted, only
// archived; so the screen asks rather than guesses.
func Resolve(db *gorm.DB, day Day, mappings Mappings) (DayPreview, error) {
	preview := DayPreview{Date: day.Date, SheetName: day.SheetName}

	agency, err := resolveAgency(db, mappings.AgencyID)
	if err != nil {
		return preview, err
	}
	preview.AgencyID = agency.ID
	preview.AgencyName = agency.Name

	programs, err := loadPrograms(db)
	if err != nil {
		return preview, err
	}

	lookups, err := newLookups(db, day, programs, mappings)
	if err != nil {
		return preview, err
	}

	// Locators seen earlier in this same day, so the second occurrence of one can be
	// told apart from the first.
	seen := map[string]bool{}

	for _, block := range day.Blocks {
		resolved := ResolvedBlock{
			Index:      block.Index,
			IssuerName: block.IssuerName,
			IssuerID:   lookups.issuerID(block.IssuerName),
		}
		for _, row := range block.Rows {
			resolved.Rows = append(resolved.Rows,
				resolveRow(row, block, lookups, programs, seen))
			seen[strings.ToUpper(row.ProgramLocator)] = true
		}
		preview.Blocks = append(preview.Blocks, resolved)
	}

	preview.PendingAccounts = lookups.pendingAccounts
	preview.PendingIssuers = lookups.pendingIssuers

	return preview, nil
}

func resolveRow(
	row Row,
	block Block,
	lookups *lookups,
	programs map[string]data.LoyaltyProgram,
	seen map[string]bool,
) ResolvedRow {
	out := ResolvedRow{
		BlockIndex:         block.Index,
		Number:             row.Number,
		Verdict:            VerdictOk,
		ProgramLocator:     row.ProgramLocator,
		AirlineLocator:     row.AirlineLocator,
		FlightDate:         row.FlightDate,
		MainPassengerName:  row.PassengerName,
		Points:             row.Points,
		ValueOfTheThousand: row.ValueOfTheThousand,
		Taxes:              row.Taxes,
		AccountLabel:       row.AccountLabel,
		IssuerName:         block.IssuerName,
		IssuerID:           lookups.issuerID(block.IssuerName),
		Observation:        row.Observation,
	}

	program, ok := programs[programKey(row.ProgramLabel)]
	if !ok {
		return invalid(out, fmt.Sprintf("Não existe programa de fidelidade %q", row.ProgramLabel))
	}
	out.LoyaltyProgramID = program.ID
	out.LoyaltyProgramName = program.Name
	if program.DefaultAirline != nil {
		out.AirlineName = program.DefaultAirline.Name
	}

	// An IATA code the airports table does not know is dropped rather than refused: the
	// route is not what makes an issuance, and one cell reading "MEXICO" should not cost
	// the whole row. It is a note so nobody discovers the gap by its absence.
	out.OriginIATACode, out.Notes = lookups.airport(row.OriginLabel, "origem", out.Notes)
	out.DestIATACode, out.Notes = lookups.airport(row.DestinationLabel, "destino", out.Notes)

	// A negative line is how the spreadsheet records an estorno: the operators write the
	// cancellation as the issuance with its signs flipped, on a row of its own. The panel
	// models a cancellation as something done *to* an issuance, so there is no such thing
	// as a negative one - the way to bring this in is to import the original and cancel it
	// from the grid, which is what the message says.
	if row.Points < 0 || row.ValueOfTheThousand < 0 || (row.Taxes != nil && *row.Taxes < 0) {
		return invalid(out, "Linha negativa: a planilha registra cancelamento assim. "+
			"Importe a emissão original e cancele pelo painel")
	}
	if row.Points == 0 || row.ValueOfTheThousand == 0 {
		return invalid(out, "A linha não tem pontos ou valor do milheiro")
	}

	if account, found := lookups.account(program, row.AccountLabel); found {
		out.AccountID = account.id
		out.AccountName = account.name
		out.AccountNicknameID = account.nicknameID
	}

	// The two decisions the spreadsheet cannot make for itself. Reported together so
	// the screen can show one row needing both, instead of one at a time.
	var missing []string
	if out.AccountID == "" {
		missing = append(missing, fmt.Sprintf("a conta %q (%s)", row.AccountLabel, program.Name))
	}
	if out.IssuerID == "" {
		missing = append(missing, fmt.Sprintf("o emissor %q", block.IssuerName))
	}
	if len(missing) > 0 {
		out.Verdict = VerdictNeedsMapping
		out.Reason = "Falta indicar " + strings.Join(missing, " e ")
		return out
	}

	// Ordered after the mapping checks on purpose: a row that is already in the
	// database needs no mapping at all, but seeing *why* it is being skipped is more
	// useful than seeing that it is also unmapped.
	if seen[strings.ToUpper(row.ProgramLocator)] {
		out.Verdict = VerdictDuplicateInFile
		out.Reason = "O localizador aparece mais de uma vez neste dia"
		return out
	}
	if lookups.alreadyImported[strings.ToUpper(row.ProgramLocator)] {
		out.Verdict = VerdictAlreadyImported
		out.Reason = "Já existe uma emissão ativa com esse localizador"
		return out
	}

	return out
}

func invalid(row ResolvedRow, reason string) ResolvedRow {
	row.Verdict = VerdictInvalid
	row.Reason = reason
	return row
}

// resolveAgency returns the agency the day's issuances will belong to: the one the
// operator picked, or the default when they picked none.
func resolveAgency(db *gorm.DB, agencyID string) (data.Agency, error) {
	var agency data.Agency
	query := db.Where("is_default = ?", true)
	if strings.TrimSpace(agencyID) != "" {
		query = db.Where("id = ?", agencyID)
	}
	if err := query.First(&agency).Error; err != nil {
		return agency, fmt.Errorf("legacyimport: resolving the agency: %w", err)
	}
	return agency, nil
}

// loadPrograms indexes the loyalty programs by a folded code, so the sheet's "A.A"
// reaches the program registered as "AA".
func loadPrograms(db *gorm.DB) (map[string]data.LoyaltyProgram, error) {
	var programs []data.LoyaltyProgram
	if err := db.Preload("DefaultAirline").Find(&programs).Error; err != nil {
		return nil, fmt.Errorf("legacyimport: loading loyalty programs: %w", err)
	}

	indexed := make(map[string]data.LoyaltyProgram, len(programs)*2)
	for _, program := range programs {
		indexed[programKey(program.Code)] = program
		// The Companhia column carries whatever the operators type, which is
		// sometimes the program's display name rather than its code.
		indexed[programKey(program.Name)] = program
	}
	return indexed, nil
}

// programKey folds a program label down to what should compare equal: "A.A", "a.a." and
// "AA" are one program, and "LATAM PASS" is "LATAMPASS".
func programKey(label string) string {
	var folded strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(label)) {
		if r == '.' || r == ' ' || r == '-' {
			continue
		}
		folded.WriteRune(r)
	}
	return folded.String()
}
