package export

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func timeptr(t time.Time) *time.Time { return &t }

// saoPaulo is loaded once so the tests describe the same calendar the export uses. Falling
// back to a fixed offset keeps them meaningful on a machine without tzdata.
func saoPaulo(t *testing.T) *time.Location {
	t.Helper()

	loc, err := time.LoadLocation(exportTimezone)
	if err != nil {
		return time.FixedZone("-03", -3*60*60)
	}
	return loc
}

func TestSaleStatus(t *testing.T) {
	styles := saleExportStyles{paid: 1, unpaid: 2, cancelled: 3}
	cancelledAt := timeptr(time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))

	cases := []struct {
		name  string
		sale  SaleRow
		want  string
		style int
	}{
		{"paid", SaleRow{Paid: true}, saleStatusPaid, styles.paid},
		{"unpaid", SaleRow{Paid: false}, saleStatusUnpaid, styles.unpaid},
		{"cancelled and unpaid", SaleRow{CancelledAt: cancelledAt}, saleStatusCancelled, styles.cancelled},
		// The case the third status exists for: the money came in and now has to go back.
		{"cancelled after being paid", SaleRow{Paid: true, CancelledAt: cancelledAt}, saleStatusCancelled, styles.cancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, gotStyle := saleStatus(tc.sale, styles)
			if got != tc.want {
				t.Errorf("status = %q, want %q", got, tc.want)
			}
			if gotStyle != tc.style {
				t.Errorf("style = %d, want %d", gotStyle, tc.style)
			}
		})
	}
}

// TestRenderSalesXLSX renders the sheet and reads it back. Unlike the issuances export there
// is no external template, so this runs everywhere, CI included.
func TestRenderSalesXLSX(t *testing.T) {
	loc := saoPaulo(t)

	sales := []SaleRow{
		{
			// 22:00 UTC is still the 8th in São Paulo. The sheet has to say 08/08/2026.
			IssuedAt:       time.Date(2026, 8, 9, 1, 30, 0, 0, time.UTC),
			ClientName:     strptr("Ana Souza"),
			ProgramLocator: "ABC123",
			AmountCents:    245050,
			Paid:           true,
		},
		{
			IssuedAt:       time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC),
			ClientName:     nil,
			ProgramLocator: "XYZ789",
			AmountCents:    2450,
			Paid:           false,
		},
		{
			IssuedAt:       time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC),
			ClientName:     strptr("Maria Lima"),
			ProgramLocator: "QRS456",
			AmountCents:    89000,
			Paid:           true,
			CancelledAt:    timeptr(time.Date(2026, 8, 9, 18, 0, 0, 0, time.UTC)),
		},
	}

	out, err := renderSalesXLSX(sales, loc)
	if err != nil {
		t.Fatalf("renderSalesXLSX: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("reopen rendered file: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Formatted values: what someone opening the file actually sees.
	formatted := map[string]string{
		"A1": "DATA",
		"B1": "CLIENTE",
		"C1": "LOCALIZADOR",
		"D1": "EMISSÃO",
		"E1": "STATUS",
		// 01:30 UTC on the 9th is 22:30 on the 8th in São Paulo.
		"A2": "08/08/2026",
		"B2": "Ana Souza",
		"C2": "ABC123",
		"E2": saleStatusPaid,
		"B3": saleExportNoClient,
		"E3": saleStatusUnpaid,
		"B4": "Maria Lima",
		"E4": saleStatusCancelled,
	}
	for cell, want := range formatted {
		got, err := f.GetCellValue(saleExportSheet, cell)
		if err != nil {
			t.Fatalf("read %s: %v", cell, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", cell, got, want)
		}
	}

	// The amount has to be stored as a number, not as pre-formatted text - that is what
	// makes the column summable in the spreadsheet.
	raw := map[string]string{
		"D2": "2450.5",
		"D3": "24.5",
		"D4": "890",
	}
	for cell, want := range raw {
		got, err := f.GetCellValue(saleExportSheet, cell, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatalf("read raw %s: %v", cell, err)
		}
		if got != want {
			t.Errorf("raw %s = %q, want %q", cell, got, want)
		}
	}
}

// An empty period is a legitimate answer, not an error: the sheet comes out with its header
// and nothing else.
func TestRenderSalesXLSXEmpty(t *testing.T) {
	out, err := renderSalesXLSX(nil, saoPaulo(t))
	if err != nil {
		t.Fatalf("renderSalesXLSX: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("reopen rendered file: %v", err)
	}
	defer func() { _ = f.Close() }()

	rows, err := f.GetRows(saleExportSheet)
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want only the header", len(rows))
	}
}
