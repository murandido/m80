package export

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/xuri/excelize/v2"
)

// The sheet mirrors the manual "Controle de Emissões - <mês> <ano>" workbook the team still
// keeps while the migration to the M-80 finishes: the same thirteen columns in the same order
// (A:M), same per-airline blocks, same Total formula, so a stretch pastes into the shared sheet
// with nothing to rearrange first. Two columns the manual sheet does not have sit after them:
// Taxa Resgate in N, and Emissor in O - every issuer is on this one tab, so Emissor is what
// says who made each row (it used to be one tab per person).
//
// It is built from scratch with excelize, like sale_export.go. It used to be a copy of a
// MODELO downloaded from Google Sheets at runtime and cached on disk; that made the export
// depend on a third party being reachable, and pinned the layout to a spreadsheet nobody in
// the repository could see.
const (
	issuanceExportTitleRow     = 1
	issuanceExportHeaderRow    = 2
	issuanceExportFirstDataRow = 3
	// The data table ends at O (Emissor); P is left empty as the gutter before the
	// per-account summary that starts at Q.
	issuanceExportLastColumn = "O"

	// The single tab holding every issuance, and what the Emissor cell shows for a row whose
	// issuer could not be resolved (a soft-deleted user, or none at all).
	issuanceExportSheetName     = "Emissões"
	issuanceExportNoIssuerLabel = "SEM EMISSOR"
)

// exportTimezone matches the timezone the issuance list uses to resolve "today", so the
// dates written to the sheet are the operator's local calendar days, not UTC.
const exportTimezone = "America/Sao_Paulo"

// The thirteen columns of the manual sheet, in its order, plus the redemption fee after them.
// The widths are not fixed here any more: autoFitColumns sizes each one to what the export
// actually wrote.
var issuanceExportColumns = []struct {
	column string
	label  string
}{
	{"A", "Nº"},
	{"B", "Companhia"},
	{"C", "Localizador"},
	{"D", "Data do Voo"},
	{"E", "Passageiro Principal"},
	{"F", "Origem"},
	{"G", "Destino"},
	{"H", "Pontos"},
	{"I", "Milheiro"},
	{"J", "Taxa"},
	{"K", "Total"},
	{"L", "Conta"},
	{"M", "Observação"},
	{"N", "Taxa Resgate"},
	// Emissor closes the table: with every issuer on one sheet it is what says who made each
	// row, the job the separate per-issuer tabs used to do.
	{"O", "Emissor"},
}

// exportRow is one issuance reduced to what the sheet writes. Resolved once, up front, so
// the table and the per-account summary can never disagree about a number: both read the
// same signed points.
type exportRow struct {
	program     string
	locator     string
	flightDate  *time.Time
	passenger   string
	origin      string
	destination string
	// Points in thousands (milheiros), which is the unit the manual sheet's Pontos column
	// holds and what its Total formula multiplies by the Milheiro rate.
	milheiros          float64
	valueOfTheThousand float64
	// Nil leaves the cell blank, as the manual sheet does when an issuance had no taxes.
	taxes   *float64
	account string
	// Free text; blank on an issuance with no observation.
	observation string
	// Who the row is attributed to: the canceller on a cancelled row, the issuer otherwise -
	// the same rule the per-issuer tabs used (issuanceSheetOwner).
	issuer string
	// The fee the exempt account (Azul Diamante Unique) did not pay, signed like the taxes.
	// Nil on an ordinary account, where nothing was saved.
	redemptionFee *float64
	// Cancelled rows are tinted so a reconciled block shows at a glance which lines are
	// giving miles back rather than spending them.
	cancelled bool
}

// issuanceSheet is the single tab holding every issuance in the exported period. Who made each
// row is carried by the Emissor column rather than by splitting the workbook into one tab per
// person, which is what it used to do.
type issuanceSheet struct {
	rows []exportRow
}

// BuildIssuancesXLSX renders the given issuances into one worksheet and returns the resulting
// .xlsx bytes.
func BuildIssuancesXLSX(issuances []data.Issuance) ([]byte, error) {
	return renderIssuancesXLSX(issuances, time.Now())
}

// renderIssuancesXLSX holds the pure rendering logic: it takes the reference time explicitly
// so it can be exercised without depending on the clock.
func renderIssuancesXLSX(issuances []data.Issuance, now time.Time) ([]byte, error) {
	loc, err := time.LoadLocation(exportTimezone)
	if err != nil {
		loc = time.UTC
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	styles := newStyleCache(f)

	// One tab, always. An empty result still gets it - a titled empty sheet says "nothing
	// matched" rather than a workbook with no sheet at all, which Excel refuses to open.
	sheet := issuanceSheet{rows: exportRowsFor(issuances, loc)}
	if err := f.SetSheetName(f.GetSheetName(0), issuanceExportSheetName); err != nil {
		return nil, fmt.Errorf("renderIssuancesXLSX: name sheet: %w", err)
	}
	if err := writeIssuanceSheet(f, issuanceExportSheetName, sheet, styles, now, loc); err != nil {
		return nil, err
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("renderIssuancesXLSX: write buffer: %w", err)
	}

	return buffer.Bytes(), nil
}

// issuanceSheetOwner names the tab an issuance belongs on: whoever cancelled it when it is
// cancelled, and whoever issued it otherwise.
//
// The cancellation branch is what makes the sheet agree with the screen. The listing surfaces
// a cancelled issuance in the flow of the person who cancelled it (applyIssuanceListFilters
// matches cancelled_by_id too), so filtering by an operator brings in issuances somebody else
// created and they cancelled. Keying the tab off the creator sent exactly those rows to a tab
// with the other person's name, and the operator - reading their own tab - saw them as missing.
func issuanceSheetOwner(issuance data.Issuance) string {
	// Keyed off CancelledAt rather than Active for the same reason toExportRow tints the row
	// off it: the cancellation timestamp is what says the estorno happened.
	if issuance.CancelledAt != nil && issuance.CancelledBy != nil && issuance.CancelledBy.Name != "" {
		return issuance.CancelledBy.Name
	}
	if issuance.Issuer != nil && issuance.Issuer.Name != "" {
		return issuance.Issuer.Name
	}
	return issuanceExportNoIssuerLabel
}

// exportRowsFor flattens the issuances in the order they came (created_at asc), which the
// airline blocks then preserve.
func exportRowsFor(issuances []data.Issuance, loc *time.Location) []exportRow {
	rows := make([]exportRow, 0, len(issuances))
	for _, issuance := range issuances {
		rows = append(rows, toExportRow(issuance, loc))
	}
	return rows
}

// toExportRow flattens one issuance into the values the sheet writes, applying the signed
// period rule to the points and the taxes.
func toExportRow(issuance data.Issuance, loc *time.Location) exportRow {
	row := exportRow{
		locator:            issuance.ProgramLocator,
		account:            accountLabel(issuance),
		valueOfTheThousand: centsToReais(issuance.ValueOfTheThousand),
	}
	if issuance.LoyaltyProgram != nil {
		row.program = issuance.LoyaltyProgram.Name
	}
	if issuance.MainPassengerName != nil {
		row.passenger = *issuance.MainPassengerName
	}
	if issuance.OriginIATACode != nil {
		row.origin = *issuance.OriginIATACode
	}
	if issuance.DestinationIATACode != nil {
		row.destination = *issuance.DestinationIATACode
	}
	if issuance.Observation != nil {
		row.observation = *issuance.Observation
	}
	if flightDate, ok := outboundFlightDate(issuance, loc); ok {
		row.flightDate = &flightDate
	}

	row.cancelled = issuance.CancelledAt != nil
	row.issuer = issuanceSheetOwner(issuance)

	points, taxes := signedIssuanceAmounts(issuance, loc)
	row.milheiros = float64(points) / 1000
	if taxes != nil {
		amount := centsToReais(*taxes)
		row.taxes = &amount
	}
	if fee := signedRedemptionFee(issuance, loc); fee != nil {
		amount := centsToReais(*fee)
		row.redemptionFee = &amount
	}

	return row
}

// The palette is lifted cell by cell from the two spreadsheets this export has to look
// like: the "Controle de Emissões" workbook for the table, and the per-account block the
// operator keeps beside it for the summary. Both use Excel's own defaults for everything
// not named here, so a pasted block is indistinguishable from a typed one.
const (
	issuanceExportFont     = "Aptos Narrow"
	issuanceSummaryFont    = "Calibri"
	issuanceExportBlack    = "000000"
	issuanceExportWhite    = "FFFFFF"
	issuanceExportBandBlue = "5B9BD5" // the blue banner over the TOTAL DAS CONTAS block
	issuanceExportHeaderBG = "4472C4" // the header row
	issuanceExportNumberBG = "404040" // the Nº column
	issuanceExportBandBG   = "0D0D0D" // the near-black totals band
	issuanceExportTaxSumBG = "C00000"
	issuanceExportSumBG    = "BFBFBF"
	issuanceExportBillBG   = "747474"
	issuanceExportBillFG   = "FFFF00"
	// A cancelled line: Excel's own "bad" pair, so it reads as an exception at a glance
	// without any explaining.
	issuanceExportVoidBG = "FFC7CE"
	issuanceExportVoidFG = "9C0006"

	// The spacer row between two airline blocks. Filled rather than left white so the break
	// reads as a deliberate separator instead of a hole in the table.
	issuanceExportSpacerBG = "8EA9DB"

	issuanceSummaryTitleBG = "385623"
	issuanceSummaryHeadBG  = "548135"
	issuanceSummaryHeadFG  = "F2F2F2"
	issuanceSummaryBodyBG  = "C5E0B3"
	issuanceSummaryTotalFG = "FF0000"
)

// Border weights, in excelize's numbering: 1 is thin, 2 is medium.
const (
	borderNone   = 0
	borderThin   = 1
	borderMedium = 2
)

// programColors is the airline colouring both spreadsheets apply to the Companhia cell.
// The table and the summary disagree on two of them - the control sheet fills Smiles
// yellow, the summary block orange - so each carries its own pair rather than one being
// declared the "right" one.
type programColors struct {
	tableFill   string
	summaryFill string
	// The table's own font colour; the summary always writes white on its fill.
	tableFont string
	// The three tints the TOTAL DAS CONTAS block uses: the airline strip, the paler cells
	// listing its accounts, and the "TOTAL PTS. DO DIA" chip on the right.
	totalsFill  string
	accountFill string
	dayFill     string
	// White text on the darker strips (A.A, and anything falling back to grey).
	totalsFont string
}

var programPalette = []struct {
	match string
	programColors
}{
	{"AZUL", programColors{
		tableFill: "A6CAEC", summaryFill: "0070C0", tableFont: issuanceExportBlack,
		totalsFill: "A6CAEC", accountFill: "DCEAF7", dayFill: "44B0E2", totalsFont: issuanceExportBlack,
	}},
	{"LATAM", programColors{
		tableFill: "FF0000", summaryFill: "FF0000", tableFont: issuanceExportBlack,
		totalsFill: "FF0000", accountFill: "C45858", dayFill: "FF0000", totalsFont: issuanceExportBlack,
	}},
	{"SMILES", programColors{
		tableFill: "FFFF00", summaryFill: "FFC000", tableFont: issuanceExportBlack,
		totalsFill: "E87331", accountFill: "FFE699", dayFill: "F57921", totalsFont: issuanceExportBlack,
	}},
	{"IBERIA", programColors{
		tableFill: "833C0C", summaryFill: "833C0C", tableFont: issuanceExportWhite,
		totalsFill: "833C0C", accountFill: "E66E6E", dayFill: "833C0C", totalsFont: issuanceExportWhite,
	}},
}

// An airline the manual sheet has no colour for: a neutral grey says "not one of the four"
// instead of borrowing a colour that already means something else.
var defaultProgramColors = programColors{
	tableFill:   "D9D9D9",
	summaryFill: "808080",
	tableFont:   issuanceExportBlack,
	totalsFill:  "002060",
	accountFill: "D9D9D9",
	dayFill:     "002060",
	totalsFont:  issuanceExportWhite,
}

// colorsForProgram matches on a substring because the M-80 stores fuller names than the
// spreadsheet's labels - "LATAM Pass" against LATAM, "TudoAzul" against AZUL.
func colorsForProgram(program string) programColors {
	upper := strings.ToUpper(program)
	for _, entry := range programPalette {
		if strings.Contains(upper, entry.match) {
			return entry.programColors
		}
	}
	return defaultProgramColors
}

// cellStyle is one cell's whole look. It is a comparable struct on purpose: styleCache keys
// on it, so the file ends up with one style per distinct appearance no matter how many
// cells wear it, instead of a new style id per cell.
type cellStyle struct {
	fill      string
	fontColor string
	fontName  string
	bold      bool
	size      float64
	numFmt    string
	left      int
	right     int
	top       int
	bottom    int
	// Everything in these sheets is centred both ways; the exceptions say so.
	horizontal string
	vertical   string
}

type styleCache struct {
	file *excelize.File
	ids  map[cellStyle]int
}

func newStyleCache(f *excelize.File) *styleCache {
	return &styleCache{file: f, ids: make(map[cellStyle]int)}
}

func (c *styleCache) id(spec cellStyle) (int, error) {
	if id, ok := c.ids[spec]; ok {
		return id, nil
	}

	style := &excelize.Style{
		Font: &excelize.Font{
			Bold:   spec.bold,
			Size:   orDefault(spec.size, 11),
			Color:  orDefaultString(spec.fontColor, issuanceExportBlack),
			Family: orDefaultString(spec.fontName, issuanceExportFont),
		},
		Alignment: &excelize.Alignment{
			Horizontal: orDefaultString(spec.horizontal, "center"),
			Vertical:   orDefaultString(spec.vertical, "center"),
		},
	}
	if spec.fill != "" {
		style.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{spec.fill}}
	}
	if spec.numFmt != "" {
		format := spec.numFmt
		style.CustomNumFmt = &format
	}
	edges := []struct {
		name  string
		width int
	}{
		{"left", spec.left},
		{"right", spec.right},
		{"top", spec.top},
		{"bottom", spec.bottom},
	}
	for _, edge := range edges {
		if edge.width == borderNone {
			continue
		}
		style.Border = append(style.Border, excelize.Border{
			Type:  edge.name,
			Style: edge.width,
			Color: issuanceExportBlack,
		})
	}

	id, err := c.file.NewStyle(style)
	if err != nil {
		return 0, fmt.Errorf("styleCache: %w", err)
	}
	c.ids[spec] = id
	return id, nil
}

func orDefault(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func orDefaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// The two number formats the control sheet uses. Points are written in thousands, which is
// why they show three decimals: 133854 points reads as 133,854 milheiros.
const (
	issuancePointsFormat = "#,##0.000"
	// The money format the filled-in sheet uses, negatives in red - which is exactly how a
	// cancellation's refunded taxes should read on a block being reconciled.
	issuanceMoneyFormat = `"R$"\ #,##0.00;[Red]\-"R$"\ #,##0.00`
	issuanceDateFormat  = "dd/mm/yy;@"
)

// writeIssuanceSheet lays out the tab: title, header, the airline blocks, the totals band,
// and the per-account summary off to the right.
func writeIssuanceSheet(
	f *excelize.File,
	sheet string,
	content issuanceSheet,
	styles *styleCache,
	now time.Time,
	loc *time.Location,
) error {
	if err := writeIssuanceTitle(f, sheet, styles, now, loc); err != nil {
		return err
	}
	if err := writeIssuanceHeader(f, sheet, styles); err != nil {
		return err
	}

	blocks, lastRow, err := writeProgramBlocks(f, sheet, content.rows, styles)
	if err != nil {
		return err
	}
	billingRow, err := writeIssuanceTotals(f, sheet, lastRow, styles)
	if err != nil {
		return err
	}
	if err := writeDailyTotals(f, sheet, blocks, billingRow, styles); err != nil {
		return err
	}

	if err := writeAccountSummary(f, sheet, content.rows, styles, now, loc); err != nil {
		return err
	}

	// Widths last, once every cell is written: excelize measures the text that is actually
	// there. Rows are left without an explicit height for the same reason - Excel then fits
	// them to their content on its own.
	return autoFitColumns(f, sheet)
}

// autoFitColumns sizes every column the sheet uses to its widest cell. The manual sheet's
// own fixed widths were dropped here: they were measured for its data, and a longer
// passenger name or observation ran under the next column.
//
// The title is skipped on purpose. It is merged across A:N, and excelize does not measure
// merged cells - it would size column A to the whole heading and push the table off screen.
func autoFitColumns(f *excelize.File, sheet string) error {
	// The merged cells are emptied first and put back afterwards. excelize does not handle
	// merges when measuring: it reads the anchor's text for every column the merge spans, so
	// the 26pt title alone would size all thirteen columns to its own length. Blanking them
	// leaves the measurement to the cells that actually sit in one column.
	restore, err := blankMergedCells(f, sheet)
	if err != nil {
		return err
	}

	columns := make([]string, 0, len(issuanceExportColumns)+4)
	for _, column := range issuanceExportColumns {
		columns = append(columns, column.column)
	}
	columns = append(columns,
		issuanceSummaryProgramColumn,
		issuanceSummaryLocatorColumn,
		issuanceSummaryPointsColumn,
		issuanceSummaryTotalColumn,
	)

	for _, column := range columns {
		if err := f.AutoFitColWidth(sheet, column); err != nil {
			return fmt.Errorf("autoFitColumns: %s: %w", column, err)
		}
	}

	return restore()
}

// blankMergedCells clears what every merged range on the sheet holds and returns the function
// that writes it back, formulas included. Styles are untouched: only the values go away, and
// only for as long as the widths are being measured.
func blankMergedCells(f *excelize.File, sheet string) (func() error, error) {
	merges, err := f.GetMergeCells(sheet)
	if err != nil {
		return nil, fmt.Errorf("blankMergedCells: read merges: %w", err)
	}

	type stashed struct {
		axis    string
		value   string
		formula string
	}

	held := make([]stashed, 0, len(merges))
	for _, merge := range merges {
		axis := merge.GetStartAxis()
		formula, err := f.GetCellFormula(sheet, axis)
		if err != nil {
			return nil, fmt.Errorf("blankMergedCells: read formula %s: %w", axis, err)
		}
		value, err := f.GetCellValue(sheet, axis)
		if err != nil {
			return nil, fmt.Errorf("blankMergedCells: read value %s: %w", axis, err)
		}
		if formula == "" && value == "" {
			continue
		}
		held = append(held, stashed{axis: axis, value: value, formula: formula})
		if err := f.SetCellValue(sheet, axis, ""); err != nil {
			return nil, fmt.Errorf("blankMergedCells: blank %s: %w", axis, err)
		}
	}

	return func() error {
		for _, cell := range held {
			if cell.formula != "" {
				if err := f.SetCellFormula(sheet, cell.axis, cell.formula); err != nil {
					return fmt.Errorf("blankMergedCells: restore formula %s: %w", cell.axis, err)
				}
				continue
			}
			if err := f.SetCellValue(sheet, cell.axis, cell.value); err != nil {
				return fmt.Errorf("blankMergedCells: restore %s: %w", cell.axis, err)
			}
		}
		return nil
	}, nil
}

func writeIssuanceTitle(
	f *excelize.File,
	sheet string,
	styles *styleCache,
	now time.Time,
	loc *time.Location,
) error {
	title := fmt.Sprintf("CONTROLE DE EMISSÕES - %s", now.In(loc).Format("02/01/2006"))
	first := fmt.Sprintf("A%d", issuanceExportTitleRow)
	last := fmt.Sprintf("%s%d", issuanceExportLastColumn, issuanceExportTitleRow)

	if err := f.MergeCell(sheet, first, last); err != nil {
		return fmt.Errorf("writeIssuerTitle: merge: %w", err)
	}
	if err := f.SetCellValue(sheet, first, title); err != nil {
		return fmt.Errorf("writeIssuerTitle: set %s: %w", first, err)
	}

	style, err := styles.id(cellStyle{
		// The same dark green as the account boxes to its right, so the two headings on the
		// sheet read as one design instead of two.
		fill:      issuanceSummaryTitleBG,
		fontColor: issuanceExportWhite,
		bold:      true,
		size:      26,
		left:      borderMedium,
		right:     borderMedium,
		top:       borderMedium,
		bottom:    borderMedium,
	})
	if err != nil {
		return err
	}
	if err := f.SetCellStyle(sheet, first, last, style); err != nil {
		return fmt.Errorf("writeIssuerTitle: style: %w", err)
	}
	return nil
}

func writeIssuanceHeader(f *excelize.File, sheet string, styles *styleCache) error {
	style, err := styles.id(cellStyle{
		fill:      issuanceExportHeaderBG,
		fontColor: issuanceExportWhite,
		bold:      true,
		left:      borderMedium,
		right:     borderMedium,
		top:       borderMedium,
		bottom:    borderMedium,
	})
	if err != nil {
		return err
	}

	for _, column := range issuanceExportColumns {
		cell := fmt.Sprintf("%s%d", column.column, issuanceExportHeaderRow)
		if err := f.SetCellValue(sheet, cell, column.label); err != nil {
			return fmt.Errorf("writeIssuanceHeader: set %s: %w", cell, err)
		}
		if err := f.SetCellStyle(sheet, cell, cell, style); err != nil {
			return fmt.Errorf("writeIssuanceHeader: style %s: %w", cell, err)
		}
	}

	// Keep the header in view while scrolling; a busy day runs well past one screen.
	panes := &excelize.Panes{
		Freeze:      true,
		YSplit:      issuanceExportHeaderRow,
		TopLeftCell: fmt.Sprintf("A%d", issuanceExportFirstDataRow),
		ActivePane:  "bottomLeft",
	}
	if err := f.SetPanes(sheet, panes); err != nil {
		return fmt.Errorf("writeIssuanceHeader: freeze panes: %w", err)
	}
	return nil
}

// writeProgramBlocks writes the rows grouped by airline, one blank row between blocks, the
// way the manual sheet separates AZUL from LATAM from SMILES. Returns the last row written,
// which is where the totals band goes. The blocks follow first appearance, and inside a
// block the rows keep the order the caller handed them (created_at asc).
func writeProgramBlocks(
	f *excelize.File,
	sheet string,
	rows []exportRow,
	styles *styleCache,
) ([]programBlock, int, error) {
	order := make([]string, 0, len(rows))
	byProgram := make(map[string][]exportRow, len(rows))
	for _, row := range rows {
		if _, seen := byProgram[row.program]; !seen {
			order = append(order, row.program)
		}
		byProgram[row.program] = append(byProgram[row.program], row)
	}

	sheetRow := issuanceExportFirstDataRow
	number := 1
	blocks := make([]programBlock, 0, len(order))
	for index, program := range order {
		// One blank spacer row between blocks, never before the first one.
		if index > 0 {
			if err := writeBlockSpacer(f, sheet, sheetRow, styles); err != nil {
				return nil, 0, err
			}
			sheetRow++
		}
		block := byProgram[program]
		placed := programBlock{program: program, firstRow: sheetRow, accounts: accountsIn(block)}
		for offset, row := range block {
			// The block is drawn as one framed box: a medium rule closes it top and bottom,
			// thin rules separate the rows inside it.
			top, bottom := borderThin, borderThin
			if offset == 0 {
				top = borderMedium
			}
			if offset == len(block)-1 {
				bottom = borderMedium
			}
			if err := writeIssuanceRow(f, sheet, sheetRow, number, row, styles, top, bottom); err != nil {
				return nil, 0, err
			}
			sheetRow++
			number++
		}
		placed.lastRow = sheetRow - 1
		blocks = append(blocks, placed)
	}

	// With no issuances at all the totals still need a range to sum, so point them at the
	// first (empty) data row instead of at the header.
	if sheetRow == issuanceExportFirstDataRow {
		return blocks, issuanceExportFirstDataRow, nil
	}
	return blocks, sheetRow - 1, nil
}

// writeBlockSpacer paints the row that separates two airline blocks across A:N. It carries no
// value and no number: it is the gap the manual sheet leaves between AZUL and SMILES, made
// visible so nobody reads it as a missing issuance.
func writeBlockSpacer(f *excelize.File, sheet string, sheetRow int, styles *styleCache) error {
	style, err := styles.id(cellStyle{
		fill:   issuanceExportSpacerBG,
		left:   borderMedium,
		right:  borderMedium,
		top:    borderMedium,
		bottom: borderMedium,
	})
	if err != nil {
		return err
	}

	first := fmt.Sprintf("A%d", sheetRow)
	last := fmt.Sprintf("%s%d", issuanceExportLastColumn, sheetRow)
	if err := f.SetCellStyle(sheet, first, last, style); err != nil {
		return fmt.Errorf("writeBlockSpacer: style %s:%s: %w", first, last, err)
	}
	return nil
}

// programBlock records where one airline's rows landed, so the TOTAL DAS CONTAS block below
// can point its SUMIFs at exactly that stretch of the table - the same shape the manual
// sheet's own formulas have.
type programBlock struct {
	program  string
	firstRow int
	lastRow  int
	accounts []string
}

// accountsIn lists the distinct accounts a block used, in alphabetical order so the totals
// come out the same on every export.
func accountsIn(rows []exportRow) []string {
	seen := make(map[string]bool, len(rows))
	accounts := make([]string, 0, len(rows))
	for _, row := range rows {
		if seen[row.account] {
			continue
		}
		seen[row.account] = true
		accounts = append(accounts, row.account)
	}
	sort.Strings(accounts)
	return accounts
}

// writeIssuanceRow fills a single data row (columns A:O) for one issuance.
func writeIssuanceRow(
	f *excelize.File,
	sheet string,
	sheetRow, number int,
	row exportRow,
	styles *styleCache,
	top, bottom int,
) error {
	colors := colorsForProgram(row.program)

	// Every cell shares the row's top/bottom rules and differs only in what sits inside it,
	// so the column specs are declared once here and applied in one pass.
	body := func(spec cellStyle) cellStyle {
		spec.top, spec.bottom = top, bottom
		if spec.left == borderNone {
			spec.left = borderThin
		}
		if spec.right == borderNone {
			spec.right = borderThin
		}
		// A cancelled line drops its own colours for the void tint, so it reads as an
		// exception at a glance.
		if row.cancelled {
			spec.fill = issuanceExportVoidBG
			spec.fontColor = issuanceExportVoidFG
		}
		return spec
	}

	// The airline cell is the exception: it keeps its airline colour even on a cancelled
	// line, because that colour is how a block is read down the page - tinting it too broke
	// the column into stripes. The strikethrough locator and the negative points already say
	// the line is cancelled.
	airlineBody := func(spec cellStyle) cellStyle {
		fill, fontColor := spec.fill, spec.fontColor
		spec = body(spec)
		spec.fill, spec.fontColor = fill, fontColor
		return spec
	}

	cells := []struct {
		column string
		value  any
		spec   cellStyle
	}{
		// A Nº - sequential within the tab, white on the dark strip the manual sheet uses.
		{"A", number, body(cellStyle{
			fill: issuanceExportNumberBG, fontColor: issuanceExportWhite,
			bold: true, size: 12, left: borderMedium,
		})},
		// B Companhia - repeated on every row of the block and coloured by airline, which is
		// what makes the blocks readable at a glance on the sheet being pasted into.
		{"B", strings.ToUpper(row.program), airlineBody(cellStyle{
			fill: colors.tableFill, fontColor: colors.tableFont, bold: true,
			left: borderMedium, right: borderMedium,
		})},
		{"C", row.locator, body(cellStyle{bold: true})},
		{"D", row.flightDate, body(cellStyle{numFmt: issuanceDateFormat})},
		{"E", row.passenger, body(cellStyle{})},
		{"F", row.origin, body(cellStyle{bold: true})},
		{"G", row.destination, body(cellStyle{bold: true})},
		// H Pontos - in thousands, negative when a cancellation gave miles back inside the
		// exported period, which is what makes the totals net out instead of counting the
		// booking twice.
		{"H", row.milheiros, body(cellStyle{numFmt: issuancePointsFormat})},
		// I Milheiro - always positive: it is a rate, and the sign of the line is carried by
		// the points and the taxes.
		{"I", row.valueOfTheThousand, body(cellStyle{numFmt: issuanceMoneyFormat})},
		{"J", row.taxes, body(cellStyle{numFmt: issuanceMoneyFormat})},
		{"K", nil, body(cellStyle{numFmt: issuanceMoneyFormat, bold: true})},
		{"L", row.account, body(cellStyle{bold: true})},
		{"M", row.observation, body(cellStyle{})},
		// N Taxa Resgate - blank on an ordinary account, like Taxa on an issuance with none.
		{"N", row.redemptionFee, body(cellStyle{numFmt: issuanceMoneyFormat})},
		// O Emissor - who made the row; closes the table on the right.
		{"O", row.issuer, body(cellStyle{right: borderMedium})},
	}

	for _, cell := range cells {
		reference := fmt.Sprintf("%s%d", cell.column, sheetRow)
		style, err := styles.id(cell.spec)
		if err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, reference, reference, style); err != nil {
			return fmt.Errorf("writeIssuanceRow: style %s: %w", reference, err)
		}
		// A nil value leaves the cell empty but styled: that is how the manual sheet shows
		// an issuance with no taxes and no flight date, and K carries a formula instead.
		if cell.value == nil {
			continue
		}
		switch value := cell.value.(type) {
		case *time.Time:
			if value == nil {
				continue
			}
			if err := f.SetCellValue(sheet, reference, *value); err != nil {
				return fmt.Errorf("writeIssuanceRow: set %s: %w", reference, err)
			}
		case *float64:
			if value == nil {
				continue
			}
			if err := f.SetCellValue(sheet, reference, *value); err != nil {
				return fmt.Errorf("writeIssuanceRow: set %s: %w", reference, err)
			}
		default:
			if err := f.SetCellValue(sheet, reference, cell.value); err != nil {
				return fmt.Errorf("writeIssuanceRow: set %s: %w", reference, err)
			}
		}
	}

	// K Total - the manual sheet's own formula, character for character, so a pasted block
	// keeps computing the same way the rows around it do.
	formula := fmt.Sprintf("SUM(H%d*I%d+J%d)", sheetRow, sheetRow, sheetRow)
	if err := f.SetCellFormula(sheet, fmt.Sprintf("K%d", sheetRow), formula); err != nil {
		return fmt.Errorf("writeIssuanceRow: set K%d formula: %w", sheetRow, err)
	}

	// No explicit row height anywhere on the sheet: every row is left for Excel to fit to
	// its own content.
	return nil
}

// writeIssuanceTotals closes the table the way the manual sheet does: a dark band carrying
// the summed taxes and the summed totals, then the billing total (what was charged minus
// what was only passed through as taxes) on the row below.
func writeIssuanceTotals(
	f *excelize.File,
	sheet string,
	lastDataRow int,
	styles *styleCache,
) (int, error) {
	sumRow := lastDataRow + 1
	billingRow := sumRow + 1

	band, err := styles.id(cellStyle{
		fill: issuanceExportBandBG, fontColor: issuanceExportWhite, top: borderMedium,
	})
	if err != nil {
		return 0, err
	}
	if err := f.SetCellStyle(sheet, fmt.Sprintf("A%d", sumRow), fmt.Sprintf("%s%d", issuanceExportLastColumn, sumRow), band); err != nil {
		return 0, fmt.Errorf("writeIssuanceTotals: band: %w", err)
	}

	sums := []struct {
		column  string
		formula string
		spec    cellStyle
	}{
		{"J", fmt.Sprintf("SUM(J%d:J%d)", issuanceExportFirstDataRow, lastDataRow), cellStyle{
			fill: issuanceExportTaxSumBG, fontColor: issuanceExportWhite, bold: true,
			numFmt: issuanceMoneyFormat,
			left:   borderMedium, right: borderMedium, top: borderMedium, bottom: borderMedium,
		}},
		{"K", fmt.Sprintf("SUM(K%d:K%d)", issuanceExportFirstDataRow, lastDataRow), cellStyle{
			fill: issuanceExportSumBG, bold: true, numFmt: issuanceMoneyFormat,
			right: borderMedium, top: borderMedium, bottom: borderMedium,
		}},
		// Summed apart from the billing: the fee was never charged to anyone, so it only
		// reports what the exempt accounts saved.
		{"N", fmt.Sprintf("SUM(N%d:N%d)", issuanceExportFirstDataRow, lastDataRow), cellStyle{
			fill: issuanceExportTaxSumBG, fontColor: issuanceExportWhite, bold: true,
			numFmt: issuanceMoneyFormat,
			left:   borderMedium, right: borderMedium, top: borderMedium, bottom: borderMedium,
		}},
	}
	for _, sum := range sums {
		cell := fmt.Sprintf("%s%d", sum.column, sumRow)
		if err := f.SetCellFormula(sheet, cell, sum.formula); err != nil {
			return 0, fmt.Errorf("writeIssuanceTotals: set %s: %w", cell, err)
		}
		style, err := styles.id(sum.spec)
		if err != nil {
			return 0, err
		}
		if err := f.SetCellStyle(sheet, cell, cell, style); err != nil {
			return 0, fmt.Errorf("writeIssuanceTotals: style %s: %w", cell, err)
		}
	}

	// TOTAL FATURAMENTO spans the columns left of the money, as it does on the manual sheet.
	labelFirst := fmt.Sprintf("A%d", billingRow)
	labelLast := fmt.Sprintf("J%d", billingRow)
	if err := f.MergeCell(sheet, labelFirst, labelLast); err != nil {
		return 0, fmt.Errorf("writeIssuanceTotals: merge label: %w", err)
	}
	if err := f.SetCellValue(sheet, labelFirst, "TOTAL FATURAMENTO"); err != nil {
		return 0, fmt.Errorf("writeIssuanceTotals: set %s: %w", labelFirst, err)
	}
	labelStyle, err := styles.id(cellStyle{
		fill: issuanceExportBandBG, fontColor: issuanceExportWhite, bold: true, size: 16,
	})
	if err != nil {
		return 0, err
	}
	if err := f.SetCellStyle(sheet, labelFirst, labelLast, labelStyle); err != nil {
		return 0, fmt.Errorf("writeIssuanceTotals: style label: %w", err)
	}

	billing := fmt.Sprintf("K%d", billingRow)
	if err := f.SetCellFormula(sheet, billing, fmt.Sprintf("K%d-J%d", sumRow, sumRow)); err != nil {
		return 0, fmt.Errorf("writeIssuanceTotals: set %s: %w", billing, err)
	}
	billingStyle, err := styles.id(cellStyle{
		fill: issuanceExportBillBG, fontColor: issuanceExportBillFG, bold: true, size: 14,
		numFmt: issuanceMoneyFormat,
		left:   borderMedium, right: borderMedium, bottom: borderMedium,
	})
	if err != nil {
		return 0, err
	}
	if err := f.SetCellStyle(sheet, billing, billing, billingStyle); err != nil {
		return 0, fmt.Errorf("writeIssuanceTotals: style %s: %w", billing, err)
	}
	// The rest of the row carries the band's colour so the block reads as one footer.
	if err := f.SetCellStyle(sheet, fmt.Sprintf("L%d", billingRow), fmt.Sprintf("%s%d", issuanceExportLastColumn, billingRow), labelStyle); err != nil {
		return 0, fmt.Errorf("writeIssuanceTotals: style footer tail: %w", err)
	}
	return billingRow, nil
}

// The TOTAL DAS CONTAS block that closes the manual sheet: on the left, how many miles each
// account gave to each airline and the GERAL that adds them all up; on the right, the day's
// points per airline and the day's billing. The left column's SUMIFs point at the airline
// blocks written above, so a corrected cell up there moves these totals too.
const (
	issuanceTotalsHeaderBG  = "5B9BD5"
	issuanceTotalsIssuerBG  = "4472C4"
	issuanceTotalsDayBG     = issuanceExportBlack
	issuanceTotalsSumBG     = "BFBFBF"
	issuanceTotalsGeneralBG = "595959"
	issuanceTotalsDaySumBG  = "404040"
	issuanceTotalsAccentFG  = "FFC000"
	issuanceTotalsBillingBG = "92D050"
	issuanceTotalsBillingFG = "3B7D23"
)

func writeDailyTotals(
	f *excelize.File,
	sheet string,
	blocks []programBlock,
	billingRow int,
	styles *styleCache,
) error {
	if len(blocks) == 0 {
		return nil
	}

	// Two blank rows below the billing line, so the block reads as its own thing.
	headerRow := billingRow + 3

	banner := func(fill string) cellStyle {
		return cellStyle{
			fill: fill, fontColor: issuanceExportWhite, bold: true, size: 14,
			left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
		}
	}
	headings := []struct {
		first, last string
		label       string
		spec        cellStyle
	}{
		{"B", "D", "TOTAL DAS CONTAS", banner(issuanceTotalsHeaderBG)},
		{"E", "E", "GERAL", banner(issuanceTotalsHeaderBG)},
		{"G", "H", "TOTAL PTS. DO DIA", banner(issuanceTotalsDayBG)},
		{"J", "K", "FATURAMENTO DO DIA", banner(issuanceTotalsDayBG)},
	}
	for _, heading := range headings {
		if err := writeMerged(f, sheet, heading.first, heading.last, headerRow, heading.label, heading.spec, styles); err != nil {
			return err
		}
	}

	// A general label for the day's block - every issuer is on this one sheet now - and beside
	// it the billing figure already computed above.
	issuerRow := headerRow + 1
	issuerStyle := cellStyle{
		fill: issuanceTotalsIssuerBG, fontColor: issuanceExportWhite, bold: true, size: 16,
		left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
	}
	if err := writeMerged(f, sheet, "B", "E", issuerRow, "TODOS OS EMISSORES", issuerStyle, styles); err != nil {
		return err
	}
	billingStyle := cellStyle{
		fill: issuanceTotalsBillingBG, fontColor: issuanceTotalsBillingFG, bold: true, size: 26,
		numFmt: issuanceMoneyFormat,
		left:   borderThin, right: borderThin, top: borderThin, bottom: borderThin,
	}
	if err := writeMerged(f, sheet, "J", "K", issuerRow, nil, billingStyle, styles); err != nil {
		return err
	}
	if err := f.SetCellFormula(sheet, fmt.Sprintf("J%d", issuerRow), fmt.Sprintf("K%d", billingRow)); err != nil {
		return fmt.Errorf("writeDailyTotals: billing formula: %w", err)
	}

	// Left column: one strip per airline, one line per account inside it.
	subtotalCells := make([]string, 0, len(blocks))
	row := issuerRow + 1
	for _, block := range blocks {
		colors := colorsForProgram(block.program)
		first, last := row, row+len(block.accounts)-1

		if err := writeMergedRows(f, sheet, "B", first, last, strings.ToUpper(block.program), cellStyle{
			fill: colors.totalsFill, fontColor: colors.totalsFont, bold: true, size: 18,
			left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
		}, styles); err != nil {
			return err
		}

		for offset, account := range block.accounts {
			accountRow := first + offset
			if err := writeCell(f, sheet, "C", accountRow, account, cellStyle{
				fill: colors.accountFill, bold: true, size: 14,
				left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
			}, styles); err != nil {
				return err
			}
			// The manual sheet's own formula: how many of this airline's miles went on this
			// account, read straight off the block above.
			formula := fmt.Sprintf(
				"SUMIF($L$%d:$L$%d,C%d,$H$%d:$H$%d)",
				block.firstRow, block.lastRow, accountRow, block.firstRow, block.lastRow,
			)
			if err := writeCell(f, sheet, "D", accountRow, nil, cellStyle{
				fill: issuanceExportWhite, size: 16, numFmt: issuancePointsFormat,
				left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
			}, styles); err != nil {
				return err
			}
			if err := f.SetCellFormula(sheet, fmt.Sprintf("D%d", accountRow), formula); err != nil {
				return fmt.Errorf("writeDailyTotals: account formula: %w", err)
			}
		}

		if err := writeMergedRows(f, sheet, "E", first, last, nil, cellStyle{
			fill: issuanceTotalsSumBG, bold: true, size: 24, numFmt: issuancePointsFormat,
			left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
		}, styles); err != nil {
			return err
		}
		subtotal := fmt.Sprintf("E%d", first)
		if err := f.SetCellFormula(sheet, subtotal, fmt.Sprintf("SUM(D%d:D%d)", first, last)); err != nil {
			return fmt.Errorf("writeDailyTotals: subtotal formula: %w", err)
		}
		subtotalCells = append(subtotalCells, subtotal)

		row = last + 1
	}

	// GERAL: every airline's subtotal added up, closed on the left by the same dark band the
	// billing line uses. Without the band the total floats loose beside three blank cells,
	// which is not how the sheet it is pasted next to ends.
	generalRow := row
	if err := writeMerged(f, sheet, "B", "D", generalRow, nil, cellStyle{
		fill: issuanceExportBandBG, fontColor: issuanceExportWhite,
		left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
	}, styles); err != nil {
		return err
	}
	if err := writeCell(f, sheet, "E", generalRow, nil, cellStyle{
		fill: issuanceTotalsGeneralBG, fontColor: issuanceTotalsAccentFG, bold: true, size: 24,
		numFmt: issuancePointsFormat,
		left:   borderThin, right: borderThin, top: borderThin, bottom: borderThin,
	}, styles); err != nil {
		return err
	}
	if err := f.SetCellFormula(sheet, fmt.Sprintf("E%d", generalRow), sumOf(subtotalCells)); err != nil {
		return fmt.Errorf("writeDailyTotals: general formula: %w", err)
	}

	// Right column: the day's points per airline, then their total.
	dayCells := make([]string, 0, len(blocks))
	for index, block := range blocks {
		dayRow := issuerRow + index
		colors := colorsForProgram(block.program)
		if err := writeCell(f, sheet, "G", dayRow, strings.ToUpper(block.program), cellStyle{
			fill: colors.dayFill, fontColor: colors.totalsFont, bold: true, size: 14,
			left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
		}, styles); err != nil {
			return err
		}
		if err := writeCell(f, sheet, "H", dayRow, nil, cellStyle{
			fill: issuanceTotalsSumBG, bold: true, size: 14, numFmt: issuancePointsFormat,
			left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
		}, styles); err != nil {
			return err
		}
		if err := f.SetCellFormula(sheet, fmt.Sprintf("H%d", dayRow), subtotalCells[index]); err != nil {
			return fmt.Errorf("writeDailyTotals: day formula: %w", err)
		}
		dayCells = append(dayCells, fmt.Sprintf("H%d", dayRow))
	}
	daySumRow := issuerRow + len(blocks)
	if err := writeCell(f, sheet, "H", daySumRow, nil, cellStyle{
		fill: issuanceTotalsDaySumBG, fontColor: issuanceTotalsAccentFG, bold: true, size: 16,
		numFmt: issuancePointsFormat,
		left:   borderThin, right: borderThin, top: borderThin, bottom: borderThin,
	}, styles); err != nil {
		return err
	}
	if err := f.SetCellFormula(sheet, fmt.Sprintf("H%d", daySumRow), sumOf(dayCells)); err != nil {
		return fmt.Errorf("writeDailyTotals: day total formula: %w", err)
	}

	return nil
}

// sumOf builds =SUM(a,b,c) over cells that are not contiguous - the airline subtotals sit on
// whichever rows their block happened to need.
func sumOf(cells []string) string {
	return "SUM(" + strings.Join(cells, ",") + ")"
}

// writeCell sets one cell's style, and its value when there is one (a nil value leaves the
// cell styled but empty, ready for a formula).
func writeCell(
	f *excelize.File,
	sheet, column string,
	row int,
	value any,
	spec cellStyle,
	styles *styleCache,
) error {
	reference := fmt.Sprintf("%s%d", column, row)
	style, err := styles.id(spec)
	if err != nil {
		return err
	}
	if err := f.SetCellStyle(sheet, reference, reference, style); err != nil {
		return fmt.Errorf("writeCell: style %s: %w", reference, err)
	}
	if value == nil {
		return nil
	}
	if err := f.SetCellValue(sheet, reference, value); err != nil {
		return fmt.Errorf("writeCell: set %s: %w", reference, err)
	}
	return nil
}

// writeMerged merges one row across columns and writes into it.
func writeMerged(
	f *excelize.File,
	sheet, firstColumn, lastColumn string,
	row int,
	value any,
	spec cellStyle,
	styles *styleCache,
) error {
	first := fmt.Sprintf("%s%d", firstColumn, row)
	last := fmt.Sprintf("%s%d", lastColumn, row)
	if firstColumn != lastColumn {
		if err := f.MergeCell(sheet, first, last); err != nil {
			return fmt.Errorf("writeMerged: merge %s:%s: %w", first, last, err)
		}
	}
	style, err := styles.id(spec)
	if err != nil {
		return err
	}
	if err := f.SetCellStyle(sheet, first, last, style); err != nil {
		return fmt.Errorf("writeMerged: style %s:%s: %w", first, last, err)
	}
	if value == nil {
		return nil
	}
	if err := f.SetCellValue(sheet, first, value); err != nil {
		return fmt.Errorf("writeMerged: set %s: %w", first, err)
	}
	return nil
}

// writeMergedRows merges one column down a range of rows and writes into it.
func writeMergedRows(
	f *excelize.File,
	sheet, column string,
	firstRow, lastRow int,
	value any,
	spec cellStyle,
	styles *styleCache,
) error {
	first := fmt.Sprintf("%s%d", column, firstRow)
	last := fmt.Sprintf("%s%d", column, lastRow)
	if lastRow > firstRow {
		if err := f.MergeCell(sheet, first, last); err != nil {
			return fmt.Errorf("writeMergedRows: merge %s:%s: %w", first, last, err)
		}
	}
	style, err := styles.id(spec)
	if err != nil {
		return err
	}
	if err := f.SetCellStyle(sheet, first, last, style); err != nil {
		return fmt.Errorf("writeMergedRows: style %s:%s: %w", first, last, err)
	}
	if value == nil {
		return nil
	}
	if err := f.SetCellValue(sheet, first, value); err != nil {
		return fmt.Errorf("writeMergedRows: set %s: %w", first, err)
	}
	return nil
}

// Where the per-account summary lives: column P is the gutter, so the boxes start at Q and
// span four columns, one box per account, stacked with a blank row between them. The look
// is the operator's own block: a dark green banner, a green header row, and rows on pale
// green with the airline cell coloured and the total in red.
const (
	issuanceSummaryProgramColumn = "Q"
	issuanceSummaryLocatorColumn = "R"
	issuanceSummaryPointsColumn  = "S"
	issuanceSummaryTotalColumn   = "T"
	// The summary writes points with the same three decimals as the table, without the
	// thousands separator the block beside the issuances uses.
	issuanceSummaryPointsFormat = "0.000"
)

// accountSummary is one box: what a single account issued on a single airline. The pair is
// the unit, not the account alone - an account used on two programmes gets two boxes, which
// is what the block being copied does (it filters on conta AND companhia) and the only way
// the box's own title stays true.
type accountSummary struct {
	account string
	program string
	rows    []exportRow
}

// writeAccountSummary reproduces, beside the table, the block the operator keeps by hand:
// one box per account used that day, listing its locators and points, with the account's
// total off to the right.
func writeAccountSummary(
	f *excelize.File,
	sheet string,
	rows []exportRow,
	styles *styleCache,
	now time.Time,
	loc *time.Location,
) error {
	summaries := summariseAccounts(rows)
	if len(summaries) == 0 {
		return nil
	}

	date := now.In(loc).Format("02/01/2006")
	row := issuanceExportTitleRow
	for _, summary := range summaries {
		next, err := writeAccountBox(f, sheet, row, summary, styles, date)
		if err != nil {
			return err
		}
		// One blank row between boxes, so two accounts never read as one list.
		row = next + 2
	}
	return nil
}

// summariseAccounts groups the rows by account and airline, ordered by account then airline
// so two exports of the same period lay the boxes out identically.
func summariseAccounts(rows []exportRow) []accountSummary {
	type key struct{ account, program string }

	boxes := make(map[key]*accountSummary)
	keys := make([]key, 0, len(rows))
	for _, row := range rows {
		id := key{account: row.account, program: row.program}
		summary, seen := boxes[id]
		if !seen {
			summary = &accountSummary{account: row.account, program: row.program}
			boxes[id] = summary
			keys = append(keys, id)
		}
		summary.rows = append(summary.rows, row)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].account != keys[j].account {
			return keys[i].account < keys[j].account
		}
		return keys[i].program < keys[j].program
	})

	summaries := make([]accountSummary, 0, len(keys))
	for _, id := range keys {
		summaries = append(summaries, *boxes[id])
	}
	return summaries
}

// writeAccountBox writes one box starting at startRow and returns the last row it used.
func writeAccountBox(
	f *excelize.File,
	sheet string,
	startRow int,
	summary accountSummary,
	styles *styleCache,
	date string,
) (int, error) {
	titleCell := fmt.Sprintf("%s%d", issuanceSummaryProgramColumn, startRow)
	titleEnd := fmt.Sprintf("%s%d", issuanceSummaryTotalColumn, startRow)
	title := fmt.Sprintf(
		"EMISSÕES %s - %s - %s",
		strings.ToUpper(summary.account),
		strings.ToUpper(summary.program),
		date,
	)
	if err := f.MergeCell(sheet, titleCell, titleEnd); err != nil {
		return 0, fmt.Errorf("writeAccountBox: merge title: %w", err)
	}
	if err := f.SetCellValue(sheet, titleCell, title); err != nil {
		return 0, fmt.Errorf("writeAccountBox: set %s: %w", titleCell, err)
	}
	titleStyle, err := styles.id(cellStyle{
		fill: issuanceSummaryTitleBG, fontColor: issuanceSummaryHeadFG,
		fontName: issuanceSummaryFont, bold: true, size: 12,
	})
	if err != nil {
		return 0, err
	}
	if err := f.SetCellStyle(sheet, titleCell, titleEnd, titleStyle); err != nil {
		return 0, fmt.Errorf("writeAccountBox: style title: %w", err)
	}

	headerRow := startRow + 1
	headerStyle, err := styles.id(cellStyle{
		fill: issuanceSummaryHeadBG, fontColor: issuanceSummaryHeadFG,
		fontName: issuanceSummaryFont, bold: true, size: 12,
		left: borderThin, right: borderThin, top: borderThin, bottom: borderThin,
	})
	if err != nil {
		return 0, err
	}
	headers := []struct {
		column string
		label  string
	}{
		{issuanceSummaryProgramColumn, "COMPANHIA"},
		{issuanceSummaryLocatorColumn, "LOCALIZADOR"},
		{issuanceSummaryPointsColumn, "PONTOS"},
		{issuanceSummaryTotalColumn, "TOTAL"},
	}
	for _, header := range headers {
		cell := fmt.Sprintf("%s%d", header.column, headerRow)
		if err := f.SetCellValue(sheet, cell, header.label); err != nil {
			return 0, fmt.Errorf("writeAccountBox: set %s: %w", cell, err)
		}
		if err := f.SetCellStyle(sheet, cell, cell, headerStyle); err != nil {
			return 0, fmt.Errorf("writeAccountBox: style %s: %w", cell, err)
		}
	}

	firstDataRow := headerRow + 1
	for index, row := range summary.rows {
		sheetRow := firstDataRow + index
		colors := colorsForProgram(row.program)
		cells := []struct {
			column string
			value  any
			spec   cellStyle
		}{
			{issuanceSummaryProgramColumn, strings.ToUpper(row.program), cellStyle{
				fill: colors.summaryFill, fontColor: issuanceExportWhite,
				fontName: issuanceSummaryFont, bold: true, size: 12,
			}},
			{issuanceSummaryLocatorColumn, row.locator, cellStyle{
				fill: issuanceSummaryBodyBG, fontName: issuanceSummaryFont, bold: true, size: 12,
			}},
			{issuanceSummaryPointsColumn, row.milheiros, cellStyle{
				fill: issuanceSummaryBodyBG, fontName: issuanceSummaryFont, bold: true, size: 12,
				numFmt: issuanceSummaryPointsFormat,
			}},
		}
		for _, cell := range cells {
			spec := cell.spec
			spec.left, spec.right = borderThin, borderThin
			spec.top, spec.bottom = borderThin, borderThin
			reference := fmt.Sprintf("%s%d", cell.column, sheetRow)
			if err := f.SetCellValue(sheet, reference, cell.value); err != nil {
				return 0, fmt.Errorf("writeAccountBox: set %s: %w", reference, err)
			}
			style, err := styles.id(spec)
			if err != nil {
				return 0, err
			}
			if err := f.SetCellStyle(sheet, reference, reference, style); err != nil {
				return 0, fmt.Errorf("writeAccountBox: style %s: %w", reference, err)
			}
		}
	}
	lastDataRow := firstDataRow + len(summary.rows) - 1

	// The total is a formula over the box's own points, not a number computed here: whoever
	// opens the sheet may correct a line, and the box has to follow.
	totalCell := fmt.Sprintf("%s%d", issuanceSummaryTotalColumn, firstDataRow)
	totalEnd := fmt.Sprintf("%s%d", issuanceSummaryTotalColumn, lastDataRow)
	if lastDataRow > firstDataRow {
		if err := f.MergeCell(sheet, totalCell, totalEnd); err != nil {
			return 0, fmt.Errorf("writeAccountBox: merge total: %w", err)
		}
	}
	formula := fmt.Sprintf(
		"SUM(%s%d:%s%d)",
		issuanceSummaryPointsColumn, firstDataRow,
		issuanceSummaryPointsColumn, lastDataRow,
	)
	if err := f.SetCellFormula(sheet, totalCell, formula); err != nil {
		return 0, fmt.Errorf("writeAccountBox: set %s: %w", totalCell, err)
	}
	totalStyle, err := styles.id(cellStyle{
		fill: issuanceSummaryBodyBG, fontColor: issuanceSummaryTotalFG,
		fontName: issuanceSummaryFont, bold: true, size: 12,
		numFmt: issuanceSummaryPointsFormat,
		left:   borderThin, right: borderThin, top: borderThin, bottom: borderThin,
	})
	if err != nil {
		return 0, err
	}
	if err := f.SetCellStyle(sheet, totalCell, totalEnd, totalStyle); err != nil {
		return 0, fmt.Errorf("writeAccountBox: style total: %w", err)
	}

	return lastDataRow, nil
}

// signedIssuanceAmounts decides what an issuance contributes to the sheet, and with which
// sign. The rule is the one the manual reconciliation uses, and it turns on whether the
// cancellation happened on the same calendar day as the booking:
//
//   - never cancelled: the miles and the taxes it spent, positive;
//   - cancelled the same day: what it actually cost, the spend minus the refund - the two
//     halves happened inside one day's sheet, so the day nets them itself;
//   - cancelled on a later day: only the refund, negative. The spend belongs to the day it
//     was made, which has its own sheet already; repeating it here would count it twice.
//
// A cancellation-only row never issued anything (data.Issuance.CancellationOnly), so only
// its refund counts whatever the dates say; without this it would pair a phantom emission
// with its own refund.
//
// "Same day" is the operator's calendar day in São Paulo, not a UTC one: a booking made at
// 22h and cancelled at 23h has to read as one day even though UTC has already rolled over.
//
// Taxes come back nil when there is no amount on either side, so the cell stays blank
// rather than claiming a zero the manual sheet never wrote.
func signedIssuanceAmounts(issuance data.Issuance, loc *time.Location) (int, *int) {
	spent := !issuance.CancellationOnly
	refunded := issuance.CancelledAt != nil
	if refunded && !sameCalendarDay(issuance.CreatedAt, *issuance.CancelledAt, loc) {
		spent = false
	}

	points := 0
	if spent {
		points += issuance.Points
	}
	if refunded && issuance.RefundedPoints != nil {
		points -= *issuance.RefundedPoints
	}

	var taxes *int
	if spent && issuance.Taxes != nil {
		total := *issuance.Taxes
		taxes = &total
	}
	if refunded && issuance.RefundedTaxes != nil {
		total := 0
		if taxes != nil {
			total = *taxes
		}
		total -= *issuance.RefundedTaxes
		taxes = &total
	}

	return points, taxes
}

// signedRedemptionFee is signedIssuanceAmounts' rule applied to the saved redemption fee.
// Nil - a blank cell - whenever the account is not exempt, because zero would claim a
// saving of nothing. Both sheets write it: Taxa Resgate here, TAXA RESGATE on the legacy one.
func signedRedemptionFee(issuance data.Issuance, loc *time.Location) *int {
	if issuance.RedemptionFeeSavedCents == nil {
		return nil
	}

	saved := !issuance.CancellationOnly
	refunded := issuance.CancelledAt != nil
	if refunded && !sameCalendarDay(issuance.CreatedAt, *issuance.CancelledAt, loc) {
		saved = false
	}

	total := 0
	if saved {
		total += *issuance.RedemptionFeeSavedCents
	}
	if refunded {
		total -= *issuance.RedemptionFeeSavedCents
	}
	return &total
}

// sameCalendarDay compares two instants as the operator's local calendar days.
func sameCalendarDay(a, b time.Time, loc *time.Location) bool {
	return calendarDate(a, loc).Equal(calendarDate(b, loc))
}

// accountLabel replicates the grid's account labelling (see grid-cell.tsx): the picked
// alias when set, otherwise the holder's name, the login, or the id as last resort.
func accountLabel(issuance data.Issuance) string {
	if issuance.AccountNickname != nil {
		return issuance.AccountNickname.Nickname
	}
	if issuance.Account != nil {
		if issuance.Account.Holder != nil && issuance.Account.Holder.Name != "" {
			return issuance.Account.Holder.Name
		}
		if issuance.Account.Login != nil && *issuance.Account.Login != "" {
			return *issuance.Account.Login
		}
	}
	return issuance.AccountID
}

func centsToReais(cents int) float64 {
	return float64(cents) / 100
}

// outboundFlightDate returns the local calendar date of the first outbound leg. The list
// query preloads only outbound flights ordered by leg_order, so index 0 is the departure.
func outboundFlightDate(issuance data.Issuance, loc *time.Location) (time.Time, bool) {
	if len(issuance.Flights) == 0 {
		return time.Time{}, false
	}
	return calendarDate(issuance.Flights[0].DepartureAt, loc), true
}

// calendarDate collapses a UTC timestamp to midnight of its local calendar day, expressed
// in UTC so excelize serialises a clean whole-number date with no time component.
func calendarDate(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}
