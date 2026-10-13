package sale

import "testing"

// PointsValueCents rounds up to the cent because the export spreadsheet applies
// ROUNDUP to the same figure. A truncating version disagrees with the sheet on
// every issuance whose points do not divide evenly by a thousand, which is most
// of them - so the rounding is the behaviour worth pinning, not an accident.
func TestPointsValueCents(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		points             int
		valueOfTheThousand int
		want               int
	}{
		{"exact thousands do not round", 10_000, 2_000, 20_000},
		{"a remainder rounds up, never down", 10_001, 2_000, 20_002},
		{"one cent short still rounds to the full cent", 1, 1, 1},
		{"no points is worth nothing", 0, 2_000, 0},
		{"no price is worth nothing", 10_000, 0, 0},
		// Only reachable through bad data, but the rounding branch is guarded by
		// total <= 0 and a negative that rounded "up" would round away from zero.
		{"a negative product truncates instead of rounding away from zero", -10_001, 2_000, -20_002},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := PointsValueCents(tc.points, tc.valueOfTheThousand); got != tc.want {
				t.Fatalf("PointsValueCents(%d, %d) = %d, want %d",
					tc.points, tc.valueOfTheThousand, got, tc.want)
			}
		})
	}
}

// The two rules that are easy to get wrong: a cancelled sale is owed nothing but
// may still owe money back, and a live sale worth zero is not settled - it is an
// issuance somebody still has to finish filling in.
func TestBalanceAndSettled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		amount      Amount
		paidCents   int
		wantBalance int
		wantSettled bool
	}{
		{
			name:        "untouched sale owes its full price",
			amount:      Amount{Cents: 50_000},
			wantBalance: 50_000,
		},
		{
			name:        "partly paid owes the rest",
			amount:      Amount{Cents: 50_000},
			paidCents:   20_000,
			wantBalance: 30_000,
		},
		{
			name:        "paid in full is settled",
			amount:      Amount{Cents: 50_000},
			paidCents:   50_000,
			wantSettled: true,
		},
		{
			name:        "overpaid owes money back",
			amount:      Amount{Cents: 50_000},
			paidCents:   60_000,
			wantBalance: -10_000,
		},
		{
			name:        "cancelled and never paid is settled, not owing its price",
			amount:      Amount{Cents: 50_000, Cancelled: true},
			wantSettled: true,
		},
		{
			name:        "cancelled after paying owes the money back",
			amount:      Amount{Cents: 50_000, Cancelled: true},
			paidCents:   50_000,
			wantBalance: -50_000,
		},
		{
			name:        "cancelled and refunded is settled",
			amount:      Amount{Cents: 50_000, Cancelled: true},
			paidCents:   0,
			wantSettled: true,
		},
		{
			name: "a live sale worth nothing is not settled - its issuance is unfinished",
			// Zero by arithmetic rather than by anyone having done anything, which is
			// the row the ?paid=false filter most needs to keep showing.
			amount:      Amount{Cents: 0},
			wantSettled: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := BalanceCents(tc.amount, tc.paidCents); got != tc.wantBalance {
				t.Fatalf("BalanceCents(%+v, %d) = %d, want %d",
					tc.amount, tc.paidCents, got, tc.wantBalance)
			}
			if got := Settled(tc.amount, tc.paidCents); got != tc.wantSettled {
				t.Fatalf("Settled(%+v, %d) = %v, want %v",
					tc.amount, tc.paidCents, got, tc.wantSettled)
			}
		})
	}
}

func TestAdjustmentClientCents(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		points, rate, taxes int
		passed              bool
		want                int
	}{
		{"charge passed on with taxes", 10000, 2000, 8000, true, 28000},
		{"charge passed on, taxes absorbed", 10000, 2000, 8000, false, 20000},
		{"absorbed entirely", 10000, 0, 8000, false, 0},
		// 1.001 points at 15,00 is 15,015 - a charge rounds up to the agency's favour.
		{"charge rounds up", 1001, 1500, 0, true, 1502},
		// The same 1.001 points given back rounds up to the client's favour.
		{"refund rounds up in magnitude", -1001, 1500, 0, true, -1502},
		{"refund of taxes only", 0, 0, -5000, true, -5000},
		{"refund kept by the agency", -5000, 0, -5000, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := AdjustmentClientCents(tc.points, tc.rate, tc.taxes, tc.passed); got != tc.want {
				t.Errorf("AdjustmentClientCents(%d, %d, %d, %v) = %d, want %d",
					tc.points, tc.rate, tc.taxes, tc.passed, got, tc.want)
			}
		})
	}
}
