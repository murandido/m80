package sale

import (
	"maps"
	"slices"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
)

// The shape the payments screen promises: settled sales, at most one partly paid, the rest
// open - and nothing lost or invented along the way, since takes plus leftover is the pool.
func TestAllocateFIFO(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		balances     []int
		pool         int
		wantTakes    []int
		wantLeftover int
	}{
		{"less than the oldest leaves it partly paid", []int{1000, 500}, 400, []int{400, 0}, 0},
		{"settles in order and stops on a partial", []int{1000, 500, 300}, 1200, []int{1000, 200, 0}, 0},
		{"exactly the total settles everything", []int{1000, 500}, 1500, []int{1000, 500}, 0},
		{"more than the total leaves a surplus", []int{1000, 500}, 2000, []int{1000, 500}, 500},
		{"settled and overpaid sales take nothing", []int{0, -200, 300}, 500, []int{0, 0, 300}, 200},
		{"no open sales keeps it all", nil, 700, []int{}, 700},
		{"an empty pool changes nothing", []int{1000}, 0, []int{0}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			takes, leftover := AllocateFIFO(tc.balances, tc.pool)
			if !slices.Equal(takes, tc.wantTakes) || leftover != tc.wantLeftover {
				t.Fatalf("AllocateFIFO(%v, %d) = %v, %d; want %v, %d",
					tc.balances, tc.pool, takes, leftover, tc.wantTakes, tc.wantLeftover)
			}
		})
	}
}

// A sale's surplus is the client's money: it leaves its sale whole, covers the open sales
// before any lump sum does, and what nobody takes stays as credit (the sum of a surplus's
// lines is minus that credit).
func TestPlanSpread(t *testing.T) {
	t.Parallel()

	cheaper := surplusSale{ID: "cheaper", Cents: 6990}
	payment := data.ClientPayment{ID: "payment", AmountCents: 10000}

	cases := []struct {
		name      string
		open      []openSale
		surpluses []surplusSale
		payments  []data.ClientPayment
		want      map[spreadKey]int
	}{
		{
			name:      "a surplus settles the sale that went up by as much",
			open:      []openSale{{ID: "dearer", Balance: 6990}},
			surpluses: []surplusSale{cheaper},
			want: map[spreadKey]int{
				{creditFromSaleID: "cheaper", saleID: "cheaper"}: -6990,
				{creditFromSaleID: "cheaper", saleID: "dearer"}:  6990,
			},
		},
		{
			name:      "a surplus with nothing open is all credit",
			surpluses: []surplusSale{cheaper},
			want: map[spreadKey]int{
				{creditFromSaleID: "cheaper", saleID: "cheaper"}: -6990,
			},
		},
		{
			name:      "a surplus is spent before the lump sums",
			open:      []openSale{{ID: "first", Balance: 5000}, {ID: "second", Balance: 8000}},
			surpluses: []surplusSale{cheaper},
			payments:  []data.ClientPayment{payment},
			want: map[spreadKey]int{
				{creditFromSaleID: "cheaper", saleID: "cheaper"}: -6990,
				{creditFromSaleID: "cheaper", saleID: "first"}:   5000,
				{creditFromSaleID: "cheaper", saleID: "second"}:  1990,
				{clientPaymentID: "payment", saleID: "second"}:   6010,
			},
		},
		{
			name:     "without surpluses the lump sums spread as before",
			open:     []openSale{{ID: "first", Balance: 5000}, {ID: "second", Balance: 8000}},
			payments: []data.ClientPayment{payment},
			want: map[spreadKey]int{
				{clientPaymentID: "payment", saleID: "first"}:  5000,
				{clientPaymentID: "payment", saleID: "second"}: 5000,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := planSpread(tc.open, tc.surpluses, tc.payments)
			if !maps.Equal(got, tc.want) {
				t.Fatalf("planSpread() = %v; want %v", got, tc.want)
			}
		})
	}
}
