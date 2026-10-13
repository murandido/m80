package export

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/xuri/excelize/v2"
)

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

func TestAccountLabel(t *testing.T) {
	tests := []struct {
		name  string
		given data.Issuance
		want  string
	}{
		{
			name:  "alias wins when present",
			given: data.Issuance{AccountNickname: &data.AccountNickname{Nickname: "Conta VIP"}},
			want:  "Conta VIP",
		},
		{
			name:  "holder name when no alias",
			given: data.Issuance{Account: &data.Account{Holder: &data.Person{Name: "Ana Souza"}}},
			want:  "Ana Souza",
		},
		{
			name:  "login when no alias and no holder",
			given: data.Issuance{Account: &data.Account{Login: strptr("ana@example.com")}},
			want:  "ana@example.com",
		},
		{
			name:  "account id as last resort",
			given: data.Issuance{AccountID: "acc-123", Account: &data.Account{}},
			want:  "acc-123",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := accountLabel(tc.given); got != tc.want {
				t.Fatalf("accountLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCentsToReais(t *testing.T) {
	if got := centsToReais(2450); got != 24.5 {
		t.Fatalf("centsToReais(2450) = %v, want 24.5", got)
	}
}

// sampleIssuance is the shape the render tests vary: everything the sheet reads, with the
// caller overriding only what its case is about.
func sampleIssuance(program, locator, account, issuer string) data.Issuance {
	return data.Issuance{
		LoyaltyProgram:      &data.LoyaltyProgram{Name: program},
		ProgramLocator:      locator,
		MainPassengerName:   strptr("Ana Souza"),
		OriginIATACode:      strptr("GRU"),
		DestinationIATACode: strptr("SDU"),
		Points:              10000,
		ValueOfTheThousand:  2000,
		Taxes:               intptr(3550),
		AccountNickname:     &data.AccountNickname{Nickname: account},
		Issuer:              &data.User{Name: issuer},
		CreatedAt:           time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC),
		Flights: []data.IssuanceFlight{
			{Direction: "outbound", LegOrder: 1, DepartureAt: time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)},
		},
	}
}

// renderForTest runs the export at a fixed instant and hands back the reopened workbook. The
// whole file is one sheet now, named issuanceExportSheetName.
func renderForTest(t *testing.T, issuances []data.Issuance) *excelize.File {
	t.Helper()

	out, err := renderIssuancesXLSX(issuances, ExportWindow{}, time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("renderIssuancesXLSX: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("reopen rendered file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func assertCells(t *testing.T, f *excelize.File, sheet string, cells map[string]string) {
	t.Helper()

	for cell, want := range cells {
		got, err := f.GetCellValue(sheet, cell)
		if err != nil {
			t.Fatalf("get %s!%s: %v", sheet, cell, err)
		}
		if got != want {
			t.Errorf("cell %s!%s = %q, want %q", sheet, cell, got, want)
		}
	}
}

// TestRenderIssuancesXLSXLayout pins the columns of the manual control sheet, plus the Emissor
// column the single-sheet export adds after Taxa Resgate.
func TestRenderIssuancesXLSXLayout(t *testing.T) {
	f := renderForTest(t, []data.Issuance{
		sampleIssuance("Smiles", "ABC123", "Conta VIP", "João Emissor"),
	})

	assertCells(t, f, issuanceExportSheetName, map[string]string{
		"A1": "CONTROLE DE EMISSÕES - 22/07/2026",
		"A2": "Nº",
		"B2": "Companhia",
		"H2": "Pontos",
		"K2": "Total",
		"M2": "Observação",
		"N2": "Taxa Resgate",
		"O2": "Emissor",
		"A3": "1",
		// The airline label is upper case, as on the sheet being pasted into.
		"B3": "SMILES",
		"C3": "ABC123",
		"D3": "01/08/26",
		"E3": "Ana Souza",
		"F3": "GRU",
		"G3": "SDU",
		// 10000 points written as thousands, which is the unit the Total formula multiplies.
		"H3": "10.000",
		"I3": "R$ 20.00",
		"J3": "R$ 35.50",
		"L3": "Conta VIP",
		// An ordinary account saved no redemption fee, so the cell stays blank.
		"N3": "",
		// Emissor names who made the row.
		"O3": "João Emissor",
	})

	formula, err := f.GetCellFormula(issuanceExportSheetName, "K3")
	if err != nil {
		t.Fatalf("get K3 formula: %v", err)
	}
	if formula != "SUM(H3*I3+J3)" {
		t.Errorf("K3 formula = %q, want %q", formula, "SUM(H3*I3+J3)")
	}
}

// A booking cancelled on the day it was made enters by its net: what it actually cost.
func TestRenderIssuancesXLSXNetsSameDayCancellation(t *testing.T) {
	cancelledAt := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)

	issuance := sampleIssuance("Smiles", "ABC123", "Conta VIP", "João Emissor")
	issuance.CancelledAt = &cancelledAt
	issuance.RefundedPoints = intptr(4000)
	issuance.RefundedTaxes = intptr(1000)

	f := renderForTest(t, []data.Issuance{issuance})

	assertCells(t, f, issuanceExportSheetName, map[string]string{
		// 10000 spent less 4000 given back.
		"H3": "6.000",
		"J3": "R$ 25.50",
	})
}

// Cancelled on a later day, only the refund shows: the spend belongs to the day it was
// made, which has its own sheet.
func TestRenderIssuancesXLSXRefundOnlyOnLaterCancellation(t *testing.T) {
	cancelledAt := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)

	issuance := sampleIssuance("Smiles", "ABC123", "Conta VIP", "João Emissor")
	issuance.CancelledAt = &cancelledAt
	issuance.RefundedPoints = intptr(10000)
	issuance.RefundedTaxes = intptr(3550)

	f := renderForTest(t, []data.Issuance{issuance})

	assertCells(t, f, issuanceExportSheetName, map[string]string{
		"H3": "-10.000",
		"J3": "-R$ 35.50",
	})
}

// A cancelled line is tinted across the row - except the airline cell, which keeps its own
// colour so the block still reads as one airline down the page.
func TestRenderIssuancesXLSXTintsCancelledRow(t *testing.T) {
	cancelledAt := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)

	issuance := sampleIssuance("Smiles", "ABC123", "Conta VIP", "Marcus")
	issuance.CancelledAt = &cancelledAt

	f := renderForTest(t, []data.Issuance{issuance})

	for _, cell := range []string{"A3", "C3", "M3"} {
		id, err := f.GetCellStyle(issuanceExportSheetName, cell)
		if err != nil {
			t.Fatalf("get style of %s: %v", cell, err)
		}
		style, err := f.GetStyle(id)
		if err != nil {
			t.Fatalf("read style of %s: %v", cell, err)
		}
		if len(style.Fill.Color) == 0 || !strings.EqualFold(style.Fill.Color[0], "FFC7CE") {
			t.Errorf("cell %s fill = %v, want FFC7CE", cell, style.Fill.Color)
		}
	}

	id, err := f.GetCellStyle(issuanceExportSheetName, "B3")
	if err != nil {
		t.Fatalf("get style of B3: %v", err)
	}
	style, err := f.GetStyle(id)
	if err != nil {
		t.Fatalf("read style of B3: %v", err)
	}
	want := colorsForProgram("Smiles").tableFill
	if len(style.Fill.Color) == 0 || !strings.EqualFold(style.Fill.Color[0], want) {
		t.Errorf("airline cell fill = %v, want the airline colour %s", style.Fill.Color, want)
	}
}

// No taxes leaves the cell blank rather than writing a zero the manual sheet never wrote.
func TestRenderIssuancesXLSXLeavesTaxesBlank(t *testing.T) {
	issuance := sampleIssuance("Smiles", "ABC123", "Conta VIP", "João Emissor")
	issuance.Taxes = nil

	f := renderForTest(t, []data.Issuance{issuance})

	assertCells(t, f, issuanceExportSheetName, map[string]string{"J3": ""})
}

// An exempt account shows the fee it did not pay, and the totals band sums it apart from the
// billing, which it never entered.
func TestRenderIssuancesXLSXWritesRedemptionFee(t *testing.T) {
	exempt := sampleIssuance("Azul", "AAA111", "Diamante", "Marcus")
	exempt.RedemptionFeeSavedCents = intptr(4500)

	f := renderForTest(t, []data.Issuance{
		exempt,
		sampleIssuance("Azul", "BBB222", "Comum", "Marcus"),
	})

	assertCells(t, f, issuanceExportSheetName, map[string]string{
		"N3": "R$ 45.00",
		"N4": "",
	})

	formula, err := f.GetCellFormula(issuanceExportSheetName, "N5")
	if err != nil {
		t.Fatalf("get N5 formula: %v", err)
	}
	if formula != "SUM(N3:N4)" {
		t.Errorf("N5 formula = %q, want %q", formula, "SUM(N3:N4)")
	}
	billing, err := f.GetCellFormula(issuanceExportSheetName, "K6")
	if err != nil {
		t.Fatalf("get K6 formula: %v", err)
	}
	if billing != "K5-J5" {
		t.Errorf("K6 formula = %q, want the billing to leave the redemption fee out", billing)
	}
}

// A booking cancelled on a later day gives its saving back, negative like its refund.
func TestRenderIssuancesXLSXGivesRedemptionFeeBackOnLaterCancellation(t *testing.T) {
	cancelledAt := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)

	issuance := sampleIssuance("Azul", "AAA111", "Diamante", "Marcus")
	issuance.RedemptionFeeSavedCents = intptr(4500)
	issuance.CancelledAt = &cancelledAt

	f := renderForTest(t, []data.Issuance{issuance})

	assertCells(t, f, issuanceExportSheetName, map[string]string{"N3": "-R$ 45.00"})
}

// Two airlines become two blocks separated by a blank row, the way the manual sheet keeps
// AZUL apart from SMILES.
func TestRenderIssuancesXLSXGroupsByProgram(t *testing.T) {
	f := renderForTest(t, []data.Issuance{
		sampleIssuance("Smiles", "AAA111", "Conta VIP", "João Emissor"),
		sampleIssuance("Azul", "BBB222", "Conta VIP", "João Emissor"),
		sampleIssuance("Smiles", "CCC333", "Conta VIP", "João Emissor"),
	})

	assertCells(t, f, issuanceExportSheetName, map[string]string{
		"C3": "AAA111",
		"C4": "CCC333",
		// Row 5 is the spacer between the two airline blocks.
		"B5": "",
		"C5": "",
		"B6": "AZUL",
		"C6": "BBB222",
		// Numbering is continuous across the blocks, as on the manual sheet.
		"A6": "3",
	})

	// The spacer is painted, so the break reads as a separator and not as a hole.
	id, err := f.GetCellStyle(issuanceExportSheetName, "C5")
	if err != nil {
		t.Fatalf("get style of C5: %v", err)
	}
	style, err := f.GetStyle(id)
	if err != nil {
		t.Fatalf("read style of C5: %v", err)
	}
	if len(style.Fill.Color) == 0 || !strings.EqualFold(style.Fill.Color[0], "8EA9DB") {
		t.Errorf("spacer fill = %v, want 8EA9DB", style.Fill.Color)
	}
}

// Every issuer is on one tab now: two issuers come back as one sheet, and the Emissor column
// is what says who made each row.
func TestRenderIssuancesXLSXSingleTabWithIssuerColumn(t *testing.T) {
	f := renderForTest(t, []data.Issuance{
		sampleIssuance("Smiles", "AAA111", "Conta VIP", "Marcus"),
		sampleIssuance("Smiles", "BBB222", "Conta VIP", "Wilmara"),
	})

	if got := f.GetSheetList(); len(got) != 1 || got[0] != issuanceExportSheetName {
		t.Fatalf("sheets = %v, want [%s]", got, issuanceExportSheetName)
	}
	assertCells(t, f, issuanceExportSheetName, map[string]string{
		"C3": "AAA111", "O3": "Marcus",
		"C4": "BBB222", "O4": "Wilmara",
	})
}

// A cancelled issuance is attributed to whoever cancelled it: that is the flow the listing puts
// it in, so the Emissor column shows the canceller, not the creator.
func TestRenderIssuancesXLSXAttributesCancelledRowToCanceller(t *testing.T) {
	cancelledAt := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)

	cancelled := sampleIssuance("Smiles", "AAA111", "Conta VIP", "Wilmara")
	cancelled.CancelledAt = &cancelledAt
	cancelled.CancelledBy = &data.User{Name: "Marcus"}
	cancelled.RefundedPoints = intptr(10000)

	f := renderForTest(t, []data.Issuance{
		cancelled,
		sampleIssuance("Smiles", "BBB222", "Conta VIP", "Wilmara"),
	})

	if got := f.GetSheetList(); len(got) != 1 || got[0] != issuanceExportSheetName {
		t.Fatalf("sheets = %v, want [%s]", got, issuanceExportSheetName)
	}
	assertCells(t, f, issuanceExportSheetName, map[string]string{
		"C3": "AAA111", "O3": "Marcus",
		"C4": "BBB222", "O4": "Wilmara",
	})
}

// The summary beside the table: one box per account used, listing that account's locators.
func TestRenderIssuancesXLSXSummarisesAccounts(t *testing.T) {
	f := renderForTest(t, []data.Issuance{
		sampleIssuance("Smiles", "AAA111", "Juliana", "Marcus"),
		sampleIssuance("Smiles", "BBB222", "Juliana", "Marcus"),
		sampleIssuance("Smiles", "CCC333", "Marcia", "Marcus"),
	})

	assertCells(t, f, issuanceExportSheetName, map[string]string{
		"Q1": "EMISSÕES JULIANA - SMILES - 22/07/2026",
		"Q2": "COMPANHIA",
		"R2": "LOCALIZADOR",
		"S2": "PONTOS",
		"T2": "TOTAL",
		"R3": "AAA111",
		"R4": "BBB222",
		// One blank row, then the next account's box.
		"Q6": "EMISSÕES MARCIA - SMILES - 22/07/2026",
		"R8": "CCC333",
	})

	formula, err := f.GetCellFormula(issuanceExportSheetName, "T3")
	if err != nil {
		t.Fatalf("get T3 formula: %v", err)
	}
	if formula != "SUM(S3:S4)" {
		t.Errorf("T3 formula = %q, want %q", formula, "SUM(S3:S4)")
	}
}

// The Companhia cell is coloured per airline, which is what makes the blocks legible on
// the sheet being pasted into. A colour drifting here would show up as a block that no
// longer matches the ones around it.
func TestRenderIssuancesXLSXColorsProgramCell(t *testing.T) {
	f := renderForTest(t, []data.Issuance{
		sampleIssuance("Smiles", "AAA111", "Conta VIP", "Marcus"),
		sampleIssuance("LATAM Pass", "BBB222", "Conta VIP", "Marcus"),
		sampleIssuance("TudoAzul", "CCC333", "Conta VIP", "Marcus"),
	})

	// B3 Smiles, B5 LATAM Pass, B7 TudoAzul - each block separated by a spacer row.
	want := map[string]string{"B3": "FFFF00", "B5": "FF0000", "B7": "A6CAEC"}
	for cell, fill := range want {
		id, err := f.GetCellStyle(issuanceExportSheetName, cell)
		if err != nil {
			t.Fatalf("get style of %s: %v", cell, err)
		}
		style, err := f.GetStyle(id)
		if err != nil {
			t.Fatalf("read style of %s: %v", cell, err)
		}
		if len(style.Fill.Color) == 0 || !strings.EqualFold(style.Fill.Color[0], fill) {
			t.Errorf("cell %s fill = %v, want %s", cell, style.Fill.Color, fill)
		}
	}
}

// The summary uses its own palette for the same airlines - the operator's block fills
// Smiles orange where the table fills it yellow.
func TestRenderIssuancesXLSXSummaryPalette(t *testing.T) {
	f := renderForTest(t, []data.Issuance{
		sampleIssuance("Smiles", "AAA111", "Juliana", "Marcus"),
	})

	id, err := f.GetCellStyle(issuanceExportSheetName, "Q3")
	if err != nil {
		t.Fatalf("get style of Q3: %v", err)
	}
	style, err := f.GetStyle(id)
	if err != nil {
		t.Fatalf("read style of Q3: %v", err)
	}
	if len(style.Fill.Color) == 0 || !strings.EqualFold(style.Fill.Color[0], "FFC000") {
		t.Errorf("Q3 fill = %v, want FFC000", style.Fill.Color)
	}
}

// One account used on two airlines gets two boxes: the block being copied filters on conta
// and companhia together, and a single box would carry a title that names only one of them.
func TestRenderIssuancesXLSXSplitsBoxesPerProgram(t *testing.T) {
	f := renderForTest(t, []data.Issuance{
		sampleIssuance("Smiles", "AAA111", "Heraldo", "Marcus"),
		sampleIssuance("Azul", "BBB222", "Heraldo", "Marcus"),
	})

	assertCells(t, f, issuanceExportSheetName, map[string]string{
		"Q1": "EMISSÕES HERALDO - AZUL - 22/07/2026",
		"R3": "BBB222",
		"Q5": "EMISSÕES HERALDO - SMILES - 22/07/2026",
		"R7": "AAA111",
	})
}

// No row names its own height any more: every row on the sheet, the TOTAL DAS CONTAS block
// included, is left for Excel to fit to its content.
func TestRenderIssuancesXLSXLeavesEveryRowAutoFitted(t *testing.T) {
	f := renderForTest(t, []data.Issuance{
		sampleIssuance("Smiles", "AAA111", "Heraldo", "Marcus"),
		sampleIssuance("Azul", "BBB222", "Isaias", "Marcus"),
	})

	// Two airline blocks of one row each (3 and 5), the sums on 6 and TOTAL FATURAMENTO on
	// 7, so the block header lands on 10: header, issuer, one row per account, then GERAL.
	def, err := f.GetRowHeight(issuanceExportSheetName, 1)
	if err != nil {
		t.Fatalf("get default height: %v", err)
	}
	for row := 1; row <= 14; row++ {
		got, err := f.GetRowHeight(issuanceExportSheetName, row)
		if err != nil {
			t.Fatalf("get height of row %d: %v", row, err)
		}
		if got != def {
			t.Errorf("row %d height = %v, want the sheet default %v", row, got, def)
		}
	}

	// The GERAL row is closed on the left by the billing band rather than left blank.
	id, err := f.GetCellStyle(issuanceExportSheetName, "B14")
	if err != nil {
		t.Fatalf("get style of B14: %v", err)
	}
	style, err := f.GetStyle(id)
	if err != nil {
		t.Fatalf("read style of B14: %v", err)
	}
	if len(style.Fill.Color) == 0 || !strings.EqualFold(style.Fill.Color[0], "0D0D0D") {
		t.Errorf("B14 fill = %v, want 0D0D0D", style.Fill.Color)
	}
}

// Every column is sized to its own content. The check that matters is column A: it is the
// narrow Nº strip, and it sits under the merged 26pt title - measuring the title would blow
// it (and every other column) up to the width of the whole heading.
func TestRenderIssuancesXLSXAutoFitsColumns(t *testing.T) {
	long := "Acordo comercial com o cliente firmado na renegociação de dezembro"
	wide := sampleIssuance("Smiles", "AAA111", "Conta VIP", "Marcus")
	wide.Observation = &long

	f := renderForTest(t, []data.Issuance{wide})

	width := func(column string) float64 {
		t.Helper()
		got, err := f.GetColWidth(issuanceExportSheetName, column)
		if err != nil {
			t.Fatalf("get width of %s: %v", column, err)
		}
		return got
	}

	if got := width("A"); got > 8 {
		t.Errorf("column A width = %v, want the Nº strip to stay narrow", got)
	}
	if got, observation := width("E"), width("M"); observation <= got {
		t.Errorf("Observação (%v) should outgrow Passageiro (%v) on this data", observation, got)
	}
}

// An empty result still has to be a file Excel will open.
func TestRenderIssuancesXLSXWithNoIssuances(t *testing.T) {
	f := renderForTest(t, nil)

	if sheets := f.GetSheetList(); len(sheets) != 1 {
		t.Fatalf("sheets = %v, want exactly one", sheets)
	}
	assertCells(t, f, issuanceExportSheetName, map[string]string{"A2": "Nº"})
}

func TestSignedIssuanceAmounts(t *testing.T) {
	loc, err := time.LoadLocation(exportTimezone)
	if err != nil {
		t.Fatalf("load %s: %v", exportTimezone, err)
	}

	// 12 August in São Paulo; 15:00Z is midday there, so both instants stay on that day.
	morning := time.Date(2026, 8, 12, 15, 0, 0, 0, time.UTC)
	evening := time.Date(2026, 8, 12, 22, 0, 0, 0, time.UTC)
	nextDay := time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC)
	cancelled := func(i data.Issuance, at time.Time) data.Issuance {
		i.Active = false
		i.CancelledAt = &at
		return i
	}

	base := data.Issuance{Points: 10000, Taxes: intptr(3000), CreatedAt: morning}

	tests := []struct {
		name       string
		given      data.Issuance
		wantPoints int
		wantTaxes  *int
	}{
		{
			name:       "never cancelled counts what it spent",
			given:      base,
			wantPoints: 10000,
			wantTaxes:  intptr(3000),
		},
		{
			name: "cancelled the same day enters by its net",
			given: func() data.Issuance {
				i := base
				i.RefundedPoints = intptr(4000)
				i.RefundedTaxes = intptr(1000)
				return cancelled(i, evening)
			}(),
			wantPoints: 6000,
			wantTaxes:  intptr(2000),
		},
		{
			name: "cancelled the same day with a full refund nets to zero",
			given: func() data.Issuance {
				i := base
				i.RefundedPoints = intptr(10000)
				i.RefundedTaxes = intptr(3000)
				return cancelled(i, evening)
			}(),
			wantPoints: 0,
			wantTaxes:  intptr(0),
		},
		{
			name: "cancelled on a later day carries only the refund",
			given: func() data.Issuance {
				i := base
				i.RefundedPoints = intptr(10000)
				i.RefundedTaxes = intptr(3000)
				return cancelled(i, nextDay)
			}(),
			wantPoints: -10000,
			wantTaxes:  intptr(-3000),
		},
		{
			name: "cancellation-only row carries the refund and no booking",
			given: func() data.Issuance {
				i := base
				i.CancellationOnly = true
				i.RefundedPoints = intptr(10000)
				i.RefundedTaxes = intptr(3000)
				return cancelled(i, evening)
			}(),
			wantPoints: -10000,
			wantTaxes:  intptr(-3000),
		},
		{
			name:       "same-day cancellation with no refund keeps the miles spent",
			given:      cancelled(base, evening),
			wantPoints: 10000,
			wantTaxes:  intptr(3000),
		},
		{
			name:       "later cancellation with no refund recorded lands on nothing",
			given:      cancelled(base, nextDay),
			wantPoints: 0,
			wantTaxes:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			points, taxes := signedIssuanceAmounts(tc.given, loc)
			if points != tc.wantPoints {
				t.Errorf("points = %d, want %d", points, tc.wantPoints)
			}
			switch {
			case taxes == nil && tc.wantTaxes != nil:
				t.Errorf("taxes = nil, want %d", *tc.wantTaxes)
			case taxes != nil && tc.wantTaxes == nil:
				t.Errorf("taxes = %d, want nil", *taxes)
			case taxes != nil && *taxes != *tc.wantTaxes:
				t.Errorf("taxes = %d, want %d", *taxes, *tc.wantTaxes)
			}
		})
	}
}

// "Same day" is the operator's calendar day, not a UTC one: a booking made late in the
// evening and cancelled after midnight in São Paulo is two days even though UTC has not
// rolled over between them - and the reverse for a pair that straddles UTC midnight.
func TestSignedIssuanceAmountsUsesLocalCalendarDay(t *testing.T) {
	loc, err := time.LoadLocation(exportTimezone)
	if err != nil {
		t.Fatalf("load %s: %v", exportTimezone, err)
	}

	// 02:00Z is 23:00 on the 11th in São Paulo; 04:00Z is 01:00 on the 12th. Same UTC day,
	// two different local days.
	created := time.Date(2026, 8, 12, 2, 0, 0, 0, time.UTC)
	cancelledAt := time.Date(2026, 8, 12, 4, 0, 0, 0, time.UTC)
	issuance := data.Issuance{
		Points:         10000,
		CreatedAt:      created,
		CancelledAt:    &cancelledAt,
		RefundedPoints: intptr(10000),
	}

	if points, _ := signedIssuanceAmounts(issuance, loc); points != -10000 {
		t.Errorf("points = %d, want -10000 (the two instants are different local days)", points)
	}

	// Both on the 12th locally: 15:00Z and 22:00Z.
	issuance.CreatedAt = time.Date(2026, 8, 12, 15, 0, 0, 0, time.UTC)
	sameDay := time.Date(2026, 8, 12, 22, 0, 0, 0, time.UTC)
	issuance.CancelledAt = &sameDay
	if points, _ := signedIssuanceAmounts(issuance, loc); points != 0 {
		t.Errorf("points = %d, want 0 (spent and refunded on one local day)", points)
	}
}

func TestSignedRedemptionFee(t *testing.T) {
	loc, err := time.LoadLocation(exportTimezone)
	if err != nil {
		t.Fatalf("load %s: %v", exportTimezone, err)
	}

	created := time.Date(2026, 8, 12, 15, 0, 0, 0, time.UTC)
	sameDay := time.Date(2026, 8, 12, 22, 0, 0, 0, time.UTC)
	laterDay := time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		given data.Issuance
		want  *int
	}{
		{
			name:  "ordinary account saves nothing",
			given: data.Issuance{CreatedAt: created},
			want:  nil,
		},
		{
			name: "exempt account keeps its saving",
			given: data.Issuance{
				CreatedAt:               created,
				RedemptionFeeSavedCents: intptr(4200),
			},
			want: intptr(4200),
		},
		{
			name: "cancelled the same day nets out",
			given: data.Issuance{
				CreatedAt:               created,
				CancelledAt:             &sameDay,
				RedemptionFeeSavedCents: intptr(4200),
			},
			want: intptr(0),
		},
		{
			name: "cancelled on a later day gives the saving back",
			given: data.Issuance{
				CreatedAt:               created,
				CancelledAt:             &laterDay,
				RedemptionFeeSavedCents: intptr(4200),
			},
			want: intptr(-4200),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := signedRedemptionFee(tc.given, loc)
			switch {
			case got == nil && tc.want != nil:
				t.Fatalf("fee = nil, want %d", *tc.want)
			case got != nil && tc.want == nil:
				t.Fatalf("fee = %d, want nil", *got)
			case got != nil && *got != *tc.want:
				t.Fatalf("fee = %d, want %d", *got, *tc.want)
			}
		})
	}
}
