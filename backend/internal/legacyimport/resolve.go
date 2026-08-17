package legacyimport

import (
	"fmt"
	"regexp"
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
	// VerdictCancellation: an estorno line. It will be written like the others, but what
	// it writes is a cancellation - see CancellationPlan for which of the two shapes.
	VerdictCancellation Verdict = "cancellation"
)

// CancellationAction is which of the two paths an estorno line takes.
type CancellationAction string

const (
	// CancelExisting: an issuance already carries this locator, so the estorno cancels
	// it - the same thing the panel's own Cancelar button does.
	CancelExisting CancellationAction = "cancel_existing"
	// CreateAndCancel: nothing carries this locator. The issuance the estorno refers to
	// was never imported, so it is created and then cancelled, in that order.
	CreateAndCancel CancellationAction = "create_and_cancel"
)

// CancellationPlan is what an estorno line will do, in the operator's terms.
type CancellationPlan struct {
	Action CancellationAction `json:"action"`
	// TargetIssuanceID is the issuance that will be cancelled when it is already in the
	// database. Empty for create_and_cancel, and for a locator an earlier row of this
	// same day is about to create.
	TargetIssuanceID string `json:"target_issuance_id"`
	// RefundedPoints and RefundedTaxes are the sheet's own figures with the sign
	// dropped - what lands in issuances.refunded_points / refunded_taxes.
	RefundedPoints int  `json:"refunded_points"`
	RefundedTaxes  *int `json:"refunded_taxes"`
	// OriginalPoints is what the issuance being cancelled is worth, when it is already
	// in the database. Nil otherwise, where the estorno line is the only figure there is.
	OriginalPoints *int `json:"original_points"`
	// Partial says the estorno gives back less than the issuance is worth - one leg of
	// a round trip, one passenger of a booking. The whole issuance is still cancelled,
	// because the panel has no half-cancelled state, so this is a warning and not a
	// refusal: 29 of the workbooks' 108 estornos are like this.
	Partial bool `json:"partial"`
}

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
	// DefaultLoyaltyProgramID is the program a row falls back to when its sheet has no
	// Companhia column at all - several of the operators' workbooks are a single program
	// and simply left the column out. It never rescues a label that is filled in and
	// unrecognised: a typo has to stay visible, not be filed under the default.
	DefaultLoyaltyProgramID string `json:"default_loyalty_program_id"`
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
	// an origin or destination the airports table does not know, a partial estorno.
	Notes []string `json:"notes"`
	// Cancellation is set on an estorno line and nil on every other row. The row's own
	// figures stay positive: what the preview shows is the issuance, and this is what
	// will be done to it.
	Cancellation *CancellationPlan `json:"cancellation,omitempty"`

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
	Date      time.Time `json:"date"`
	SheetName string    `json:"sheet_name"`
	// PointsUnit is how the sheet wrote its Pontos column, "raw" or "thousands". The
	// screen says which reading it used, because getting it wrong is a factor of a
	// thousand and the panel has nothing to check the points against.
	PointsUnit      string           `json:"points_unit"`
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
	preview := DayPreview{Date: day.Date, SheetName: day.SheetName, PointsUnit: day.PointsUnit}

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
	// told apart from the first - and, since an issuance and its estorno on one sheet is
	// the ordinary shape, so the estorno can tell which of the two it is looking at.
	seen := map[string]rowKind{}

	// Locators this day cancels, known before the first row is looked at. A day that
	// issues and cancels the same booking leaves a cancelled issuance behind, and on a
	// second run the positive row would otherwise read as a booking re-issued under a
	// freed locator - writing the pair again, and again on every run after that.
	cancelledHere := estornoLocators(day)

	for _, block := range day.Blocks {
		resolved := ResolvedBlock{
			Index:      block.Index,
			IssuerName: block.IssuerName,
			IssuerID:   lookups.issuerID(block.IssuerName),
		}
		for _, row := range block.Rows {
			out := resolveRow(row, block, day, lookups, programs, mappings, seen, cancelledHere)
			resolved.Rows = append(resolved.Rows, out)
			// From the verdict, not from the row: a line nobody can import creates
			// nothing, so it must not make the next one look like a repeat.
			if kind := kindOf(out); kind != kindNone {
				seen[strings.ToUpper(row.ProgramLocator)] = kind
			}
		}
		preview.Blocks = append(preview.Blocks, resolved)
	}

	preview.PendingAccounts = lookups.pendingAccounts
	preview.PendingIssuers = lookups.pendingIssuers

	return preview, nil
}

// rowKind is what a locator seen earlier in this same day left behind.
type rowKind int

const (
	kindNone rowKind = iota
	// kindIssuance: a row that will create this locator when the day is committed.
	kindIssuance
	// kindCancellation: an estorno line for this locator.
	kindCancellation
)

// estornoLocators is every locator this day writes an estorno for, upper-cased.
func estornoLocators(day Day) map[string]bool {
	out := map[string]bool{}
	forEachRow(day, func(_ Block, row Row) {
		if row.Points < 0 {
			out[strings.ToUpper(strings.TrimSpace(row.ProgramLocator))] = true
		}
	})
	return out
}

// issuerNameOf is who a row's issuance belongs to: its own Emissor cell where the layout
// has that column, and the name in the block's title everywhere else.
func issuerNameOf(row Row, block Block) string {
	if name := strings.TrimSpace(row.IssuerName); name != "" {
		return name
	}
	return block.IssuerName
}

func kindOf(row ResolvedRow) rowKind {
	switch row.Verdict {
	case VerdictOk:
		return kindIssuance
	case VerdictCancellation:
		return kindCancellation
	default:
		return kindNone
	}
}

// cancellationWords are what the operators write next to an estorno. On a row whose points
// are positive the word is about a cancellation that is coming, or one written on another
// line - the row is imported normally and the operator gets to see why it caught our eye.
var cancellationWords = regexp.MustCompile(`(?i)cancel|estorn|reembols`)

func resolveRow(
	row Row,
	block Block,
	day Day,
	lookups *lookups,
	programs map[string]data.LoyaltyProgram,
	mappings Mappings,
	seen map[string]rowKind,
	cancelledHere map[string]bool,
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
		// The row's own Emissor column when the sheet has one - the shared workbook
		// mixes three issuers inside a single day - and the block's title otherwise.
		IssuerName:  issuerNameOf(row, block),
		Observation: row.Observation,
	}
	out.IssuerID = lookups.issuerID(out.IssuerName)

	program, ok := effectiveProgram(row, programs, mappings)
	if !ok {
		if strings.TrimSpace(row.ProgramLabel) == "" {
			return invalid(out, "A linha não diz o programa e nenhum programa padrão foi "+
				"escolhido para esta importação")
		}
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

	// The operators write an estorno as the issuance with its signs flipped, on a row of
	// its own. Detection is by the points and nothing else: a negative milheiro or tax on
	// a row whose points are positive is a typo, not a cancellation.
	if row.Points < 0 {
		out = resolveCancellation(out, row, day, lookups, seen)
		if out.Verdict != VerdictCancellation {
			return out
		}
	} else {
		if cancellationWords.MatchString(row.Observation) {
			out.Notes = append(out.Notes, "A observação fala em cancelamento, mas a "+
				"linha é positiva — ela entra como emissão normal")
		}
		if row.ValueOfTheThousand < 0 || (row.Taxes != nil && *row.Taxes < 0) {
			return invalid(out, "A linha tem valor negativo fora da coluna de pontos")
		}
		if row.Points == 0 || row.ValueOfTheThousand == 0 {
			return invalid(out, "A linha não tem pontos ou valor do milheiro")
		}
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
		missing = append(missing, fmt.Sprintf("o emissor %q", out.IssuerName))
	}
	if len(missing) > 0 {
		out.Verdict = VerdictNeedsMapping
		out.Reason = "Falta indicar " + strings.Join(missing, " e ")
		return out
	}

	// An estorno line answered both of these for itself, against a locator that is
	// supposed to have been seen before - that is what it is cancelling.
	if out.Cancellation != nil {
		return out
	}

	// Ordered after the mapping checks on purpose: a row that is already in the
	// database needs no mapping at all, but seeing *why* it is being skipped is more
	// useful than seeing that it is also unmapped.
	if seen[strings.ToUpper(row.ProgramLocator)] != kindNone {
		out.Verdict = VerdictDuplicateInFile
		out.Reason = "O localizador aparece mais de uma vez neste dia"
		return out
	}
	// Active on purpose: a locator whose only issuance was cancelled is free to be
	// issued again - that is exactly what the partial unique index allows - so a row
	// carrying it is a new booking and not a re-import.
	if existing, found := lookups.locator(row.ProgramLocator); found {
		if existing.active {
			out.Verdict = VerdictAlreadyImported
			out.Reason = "Já existe uma emissão ativa com esse localizador"
			return out
		}
		// Unless this very day is what cancelled it. Then the cancelled issuance in the
		// database *is* this row, already imported once, and writing it again would add
		// a fresh pair on every run.
		if cancelledHere[strings.ToUpper(row.ProgramLocator)] {
			out.Verdict = VerdictAlreadyImported
			out.Reason = "Essa emissão já foi importada e cancelada pelo estorno deste mesmo dia"
			return out
		}
	}

	return out
}

// resolveCancellation classifies an estorno line. The spreadsheet has no way of saying
// "cancel this": the operators write the issuance again with its signs flipped, so what
// the row asks for is what the panel's Cancelar button does to the issuance its locator
// names. When no issuance carries that locator - because the day it was issued was never
// imported - it is created first, which is what the operator would do by hand.
//
// The row it returns carries positive figures, so the preview shows an issuance and the
// commit can reuse createIssuance untouched; the refund lives in the plan.
func resolveCancellation(
	out ResolvedRow,
	row Row,
	day Day,
	lookups *lookups,
	seen map[string]rowKind,
) ResolvedRow {
	out.Points = -row.Points
	out.ValueOfTheThousand = abs(row.ValueOfTheThousand)
	if row.Taxes != nil {
		taxes := abs(*row.Taxes)
		out.Taxes = &taxes
	}

	plan := CancellationPlan{RefundedPoints: out.Points, RefundedTaxes: out.Taxes}
	key := strings.ToUpper(row.ProgramLocator)

	switch seen[key] {
	case kindIssuance:
		// Issued and cancelled on the same sheet, which is the ordinary shape. There is
		// no id to point at yet - the issuance is created a few rows above, in this same
		// transaction - so the commit looks the locator up again when it gets there.
		plan.Action = CancelExisting
		out.Notes = append(out.Notes,
			"A emissão foi criada por uma linha anterior deste mesmo dia")
	case kindCancellation:
		out.Cancellation = &plan
		out.Verdict = VerdictDuplicateInFile
		out.Reason = "O estorno desse localizador aparece mais de uma vez neste dia"
		return out
	default:
		existing, found := lookups.locator(row.ProgramLocator)
		switch {
		case !found:
			plan.Action = CreateAndCancel
		case !existing.active:
			// The idempotency of a second run over the same day: cancelling releases
			// the locator from idx_issuances_program_locator_active, so nothing in the
			// database would stop a second cancelled issuance from being written.
			plan.Action = CancelExisting
			plan.TargetIssuanceID = existing.id
			out.Cancellation = &plan
			out.Verdict = VerdictAlreadyImported
			out.Reason = "A emissão desse localizador já está cancelada"
			return out
		default:
			plan.Action = CancelExisting
			plan.TargetIssuanceID = existing.id
			points := existing.points
			plan.OriginalPoints = &points
			plan.Partial = plan.RefundedPoints < points
			// A locator is reused: a booking cancelled in one month and another issued
			// under the same code later. The active one is the one being pointed at,
			// and if it was created after this day it is probably not the one meant.
			if existing.createdAt.After(day.Date.AddDate(0, 0, 1)) {
				out.Notes = append(out.Notes, "A emissão com esse localizador é "+
					"posterior a este dia — confira se é mesmo ela")
			}
		}
	}

	if plan.Partial && plan.OriginalPoints != nil {
		out.Notes = append(out.Notes, fmt.Sprintf(
			"Estorno parcial: a planilha devolve %d de %d pontos, e a emissão é "+
				"cancelada por inteiro", plan.RefundedPoints, *plan.OriginalPoints))
	}

	// Only the create half needs a price: cancelling an issuance that already exists
	// does not touch what it was sold for.
	if plan.Action == CreateAndCancel && (out.Points == 0 || out.ValueOfTheThousand == 0) {
		return invalid(out, "A linha não tem pontos ou valor do milheiro")
	}

	out.Cancellation = &plan
	out.Verdict = VerdictCancellation
	out.Reason = cancellationReason(plan)
	return out
}

func cancellationReason(plan CancellationPlan) string {
	if plan.Action == CreateAndCancel {
		return "A emissão não está no sistema: ela é criada e cancelada em seguida"
	}
	return "Cancela a emissão que já existe com esse localizador"
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
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

	indexed := make(map[string]data.LoyaltyProgram, len(programs)*3)
	for _, program := range programs {
		indexed[programKey(program.Code)] = program
		// The Companhia column carries whatever the operators type, which is
		// sometimes the program's display name rather than its code.
		indexed[programKey(program.Name)] = program
		// And by id, for the default the operator picks on the screen. Ids are
		// uuids, so they cannot collide with a folded code or name.
		indexed[program.ID] = program
	}
	return indexed, nil
}

// effectiveProgram is the row's own Companhia cell, or the default the operator picked for
// this import when the sheet has no program column. A label that is filled in but names
// nothing is *not* rescued by the default: filing a typo under the wrong program is worse
// than refusing the row, because nothing downstream would ever notice.
func effectiveProgram(
	row Row,
	programs map[string]data.LoyaltyProgram,
	mappings Mappings,
) (data.LoyaltyProgram, bool) {
	if strings.TrimSpace(row.ProgramLabel) != "" {
		program, ok := programs[programKey(row.ProgramLabel)]
		return program, ok
	}
	if id := strings.TrimSpace(mappings.DefaultLoyaltyProgramID); id != "" {
		program, ok := programs[id]
		return program, ok
	}
	return data.LoyaltyProgram{}, false
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
