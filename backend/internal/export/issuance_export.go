package export

import (
	"bytes"
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/xuri/excelize/v2"
)

// Layout of the MODELO template (see backend/assets/templates/emissoes-gerais-modelo.xlsx).
const (
	issuanceExportSheet = "MODELO"
	// The data table lives in B:P; row 1 is the title, row 2 the column headers, so the
	// first issuance goes on row 3. The summary block (columns T:AH) sums the fixed range
	// $3:$152 (150 issuances); rows beyond that still receive their data but are not picked
	// up by those totals - acceptable for the day/period-scoped filters this export uses.
	issuanceExportFirstDataRow = 3
	// R2 feeds the A1 title formula ="EMISSÕES GERAIS "&TEXT(R2,"dd/mm/yyyy").
	issuanceExportTitleDateCell = "R2"
	// Q is the first free column after the manual sheet's own table (B:P) and stops short
	// of the summary block (T:AH), so the redemption fee can be added without moving a
	// single formula. It does not exist in the MODELO, which is why the header is written
	// on every render rather than lifted from the template.
	issuanceExportRedemptionFeeColumn = "Q"
	issuanceExportRedemptionFeeHeader = "TAXA RESGATE"
	// The header row and the taxes column, whose styling the new column borrows so it
	// looks like it was always part of the sheet.
	issuanceExportHeaderRow     = 2
	issuanceExportTaxesColumn   = "K"
	issuanceExportLastStyledRow = 152
)

// exportTimezone matches the timezone the issuance list uses to resolve "today", so the
// dates written to the sheet are the operator's local calendar days, not UTC.
const exportTimezone = "America/Sao_Paulo"

// BuildIssuancesXLSX renders the given issuances into a copy of the MODELO template,
// preserving its styling and summary formulas, and returns the resulting .xlsx bytes.
//
// from/to are the same half-open period the listing was filtered by (either may be nil for
// an unbounded end). They decide the sign of each row - see signedIssuanceAmounts.
func BuildIssuancesXLSX(issuances []data.Issuance, from, to *time.Time) ([]byte, error) {
	template, err := loadIssuanceTemplate()
	if err != nil {
		return nil, err
	}
	return renderIssuancesXLSX(template, issuances, from, to, time.Now())
}

// renderIssuancesXLSX holds the pure rendering logic: it takes the template bytes and the
// reference time explicitly so it can be exercised without touching disk or the network.
func renderIssuancesXLSX(
	template []byte,
	issuances []data.Issuance,
	from, to *time.Time,
	now time.Time,
) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(template))
	if err != nil {
		return nil, fmt.Errorf("renderIssuancesXLSX: open template: %w", err)
	}
	defer func() { _ = f.Close() }()

	loc, err := time.LoadLocation(exportTimezone)
	if err != nil {
		loc = time.UTC
	}

	if err := writeRedemptionFeeHeader(f); err != nil {
		return nil, err
	}

	// The title date is the moment the export is produced.
	if err := f.SetCellValue(issuanceExportSheet, issuanceExportTitleDateCell, calendarDate(now, loc)); err != nil {
		return nil, fmt.Errorf("renderIssuancesXLSX: set title date: %w", err)
	}

	for index, issuance := range issuances {
		row := issuanceExportFirstDataRow + index
		if err := writeIssuanceRow(f, row, issuance, from, to, loc); err != nil {
			return nil, err
		}
	}

	// Drop cached formula results so Excel/LibreOffice recomputes the summary block
	// (columns T:AH) against the freshly written data when the file is opened.
	if err := f.UpdateLinkedValue(); err != nil {
		return nil, fmt.Errorf("renderIssuancesXLSX: refresh formulas: %w", err)
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("renderIssuancesXLSX: write buffer: %w", err)
	}

	return buffer.Bytes(), nil
}

// writeRedemptionFeeHeader adds the TAXA RESGATE column the MODELO does not have, styled
// like the headers beside it so the sheet still reads as one table.
func writeRedemptionFeeHeader(f *excelize.File) error {
	cell := fmt.Sprintf("%s%d", issuanceExportRedemptionFeeColumn, issuanceExportHeaderRow)
	if err := f.SetCellValue(issuanceExportSheet, cell, issuanceExportRedemptionFeeHeader); err != nil {
		return fmt.Errorf("writeRedemptionFeeHeader: set %s: %w", cell, err)
	}

	// Borrowed from TAXAS: the same header styling, and below it the same currency format,
	// so the amounts line up with the money column they sit next to.
	source := fmt.Sprintf("%s%d", issuanceExportTaxesColumn, issuanceExportHeaderRow)
	style, err := f.GetCellStyle(issuanceExportSheet, source)
	if err != nil {
		return fmt.Errorf("writeRedemptionFeeHeader: read style of %s: %w", source, err)
	}
	if err := f.SetCellStyle(issuanceExportSheet, cell, cell, style); err != nil {
		return fmt.Errorf("writeRedemptionFeeHeader: style %s: %w", cell, err)
	}

	// The data rows are styled in one go rather than per issuance, so a sheet with three
	// issuances still shows the column running the same length as the ones beside it.
	first := fmt.Sprintf("%s%d", issuanceExportTaxesColumn, issuanceExportFirstDataRow)
	dataStyle, err := f.GetCellStyle(issuanceExportSheet, first)
	if err != nil {
		return fmt.Errorf("writeRedemptionFeeHeader: read style of %s: %w", first, err)
	}
	from := fmt.Sprintf("%s%d", issuanceExportRedemptionFeeColumn, issuanceExportFirstDataRow)
	to := fmt.Sprintf("%s%d", issuanceExportRedemptionFeeColumn, issuanceExportLastStyledRow)
	if err := f.SetCellStyle(issuanceExportSheet, from, to, dataStyle); err != nil {
		return fmt.Errorf("writeRedemptionFeeHeader: style %s:%s: %w", from, to, err)
	}

	// Wide enough for the header, which is longer than any amount it holds.
	if err := f.SetColWidth(issuanceExportSheet, issuanceExportRedemptionFeeColumn, issuanceExportRedemptionFeeColumn, 16); err != nil {
		return fmt.Errorf("writeRedemptionFeeHeader: set column width: %w", err)
	}

	return nil
}

// writeIssuanceRow fills a single data row (columns B:Q) for one issuance.
func writeIssuanceRow(
	f *excelize.File,
	row int,
	issuance data.Issuance,
	from, to *time.Time,
	loc *time.Location,
) error {
	set := func(column string, value any) error {
		if err := f.SetCellValue(issuanceExportSheet, fmt.Sprintf("%s%d", column, row), value); err != nil {
			return fmt.Errorf("writeIssuanceRow: set %s%d: %w", column, row, err)
		}
		return nil
	}

	// B COMPANHIA - loyalty program name (matches the AZUL/LATAM/SMILES summary labels).
	if issuance.LoyaltyProgram != nil {
		if err := set("B", issuance.LoyaltyProgram.Name); err != nil {
			return err
		}
	}
	// C LOCALIZADOR
	if err := set("C", issuance.ProgramLocator); err != nil {
		return err
	}
	// D DATA COMPRA - no dedicated purchase date exists; the registration date is the
	// closest equivalent and is what the operators treat as the purchase day.
	if err := set("D", calendarDate(issuance.CreatedAt, loc)); err != nil {
		return err
	}
	// E DATA VOO - departure of the outbound flight, when present.
	if flightDate, ok := outboundFlightDate(issuance, loc); ok {
		if err := set("E", flightDate); err != nil {
			return err
		}
	}
	// F NOME PASSAGEIRO
	if issuance.MainPassengerName != nil {
		if err := set("F", *issuance.MainPassengerName); err != nil {
			return err
		}
	}
	// G ORIGEM / H DESTINO
	if issuance.OriginIATACode != nil {
		if err := set("G", *issuance.OriginIATACode); err != nil {
			return err
		}
	}
	if issuance.DestinationIATACode != nil {
		if err := set("H", *issuance.DestinationIATACode); err != nil {
			return err
		}
	}
	points, taxes := signedIssuanceAmounts(issuance, from, to)
	// I PONTOS - the column is formatted as thousands (numFmt "0.000"), which is what the
	// template's L=ROUNDUP(I*J+K,2) total relies on, so points are written divided by 1000.
	// Negative when a cancellation gave miles back inside the exported period, which is
	// what makes the totals below net out instead of counting the booking twice.
	if err := set("I", float64(points)/1000); err != nil {
		return err
	}
	// J VLR. MILHEIRO - value_of_the_thousand is stored in centavos. Always positive: it is
	// a rate, and the sign of the line is carried by the points and the taxes.
	if err := set("J", centsToReais(issuance.ValueOfTheThousand)); err != nil {
		return err
	}
	// K TAXAS - centavos, only written when present (blank == no taxes on the manual sheet).
	if taxes != nil {
		if err := set("K", centsToReais(*taxes)); err != nil {
			return err
		}
	}
	// L VLR. TOTAL is left to the template's own currency formula (I*J+K); its inputs are now
	// all set, so Excel recomputes it on open (see UpdateLinkedValue in renderIssuancesXLSX).
	// M CONTA
	if err := set("M", accountLabel(issuance)); err != nil {
		return err
	}
	// N OBS.
	if issuance.Observation != nil {
		if err := set("N", *issuance.Observation); err != nil {
			return err
		}
	}
	// O EMISSOR
	if issuance.Issuer != nil {
		if err := set("O", issuance.Issuer.Name); err != nil {
			return err
		}
	}
	// P AGÊNCIA
	if issuance.Agency != nil {
		if err := set("P", issuance.Agency.Name); err != nil {
			return err
		}
	}
	// Q TAXA RESGATE - the airline fee the exempt account (Azul Diamante Unique) did not
	// pay, in centavos. Blank on every ordinary account, where nothing was saved and
	// nothing is recorded. It follows the same period rule as the points, so a booking
	// cancelled inside the period gives its saving back instead of claiming one that was
	// undone.
	if fee := signedRedemptionFee(issuance, from, to); fee != nil {
		if err := set(issuanceExportRedemptionFeeColumn, centsToReais(*fee)); err != nil {
			return err
		}
	}

	return nil
}

// signedIssuanceAmounts decides what an issuance contributes to the exported period, and
// with which sign. It is the same rule the supplier balance applies
// (handlers.aggregateSupplierBalances): miles issued inside the period count positive,
// miles a cancellation gave back inside the period count negative, and a row can be on
// both sides - a booking issued and cancelled on the same day nets to zero, exactly as it
// does on the sheet being reconciled.
//
// A cancellation-only row never issued anything (data.Issuance.CancellationOnly), so only
// its refund counts; without this it would pair a phantom emission with its own refund.
//
// Taxes come back nil when neither side has an amount, so the cell stays blank rather than
// claiming a zero the manual sheet never wrote.
func signedIssuanceAmounts(issuance data.Issuance, from, to *time.Time) (int, *int) {
	issued := !issuance.CancellationOnly && inPeriod(&issuance.CreatedAt, from, to)
	refunded := inPeriod(issuance.CancelledAt, from, to)

	points := 0
	if issued {
		points += issuance.Points
	}
	if refunded && issuance.RefundedPoints != nil {
		points -= *issuance.RefundedPoints
	}

	var taxes *int
	if issued && issuance.Taxes != nil {
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

// signedRedemptionFee is signedIssuanceAmounts' rule applied to the saved redemption fee:
// counted when the booking was made inside the period, given back when the cancellation
// fell inside it. Nil - a blank cell - whenever the account is not exempt or neither side
// falls in the period, because zero would claim a saving of nothing.
func signedRedemptionFee(issuance data.Issuance, from, to *time.Time) *int {
	if issuance.RedemptionFeeSavedCents == nil {
		return nil
	}

	total := 0
	counted := false
	if !issuance.CancellationOnly && inPeriod(&issuance.CreatedAt, from, to) {
		total += *issuance.RedemptionFeeSavedCents
		counted = true
	}
	if inPeriod(issuance.CancelledAt, from, to) {
		total -= *issuance.RedemptionFeeSavedCents
		counted = true
	}
	if !counted {
		return nil
	}

	return &total
}

// inPeriod reports whether an instant falls inside the half-open [from, to) interval the
// listing was filtered by. A nil bound is unbounded on that side; a nil instant is never in
// range, which is what makes a never-cancelled issuance skip the refund branch above.
// Mirrors handlers.inRange, duplicated rather than imported so this package keeps its one
// dependency on data and none on the HTTP layer.
func inPeriod(instant, from, to *time.Time) bool {
	if instant == nil {
		return false
	}
	if from != nil && instant.Before(*from) {
		return false
	}
	if to != nil && !instant.Before(*to) {
		return false
	}
	return true
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
