package statement

import (
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
)

// The stored kind decides how a synced line reconciles. The reconcilable kinds must map to
// their statement.Kind counterparts; anything unrecognised must land on KindOther, which
// Reconcile counts in the net but never turns into a booking or a credit.
func TestSmilesLineKindMapping(t *testing.T) {
	t.Parallel()

	cases := map[string]Kind{
		data.SmilesLineRedemption: KindRedemption,
		data.SmilesLineRefund:     KindRefund,
		// A purchase needs a cost basis exactly like an accrual, so both reconcile the same
		// way; the split exists only for the page's label.
		data.SmilesLineAccrual:  KindAccrual,
		data.SmilesLinePurchase: KindAccrual,
		data.SmilesLineOther:    KindOther,
		"something-new":         KindOther,
		"":                      KindOther,
	}
	for stored, want := range cases {
		if got := smilesLineKind(stored); got != want {
			t.Errorf("smilesLineKind(%q) = %q, want %q", stored, got, want)
		}
	}
}

// A flight credit names the booking it came from. Reconciling on that locator would pair the
// credit with the very issuance it refers to and settle the booking twice, so credits reach
// Reconcile without one - while a refund keeps its locator, because matching a refund to its
// booking is the whole point.
func TestCreditsDropTheirLocatorAndRefundsKeepIt(t *testing.T) {
	t.Parallel()

	locator := "IOTFQF"
	cases := []struct {
		kind string
		want string
	}{
		{data.SmilesLineRedemption, locator},
		{data.SmilesLineRefund, locator},
		{data.SmilesLineAccrual, ""},
		{data.SmilesLinePurchase, ""},
	}
	for _, c := range cases {
		row := data.SmilesStatementLine{Kind: c.kind, Locator: &locator}
		if got := smilesReconcilableLocator(row); got != c.want {
			t.Errorf("smilesReconcilableLocator(%s) = %q, want %q", c.kind, got, c.want)
		}
	}
}

// The estorno of a cancelled redemption arrives as a credit, not as a negative redemption,
// so the two rows coexist and the net is what reports the account's real movement. Netting
// them against each other here would hide both from the reconciliation.
func TestACancelledBookingKeepsBothItsRows(t *testing.T) {
	t.Parallel()

	movements := []Movement{
		{OccurredOn: day("2026-09-10"), Kind: KindRedemption, Locator: "IOTFQF", Points: -27400},
		{OccurredOn: day("2026-09-12"), Kind: KindRefund, Locator: "IOTFQF", Points: 27400},
	}
	result := Reconcile(movements, nil, nil)

	if result.StatementNetPoints != 0 {
		t.Errorf("net = %d, want 0 - the refund undoes the redemption", result.StatementNetPoints)
	}
	if len(result.Credits) != 0 {
		t.Errorf("got %d credits, want 0 - a refund is not miles to be priced", len(result.Credits))
	}
}
