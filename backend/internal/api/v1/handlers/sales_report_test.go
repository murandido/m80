package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/supplierreport"
)

func intPtr(v int) *int { return &v }

// spTime parses a calendar day the way the handlers do, so a period built here has the same
// UTC bounds parseDateRange would have produced.
func spTime(t *testing.T, day string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		t.Fatalf("failed to load %s: %v", listTimezone, err)
	}
	parsed, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		t.Fatalf("failed to parse %q: %v", day, err)
	}
	return parsed.UTC()
}

func statementLine(kind string, points int, taxes, cost *int) schemas.SupplierIssuanceResponse {
	return statementLineWithTaxes(kind, points, taxes, cost, taxes != nil)
}

func statementLineWithTaxes(kind string, points int, taxes, cost *int, taxesIncluded bool) schemas.SupplierIssuanceResponse {
	return schemas.SupplierIssuanceResponse{
		IssuanceID:          "00000000-0000-0000-0000-000000000001",
		Kind:                kind,
		Date:                time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
		ProgramLocator:      "ABC123",
		OriginIATACode:      strPtr("GRU"),
		DestinationIATACode: strPtr("MIA"),
		Points:              points,
		Taxes:               taxes,
		CostOfTheThousand:   cost,
		TaxesIncludedInCost: taxesIncluded,
	}
}

func oneBalance(lines ...schemas.SupplierIssuanceResponse) []schemas.SupplierBalanceResponse {
	return []schemas.SupplierBalanceResponse{{
		AccountID:         "acc-1",
		AccountLabel:      "João da Silva",
		AdministratorName: strPtr("Maria Souza"),
		Issuances:         lines,
	}}
}

var noBrands = map[string]supplierReportBrand{}

// The amount is the one figure a supplier acts on, so the rounding has to be the project's
// own: points/1000 * rate, rounded half up, in centavos - the same arithmetic the panel and
// the bot's checksum use.
func TestSupplierLineAmount_RoundsLikeTheRestOfTheProject(t *testing.T) {
	for _, tc := range []struct {
		name          string
		points, rate  int
		taxes         *int
		taxesIncluded bool
		want          int
	}{
		{name: "whole thousands", points: 50000, rate: 1650, want: 82500},
		{name: "rounds half up", points: 1500, rate: 1667, want: 2501},
		{name: "taxes left out when the issuance says so", points: 10000, rate: 2000, taxes: intPtr(8400), want: 20000},
		{name: "taxes added when the issuance says so", points: 10000, rate: 2000, taxes: intPtr(8400), taxesIncluded: true, want: 28400},
		{name: "a credit is negative", points: -10000, rate: 2000, want: -20000},
		{name: "a credit's taxes come back too", points: -10000, rate: 2000, taxes: intPtr(-8400), taxesIncluded: true, want: -28400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := statementLineWithTaxes("issued", tc.points, tc.taxes, intPtr(tc.rate), tc.taxesIncluded)
			if got := supplierLineAmount(line); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// A booking and the credit that reverses it have to cancel out exactly. If they did not, a
// cancelled-and-rebooked issuance would leave a few centavos of phantom debt behind every
// time - the kind of error nobody finds until a supplier disputes a total.
func TestBuildSupplierReport_ACreditCancelsItsBooking(t *testing.T) {
	cost := intPtr(1667)
	balances := oneBalance(
		statementLine("issued", 1500, intPtr(8400), cost),
		statementLine("refund", -1500, intPtr(-8400), cost),
	)

	model, missing := buildSupplierReport(balances, noBrands, "03/08/2026")
	if missing != 0 {
		t.Fatalf("missing: got %d, want 0", missing)
	}
	if len(model.Sections) != 1 {
		t.Fatalf("sections: got %d, want 1", len(model.Sections))
	}
	if got := model.Sections[0].TotalAmount; got != "R$ 0,00" {
		t.Errorf("total: got %q, want %q", got, "R$ 0,00")
	}
	if !model.Sections[0].Lines[1].Refund {
		t.Error("the credit line must be marked as a refund so the document shows why")
	}
}

// Missing cost is the one condition that must stop the document rather than shrink it.
func TestBuildSupplierReport_CountsIssuancesWithoutACost(t *testing.T) {
	balances := oneBalance(
		statementLine("issued", 10000, nil, intPtr(1650)),
		statementLine("issued", 20000, nil, nil),
		statementLine("issued", 30000, nil, nil),
	)

	_, missing := buildSupplierReport(balances, noBrands, "03/08/2026")
	if missing != 2 {
		t.Errorf("missing: got %d, want 2", missing)
	}
	if got := supplierMissingCostMessage(missing); !strings.Contains(got, "2 emissões") {
		t.Errorf("message should name the count, got %q", got)
	}
	if got := supplierMissingCostMessage(1); !strings.Contains(got, "1 emissão") {
		t.Errorf("singular message: got %q", got)
	}
}

// The document is sent to the supplier, so a tax the agency is not reimbursing must not
// appear in it at all - not greyed, not struck through, not anywhere. Asserting on the
// rendered document rather than on the model is the point: the figure must be absent from
// the bytes that leave the building.
func TestBuildSupplierReport_HidesTaxesThatAreNotBeingPaid(t *testing.T) {
	balances := oneBalance(
		statementLineWithTaxes("issued", 10000, intPtr(8400), intPtr(2000), false),
		statementLineWithTaxes("issued", 10000, intPtr(7700), intPtr(2000), true),
	)

	model, missing := buildSupplierReport(balances, noBrands, "03/08/2026")
	if missing != 0 {
		t.Fatalf("missing: got %d, want 0", missing)
	}

	hidden, shown := model.Sections[0].Lines[0], model.Sections[0].Lines[1]
	if hidden.Taxes != "—" {
		t.Errorf("an unreimbursed tax must render as a dash, got %q", hidden.Taxes)
	}
	if shown.Taxes != "R$ 77,00" {
		t.Errorf("a reimbursed tax must show its figure, got %q", shown.Taxes)
	}

	document, err := supplierreport.Render(model)
	if err != nil {
		t.Fatalf("rendering failed: %v", err)
	}
	if strings.Contains(document, "84,00") {
		t.Error("the unreimbursed tax leaked into the document the supplier receives")
	}
	if !strings.Contains(document, "77,00") {
		t.Error("the reimbursed tax should be in the document")
	}
	// It is also left out of the column total, so the total cannot be used to work the
	// hidden figure back out by subtraction.
	if model.Sections[0].TotalTaxes != "R$ 77,00" {
		t.Errorf("tax total: got %q, want %q", model.Sections[0].TotalTaxes, "R$ 77,00")
	}
}

// The grand total only exists on a multi-account document; on a single one the section's
// own total is already the answer and a second figure just invites doubt.
func TestBuildSupplierReport_GrandTotalOnlyWhenSeveralAccounts(t *testing.T) {
	line := statementLine("issued", 10000, nil, intPtr(2000))

	single, _ := buildSupplierReport(oneBalance(line), noBrands, "03/08/2026")
	if single.ShowGrandTotal {
		t.Error("a single-account document must not repeat its total as a grand total")
	}

	two := append(oneBalance(line), schemas.SupplierBalanceResponse{
		AccountID: "acc-2", AccountLabel: "Outra Conta", Issuances: []schemas.SupplierIssuanceResponse{line},
	})
	multi, _ := buildSupplierReport(two, noBrands, "03/08/2026")
	if !multi.ShowGrandTotal {
		t.Fatal("a multi-account document needs a grand total")
	}
	if multi.GrandTotal != "R$ 400,00" {
		t.Errorf("grand total: got %q, want %q", multi.GrandTotal, "R$ 400,00")
	}
}

// date_to arrives as the exclusive start of the next day, so a naive label would name a day
// the operator never picked - and the document's own title would contradict its contents.
func TestSupplierPeriodLabel_NamesTheInclusiveWindow(t *testing.T) {
	from := spTime(t, "2026-08-01")
	to := spTime(t, "2026-08-13") // exclusive bound for a period ending on the 12th

	for _, tc := range []struct {
		name     string
		from, to *time.Time
		want     string
	}{
		{"a range", &from, &to, "01/08/2026 a 12/08/2026"},
		{"open ended", nil, nil, "todo o período"},
		{"only a start", &from, nil, "a partir de 01/08/2026"},
		{"only an end", nil, &to, "até 12/08/2026"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := supplierPeriodLabel(tc.from, tc.to); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSupplierPeriodLabel_SingleDayIsNotARange(t *testing.T) {
	from := spTime(t, "2026-08-11")
	to := spTime(t, "2026-08-12")
	if got := supplierPeriodLabel(&from, &to); got != "11/08/2026" {
		t.Errorf("got %q, want %q", got, "11/08/2026")
	}
}

// The label goes into a Content-Disposition header, so anything that could close the quoted
// string has to be gone before it gets there.
func TestSupplierReportFilename_IsSafeInAHeader(t *testing.T) {
	model := supplierreport.ViewModel{Sections: []supplierreport.Section{
		{AccountLabel: `João "Zé" da Silva; rm -rf`},
	}}

	got := supplierReportFilename(model)
	if strings.ContainsAny(got, `";`) {
		t.Errorf("filename must not carry quotes or semicolons, got %q", got)
	}
	if got != "resumo-jo-o-z-da-silva-rm-rf.pdf" {
		t.Errorf("filename: got %q", got)
	}

	many := supplierreport.ViewModel{Sections: []supplierreport.Section{{}, {}}}
	if got := supplierReportFilename(many); got != "resumo-fornecedores.pdf" {
		t.Errorf("multi-account filename: got %q", got)
	}
}

// The document renders as a complete page from a realistic model. This is the cheap guard
// against a template that only fails at the moment somebody tries to send a summary out.
func TestBuildSupplierReport_RendersACompleteDocument(t *testing.T) {
	balances := oneBalance(
		statementLine("issued", 50000, intPtr(8400), intPtr(1650)),
		statementLine("refund", -20000, intPtr(-3000), intPtr(1650)),
	)

	model, missing := buildSupplierReport(balances, noBrands, "01/08/2026 a 12/08/2026")
	if missing != 0 {
		t.Fatalf("missing: got %d, want 0", missing)
	}

	document, err := supplierreport.Render(model)
	if err != nil {
		t.Fatalf("rendering failed: %v", err)
	}

	for _, want := range []string{
		"João da Silva",
		"Maria Souza",
		"01/08/2026 a 12/08/2026",
		"ABC123",
		"GRU",
		"Total a pagar",
		"estorno",
		"Localizador",
		"Taxas",
		"</html>",
	} {
		if !strings.Contains(document, want) {
			t.Errorf("document is missing %q", want)
		}
	}
	// WeasyPrint 61.2 drops all three of these silently, taking the styling with them.
	for _, forbidden := range []string{"oklch(", "color-mix(", "display: grid", "@media"} {
		if strings.Contains(document, forbidden) {
			t.Errorf("document uses %q, which WeasyPrint 61.2 does not render", forbidden)
		}
	}
}
