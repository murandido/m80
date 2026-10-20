package statement

import (
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/businessday"
	"gorm.io/gorm"
)

// The statement's period is a run of São Paulo calendar days, and the panel side has to be
// read over the same days. Read as raw midnight-UTC instants the window ran three hours
// early on both ends: a booking at 22:00 on the period's last day was left out (and
// reported missing from the panel), and one at 22:00 on the day before its first day was
// pulled in.
func TestLoadPanelReadsTheStatementPeriodInSaoPauloDays(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		stamp := func(locator string, at time.Time) {
			issuance := seedLinkIssuance(t, tx, f, locator, "2026-01-01", 1000)
			if err := tx.Model(&issuance).UpdateColumn("created_at", at).Error; err != nil {
				t.Fatalf("stamping %s: %v", locator, err)
			}
		}
		stamp("BEFORE", time.Date(2026, 9, 30, 22, 0, 0, 0, businessday.Location))
		stamp("FIRST", time.Date(2026, 10, 1, 0, 30, 0, 0, businessday.Location))
		stamp("LAST", time.Date(2026, 10, 31, 22, 0, 0, 0, businessday.Location))
		stamp("AFTER", time.Date(2026, 11, 1, 0, 30, 0, 0, businessday.Location))

		from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
		bookings, _, err := LoadPanel(tx, f.account.ID, from, to)
		if err != nil {
			t.Fatalf("LoadPanel: %v", err)
		}

		got := map[string]bool{}
		for _, booking := range bookings {
			got[booking.Locator] = true
		}
		for locator, want := range map[string]bool{"BEFORE": false, "FIRST": true, "LAST": true, "AFTER": false} {
			if got[locator] != want {
				t.Errorf("%s in the October window = %v, want %v", locator, got[locator], want)
			}
		}
	})
}
