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
)

// exportTimezone matches the timezone the issuance list uses to resolve "today", so the
// dates written to the sheet are the operator's local calendar days, not UTC.
const exportTimezone = "America/Sao_Paulo"

// BuildIssuancesXLSX renders the given issuances into a copy of the MODELO template,
// preserving its styling and summary formulas, and returns the resulting .xlsx bytes.
func BuildIssuancesXLSX(issuances []data.Issuance) ([]byte, error) {
	template, err := loadIssuanceTemplate()
	if err != nil {
		return nil, err
	}
	return renderIssuancesXLSX(template, issuances, time.Now())
}

// renderIssuancesXLSX holds the pure rendering logic: it takes the template bytes and the
// reference time explicitly so it can be exercised without touching disk or the network.
func renderIssuancesXLSX(template []byte, issuances []data.Issuance, now time.Time) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(template))
	if err != nil {
		return nil, fmt.Errorf("renderIssuancesXLSX: open template: %w", err)
	}
	defer func() { _ = f.Close() }()

	loc, err := time.LoadLocation(exportTimezone)
	if err != nil {
		loc = time.UTC
	}

	// The title date is the moment the export is produced.
	if err := f.SetCellValue(issuanceExportSheet, issuanceExportTitleDateCell, calendarDate(now, loc)); err != nil {
		return nil, fmt.Errorf("renderIssuancesXLSX: set title date: %w", err)
	}

	for index, issuance := range issuances {
		row := issuanceExportFirstDataRow + index
		if err := writeIssuanceRow(f, row, issuance, loc); err != nil {
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

// writeIssuanceRow fills a single data row (columns B:P) for one issuance.
func writeIssuanceRow(f *excelize.File, row int, issuance data.Issuance, loc *time.Location) error {
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
	// I PONTOS - the column is formatted as thousands (numFmt "0.000"), which is what the
	// template's L=ROUNDUP(I*J+K,2) total relies on, so points are written divided by 1000.
	if err := set("I", float64(issuance.Points)/1000); err != nil {
		return err
	}
	// J VLR. MILHEIRO - value_of_the_thousand is stored in centavos.
	if err := set("J", centsToReais(issuance.ValueOfTheThousand)); err != nil {
		return err
	}
	// K TAXAS - centavos, only written when present (blank == no taxes on the manual sheet).
	if issuance.Taxes != nil {
		if err := set("K", centsToReais(*issuance.Taxes)); err != nil {
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

	return nil
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
