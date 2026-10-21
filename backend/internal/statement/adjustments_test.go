package statement

import (
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// An altered ticket carries its extra debit on the same locator. The statement's total for
// that locator is the booking plus the charge, and so is what the panel expects: both the
// resolved link and the period comparison fold the alterations in.
func TestAlterationsRideOnTheBookingsLocator(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		issuance := seedLinkIssuance(t, tx, f, "ALT123", "2026-08-10", 50000)

		for _, points := range []int{12000, -3000} {
			mustCreate(t, tx, &data.IssuanceAdjustment{
				IssuanceID:  issuance.ID,
				OccurredOn:  day("2026-08-15"),
				Points:      points,
				Reason:      "remarcação",
				CreatedByID: f.user.ID,
			})
		}

		resolved, err := ResolveBookingLinks(tx, f.account.ID, []string{"ALT123"})
		if err != nil {
			t.Fatalf("ResolveBookingLinks: %v", err)
		}
		booking := resolved["ALT123"]
		if booking.RedeemedPoints() != 62000 || booking.RefundedPoints() != 3000 {
			t.Errorf("redeemed %d / refunded %d, want 62000 / 3000", booking.RedeemedPoints(), booking.RefundedPoints())
		}

		bookings, _, err := LoadPanel(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"))
		if err != nil {
			t.Fatalf("LoadPanel: %v", err)
		}
		if len(bookings) != 1 || bookings[0].Points != 62000 || bookings[0].RefundedPoints != 3000 {
			t.Errorf("panel bookings = %+v, want one of 62000 points with 3000 refunded", bookings)
		}

		statement := []Movement{
			{Locator: "ALT123", Kind: KindRedemption, Points: -50000, OccurredOn: day("2026-08-10")},
			{Locator: "ALT123", Kind: KindRedemption, Points: -12000, OccurredOn: day("2026-08-15")},
			{Locator: "ALT123", Kind: KindRefund, Points: 3000, OccurredOn: day("2026-08-15")},
		}
		result := Reconcile(statement, bookings, nil)
		for _, line := range result.Bookings {
			if line.Verdict != VerdictMatched {
				t.Errorf("booking line %+v, want matched", line)
			}
		}
		for _, line := range result.Refunds {
			if line.Verdict != VerdictMatched {
				t.Errorf("refund line %+v, want matched", line)
			}
		}
	})
}
