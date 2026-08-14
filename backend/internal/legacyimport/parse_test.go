package legacyimport

import (
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// sheet is a fixture sheet: its name plus the cells to write, addressed the way the
// spreadsheet itself is ("A1", "C3"), so a test reads like the thing it describes.
type sheet struct {
	name  string
	cells map[string]any
}

// buildWorkbook renders the fixture sheets into real .xlsx bytes. Building the workbook
// rather than committing one keeps every trap a test exercises visible in the test.
func buildWorkbook(t *testing.T, sheets ...sheet) []byte {
	t.Helper()

	file := excelize.NewFile()
	defer func() { _ = file.Close() }()

	for _, s := range sheets {
		if _, err := file.NewSheet(s.name); err != nil {
			t.Fatalf("creating sheet %q: %v", s.name, err)
		}
		for axis, value := range s.cells {
			if err := file.SetCellValue(s.name, axis, value); err != nil {
				t.Fatalf("setting %s!%s: %v", s.name, axis, err)
			}
		}
	}

	// NewFile opens with a "Sheet1" nobody asked for, and leaving it in would have the
	// parser skip it silently - true, but it would also hide a fixture typo.
	if index, err := file.GetSheetIndex("Sheet1"); err == nil && index >= 0 {
		if err := file.DeleteSheet("Sheet1"); err != nil {
			t.Fatalf("deleting the default sheet: %v", err)
		}
	}

	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("writing the workbook: %v", err)
	}

	return buffer.Bytes()
}

// header is the column row every block repeats, keyed from a row number.
func header(row string) map[string]any {
	return map[string]any{
		"A" + row: "Nº", "B" + row: "Companhia", "C" + row: "Localizador",
		"D" + row: "Data do Voo", "E" + row: "Passageiro Principal", "F" + row: "Origem",
		"G" + row: "Destino", "H" + row: "Pontos", "I" + row: "Milheiro", "J" + row: "Taxa",
		"K" + row: "Total", "L" + row: "Conta", "M" + row: "Observação",
	}
}

func merge(maps ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func TestParse_ReadsOneDayWithTwoBlocks(t *testing.T) {
	content := buildWorkbook(t, sheet{
		name: "01-08-26",
		cells: merge(
			map[string]any{"A1": "CONTROLE DE EMISSÕES - 01/08/2026 - MARCUS"},
			header("2"),
			map[string]any{
				"A3": 1, "B3": "AZUL", "C3": "EQF8VK", "D3": 46339, "E3": "RENAN RIBEIRO",
				"F3": "SSA", "G3": "REC", "H3": 18, "I3": 16.25, "J3": 253.06,
				"L3": "MARISE", "M3": "EMISSAO WILMARA",
			},
			// A spare template row: program filled in, no locator. Not an issuance.
			map[string]any{"B4": "AZUL", "K4": 0, "M4": "-"},
			map[string]any{"A5": "TOTAL FATURAMENTO", "K5": 545.56},
			// The operator's own summary table, below the block's total.
			map[string]any{"B7": "AZUL", "C7": "CRISTIANO", "D7": 56},
			map[string]any{"A9": "CONTROLE DE EMISSÕES - 01/08/2026 - WILMARA/CLARA"},
			header("10"),
			map[string]any{
				"A11": 1, "B11": "SMILES", "C11": "JEUZWX", "D11": 46239, "E11": "JOCIANE",
				"F11": "STM", "G11": "BEL", "H11": 282.9, "I11": 16.5, "J11": 640.17,
				"L11": "VANDA",
			},
			map[string]any{"A12": "TOTAL FATURAMENTO"},
		),
	})

	days, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(days) != 1 {
		t.Fatalf("got %d days, want 1", len(days))
	}
	day := days[0]
	if want := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC); !day.Date.Equal(want) {
		t.Errorf("date = %v, want %v", day.Date, want)
	}
	if len(day.Blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(day.Blocks))
	}

	first := day.Blocks[0]
	if first.IssuerName != "MARCUS" {
		t.Errorf("issuer = %q, want %q", first.IssuerName, "MARCUS")
	}
	// One row, not three: the spare template row and the summary table below the
	// block's total are both excluded.
	if len(first.Rows) != 1 {
		t.Fatalf("got %d rows in the first block, want 1", len(first.Rows))
	}

	row := first.Rows[0]
	if row.Number != 3 {
		t.Errorf("row number = %d, want 3", row.Number)
	}
	if row.ProgramLabel != "AZUL" || row.ProgramLocator != "EQF8VK" {
		t.Errorf("program/locator = %q/%q", row.ProgramLabel, row.ProgramLocator)
	}
	if row.Points != 18000 {
		t.Errorf("points = %d, want 18000 (the column is in thousands)", row.Points)
	}
	if row.ValueOfTheThousand != 1625 {
		t.Errorf("value of the thousand = %d, want 1625 centavos", row.ValueOfTheThousand)
	}
	if row.Taxes == nil || *row.Taxes != 25306 {
		t.Errorf("taxes = %v, want 25306 centavos", row.Taxes)
	}
	if row.FlightDate == nil {
		t.Fatal("flight date = nil, want a date")
	}
	if want := time.Date(2026, time.November, 13, 0, 0, 0, 0, time.UTC); !row.FlightDate.Equal(want) {
		t.Errorf("flight date = %v, want %v", row.FlightDate, want)
	}
	if row.AccountLabel != "MARISE" {
		t.Errorf("account = %q, want MARISE", row.AccountLabel)
	}

	second := day.Blocks[1]
	if second.Index != 1 {
		t.Errorf("second block index = %d, want 1", second.Index)
	}
	// Two operators sharing a block is exactly why the issuer is mapped on screen
	// instead of being resolved here.
	if second.IssuerName != "WILMARA/CLARA" {
		t.Errorf("issuer = %q, want WILMARA/CLARA", second.IssuerName)
	}
	if len(second.Rows) != 1 || second.Rows[0].Points != 282900 {
		t.Errorf("second block rows = %+v", second.Rows)
	}
}

func TestParse_SkipsSheetsThatAreNotDays(t *testing.T) {
	content := buildWorkbook(t,
		sheet{name: "MODELO", cells: merge(
			map[string]any{"A1": "CONTROLE DE EMISSÕES - ##/08/2026 - MARCUS"},
			header("2"),
			map[string]any{"B3": "AZUL", "C3": "XXXXXX", "H3": 10, "I3": 15},
		)},
		sheet{name: "AZUL", cells: map[string]any{"A1": "AZUL - ### - ##/08/2026"}},
		sheet{name: "13-08-26", cells: merge(
			map[string]any{"A1": "CONTROLE DE EMISSÕES - 13/08/2026 - MARCUS"},
			header("2"),
			map[string]any{"B3": "AZUL", "C3": "FT23VR", "H3": 120, "I3": 15},
		)},
	)

	days, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(days) != 1 || days[0].SheetName != "13-08-26" {
		t.Fatalf("got %d days (%+v), want only 13-08-26", len(days), days)
	}
}

// The day sheet whose title still carries the template's placeholder is why the date is
// read off the sheet name and never off the title.
func TestParse_TakesTheDateFromTheSheetNameNotTheTitle(t *testing.T) {
	content := buildWorkbook(t, sheet{
		name: "09-08-26",
		cells: merge(
			map[string]any{"A1": "CONTROLE DE EMISSÕES - ##/08/2026 - MARCUS"},
			header("2"),
			map[string]any{"B3": "AZUL", "C3": "ABC123", "H3": 10, "I3": 15},
		),
	})

	days, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := time.Date(2026, time.August, 9, 0, 0, 0, 0, time.UTC)
	if len(days) != 1 || !days[0].Date.Equal(want) {
		t.Fatalf("got %+v, want a single day dated %v", days, want)
	}
}

func TestParse_ReadsCellQuirks(t *testing.T) {
	content := buildWorkbook(t, sheet{
		name: "05-08-26",
		cells: merge(
			map[string]any{"A1": "CONTROLE DE EMISSÕES - 05/08/2026 - MARCUS"},
			header("2"),
			// "-" is how the operators write "nothing here"; the taxes cell is left
			// blank instead, which means something else again.
			map[string]any{
				"A3": 1, "B3": "A.A", "C3": "CWL77K / JGBJNN", "D3": "-", "E3": "ANA",
				"F3": "-", "G3": "-", "H3": 29.577, "I3": 27.5, "L3": "WESLEY", "M3": "-",
			},
			map[string]any{"A4": "TOTAL FATURAMENTO"},
		),
	})

	days, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(days) != 1 || len(days[0].Blocks) != 1 || len(days[0].Blocks[0].Rows) != 1 {
		t.Fatalf("got %+v, want a single row", days)
	}

	row := days[0].Blocks[0].Rows[0]
	if row.ProgramLabel != "A.A" {
		t.Errorf("program label = %q, want A.A verbatim (resolve.go normalises it)", row.ProgramLabel)
	}
	if row.ProgramLocator != "CWL77K" || row.AirlineLocator != "JGBJNN" {
		t.Errorf("locators = %q/%q, want CWL77K/JGBJNN", row.ProgramLocator, row.AirlineLocator)
	}
	if row.FlightDate != nil {
		t.Errorf("flight date = %v, want nil for a \"-\" cell", row.FlightDate)
	}
	if row.OriginLabel != "" || row.DestinationLabel != "" || row.Observation != "" {
		t.Errorf("\"-\" cells came through as %q/%q/%q, want empty",
			row.OriginLabel, row.DestinationLabel, row.Observation)
	}
	if row.Taxes != nil {
		t.Errorf("taxes = %v, want nil for a blank cell", row.Taxes)
	}
	if row.Points != 29577 {
		t.Errorf("points = %d, want 29577", row.Points)
	}
}

func TestParseSheetDate(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
		want time.Time
	}{
		{name: "01-08-26", ok: true, want: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)},
		{name: "13-08-26", ok: true, want: time.Date(2026, time.August, 13, 0, 0, 0, 0, time.UTC)},
		{name: "MODELO", ok: false},
		{name: "A.A", ok: false},
		{name: "1-8-26", ok: false},
		// time.Date would roll this into March rather than refusing it.
		{name: "31-02-26", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseSheetDate(tc.name)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && !got.Equal(tc.want) {
				t.Fatalf("date = %v, want %v", got, tc.want)
			}
		})
	}
}
