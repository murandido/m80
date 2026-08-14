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

// Columns of a block's table, in the fixed order the operators' template uses:
// A:Nº B:Companhia C:Localizador D:Data do Voo E:Passageiro Principal F:Origem
// G:Destino H:Pontos I:Milheiro J:Taxa K:Total L:Conta M:Observação.
const (
	columnSequence = iota
	columnProgram
	columnLocator
	columnFlightDate
	columnPassenger
	columnOrigin
	columnDestination
	columnPoints
	columnValueOfTheThousand
	columnTaxes
	columnTotal
	columnAccount
	columnObservation
)

// The three markers that give a sheet its shape. A day sheet is a stack of blocks, one per
// operator, and each block is a title, a header row, the issuances, and a closing total -
// after which come the operator's own summary tables, which are not issuances. Bounding a
// block by header -> total is what keeps those tables out.
const (
	blockTitlePrefix = "CONTROLE DE EMISSÕES"
	blockHeaderCell  = "Nº"
	blockFooterCell  = "TOTAL FATURAMENTO"
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
	Observation        string
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

		days = append(days, Day{Date: date, SheetName: name, Blocks: parseBlocks(rows)})
	}

	return days, nil
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

func parseBlocks(rows [][]string) []Block {
	var blocks []Block
	var current *Block
	inTable := false
	// The Companhia cell is filled on every row in practice, but a merged or blank one
	// means "same as above" rather than "no program", so the last seen value carries.
	programLabel := ""

	for index, cells := range rows {
		first := strings.TrimSpace(cell(cells, columnSequence))

		switch {
		case strings.HasPrefix(first, blockTitlePrefix):
			blocks = closeBlock(blocks, current)
			current = &Block{Index: len(blocks), IssuerName: issuerFromTitle(first)}
			inTable = false
			programLabel = ""
			continue
		case first == blockHeaderCell:
			inTable = true
			continue
		case first == blockFooterCell:
			inTable = false
			continue
		}

		if !inTable || current == nil {
			continue
		}

		if label := text(cell(cells, columnProgram)); label != "" {
			programLabel = label
		}

		// The template ships a few dozen blank rows per program so the operators can
		// fill them in; the locator is what tells a used row from a spare one.
		locator := text(cell(cells, columnLocator))
		if locator == "" {
			continue
		}

		current.Rows = append(current.Rows, parseRow(index+1, programLabel, locator, cells))
	}

	return closeBlock(blocks, current)
}

func closeBlock(blocks []Block, current *Block) []Block {
	if current == nil {
		return blocks
	}
	return append(blocks, *current)
}

func parseRow(number int, programLabel, locator string, cells []string) Row {
	programLocator, airlineLocator := splitLocator(locator)

	return Row{
		Number:             number,
		ProgramLabel:       programLabel,
		ProgramLocator:     programLocator,
		AirlineLocator:     airlineLocator,
		FlightDate:         parseSerialDate(cell(cells, columnFlightDate)),
		PassengerName:      text(cell(cells, columnPassenger)),
		OriginLabel:        text(cell(cells, columnOrigin)),
		DestinationLabel:   text(cell(cells, columnDestination)),
		Points:             parseThousands(cell(cells, columnPoints)),
		ValueOfTheThousand: parseCents(cell(cells, columnValueOfTheThousand)),
		Taxes:              optionalCents(cell(cells, columnTaxes)),
		AccountLabel:       text(cell(cells, columnAccount)),
		Observation:        text(cell(cells, columnObservation)),
	}
}

// issuerFromTitle reads the operator's name off "CONTROLE DE EMISSÕES - 01/08/2026 - MARCUS".
// The name is the last segment because the date in the middle is sometimes missing.
func issuerFromTitle(title string) string {
	parts := strings.Split(title, " - ")
	return strings.TrimSpace(parts[len(parts)-1])
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
	if index >= len(cells) {
		return ""
	}
	return cells[index]
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

// parseThousands reads the Pontos column, which the sheet keeps in thousands ("29.577"
// meaning 29577 points) while issuances.points counts them one by one.
func parseThousands(value string) int {
	number, ok := parseFloat(value)
	if !ok {
		return 0
	}
	return int(math.Round(number * 1000))
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
