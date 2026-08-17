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

// The four layouts below are the operators' own workbooks, one test each. They are not
// variations on the shared template: they name the issuer in the title's first segment,
// close the table with a different total, drop columns the template has, add ones it does
// not, and write the points whole instead of in thousands.

// headerFrom lays a header row out from the column names, left to right.
func headerFrom(row string, labels ...string) map[string]any {
	cells := map[string]any{}
	for index, label := range labels {
		column, err := excelize.ColumnNumberToName(index + 1)
		if err != nil {
			panic(err)
		}
		cells[column+row] = label
	}
	return cells
}

// TestParse_ReadsAPersonalWorkbookWithNoProgramColumn covers the commonest layout of the
// four - 993 of the 2006 day sheets - which is also the one with the most to get wrong:
// no Companhia column, points written whole, and the issuer inside "CONTROLE AARON".
func TestParse_ReadsAPersonalWorkbookWithNoProgramColumn(t *testing.T) {
	content := buildWorkbook(t, sheet{
		name: "29-06-26",
		cells: merge(
			map[string]any{"A1": "CONTROLE AARON - 29/06/2026"},
			headerFrom("2", "Nº", "Localizador", "Data do Voo", "Passageiro Principal",
				"Origem", "Destino", "Pontos", "Milheiro", "Total", "Conta", "Observação"),
			map[string]any{
				"A3": 1, "B3": "SMU5WW", "C3": 46339, "D3": "MARIANA CURSINO",
				"E3": "GRU", "F3": "POA", "G3": 80400, "H3": 16, "I3": 1286.4,
				"J3": "WILMARA", "K3": "CANCELADA",
			},
			// The spare rows the template ships, and the operator's own totals below.
			map[string]any{"A4": 2, "I4": 0, "K4": "-"},
			map[string]any{"A6": "TOTAL DAS CONTAS GERAL"},
			map[string]any{"B7": "WILMARA", "C7": 80400},
		),
	})

	days, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(days) != 1 || len(days[0].Blocks) != 1 {
		t.Fatalf("got %+v, want one day with one block", days)
	}

	day := days[0]
	if day.PointsUnit != "raw" {
		t.Errorf("points unit = %q, want raw", day.PointsUnit)
	}
	if day.Blocks[0].IssuerName != "AARON" {
		t.Errorf("issuer = %q, want AARON from the title", day.Blocks[0].IssuerName)
	}
	if len(day.Blocks[0].Rows) != 1 {
		t.Fatalf("got %d rows, want 1 - the totals below are not issuances", len(day.Blocks[0].Rows))
	}

	row := day.Blocks[0].Rows[0]
	// The whole reason the unit is detected: read as thousands this would be 80.4
	// million points, and nothing downstream compares points against a balance.
	if row.Points != 80400 {
		t.Errorf("points = %d, want 80400", row.Points)
	}
	if row.ProgramLabel != "" {
		t.Errorf("program label = %q, want empty - this layout has no Companhia column", row.ProgramLabel)
	}
	if row.ProgramLocator != "SMU5WW" || row.AccountLabel != "WILMARA" {
		t.Errorf("locator/account = %q/%q", row.ProgramLocator, row.AccountLabel)
	}
	if row.ValueOfTheThousand != 1600 {
		t.Errorf("value of the thousand = %d, want 1600 centavos", row.ValueOfTheThousand)
	}
	// This layout has no Taxa column at all, which is different from a blank one.
	if row.Taxes != nil {
		t.Errorf("taxes = %v, want nil", row.Taxes)
	}
	if row.Observation != "CANCELADA" {
		t.Errorf("observation = %q, want CANCELADA", row.Observation)
	}
}

func TestParse_ReadsTheLayoutWithACiaAereaColumn(t *testing.T) {
	content := buildWorkbook(t, sheet{
		name: "04-11-25",
		cells: merge(
			map[string]any{"A1": "CONTROLE RUAN - 04/11/2025"},
			headerFrom("2", "Nº", "Cia. Aérea", "Localizador", "Data do Voo",
				"Passageiro Principal", "Origem", "Destino", "Pontos", "Milheiro",
				"Total", "Conta", "Observação"),
			map[string]any{
				"A3": 1, "B3": "AZUL", "C3": "YFPZ3L", "D3": 46339, "E3": "ANTONIA",
				"F3": "MAO", "G3": "ITB", "H3": -36000, "I3": 15.75, "J3": -567,
				"K3": "LUIS", "L3": "CANCELAMENTO",
			},
			map[string]any{"A5": "TOTAL R$ GERAL"},
		),
	})

	days, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	row := days[0].Blocks[0].Rows[0]

	if days[0].PointsUnit != "raw" {
		t.Errorf("points unit = %q, want raw", days[0].PointsUnit)
	}
	if row.ProgramLabel != "AZUL" {
		t.Errorf("program label = %q, want AZUL", row.ProgramLabel)
	}
	// An estorno keeps its sign here; reading it as a cancellation is resolve.go's job.
	if row.Points != -36000 {
		t.Errorf("points = %d, want -36000", row.Points)
	}
	if row.AccountLabel != "LUIS" || row.Observation != "CANCELAMENTO" {
		t.Errorf("account/observation = %q/%q", row.AccountLabel, row.Observation)
	}
}

func TestParse_ReadsTheIssuerFromTheRowWhenTheSheetHasThatColumn(t *testing.T) {
	content := buildWorkbook(t, sheet{
		name: "18-11-25",
		cells: merge(
			map[string]any{"A1": "CONTROLE TROPA AZUL - 18/11/2025"},
			headerFrom("2", "Nº", "Companhia", "Localizador", "Data do Voo",
				"Passageiro Principal", "Origem", "Destino", "Pontos", "Milheiro",
				"Total", "Conta", "Emissor", "Observação", "Extrato Conferido?"),
			map[string]any{
				"A3": 1, "B3": "AZUL", "C3": "GMH2FQ", "D3": 46339, "E3": "EDUARDA ALMEIDA",
				"F3": "REC", "G3": "POA", "H3": 11400, "I3": 16, "J3": 182.4,
				"K3": "HELLES", "L3": "DENILSON", "M3": "OK", "N3": false,
			},
			map[string]any{
				"A4": 2, "B4": "AZUL", "C4": "MESB4L", "D4": 46339, "E4": "WENDSON BRAGA",
				"F4": "REC", "G4": "CNF", "H4": 174000, "I4": 15.75, "J4": 2740.5,
				"K4": "CARLOS", "L4": "PEDRO", "M4": "OK", "N4": true,
			},
			map[string]any{"A6": "TOTAL POR EMISSOR"},
		),
	})

	days, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rows := days[0].Blocks[0].Rows
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// Three people share a day in this workbook, so the issuer is a column and the
	// title names the team. Without this the operator would map "TROPA AZUL" to one
	// person and credit all three days' work to them.
	if rows[0].IssuerName != "DENILSON" || rows[1].IssuerName != "PEDRO" {
		t.Errorf("issuers = %q/%q, want DENILSON/PEDRO", rows[0].IssuerName, rows[1].IssuerName)
	}
	if days[0].Blocks[0].IssuerName != "TROPA AZUL" {
		t.Errorf("block issuer = %q, want TROPA AZUL", days[0].Blocks[0].IssuerName)
	}
	// The Extrato Conferido? checkbox is a control of theirs, not a column of ours.
	if rows[0].Observation != "OK" {
		t.Errorf("observation = %q, want OK - the checkbox column must not shift it", rows[0].Observation)
	}
	if rows[1].Points != 174000 {
		t.Errorf("points = %d, want 174000", rows[1].Points)
	}
}

func TestParse_ReadsTheLayoutWithAnIssuerButNoProgramColumn(t *testing.T) {
	content := buildWorkbook(t, sheet{
		name: "20-05-25",
		cells: merge(
			map[string]any{"A1": "CONTROLE TROPA AZUL - 20/05/2025"},
			headerFrom("2", "Nº", "Localizador", "Data do Voo", "Passageiro Principal",
				"Origem", "Destino", "Pontos", "Milheiro", "Total", "Conta", "Emissor",
				"Observação"),
			map[string]any{
				"A3": 1, "B3": "PNM3TQ", "C3": 46339, "D3": "JOCILENE VENÂNCIO",
				"E3": "REC", "F3": "GRU", "G3": 64000, "H3": 16, "I3": 1024,
				"J3": "MARIA CLARA", "K3": "PEDRO", "L3": "CANCELADA MILHAS REEMBOLSADAS",
			},
			map[string]any{"A5": "TOTAL DE PONTOS GERAL"},
		),
	})

	days, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	row := days[0].Blocks[0].Rows[0]

	if row.Points != 64000 {
		t.Errorf("points = %d, want 64000", row.Points)
	}
	if row.IssuerName != "PEDRO" {
		t.Errorf("issuer = %q, want PEDRO", row.IssuerName)
	}
	if row.AccountLabel != "MARIA CLARA" {
		t.Errorf("account = %q, want MARIA CLARA", row.AccountLabel)
	}
	if row.ProgramLabel != "" {
		t.Errorf("program label = %q, want empty", row.ProgramLabel)
	}
}

func TestParseDay_ReadsOnlyTheSheetItWasAskedFor(t *testing.T) {
	content := buildWorkbook(t,
		sheet{name: "12-08-26", cells: merge(
			map[string]any{"A1": "CONTROLE AARON - 12/08/2026"},
			headerFrom("2", "Nº", "Localizador", "Pontos", "Milheiro", "Total", "Conta"),
			map[string]any{"A3": 1, "B3": "AAAAAA", "C3": 10000, "D3": 16, "E3": 160, "F3": "ISAIAS"},
		)},
		sheet{name: "13-08-26", cells: merge(
			map[string]any{"A1": "CONTROLE AARON - 13/08/2026"},
			headerFrom("2", "Nº", "Localizador", "Pontos", "Milheiro", "Total", "Conta"),
			map[string]any{"A3": 1, "B3": "BBBBBB", "C3": 20000, "D3": 16, "E3": 320, "F3": "ISAIAS"},
		)},
	)

	day, found, err := ParseDay(content, time.Date(2026, time.August, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseDay: %v", err)
	}
	if !found {
		t.Fatal("found = false, want the 13-08-26 sheet")
	}
	if len(day.Blocks) != 1 || len(day.Blocks[0].Rows) != 1 {
		t.Fatalf("got %+v, want one row", day.Blocks)
	}
	if got := day.Blocks[0].Rows[0].ProgramLocator; got != "BBBBBB" {
		t.Errorf("locator = %q, want BBBBBB", got)
	}

	_, found, err = ParseDay(content, time.Date(2026, time.August, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseDay of a day the file lacks: %v", err)
	}
	if found {
		t.Error("found = true for a day the workbook does not have")
	}
}

func TestIssuerFromTitle(t *testing.T) {
	tests := []struct{ title, want string }{
		{"CONTROLE DE EMISSÕES - 01/08/2026 - MARCUS", "MARCUS"},
		{"CONTROLE DE EMISSÕES - ##/08/2026 - WILMARA/CLARA", "WILMARA/CLARA"},
		// No name to be had: the shared template's own words and a date.
		{"CONTROLE DE EMISSÕES - 01/08/2026", ""},
		{"CONTROLE AARON - 15/08/2026", "AARON"},
		{"CONTROLE TROPA AZUL - %%/08/2026", "TROPA AZUL"},
		{"CONTROLE MÁRCIO", "MÁRCIO"},
	}

	for _, tc := range tests {
		if got := issuerFromTitle(tc.title); got != tc.want {
			t.Errorf("issuerFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
		}
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
