package export

import (
	"bytes"
	_ "embed"
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/xuri/excelize/v2"
)

// The legacy export is the "EMISSÕES GERAIS" sheet the team used before the control-sheet
// layout: one table for everyone, filled into a copy of the MODELO spreadsheet so its styling
// and its summary formulas (T:AH) come for free. Some operators still reconcile against it,
// so it is offered beside the new layout rather than replaced by it.
//
// The MODELO used to be downloaded from Google Sheets at runtime and cached on disk. It is
// embedded now: the export no longer depends on a third party being reachable from the
// production machine, and a change to the template goes through a commit like any other.
//
//go:embed templates/emissoes-gerais-modelo.xlsx
var legacyIssuanceTemplate []byte

// Layout of the MODELO template.
const (
	legacyIssuanceSheet = "MODELO"
	// The data table lives in B:P; row 1 is the title, row 2 the column headers, so the
	// first issuance goes on row 3. The summary block (columns T:AH) sums the fixed range
	// $3:$152 (150 issuances); rows beyond that still receive their data but are not picked
	// up by those totals - acceptable for the day/period-scoped filters this export uses.
	legacyIssuanceFirstDataRow = 3
	// R2 feeds the A1 title formula ="EMISSÕES GERAIS "&TEXT(R2,"dd/mm/yyyy").
	legacyIssuanceTitleDateCell = "R2"
	// Q is the first free column after the MODELO's own table (B:P) and stops short of the
	// summary block (T:AH), so the redemption fee can be added without moving a single
	// formula. It does not exist in the MODELO, which is why the header is written on every
	// render rather than lifted from the template.
	legacyIssuanceRedemptionFeeColumn = "Q"
	legacyIssuanceRedemptionFeeHeader = "TAXA RESGATE"
	// The header row and the taxes column, whose styling the new column borrows so it
	// looks like it was always part of the sheet.
	legacyIssuanceHeaderRow     = 2
	legacyIssuanceTaxesColumn   = "K"
	legacyIssuanceLastStyledRow = 152
)

// BuildLegacyIssuancesXLSX renders the given issuances into a copy of the MODELO template,
// preserving its styling and summary formulas, and returns the resulting .xlsx bytes.
func BuildLegacyIssuancesXLSX(issuances []data.Issuance) ([]byte, error) {
	return renderLegacyIssuancesXLSX(legacyIssuanceTemplate, issuances, time.Now())
}

// renderLegacyIssuancesXLSX holds the pure rendering logic: it takes the template bytes and
// the reference time explicitly so it can be exercised without depending on the clock.
func renderLegacyIssuancesXLSX(template []byte, issuances []data.Issuance, now time.Time) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(template))
	if err != nil {
		return nil, fmt.Errorf("renderLegacyIssuancesXLSX: open template: %w", err)
	}
	defer func() { _ = f.Close() }()

	loc, err := time.LoadLocation(exportTimezone)
	if err != nil {
		loc = time.UTC
	}

	if err := writeLegacyRedemptionFeeHeader(f); err != nil {
		return nil, err
	}

	// The title date is the moment the export is produced.
	if err := f.SetCellValue(legacyIssuanceSheet, legacyIssuanceTitleDateCell, calendarDate(now, loc)); err != nil {
		return nil, fmt.Errorf("renderLegacyIssuancesXLSX: set title date: %w", err)
	}

	for index, issuance := range issuances {
		row := legacyIssuanceFirstDataRow + index
		if err := writeLegacyIssuanceRow(f, row, issuance, loc); err != nil {
			return nil, err
		}
	}

	// Drop cached formula results so Excel/LibreOffice recomputes the summary block
	// (columns T:AH) against the freshly written data when the file is opened.
	if err := f.UpdateLinkedValue(); err != nil {
		return nil, fmt.Errorf("renderLegacyIssuancesXLSX: refresh formulas: %w", err)
	}

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("renderLegacyIssuancesXLSX: write buffer: %w", err)
	}

	return buffer.Bytes(), nil
}

// writeLegacyRedemptionFeeHeader adds the TAXA RESGATE column the MODELO does not have,
// styled like the headers beside it so the sheet still reads as one table.
func writeLegacyRedemptionFeeHeader(f *excelize.File) error {
	cell := fmt.Sprintf("%s%d", legacyIssuanceRedemptionFeeColumn, legacyIssuanceHeaderRow)
	if err := f.SetCellValue(legacyIssuanceSheet, cell, legacyIssuanceRedemptionFeeHeader); err != nil {
		return fmt.Errorf("writeLegacyRedemptionFeeHeader: set %s: %w", cell, err)
	}

	// Borrowed from TAXAS: the same header styling, and below it the same currency format,
	// so the amounts line up with the money column they sit next to.
	source := fmt.Sprintf("%s%d", legacyIssuanceTaxesColumn, legacyIssuanceHeaderRow)
	style, err := f.GetCellStyle(legacyIssuanceSheet, source)
	if err != nil {
		return fmt.Errorf("writeLegacyRedemptionFeeHeader: read style of %s: %w", source, err)
	}
	if err := f.SetCellStyle(legacyIssuanceSheet, cell, cell, style); err != nil {
		return fmt.Errorf("writeLegacyRedemptionFeeHeader: style %s: %w", cell, err)
	}

	// The data rows are styled in one go rather than per issuance, so a sheet with three
	// issuances still shows the column running the same length as the ones beside it.
	first := fmt.Sprintf("%s%d", legacyIssuanceTaxesColumn, legacyIssuanceFirstDataRow)
	dataStyle, err := f.GetCellStyle(legacyIssuanceSheet, first)
	if err != nil {
		return fmt.Errorf("writeLegacyRedemptionFeeHeader: read style of %s: %w", first, err)
	}
	from := fmt.Sprintf("%s%d", legacyIssuanceRedemptionFeeColumn, legacyIssuanceFirstDataRow)
	to := fmt.Sprintf("%s%d", legacyIssuanceRedemptionFeeColumn, legacyIssuanceLastStyledRow)
	if err := f.SetCellStyle(legacyIssuanceSheet, from, to, dataStyle); err != nil {
		return fmt.Errorf("writeLegacyRedemptionFeeHeader: style %s:%s: %w", from, to, err)
	}

	// Wide enough for the header, which is longer than any amount it holds.
	if err := f.SetColWidth(legacyIssuanceSheet, legacyIssuanceRedemptionFeeColumn, legacyIssuanceRedemptionFeeColumn, 16); err != nil {
		return fmt.Errorf("writeLegacyRedemptionFeeHeader: set column width: %w", err)
	}

	return nil
}

// writeLegacyIssuanceRow fills a single data row (columns B:Q) for one issuance.
//
// The sign of each amount follows the same rule as the control sheet (signedIssuanceAmounts),
// not the period rule this sheet used before: the two layouts are exports of the same list,
// and a cancelled booking that nets out on one of them has to net out on the other.
func writeLegacyIssuanceRow(f *excelize.File, row int, issuance data.Issuance, loc *time.Location) error {
	set := func(column string, value any) error {
		if err := f.SetCellValue(legacyIssuanceSheet, fmt.Sprintf("%s%d", column, row), value); err != nil {
			return fmt.Errorf("writeLegacyIssuanceRow: set %s%d: %w", column, row, err)
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
	points, taxes := signedIssuanceAmounts(issuance, loc)
	// I PONTOS - the column is formatted as thousands (numFmt "0.000"), which is what the
	// template's L=ROUNDUP(I*J+K,2) total relies on, so points are written divided by 1000.
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
	// all set, so Excel recomputes it on open (see UpdateLinkedValue above).
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
	// pay. Blank on every ordinary account, where nothing was saved.
	if fee := signedRedemptionFee(issuance, loc); fee != nil {
		if err := set(legacyIssuanceRedemptionFeeColumn, centsToReais(*fee)); err != nil {
			return err
		}
	}

	return nil
}
