// Package clientreport renders the payment statement an agency sends to a client: the
// issuances bought in a period, what each one cost, what was already paid and what is still
// owed.
//
// It is the client-side counterpart of internal/supplierreport and follows its two rules for
// the same reasons: everything is formatted in Go, so the template does no arithmetic and no
// database type reaches it; and the markup stays inside WeasyPrint 61.2's subset (see
// report.gohtml), because the string this package produces goes straight to the PDF service.
//
// This package runs no queries. The caller loads the sales with the same filters the screen
// used, which is what keeps the statement and the screen describing the same set.
package clientreport

import (
	"bytes"
	"fmt"
	"html/template"

	_ "embed"
)

//go:embed report.gohtml
var templateSource string

var tpl = template.Must(template.New("report.gohtml").Parse(templateSource))

// ViewModel is one document: one section per client the filter picked.
type ViewModel struct {
	AgencyName  string
	PeriodLabel string
	// GeneratedAt says when the figures were taken. The statement goes to a client who
	// may read it days later, after more payments came in.
	GeneratedAt string
	Sections    []Section
}

// Section is one client's statement.
type Section struct {
	ClientName string
	Lines      []Line

	TotalAmount string
	TotalPaid   string
	// TotalBalance is the signed sum of the lines' balances - what the screen's "Total
	// devido" shows - so a refund still owed to the client lowers it.
	TotalBalance string
	// Settled draws the balance panel as good news instead of as a debt.
	Settled bool
	// Credit is what the client paid that no issuance took yet, empty when there is none.
	Credit string
}

// Line is one issuance.
type Line struct {
	Date      string
	Locator   string
	Passenger string
	Amount    string
	Paid      string
	Balance   string
	Status    string
	// StatusKind picks the colour of the status tag: paid, partial, unpaid, refund,
	// cancelled or novalue - the same states the panel's button has.
	StatusKind string
	Cancelled  bool
}

// Render produces the document.
func Render(model ViewModel) (string, error) {
	var out bytes.Buffer
	if err := tpl.Execute(&out, model); err != nil {
		return "", fmt.Errorf("clientreport: rendering: %w", err)
	}
	return out.String(), nil
}
