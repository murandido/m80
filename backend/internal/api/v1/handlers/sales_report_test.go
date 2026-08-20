package handlers

import (
	"reflect"
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

	model, missing := buildSupplierReport(balances, noBrands, "03/08/2026", groupByAccount)
	if missing != 0 {
		t.Fatalf("missing: got %d, want 0", missing)
	}
	if model.SectionCount() != 1 {
		t.Fatalf("sections: got %d, want 1", model.SectionCount())
	}
	if got := model.Groups[0].Sections[0].TotalAmount; got != "R$ 0,00" {
		t.Errorf("total: got %q, want %q", got, "R$ 0,00")
	}
	if !model.Groups[0].Sections[0].Lines[1].Refund {
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

	_, missing := buildSupplierReport(balances, noBrands, "03/08/2026", groupByAccount)
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

	model, missing := buildSupplierReport(balances, noBrands, "03/08/2026", groupByAccount)
	if missing != 0 {
		t.Fatalf("missing: got %d, want 0", missing)
	}

	hidden, shown := model.Groups[0].Sections[0].Lines[0], model.Groups[0].Sections[0].Lines[1]
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
	if model.Groups[0].Sections[0].TotalTaxes != "R$ 77,00" {
		t.Errorf("tax total: got %q, want %q", model.Groups[0].Sections[0].TotalTaxes, "R$ 77,00")
	}
}

// The grand total only exists on a multi-account document; on a single one the section's
// own total is already the answer and a second figure just invites doubt.
func TestBuildSupplierReport_GrandTotalOnlyWhenSeveralAccounts(t *testing.T) {
	line := statementLine("issued", 10000, nil, intPtr(2000))

	single, _ := buildSupplierReport(oneBalance(line), noBrands, "03/08/2026", groupByAccount)
	if single.ShowGrandTotal {
		t.Error("a single-account document must not repeat its total as a grand total")
	}

	two := append(oneBalance(line), schemas.SupplierBalanceResponse{
		AccountID: "acc-2", AccountLabel: "Outra Conta", Issuances: []schemas.SupplierIssuanceResponse{line},
	})
	multi, _ := buildSupplierReport(two, noBrands, "03/08/2026", groupByAccount)
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
	model := supplierreport.ViewModel{Groups: []supplierreport.Group{
		{Sections: []supplierreport.Section{{AccountLabel: `João "Zé" da Silva; rm -rf`}}},
	}}

	got := supplierReportFilename(model)
	if strings.ContainsAny(got, `";`) {
		t.Errorf("filename must not carry quotes or semicolons, got %q", got)
	}
	if got != "resumo-jo-o-z-da-silva-rm-rf.pdf" {
		t.Errorf("filename: got %q", got)
	}

	many := supplierreport.ViewModel{Groups: []supplierreport.Group{
		{Sections: []supplierreport.Section{{}, {}}},
	}}
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

	model, missing := buildSupplierReport(balances, noBrands, "01/08/2026 a 12/08/2026", groupByAccount)
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

// A line grouped by administrator has to say which account it came out of, because the row
// above it no longer does. Everything downstream - the split into sections, the brand
// lookup, the file name - keys off that, so a line without it silently collapses several
// accounts into one nameless section.
func administratorBalance(lines ...schemas.SupplierIssuanceResponse) []schemas.SupplierBalanceResponse {
	return []schemas.SupplierBalanceResponse{{
		GroupKey:          "adm-1",
		GroupLabel:        "Maria Souza",
		AccountCount:      2,
		AccountID:         "acc-1",
		AccountLabel:      "João da Silva",
		AdministratorName: strPtr("Maria Souza"),
		Issuances:         lines,
	}}
}

func lineFromAccount(id, label string, points int, cost *int) schemas.SupplierIssuanceResponse {
	line := statementLine("issued", points, nil, cost)
	line.AccountID = id
	line.AccountLabel = label
	return line
}

func TestBuildSupplierReport_GroupsAccountsUnderTheirAdministrator(t *testing.T) {
	balances := administratorBalance(
		lineFromAccount("acc-2", "Zulmira", 10000, intPtr(1000)),
		lineFromAccount("acc-1", "Ambrosina", 20000, intPtr(1000)),
		lineFromAccount("acc-2", "Zulmira", 30000, intPtr(1000)),
	)

	model, missing := buildSupplierReport(balances, noBrands, "03/08/2026", groupByAdministrator)
	if missing != 0 {
		t.Fatalf("missing: got %d, want 0", missing)
	}

	if len(model.Groups) != 1 {
		t.Fatalf("groups: got %d, want 1", len(model.Groups))
	}
	group := model.Groups[0]
	if group.Label != "Maria Souza" {
		t.Errorf("group label: got %q, want %q", group.Label, "Maria Souza")
	}

	// One section per account, so each keeps its own programme colour - and in a stable
	// order, or two identical requests would produce different documents.
	if len(group.Sections) != 2 {
		t.Fatalf("sections: got %d, want 2", len(group.Sections))
	}
	if group.Sections[0].AccountLabel != "Ambrosina" || group.Sections[1].AccountLabel != "Zulmira" {
		t.Errorf("sections out of order: %q then %q",
			group.Sections[0].AccountLabel, group.Sections[1].AccountLabel)
	}
	if len(group.Sections[1].Lines) != 2 {
		t.Errorf("Zulmira's lines: got %d, want 2", len(group.Sections[1].Lines))
	}

	// The subtotal is the whole administrator, not whichever account happened to be last.
	// 60.000 pontos a R$ 10,00 o milheiro.
	if group.TotalAmount != "R$ 600,00" {
		t.Errorf("group total: got %q, want %q", group.TotalAmount, "R$ 600,00")
	}
	if group.TotalPoints != "60.000" {
		t.Errorf("group points: got %q, want %q", group.TotalPoints, "60.000")
	}
}

// Grouping must not change what the document is worth - only how it is arranged. This is
// the check that catches a bucket dropped on the way through.
func TestBuildSupplierReport_GroupingDoesNotChangeTheTotals(t *testing.T) {
	lines := []schemas.SupplierIssuanceResponse{
		lineFromAccount("acc-1", "Ambrosina", 20000, intPtr(1650)),
		lineFromAccount("acc-2", "Zulmira", 31000, intPtr(1650)),
	}

	byAccount, _ := buildSupplierReport(oneBalance(lines...), noBrands, "03/08/2026", groupByAccount)
	byAdmin, _ := buildSupplierReport(administratorBalance(lines...), noBrands, "03/08/2026", groupByAdministrator)

	if byAccount.GrandTotal != "" && byAccount.GrandTotal == byAdmin.GrandTotal {
		return // both multi-section and equal, which is the point
	}
	// One section against two means only the sections' own totals are comparable.
	sum := byAdmin.Groups[0].TotalAmount
	if want := byAccount.Groups[0].Sections[0].TotalAmount; sum != want {
		t.Errorf("administrator subtotal %q does not match the account total %q", sum, want)
	}
}

// A document grouped by administrator has no single account to be named after.
func TestSupplierReportFilename_UsesTheGroupWhenThereIsOne(t *testing.T) {
	model := supplierreport.ViewModel{Groups: []supplierreport.Group{{
		Label:    "Maria Souza",
		Sections: []supplierreport.Section{{AccountLabel: "Ambrosina"}, {AccountLabel: "Zulmira"}},
	}}}

	if got := supplierReportFilename(model); got != "resumo-maria-souza.pdf" {
		t.Errorf("filename: got %q, want %q", got, "resumo-maria-souza.pdf")
	}
}

// balanceFor is one account's balance the way the account-grouped view produces it, with the
// administrator filled in - which the older helpers above deliberately leave off, since they
// predate the recap and cover the document body instead.
func balanceFor(
	adminID, adminName, accountID, accountLabel string,
	lines ...schemas.SupplierIssuanceResponse,
) schemas.SupplierBalanceResponse {
	for i := range lines {
		lines[i].AccountID = accountID
		lines[i].AccountLabel = accountLabel
	}
	return schemas.SupplierBalanceResponse{
		GroupKey:          accountID,
		GroupLabel:        accountLabel,
		AccountCount:      1,
		AccountID:         accountID,
		AccountLabel:      accountLabel,
		AdministratorID:   strPtr(adminID),
		AdministratorName: strPtr(adminName),
		Issuances:         lines,
	}
}

// Two accounts of the same administrator in different programmes, plus a third account of
// somebody else. R$ 10,00 the thousand throughout, so the arithmetic is readable: Maria is
// owed 20.000 + 30.000 points = R$ 500,00 and Joana 10.000 points plus R$ 84,00 of taxes.
func recapBalances() ([]schemas.SupplierBalanceResponse, map[string]supplierReportBrand) {
	balances := []schemas.SupplierBalanceResponse{
		balanceFor("adm-1", "Maria Souza", "acc-1", "Ambrosina",
			statementLine("issued", 20000, nil, intPtr(1000))),
		balanceFor("adm-1", "Maria Souza", "acc-2", "Zulmira",
			statementLine("issued", 30000, nil, intPtr(1000))),
		balanceFor("adm-2", "Joana Lima", "acc-3", "Benedita",
			statementLineWithTaxes("issued", 10000, intPtr(8400), intPtr(1000), true)),
	}
	brands := map[string]supplierReportBrand{
		"acc-1": {programName: "TudoAzul", programColor: "#0a3d91"},
		"acc-2": {programName: "Smiles", programColor: "#ff7a00"},
		"acc-3": {programName: "TudoAzul", programColor: "#0a3d91"},
	}
	return balances, brands
}

// The recap is the only place in the document that answers "who gets paid how much" in one
// line, so an administrator's accounts have to meet there even though each one prints its
// own statement further up.
func TestBuildSupplierReport_SummaryAddsUpPerAdministrator(t *testing.T) {
	balances, brands := recapBalances()

	model, missing := buildSupplierReport(balances, brands, "03/08/2026", groupByAccount)
	if missing != 0 {
		t.Fatalf("missing: got %d, want 0", missing)
	}
	if !model.ShowSummary {
		t.Fatal("a document with three accounts must carry the recap")
	}
	if len(model.Summary) != 2 {
		t.Fatalf("recap rows: got %d, want 2", len(model.Summary))
	}

	// Biggest debt first, the same order the screen is in.
	maria := model.Summary[0]
	if maria.Administrator != "Maria Souza" {
		t.Fatalf("first row: got %q, want %q", maria.Administrator, "Maria Souza")
	}
	if maria.AccountCount != "2 contas" {
		t.Errorf("accounts: got %q, want %q", maria.AccountCount, "2 contas")
	}
	if maria.Points != "50.000" {
		t.Errorf("points: got %q, want %q", maria.Points, "50.000")
	}
	if maria.Amount != "R$ 500,00" {
		t.Errorf("amount: got %q, want %q", maria.Amount, "R$ 500,00")
	}
	// Nothing was reimbursed, and a "R$ 0,00" there would read as a figure that was checked.
	if maria.Taxes != "—" {
		t.Errorf("taxes: got %q, want a dash", maria.Taxes)
	}
	// One badge per programme, not one per account, and in a stable order.
	if len(maria.Programs) != 2 ||
		maria.Programs[0].Name != "Smiles" || maria.Programs[1].Name != "TudoAzul" {
		t.Errorf("programmes: got %+v, want Smiles then TudoAzul", maria.Programs)
	}
	if maria.Programs[0].Brand != "#ff7a00" || maria.Programs[0].BrandOn == "" {
		t.Errorf("the Smiles badge lost its colour: %+v", maria.Programs[0])
	}

	joana := model.Summary[1]
	if joana.Administrator != "Joana Lima" || joana.AccountCount != "1 conta" {
		t.Errorf("second row: got %q with %q", joana.Administrator, joana.AccountCount)
	}
	if joana.Amount != "R$ 184,00" {
		t.Errorf("amount: got %q, want %q", joana.Amount, "R$ 184,00")
	}
	if joana.Taxes != "R$ 84,00" {
		t.Errorf("taxes: got %q, want %q", joana.Taxes, "R$ 84,00")
	}

	if model.SummaryAccountCount != "3 contas" {
		t.Errorf("total accounts: got %q, want %q", model.SummaryAccountCount, "3 contas")
	}
}

// The recap answers a question about people, and the grouping only decides how the pages
// beneath it are arranged - so the two documents have to close with the same table. This is
// the invariant that catches an administrator dropped on the way through one of the branches.
func TestBuildSupplierReport_SummaryIsTheSameInBothGroupings(t *testing.T) {
	balances, brands := recapBalances()

	byAccount, _ := buildSupplierReport(balances, brands, "03/08/2026", groupByAccount)

	// The same issuances as the administrator-grouped view produces them: one balance per
	// administrator, each carrying the lines of all their accounts.
	maria := balances[0]
	maria.GroupKey, maria.GroupLabel, maria.AccountCount = "adm-1", "Maria Souza", 2
	maria.Issuances = append(append([]schemas.SupplierIssuanceResponse{}, balances[0].Issuances...),
		balances[1].Issuances...)
	joana := balances[2]
	joana.GroupKey, joana.GroupLabel = "adm-2", "Joana Lima"

	byAdmin, _ := buildSupplierReport(
		[]schemas.SupplierBalanceResponse{maria, joana}, brands, "03/08/2026", groupByAdministrator)

	if len(byAccount.Summary) != len(byAdmin.Summary) {
		t.Fatalf("recap rows: %d grouped by account, %d by administrator",
			len(byAccount.Summary), len(byAdmin.Summary))
	}
	for i := range byAccount.Summary {
		if !reflect.DeepEqual(byAccount.Summary[i], byAdmin.Summary[i]) {
			t.Errorf("row %d differs:\n by account: %+v\n by admin:   %+v",
				i, byAccount.Summary[i], byAdmin.Summary[i])
		}
	}
	if byAccount.SummaryAccountCount != byAdmin.SummaryAccountCount {
		t.Errorf("account count: %q vs %q",
			byAccount.SummaryAccountCount, byAdmin.SummaryAccountCount)
	}
}

// An account nobody administers still owes miles. Dropping it would make the recap quietly
// add up to less than the grand total printed right above it.
func TestBuildSupplierReport_SummaryBucketsAccountsWithoutAnAdministrator(t *testing.T) {
	orphan := balanceFor("", "", "acc-9", "Órfã", statementLine("issued", 10000, nil, intPtr(1000)))
	orphan.AdministratorID = nil
	orphan.AdministratorName = nil

	balances := []schemas.SupplierBalanceResponse{
		balanceFor("adm-1", "Maria Souza", "acc-1", "Ambrosina",
			statementLine("issued", 20000, nil, intPtr(1000))),
		orphan,
	}

	model, _ := buildSupplierReport(balances, noBrands, "03/08/2026", groupByAccount)
	if len(model.Summary) != 2 {
		t.Fatalf("recap rows: got %d, want 2", len(model.Summary))
	}
	// Same wording as the group heading uses, so the two never name the same bucket
	// differently in one document.
	if model.Summary[1].Administrator != "Sem administrador" {
		t.Errorf("orphan row: got %q, want %q", model.Summary[1].Administrator, "Sem administrador")
	}
	if model.SummaryAccountCount != "2 contas" {
		t.Errorf("total accounts: got %q, want %q", model.SummaryAccountCount, "2 contas")
	}
}

// On a single-account document the recap would be that account, restated with fewer columns.
func TestBuildSupplierReport_NoSummaryOnASingleAccount(t *testing.T) {
	balances := []schemas.SupplierBalanceResponse{
		balanceFor("adm-1", "Maria Souza", "acc-1", "Ambrosina",
			statementLine("issued", 20000, nil, intPtr(1000))),
	}

	model, _ := buildSupplierReport(balances, noBrands, "03/08/2026", groupByAccount)
	if model.ShowSummary {
		t.Error("a one-account document must not repeat itself as a recap")
	}
}

// The recap's TOTAL row and the "Total geral a pagar" above it are two routes to the same
// sum. If they ever disagree, the reader has no way to tell which one to send money against.
func TestBuildSupplierReport_SummaryTotalMatchesTheGrandTotal(t *testing.T) {
	balances := []schemas.SupplierBalanceResponse{
		balanceFor("adm-1", "Maria Souza", "acc-1", "Ambrosina",
			statementLine("issued", 20000, nil, intPtr(1650))),
		balanceFor("adm-1", "Maria Souza", "acc-2", "Zulmira",
			statementLine("issued", 31000, nil, intPtr(1650))),
	}

	model, _ := buildSupplierReport(balances, noBrands, "03/08/2026", groupByAccount)
	if len(model.Summary) != 1 {
		t.Fatalf("recap rows: got %d, want 1", len(model.Summary))
	}
	// One administrator holds everything, so their row is the whole document.
	if model.Summary[0].Amount != model.GrandTotal {
		t.Errorf("recap says %q, grand total says %q", model.Summary[0].Amount, model.GrandTotal)
	}
	if model.Summary[0].Points != model.GrandTotalPoints {
		t.Errorf("recap points %q, grand total points %q",
			model.Summary[0].Points, model.GrandTotalPoints)
	}
}

// The recap has to survive the template as well as the arithmetic, and it is the last block
// on the page - which is exactly where a broken conditional shows up as silence.
func TestBuildSupplierReport_RendersTheRecap(t *testing.T) {
	balances, brands := recapBalances()

	model, _ := buildSupplierReport(balances, brands, "01/08/2026 a 12/08/2026", groupByAccount)
	document, err := supplierreport.Render(model)
	if err != nil {
		t.Fatalf("rendering failed: %v", err)
	}

	for _, want := range []string{
		"Pontos &times; pagamentos",
		"Administrador",
		"Maria Souza",
		"Joana Lima",
		"2 contas",
		"TOTAL",
		"#ff7a00",
	} {
		if !strings.Contains(document, want) {
			t.Errorf("recap is missing %q", want)
		}
	}
	if strings.Index(document, "Pontos &times; pagamentos") < strings.Index(document, "Total geral a pagar") {
		t.Error("the recap has to close the document, after the grand total")
	}
}
