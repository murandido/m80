package statement

import (
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
)

// The stored kind decides how a synced line reconciles. The three reconcilable kinds must map
// to their statement.Kind counterparts; expiration and anything unrecognised must land on the
// pass-through kinds that Reconcile counts in the net but never turns into a booking or credit
// - which is how point expiration stays out of the FIFO.
func TestAzulLineKindMapping(t *testing.T) {
	t.Parallel()

	cases := map[string]Kind{
		data.AzulLineRedemption: KindRedemption,
		data.AzulLineRefund:     KindRefund,
		data.AzulLineAccrual:    KindAccrual,
		data.AzulLineExpiration: KindExpiration,
		data.AzulLineOther:      KindOther,
		"something-new":         KindOther,
		"":                      KindOther,
	}
	for stored, want := range cases {
		if got := azulLineKind(stored); got != want {
			t.Errorf("azulLineKind(%q) = %q, want %q", stored, got, want)
		}
	}
}

// Only the three reconcilable kinds ever build a booking or credit; expiration and other are
// counted in the net so a balance check still sees them but never enter Reconcile's switch.
func TestExpirationCountsInNetButNotInCredits(t *testing.T) {
	t.Parallel()

	movements := []Movement{
		{OccurredOn: day("2026-09-10"), Kind: KindAccrual, Points: 5000, Description: "Livelo"},
		{OccurredOn: day("2026-09-08"), Kind: KindExpiration, Points: -1000},
	}
	result := Reconcile(movements, nil, nil)

	if result.StatementNetPoints != 4000 {
		t.Errorf("net = %d, want 4000 (accrual + expiration both counted)", result.StatementNetPoints)
	}
	if len(result.Credits) != 1 {
		t.Fatalf("got %d credits, want 1 (only the accrual; expiration is out of the FIFO)", len(result.Credits))
	}
	if result.Credits[0].Points != 5000 {
		t.Errorf("credit points = %d, want 5000", result.Credits[0].Points)
	}
}
