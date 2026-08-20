// Package legacyimport reads the hand-kept "Controle de Emissões" spreadsheet the team
// used before the panel existed, so a day of it can be brought into the database.
//
// It is a bridge, not a platform: when the migration off the spreadsheets is finished the
// whole package goes away, along with internal/api/v1/{handlers,routes}/legacy_import.go,
// frontend/src/{app/dashboard/import,components/legacy-import,actions/legacy-import.ts},
// the RegisterLegacyImportRoutes call and the navigation entry. The one thing that stays
// behind is issuances.imported - it describes rows that outlive the screen, and it is what
// keeps Traveos from ever being asked to issue a ticket for a booking from August 2026.
//
// This file is the pure half: bytes in, rows out, no database.
package legacyimport

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// The columns of a block's table, read off its header row rather than by position. There
// is no single template: the team's workbooks carry four different layouts, differing in
// whether they have a Companhia column at all, whether the taxes are broken out, and
// whether the issuer is a column instead of part of the title. Position was fine while
// there was one spreadsheet; with four, the same index means a different thing per file.
const (
	columnSequence           = "sequence"
	columnProgram            = "program"
	columnLocator            = "locator"
	columnFlightDate         = "flight_date"
	columnPassenger          = "passenger"
	columnOrigin             = "origin"
	columnDestination        = "destination"
	columnPoints             = "points"
	columnValueOfTheThousand = "value_of_the_thousand"
	columnTaxes              = "taxes"
	columnTotal              = "total"
	columnAccount            = "account"
	columnIssuer             = "issuer"
	columnObservation        = "observation"
)

// columns maps each column this package knows to where it sits in a particular sheet.
type columns map[string]int

// The markers that give a sheet its shape. A day sheet is a stack of blocks - one per
// operator in the shared workbook, exactly one in each of the personal ones - and a block
// is a title, a header row, the issuances, and a closing total, after which come the
// operator's own summary tables. Bounding a block by header -> total is what keeps those
// tables out, and every workbook writes that total differently ("TOTAL FATURAMENTO",
// "TOTAL DAS CONTAS GERAL", "TOTAL R$ GERAL"), so the prefix is all that is checked.
const (
	blockTitlePrefix  = "CONTROLE"
	blockFooterPrefix = "TOTAL"
)

// Day sheets are named DD-MM-YY. The workbook also carries MODELO and one sheet per
// loyalty program, which this pattern is what excludes.
var daySheetName = regexp.MustCompile(`^(\d{2})-(\d{2})-(\d{2})$`)

// A Row is one issuance as the spreadsheet has it, with the units already converted to the
// ones the database uses. Everything here is still text as typed - resolving a program
// label or an account alias into an id is resolve.go's job.
type Row struct {
	// Number of the row in the sheet, so the panel can point at the source when
	// something does not resolve.
	Number int
	// ProgramLabel is the Companhia cell: "AZUL", "SMILES", "A.A".
	ProgramLabel string
	// ProgramLocator and AirlineLocator come from one cell: "CWL77K / JGBJNN" carries
	// both, the same "/" convention the issuances grid uses.
	ProgramLocator     string
	AirlineLocator     string
	FlightDate         *time.Time
	PassengerName      string
	OriginLabel        string
	DestinationLabel   string
	Points             int
	ValueOfTheThousand int
	Taxes              *int
	AccountLabel       string
	// IssuerName is the Emissor cell, in the workbooks that have that column. Empty
	// means the block's title names the issuer, which is how the rest of them do it.
	IssuerName  string
	Observation string
}

// A Block is one operator's table inside a day. The title names them - sometimes two of
// them ("WILMARA/CLARA"), which is why mapping the name to a user is a decision for the
// screen and not something this package tries to guess.
type Block struct {
	// Index within the sheet, top to bottom. It is what the panel sends back to say
	// which rows were ticked, since the issuer name is not unique across a workbook.
	Index      int
	IssuerName string
	Rows       []Row
}

// A Day is one sheet.
type Day struct {
	// Date comes from the sheet name, never from the title. The title is typed by hand
	// and at least one day in the sample workbook still carries the template's
	// "##/08/2026" placeholder.
	Date      time.Time
	SheetName string
	Blocks    []Block
	// PointsUnit is how this sheet wrote its Pontos column, so the screen can say which
	// reading it used. Getting it wrong is a factor of a thousand that nothing else
	// would catch.
	PointsUnit string
}

// Parse reads every day sheet in the workbook. Sheets that are not named DD-MM-YY
// (MODELO, AZUL, LATAM, ...) are skipped: they are the operators' own aids, not data.
func Parse(content []byte) ([]Day, error) {
	file, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("legacyimport.Parse: open workbook: %w", err)
	}
	defer func() { _ = file.Close() }()

	var days []Day
	for _, name := range file.GetSheetList() {
		date, ok := parseSheetDate(name)
		if !ok {
			continue
		}

		// RawCellValue keeps dates as their serial number and decimals unrounded;
		// the formatted value would come back rendered by whatever style the cell
		// happens to carry, which differs from block to block in this workbook.
		rows, err := file.GetRows(name, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, fmt.Errorf("legacyimport.Parse: read sheet %q: %w", name, err)
		}

		unit := detectPointsUnit(rows)
		days = append(days, Day{
			Date:       date,
			SheetName:  name,
			Blocks:     parseBlocks(rows, unit),
			PointsUnit: unit.String(),
		})
	}

	return days, nil
}

// ParseDay reads a single day out of the workbook, for the caller that already knows which
// one it wants. The personal workbooks carry a sheet per day for over a year - 500 of them,
// where the shared template had a handful - and reading the other 499 to write one is most
// of the time a commit takes.
func ParseDay(content []byte, date time.Time) (Day, bool, error) {
	file, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return Day{}, false, fmt.Errorf("legacyimport.ParseDay: open workbook: %w", err)
	}
	defer func() { _ = file.Close() }()

	for _, name := range file.GetSheetList() {
		sheetDate, ok := parseSheetDate(name)
		if !ok || !sheetDate.Equal(date) {
			continue
		}

		rows, err := file.GetRows(name, excelize.Options{RawCellValue: true})
		if err != nil {
			return Day{}, false, fmt.Errorf("legacyimport.ParseDay: read sheet %q: %w", name, err)
		}

		unit := detectPointsUnit(rows)
		return Day{
			Date:       sheetDate,
			SheetName:  name,
			Blocks:     parseBlocks(rows, unit),
			PointsUnit: unit.String(),
		}, true, nil
	}

	return Day{}, false, nil
}

// parseSheetDate turns a "DD-MM-YY" sheet name into the calendar day it stands for.
func parseSheetDate(name string) (time.Time, bool) {
	match := daySheetName.FindStringSubmatch(strings.TrimSpace(name))
	if match == nil {
		return time.Time{}, false
	}

	day, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[2])
	year, _ := strconv.Atoi(match[3])

	date := time.Date(2000+year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	// Rejects a name like "31-02-26": time.Date rolls it over to March instead of
	// failing, and a day that silently becomes another day is worse than no day.
	if date.Day() != day || int(date.Month()) != month {
		return time.Time{}, false
	}

	return date, true
}

func parseBlocks(rows [][]string, unit pointsUnit) []Block {
	var blocks []Block
	var current *Block
	var cols columns
	// The Companhia cell is filled on every row in practice, but a merged or blank one
	// means "same as above" rather than "no program", so the last seen value carries.
	programLabel := ""

	for index, cells := range rows {
		first := strings.ToUpper(strings.TrimSpace(cell(cells, 0)))

		if strings.HasPrefix(first, blockTitlePrefix) {
			blocks = closeBlock(blocks, current)
			current = &Block{Index: len(blocks), IssuerName: issuerFromTitle(cell(cells, 0))}
			cols = nil
			programLabel = ""
			continue
		}
		if isBlockFooter(cells) {
			cols = nil
			continue
		}
		if header, ok := headerColumns(cells); ok {
			cols = header
			// A sheet whose table has no title above it still has issuances in it. The
			// personal workbooks all have one, but two days of one of them lost theirs.
			if current == nil {
				current = &Block{Index: len(blocks)}
			}
			continue
		}

		if cols == nil || current == nil {
			continue
		}

		if label := text(at(cells, cols, columnProgram)); label != "" {
			programLabel = label
		}

		// The template ships a few dozen blank rows per program so the operators can
		// fill them in; the locator is what tells a used row from a spare one.
		locator := text(at(cells, cols, columnLocator))
		if locator == "" {
			continue
		}

		current.Rows = append(current.Rows, parseRow(index+1, programLabel, locator, cells, cols, unit))
	}

	return closeBlock(blocks, current)
}

// isBlockFooter says whether a row is the total that closes a block's table. The word is
// not always in the first column: the personal workbooks write "TOTAL DAS CONTAS GERAL"
// starting a column or two in, and reading only the first cell let the summary tables
// below it come through as issuances - with "TOTAL DAS CONTAS" as their locator.
func isBlockFooter(cells []string) bool {
	for index := 0; index < len(cells) && index < footerSearchColumns; index++ {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(cells[index])), blockFooterPrefix) {
			return true
		}
	}
	return false
}

// How far into a row the closing total is looked for. Wide enough for the sequence column
// and an indent or two, narrow enough that a "TOTAL" written in a late column - the sum of
// a money column, which sits under the row it totals - does not end the table early.
const footerSearchColumns = 4

// headerColumns reads a header row into the map of where each column sits. A row is the
// header when it names the locator: every layout has that column, under that word, and no
// other row of a sheet says it.
func headerColumns(cells []string) (columns, bool) {
	found := columns{}
	for index, raw := range cells {
		label := foldHeader(raw)
		if label == "" {
			continue
		}

		switch {
		// Checked first: it is the only header that contains a word another one wants
		// ("conferido" is harmless, but the column is a checkbox and not data).
		case strings.Contains(label, "extrato"):
			continue
		// Also checked before the airline: "agencia" contains "cia", so it matched the
		// Companhia case below and - being to the right of it - overwrote the real index,
		// leaving every row claiming to be issued on a loyalty program named after an
		// agency. There is no agency column to read here: the agency of an imported day
		// comes from the operator's mapping, not from the sheet.
		case strings.Contains(label, "agencia"):
			continue
		case strings.Contains(label, "localizador"):
			found[columnLocator] = index
		case strings.Contains(label, "cia") || strings.Contains(label, "companhia"):
			found[columnProgram] = index
		// "Data do Voo" in some layouts and a bare "Data Voo" in others - including the
		// template this repository ships - so the word that identifies the column is
		// "voo". Matching on the full phrase silently left the flight date empty, and
		// "data" alone would swallow the Data da Compra column beside it.
		case strings.Contains(label, "voo"):
			found[columnFlightDate] = index
		case strings.Contains(label, "passageiro"):
			found[columnPassenger] = index
		case label == "origem":
			found[columnOrigin] = index
		case label == "destino":
			found[columnDestination] = index
		case label == "pontos":
			found[columnPoints] = index
		case strings.Contains(label, "milheiro"):
			found[columnValueOfTheThousand] = index
		// "Taxa" and "Taxa de Resgate" are the same column under two names; only one
		// of the four layouts has it at all.
		case strings.HasPrefix(label, "taxa"):
			found[columnTaxes] = index
		// "Total" bare in some layouts, "Vlr. Total" in the template this repository
		// ships. Worth matching loosely: this column is not imported, it is what
		// detectPointsUnit divides by to work out whether a "64" in the points column
		// means 64 points or 64 mil. Without it that decision falls back to a much
		// weaker guess, and getting it wrong is a thousand-fold error on every row.
		case strings.Contains(label, "total"):
			found[columnTotal] = index
		case strings.Contains(label, "conta"):
			found[columnAccount] = index
		case strings.Contains(label, "emissor"):
			found[columnIssuer] = index
		// Spelled out in some layouts, abbreviated to "Obs." in the template this
		// repository ships - where it was matching neither, so every note the operators
		// had written beside a booking was dropped in silence.
		case strings.Contains(label, "observ") || strings.HasPrefix(label, "obs"):
			found[columnObservation] = index
		case label == "no" || label == "n":
			found[columnSequence] = index
		}
	}

	if _, ok := found[columnLocator]; !ok {
		return nil, false
	}
	return found, true
}

// foldHeader normalises a header cell to what it should compare as: lower case, no
// accents, no surrounding space. "Nº" becomes "no", "Cia. Aérea" becomes "cia. aerea".
func foldHeader(value string) string {
	var folded strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch r {
		case 'á', 'à', 'â', 'ã', 'ä':
			folded.WriteRune('a')
		case 'é', 'ê', 'ë':
			folded.WriteRune('e')
		case 'í', 'î', 'ï':
			folded.WriteRune('i')
		case 'ó', 'ô', 'õ', 'ö':
			folded.WriteRune('o')
		case 'ú', 'û', 'ü':
			folded.WriteRune('u')
		case 'ç':
			folded.WriteRune('c')
		case 'º', '°', 'ª':
			folded.WriteRune('o')
		default:
			folded.WriteRune(r)
		}
	}
	return folded.String()
}

func closeBlock(blocks []Block, current *Block) []Block {
	if current == nil {
		return blocks
	}
	return append(blocks, *current)
}

func parseRow(number int, programLabel, locator string, cells []string, cols columns, unit pointsUnit) Row {
	programLocator, airlineLocator := splitLocator(locator)

	return Row{
		Number:             number,
		ProgramLabel:       programLabel,
		ProgramLocator:     programLocator,
		AirlineLocator:     airlineLocator,
		FlightDate:         parseSerialDate(at(cells, cols, columnFlightDate)),
		PassengerName:      text(at(cells, cols, columnPassenger)),
		OriginLabel:        text(at(cells, cols, columnOrigin)),
		DestinationLabel:   text(at(cells, cols, columnDestination)),
		Points:             parsePoints(at(cells, cols, columnPoints), unit),
		ValueOfTheThousand: parseCents(at(cells, cols, columnValueOfTheThousand)),
		Taxes:              optionalCents(at(cells, cols, columnTaxes)),
		AccountLabel:       text(at(cells, cols, columnAccount)),
		IssuerName:         text(at(cells, cols, columnIssuer)),
		Observation:        text(at(cells, cols, columnObservation)),
	}
}

// issuerFromTitle reads the operator's name off the block title. Two conventions are in
// use: the shared workbook writes "CONTROLE DE EMISSÕES - 01/08/2026 - MARCUS", where the
// name is the last segment, and each personal workbook writes "CONTROLE AARON - 15/08/2026",
// where it is part of the first. Segments that are only a date are dropped either way,
// since the date is sometimes missing and sometimes still the template's placeholder.
func issuerFromTitle(title string) string {
	var named []string
	for _, part := range strings.Split(title, " - ") {
		part = strings.TrimSpace(part)
		if part == "" || looksLikeDate(part) {
			continue
		}
		named = append(named, part)
	}
	if len(named) == 0 {
		return ""
	}

	last := named[len(named)-1]
	if !strings.HasPrefix(strings.ToUpper(last), blockTitlePrefix) {
		return last
	}
	// "CONTROLE AARON" names Aaron; "CONTROLE DE EMISSÕES" names nobody, and its own
	// issuer would have been a later segment.
	name := strings.TrimSpace(last[len(blockTitlePrefix):])
	if foldHeader(name) == "de emissoes" {
		return ""
	}
	return name
}

// looksLikeDate covers the two shapes a title's date segment takes, including the
// "%%/08/2026" the template ships with and at least one day still carries.
var titleDate = regexp.MustCompile(`^[\d%]{1,2}/[\d%]{1,2}/[\d%]{2,4}$`)

func looksLikeDate(part string) bool {
	return titleDate.MatchString(part)
}

// splitLocator separates the program's locator from the airline's, which share one cell
// separated by "/" - the same syntax the issuances grid takes in its Localizador column.
func splitLocator(value string) (program, airline string) {
	program, airline, found := strings.Cut(value, "/")
	if !found {
		return strings.TrimSpace(value), ""
	}
	return strings.TrimSpace(program), strings.TrimSpace(airline)
}

func cell(cells []string, index int) string {
	if index < 0 || index >= len(cells) {
		return ""
	}
	return cells[index]
}

// at reads the cell of a named column, or "" when this sheet's layout does not have it.
func at(cells []string, cols columns, column string) string {
	index, ok := cols[column]
	if !ok {
		return ""
	}
	return cell(cells, index)
}

// text trims a cell and reads the placeholder the operators type into columns they have
// nothing for as the empty value it stands for.
func text(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "-" {
		return ""
	}
	return trimmed
}

func parseFloat(value string) (float64, bool) {
	trimmed := text(value)
	if trimmed == "" {
		return 0, false
	}
	number, err := strconv.ParseFloat(strings.ReplaceAll(trimmed, ",", "."), 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// A pointsUnit is how one sheet writes its Pontos column. The workbooks disagree: some
// keep points in thousands ("29.577" meaning 29577 points) and the personal ones write
// them out whole ("64000"). Reading one as the other is off by a factor of a thousand,
// and nothing downstream compares points against a balance, so it would never be caught.
type pointsUnit int

const (
	unitThousands pointsUnit = iota
	unitRaw
)

func (u pointsUnit) String() string {
	if u == unitRaw {
		return "raw"
	}
	return "thousands"
}

// parsePoints reads the Pontos column into the count issuances.points holds.
func parsePoints(value string, unit pointsUnit) int {
	number, ok := parseFloat(value)
	if !ok {
		return 0
	}
	if unit == unitRaw {
		return int(math.Round(number))
	}
	return int(math.Round(number * 1000))
}

// detectPointsUnit works the unit out from the sheet itself rather than from which file it
// came in. The Total column is the arithmetic that settles it: it is points ÷ 1000 ×
// milheiro when the points are whole, and points × milheiro when they are in thousands.
// Every row where the three cells are numbers casts a vote.
//
// With no row to go by - a sheet whose Total is blank or a formula error - the shape of
// the numbers decides: a whole number of a thousand or more is a point count, and anything
// fractional or smaller is a count of thousands.
func detectPointsUnit(rows [][]string) pointsUnit {
	var cols columns
	var raw, thousands int
	var sawFractional, sawWhole bool

	for _, cells := range rows {
		if header, ok := headerColumns(cells); ok {
			cols = header
			continue
		}
		if cols == nil {
			continue
		}

		points, okPoints := parseFloat(at(cells, cols, columnPoints))
		if !okPoints || points == 0 {
			continue
		}
		if points != math.Trunc(points) || math.Abs(points) < 1000 {
			sawFractional = true
		} else {
			sawWhole = true
		}

		thousand, okThousand := parseFloat(at(cells, cols, columnValueOfTheThousand))
		total, okTotal := parseFloat(at(cells, cols, columnTotal))
		if !okThousand || !okTotal || thousand == 0 || total == 0 {
			continue
		}

		// Compared on the absolute values: an estorno line writes the points negative
		// and the total sometimes positive, and the sign is not what is being decided.
		asRaw := math.Abs(math.Abs(points)*thousand/1000 - math.Abs(total))
		asThousands := math.Abs(math.Abs(points)*thousand - math.Abs(total))
		if asRaw < asThousands {
			raw++
		} else {
			thousands++
		}
	}

	switch {
	case raw > thousands:
		return unitRaw
	case thousands > raw:
		return unitThousands
	case sawWhole && !sawFractional:
		return unitRaw
	default:
		return unitThousands
	}
}

// parseCents reads a money column. The sheet is in reais and the database in centavos.
func parseCents(value string) int {
	number, ok := parseFloat(value)
	if !ok {
		return 0
	}
	return int(math.Round(number * 100))
}

// optionalCents distinguishes "no taxes on this booking" (nil) from "the taxes were zero",
// which the Taxas column expresses by being blank rather than by holding a 0.
func optionalCents(value string) *int {
	number, ok := parseFloat(value)
	if !ok {
		return nil
	}
	cents := int(math.Round(number * 100))
	return &cents
}

// parseSerialDate turns Excel's day-count into a calendar day. Only the date matters here -
// the flight time is not something the spreadsheet ever carried.
func parseSerialDate(value string) *time.Time {
	serial, ok := parseFloat(value)
	if !ok || serial <= 0 {
		return nil
	}

	parsed, err := excelize.ExcelDateToTime(serial, false)
	if err != nil {
		return nil
	}

	date := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
	return &date
}
