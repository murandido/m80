package sale

import (
	"slices"
	"testing"
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
