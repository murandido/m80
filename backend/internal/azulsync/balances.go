package azulsync

import (
	"sort"

	"github.com/murandido/m-80-backend/internal/data"
)

// ClosingBalances works out what the account held at the end of each stored day, keyed by the
// day as "2006-01-02".
//
// The airline's own per-day figure (the stored DayBalance) cannot be shown as it is: the legacy
// endpoint computes it backwards from the account's CURRENT balance using only the movements of
// that one response. Every response - each half of a split window, and each earlier round's
// window - therefore ends on the balance of the moment it was read, and the figures jump at every
// seam between two responses (measured in production: each block's last day read exactly the
// live balance). Only the latest day of the latest round is an absolute figure, and that is the
// one the sync validates against points/resume before storing anything.
//
// So this anchors on that latest day and walks backwards, subtracting each day's net - the same
// arithmetic as smilessync.fillDayBalances, run in reverse. It is done at read time rather than
// in the sync so the rows older than the window, stored by earlier rounds, come out right too.
// As with Smiles, a figure derived from the movements always agrees with them: it cannot flag a
// missing movement, it would just be off by that amount from the gap backwards.
//
// Returns nil when there is nothing to anchor on (no rows, or a latest day with no balance).
func ClosingBalances(rows []data.AzulStatementLine) map[string]int {
	const dayLayout = "2006-01-02"

	net := make(map[string]int)
	days := make([]string, 0)
	latestDay := ""
	var anchor *int
	for _, row := range rows {
		day := row.OccurredOn.Format(dayLayout)
		if _, seen := net[day]; !seen {
			days = append(days, day)
		}
		net[day] += row.Points

		if day > latestDay {
			latestDay, anchor = day, row.DayBalance
		} else if day == latestDay && anchor == nil {
			anchor = row.DayBalance
		}
	}
	if anchor == nil {
		return nil
	}
	sort.Strings(days)

	closings := make(map[string]int, len(days))
	closing := *anchor
	for i := len(days) - 1; i >= 0; i-- {
		closings[days[i]] = closing
		closing -= net[days[i]]
	}
	return closings
}
