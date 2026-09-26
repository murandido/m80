package azulsync

import (
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
)

func storedLine(onDay string, points, dayBalance int) data.AzulStatementLine {
	return data.AzulStatementLine{OccurredOn: day(onDay), Points: points, DayBalance: &dayBalance}
}

func TestClosingBalancesChainsAcrossResponseSeams(t *testing.T) {
	t.Parallel()
	// The production shape: two responses of one split window, each computed backwards from the
	// live balance (518217), so the first block's last day wrongly reads 518217 too and the
	// airline's figures jump at the seam between 09-02 and 09-03.
	rows := []data.AzulStatementLine{
		storedLine("2026-09-01", -226000, 744217), // first block: 09-01 and 09-02
		storedLine("2026-09-02", 0, 518217),
		storedLine("2026-09-03", 4000, 29533), // second block starts here
		storedLine("2026-09-03", 8316, 29533),
		storedLine("2026-09-04", 754388, 783921),
		storedLine("2026-09-05", -265704, 518217), // latest day: the absolute figure
	}

	got := ClosingBalances(rows)
	want := map[string]int{
		"2026-09-05": 518217,
		"2026-09-04": 518217 + 265704,
		"2026-09-03": 518217 + 265704 - 754388,
		"2026-09-02": 518217 + 265704 - 754388 - 12316,
		"2026-09-01": 518217 + 265704 - 754388 - 12316 - 0, // 09-02 moved nothing
	}
	for day, balance := range want {
		if got[day] != balance {
			t.Errorf("closing on %s = %d, want %d", day, got[day], balance)
		}
	}
	// Every day now closes: previous closing + the day's net == the day's closing.
	if got["2026-09-02"]+12316 != got["2026-09-03"] {
		t.Errorf("the seam still does not close: %d + 12316 != %d", got["2026-09-02"], got["2026-09-03"])
	}
}

func TestClosingBalancesWithoutAnchor(t *testing.T) {
	t.Parallel()
	if got := ClosingBalances(nil); got != nil {
		t.Errorf("no rows gave %v, want nil", got)
	}
	unbalanced := []data.AzulStatementLine{{OccurredOn: day("2026-09-05"), Points: -1000}}
	if got := ClosingBalances(unbalanced); got != nil {
		t.Errorf("latest day without a balance gave %v, want nil", got)
	}
}
