// Package supplierreport renders the payment summary an agency sends to an account's
// administrator: what was issued out of that account in a period, and what is owed for it.
//
// It is the accounting counterpart of internal/ticket, and follows the same two rules for
// the same reasons. Everything is formatted in Go and no database type crosses into the
// template, so a nil can never surface as a half-rendered page. And the markup stays inside
// WeasyPrint 61.2's subset - see report.gohtml - because the string this package produces
// is handed straight to the PDF service.
//
// This package runs no queries. The caller loads the data and decides which issuances the
// document covers, which is what keeps the summary and the screen it was generated from
// describing the same set.
package supplierreport

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"

	_ "embed"

	"github.com/murandido/m-80-backend/internal/ticket"
)

//go:embed report.gohtml
var templateSource string

var tpl = template.Must(template.New("report.gohtml").Parse(templateSource))

// ViewModel is one document. It carries several sections when the operator asked for every
// filtered account at once, and exactly one when they asked for a single account's summary.
type ViewModel struct {
	// Groups wrap the sections. Grouped by account there is one section per group and no
	// group heading, so the document renders exactly as it did before groups existed;
	// grouped by administrator a group is the administrator and its sections are their
	// accounts, each keeping its own programme colour.
	Groups []Group

	// ShowGrandTotal is set only on a multi-account document. On a single-account one the
	// section's own total is already the answer, and repeating it reads like a second,
	// different number.
	ShowGrandTotal bool
	GrandTotal     string
	// The same two figures the sections already close with, added up across the document.
	// They sit under the money on purpose and with less weight: what is owed is the number
	// the reader came for, and these are the two it was built from.
	GrandTotalPoints string
	GrandTotalTaxes  string

	// Summary is the closing recap: one line per account administrator, whichever way the
	// document itself was grouped. It answers the question the body of the document only
	// answers by hunting - who gets paid how much - and it is deliberately built from the
	// same section totals the statements above print, so the two can never disagree.
	ShowSummary bool
	Summary     []SummaryRow
	// SummaryAccountCount closes the "Contas" column. The money columns of that row reuse
	// the grand total, which is the same sum arrived at by a different route.
	SummaryAccountCount string

	PageHeightMM int
}

// SummaryRow is one administrator in the closing recap.
type SummaryRow struct {
	Administrator string
	// Programs are the loyalty programs behind this administrator's accounts. Several,
	// because an administrator can run accounts in more than one - which is also why the
	// row itself carries no single brand colour.
	Programs     []ProgramChip
	AccountCount string

	Points string
	// Taxes is a dash when nothing was reimbursed, for the same reason the statement lines
	// use one: a figure printed there would read as money owed.
	Taxes  string
	Amount string
}

// ProgramChip is one loyalty program badge, drawn in the program's own colour.
type ProgramChip struct {
	Name    string
	Brand   string
	BrandOn string
}

// Group is a run of sections that belong together. An empty Label means the group is only
// a container - there is nothing to say above the section that the section does not already
// say - and neither the heading nor the subtotal is drawn.
type Group struct {
	Label       string
	Sections    []Section
	TotalPoints string
	TotalTaxes  string
	TotalAmount string
}

// Section is one account's statement.
type Section struct {
	AccountLabel  string
	PeriodLabel   string
	Administrator string

	ProgramName string
	// Logo is inline SVG, marked safe because it comes from internal/ticket's own
	// embedded files and never from the database.
	Logo template.HTML
	// Brand is the loyalty program's colour and BrandOn the text colour that stays
	// legible on it - the same pairing internal/ticket uses, for the same reason: half
	// the real brand colours fail against white.
	Brand   string
	BrandOn string

	Lines []Line

	TotalPoints string
	TotalTaxes  string
	TotalAmount string
}

// Line is one issuance as the document draws it. Everything is a formatted string: the
// template does no arithmetic, so the figures in the PDF are the ones this package computed.
type Line struct {
	Locator     string
	Date        string
	Origin      string
	Destination string
	Points      string
	Milheiro    string
	Amount      string

	// Taxes is the line's taxes when they are part of what is being paid, and a dash when
	// they are not. The dash is deliberate: this document goes to the supplier, and a tax
	// the agency is not reimbursing is none of their business. Printing the figure and
	// striking it through would disclose it just the same.
	Taxes string

	// Refund marks a cancellation credit - miles that were paid for and came back. The
	// template renders it in the negative so a supplier reading the column can see why
	// the total is lower than the bookings above it.
	Refund bool
}

// Render produces the document. The same string could be shown as a page, but today it only
// ever goes to WeasyPrint.
func Render(model ViewModel) (string, error) {
	var out bytes.Buffer
	if err := tpl.Execute(&out, model); err != nil {
		return "", fmt.Errorf("supplierreport: rendering: %w", err)
	}
	return out.String(), nil
}

// LogoFor resolves a brand mark for a section: the loyalty program's own artwork when there
// is any, and the program's default airline otherwise.
//
// Program first is deliberate and not interchangeable. TudoAzul is drawn with AZUL's mark
// and Latam Pass with LATAM's, but Smiles is its own brand and would be wrong as GOL - a
// supplier reading a Smiles statement expects the Smiles logo.
func LogoFor(programCode string, defaultAirlineCode *string) template.HTML {
	if logo := ticket.LogoFor(programCode); logo != "" {
		return logo
	}
	if defaultAirlineCode != nil {
		return ticket.LogoFor(*defaultAirlineCode)
	}
	return ""
}

// ContrastColour picks black or white text for a brand background, by the WCAG relative
// luminance formula. Half the real brand colours fail against white - GOL's orange and
// AZUL's light blue among them - so a hardcoded white would leave those headers unreadable.
func ContrastColour(hex string) string {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return "#ffffff"
	}

	channel := func(value int) float64 {
		v := float64(value) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}

	luminance := 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
	if luminance > 0.45 {
		return "#0f172a"
	}
	return "#ffffff"
}

func parseHex(hex string) (r, g, b int, ok bool) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	value, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(value>>16) & 0xff, int(value>>8) & 0xff, int(value) & 0xff, true
}

// FormatBRL renders centavos as Brazilian currency, thousands separator included:
// 2000000 becomes "20.000,00". Exported because the caller composes the grand total.
//
// Deliberately not the bot's formatCents, which writes "20000,00" with no separator on
// purpose - that string is parsed back by the bot. This one is only ever read by a person,
// and a payment figure without separators is a figure waiting to be misread by a factor
// of ten.
func FormatBRL(cents int) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}

	reais := strconv.Itoa(cents / 100)
	centavos := cents % 100

	var grouped strings.Builder
	for i, digit := range reais {
		if i > 0 && (len(reais)-i)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}

	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%sR$ %s,%02d", sign, grouped.String(), centavos)
}

// FormatPoints renders a mile count with thousands separators: 1500000 becomes "1.500.000".
func FormatPoints(points int) string {
	negative := points < 0
	if negative {
		points = -points
	}

	digits := strconv.Itoa(points)
	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}

	if negative {
		return "-" + grouped.String()
	}
	return grouped.String()
}

// The page is 216mm wide, matching internal/ticket so the two documents an agency sends out
// share a paper size, with a height computed from the content. Same trade as the ticket:
// overestimating costs white space, underestimating costs a second page.
const (
	// pageChromeMM covers the two page margins. Everything else is per section.
	pageChromeMM = 30
	// sectionMM is the brand band, the column heading, the totals row, the "Total a
	// pagar" panel and the gap to the next section - measured from real renders at
	// 62dpi, not guessed on paper, and rounded up. Underestimating this is what pushed
	// the grand total onto a second page the first time.
	sectionMM = 56
	lineMM    = 7
	// Two lines now, not one: the breakdown under the payable sits inside the same block.
	// Underestimating this is what pushed the grand total onto a second page once already.
	grandTotalMM = 40
	// groupMM is the administrator heading plus its subtotal panel, drawn only on a grouped
	// document. Same trade as sectionMM: measured generously, because the cost of guessing
	// low is a page break in the middle of a total.
	groupMM = 34
	// summaryChromeMM is the recap's heading, its column head, its TOTAL row and the gap
	// above it; summaryRowMM is one administrator. Measured generously, the same trade as
	// sectionMM: guessing low costs a page break in the middle of a total.
	summaryChromeMM = 34
	summaryRowMM    = 8
	pageMinMM       = 160
	pageMaxMM       = 1200
	pageRoundToMM   = 10
)

// pageHeightMM sizes the paper to the content, rounded up to a round number of millimetres
// so two documents of similar length come out the same size.
func pageHeightMM(model ViewModel) int {
	height := pageChromeMM
	for _, group := range model.Groups {
		if group.Label != "" {
			height += groupMM
		}
		for _, section := range group.Sections {
			height += sectionMM + lineMM*len(section.Lines)
		}
	}
	if model.ShowGrandTotal {
		height += grandTotalMM
	}
	if model.ShowSummary {
		height += summaryChromeMM + summaryRowMM*len(model.Summary)
	}

	if remainder := height % pageRoundToMM; remainder != 0 {
		height += pageRoundToMM - remainder
	}
	if height < pageMinMM {
		return pageMinMM
	}
	if height > pageMaxMM {
		// A statement long enough to hit this is going to break across pages whatever
		// we ask for; page-break-inside: avoid on the rows makes that a clean break.
		return pageMaxMM
	}
	return height
}

// SectionCount is how many statements the document carries, across every group. It is what
// decides whether a grand total means anything: on a document with one statement the
// section's own total already is the answer.
func (m ViewModel) SectionCount() int {
	count := 0
	for _, group := range m.Groups {
		count += len(group.Sections)
	}
	return count
}

// Totals is what a document adds up to across its sections. A struct rather than three int
// arguments: points and centavos are both plain ints, so a positional call is one that
// silently compiles with the taxes and the amount swapped.
type Totals struct {
	Points      int
	TaxesCents  int
	AmountCents int
}

// Finalize computes what the document needs but a caller should not have to: the grand
// totals across sections and the paper height. Callers build the sections and hand the model
// here rather than doing this arithmetic themselves.
func Finalize(model ViewModel, totals Totals) ViewModel {
	model.ShowGrandTotal = model.SectionCount() > 1
	// Same rule as the grand total, for the same reason: on a document with a single
	// statement the recap would be that statement, restated with fewer columns.
	model.ShowSummary = model.ShowGrandTotal && len(model.Summary) > 0
	if model.ShowGrandTotal {
		model.GrandTotal = FormatBRL(totals.AmountCents)
		model.GrandTotalPoints = FormatPoints(totals.Points)
		model.GrandTotalTaxes = FormatBRL(totals.TaxesCents)
	}
	model.PageHeightMM = pageHeightMM(model)
	return model
}
