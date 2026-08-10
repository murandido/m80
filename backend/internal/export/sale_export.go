package export

import (
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// Unlike the issuances export, this sheet is built from scratch rather than from a MODELO
// template (see template.go). Five columns do not justify an external file that has to be
// downloaded and cached at runtime - building it here behaves identically in dev, in CI and
// in production, and lets the test actually run instead of skipping for a missing asset.
const (
	saleExportSheet = "VENDAS"
	// Row 1 is the header, so the first sale goes on row 2.
	saleExportFirstDataRow = 2
)

// The three values the STATUS column can hold. PG/APG were the pair asked for; CANC exists
// because paid and cancelled are independent facts - a sale can be paid *and* have had its
// issuance cancelled, which is money already collected that has to go back. Collapsing that
// into PG would hide the only row on the sheet that needs acting on.
const (
	saleStatusPaid      = "PG"
	saleStatusUnpaid    = "APG"
	saleStatusCancelled = "CANC"
)

// A sale with no client linked yet. Left blank the row would read as an oversight in the
// spreadsheet rather than what it is: a charge with nobody to bill.
const saleExportNoClient = "(sem cliente)"

// SaleRow is what the spreadsheet needs from one sale. The export package defines its own
// input shape because the handlers package imports this one - the dependency cannot be
// reversed - so the handler maps its query rows onto this.
type SaleRow struct {
	IssuedAt       time.Time
	ClientName     *string
	ProgramLocator string
	// What the client owes: points/1000 * value_of_the_thousand + taxes, in centavos.
	// Computed by the caller so the sheet and the screen can never disagree by a cent.
	AmountCents int
	Paid        bool
	CancelledAt *time.Time
}

// BuildSalesXLSX renders the given sales into a new spreadsheet and returns the .xlsx bytes.
func BuildSalesXLSX(sales []SaleRow) ([]byte, error) {
	loc, err := time.LoadLocation(exportTimezone)
	if err != nil {
		loc = time.UTC
	}
	return renderSalesXLSX(sales, loc)
}

// renderSalesXLSX holds the pure rendering logic, with the location passed in explicitly so
// it can be exercised without depending on the host's tzdata.
func renderSalesXLSX(sales []SaleRow, loc *time.Location) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	if err := f.SetSheetName(f.GetSheetName(0), saleExportSheet); err != nil {
		return nil, fmt.Errorf("renderSalesXLSX: name sheet: %w", err)
	}

	styles, err := newSaleExportStyles(f)
	if err != nil {
		return nil, err
	}

	if err := writeSaleHeader(f, styles); err != nil {
		return nil, err
	}

	for index, sale := range sales {
		if err := writeSaleRow(f, saleExportFirstDataRow+index, sale, styles, loc); err != nil {
			return nil, err
		}
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("renderSalesXLSX: write buffer: %w", err)
	}

	return buffer.Bytes(), nil
}

// saleExportStyles holds the style ids excelize hands back, resolved once per file.
type saleExportStyles struct {
	header    int
	date      int
	text      int
	amount    int
	paid      int
	unpaid    int
	cancelled int
}

func newSaleExportStyles(f *excelize.File) (saleExportStyles, error) {
	var styles saleExportStyles

	// A status style is a centred cell in one of three colours. Only the colours differ, so
	// the shape is built once here.
	statusStyle := func(fontColor, fillColor string) *excelize.Style {
		return &excelize.Style{
			Font:      &excelize.Font{Bold: true, Color: fontColor},
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{fillColor}},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		}
	}

	definitions := []struct {
		target *int
		style  *excelize.Style
	}{
		{&styles.header, &excelize.Style{
			Font:      &excelize.Font{Bold: true, Color: "FF1E293B"},
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFE2E8F0"}},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		}},
		// A real date with a display format, not a pre-formatted string, so the column can
		// be sorted and filtered as a date.
		{&styles.date, &excelize.Style{
			CustomNumFmt: stringPtr("dd/mm/yyyy"),
			Alignment:    &excelize.Alignment{Horizontal: "center"},
		}},
		{&styles.text, &excelize.Style{
			Alignment: &excelize.Alignment{Vertical: "center"},
		}},
		// Likewise a real number: this is what lets whoever opens the sheet select the
		// column and read the total off the status bar.
		{&styles.amount, &excelize.Style{
			CustomNumFmt: stringPtr(`"R$" #,##0.00`),
		}},
		{&styles.paid, statusStyle("FF166534", "FFDCFCE7")},
		{&styles.unpaid, statusStyle("FF475569", "FFF1F5F9")},
		{&styles.cancelled, statusStyle("FF991B1B", "FFFEE2E2")},
	}

	for _, definition := range definitions {
		id, err := f.NewStyle(definition.style)
		if err != nil {
			return styles, fmt.Errorf("newSaleExportStyles: %w", err)
		}
		*definition.target = id
	}

	return styles, nil
}

func writeSaleHeader(f *excelize.File, styles saleExportStyles) error {
	headers := []struct {
		column string
		label  string
		width  float64
	}{
		{"A", "DATA", 14},
		{"B", "CLIENTE", 32},
		{"C", "LOCALIZADOR", 18},
		{"D", "EMISSÃO", 16},
		{"E", "STATUS", 12},
	}

	for _, header := range headers {
		cell := header.column + "1"
		if err := f.SetCellValue(saleExportSheet, cell, header.label); err != nil {
			return fmt.Errorf("writeSaleHeader: set %s: %w", cell, err)
		}
		if err := f.SetCellStyle(saleExportSheet, cell, cell, styles.header); err != nil {
			return fmt.Errorf("writeSaleHeader: style %s: %w", cell, err)
		}
		if err := f.SetColWidth(saleExportSheet, header.column, header.column, header.width); err != nil {
			return fmt.Errorf("writeSaleHeader: width %s: %w", header.column, err)
		}
	}

	// Keep the header in view while scrolling - these sheets are read a period at a time.
	panes := &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}
	if err := f.SetPanes(saleExportSheet, panes); err != nil {
		return fmt.Errorf("writeSaleHeader: freeze panes: %w", err)
	}

	return nil
}

func writeSaleRow(f *excelize.File, row int, sale SaleRow, styles saleExportStyles, loc *time.Location) error {
	set := func(column string, value any, style int) error {
		cell := fmt.Sprintf("%s%d", column, row)
		if err := f.SetCellValue(saleExportSheet, cell, value); err != nil {
			return fmt.Errorf("writeSaleRow: set %s: %w", cell, err)
		}
		if err := f.SetCellStyle(saleExportSheet, cell, cell, style); err != nil {
			return fmt.Errorf("writeSaleRow: style %s: %w", cell, err)
		}
		return nil
	}

	// A DATA - the day the issuance was registered, in the operator's calendar.
	if err := set("A", calendarDate(sale.IssuedAt, loc), styles.date); err != nil {
		return err
	}
	// B CLIENTE
	clientName := saleExportNoClient
	if sale.ClientName != nil && *sale.ClientName != "" {
		clientName = *sale.ClientName
	}
	if err := set("B", clientName, styles.text); err != nil {
		return err
	}
	// C LOCALIZADOR - one sale is one issuance (sales.issuance_id is unique), so exactly one.
	if err := set("C", sale.ProgramLocator, styles.text); err != nil {
		return err
	}
	// D EMISSÃO - what the client owes for this sale.
	if err := set("D", centsToReais(sale.AmountCents), styles.amount); err != nil {
		return err
	}
	// E STATUS
	status, statusStyle := saleStatus(sale, styles)
	return set("E", status, statusStyle)
}

// saleStatus resolves the STATUS cell. Cancellation wins over payment: once the issuance is
// gone, whether the money came in is a detail of *how* it has to be undone, not of whether.
func saleStatus(sale SaleRow, styles saleExportStyles) (string, int) {
	switch {
	case sale.CancelledAt != nil:
		return saleStatusCancelled, styles.cancelled
	case sale.Paid:
		return saleStatusPaid, styles.paid
	default:
		return saleStatusUnpaid, styles.unpaid
	}
}

func stringPtr(value string) *string {
	return &value
}
