package export

import (
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
)

// An old booking altered today is on today's list for the alteration alone: the sheet writes
// the alteration's line and not the booking a second time.
func TestExportWritesOnlyTheAlterationOfAnOldBooking(t *testing.T) {
	t.Parallel()

	issuance := sampleIssuance("Smiles", "OLD123", "CONTA", "Ana")
	issuance.Adjustments = []data.IssuanceAdjustment{{
		OccurredOn:          time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC),
		Points:              -4000,
		Taxes:               -2000,
		ValueOfTheThousand:  2000,
		TaxesPassedToClient: true,
		Reason:              "voo trocado",
		CreatedByID:         "bia-id",
		CreatedBy:           &data.User{Name: "Bia"},
	}}
	from := time.Date(2026, 7, 25, 3, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	loc, _ := time.LoadLocation(exportTimezone)

	rows := exportRowsFor([]data.Issuance{issuance}, ExportWindow{From: &from, To: &to}, loc)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want only the alteration's", len(rows))
	}
	row := rows[0]
	if row.milheiros != -4 || row.taxes == nil || *row.taxes != -20 || row.issuer != "Bia" || !row.cancelled {
		t.Errorf("alteration row = %+v, want -4 milheiros, -20 taxes, by Bia, tinted", row)
	}
	if !strings.HasPrefix(row.observation, "ALTERAÇÃO reembolso devolvido ao cliente") {
		t.Errorf("observation = %q", row.observation)
	}

	// Somebody else's flow does not get Bia's alteration.
	rows = exportRowsFor([]data.Issuance{issuance}, ExportWindow{From: &from, To: &to, Issuers: []string{"other"}}, loc)
	if len(rows) != 0 {
		t.Errorf("got %d rows for another issuer, want none", len(rows))
	}

	// With no window (a search), the booking and every alteration are written.
	rows = exportRowsFor([]data.Issuance{issuance}, ExportWindow{}, loc)
	if len(rows) != 2 {
		t.Errorf("got %d rows with no window, want booking + alteration", len(rows))
	}
}
