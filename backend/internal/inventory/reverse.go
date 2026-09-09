package inventory

import "time"

// Reverse gives points back to the batches that supplied them, most recently consumed
// first, and returns the negative slices that record it.
//
// Two things make this the right shape rather than simply deleting the original slices.
//
// The first is closed periods. A booking cancelled months later sits in a month that was
// already closed and settled between the agencies - reopening it would revise a figure
// somebody has already been paid on. So the original slices stay untouched and the return
// lands in the current period instead, dated by `on`, as an ordinary accounting reversal.
//
// The second is what the points are worth. The spreadsheet's workaround for an old
// cancellation was to book the refund as a fresh credit, which quietly made those points
// free and cheapened the whole remaining stock. Here they go back to the batch they came
// from, carrying the cost they had.
//
// `existing` is every slice already recorded for the issuance, consumption and earlier
// reversals alike, in consumption order. Passing the earlier reversals too is what makes
// repeated partial refunds add up correctly - the airline refunds one booking in
// instalments (MM8LVJ came back as 26.000 and then 11.600 points), and each call has to
// see what the previous one already gave back.
//
// Returning more than was consumed is not an error and not something to reject: the
// airline is the one that decides, and a refund that overshoots what we recorded means
// our figure was wrong. It gives back everything there is and stops there.
func Reverse(existing []Allocation, points int, on time.Time) []Allocation {
	return reverseAgainst(existing, points, on)
}

// reverseAgainst is the body Allocate calls inside its pass, where `existing` is what the
// issuance is holding at that point in the timeline rather than its final state.
func reverseAgainst(existing []Allocation, points int, on time.Time) []Allocation {
	if points <= 0 {
		return nil
	}

	// Net each batch out to what it is currently owed, keeping first-consumption order so
	// "most recently consumed" means what it says. Pending slices (empty BatchID) net
	// together under their own key and, being last in, come back first.
	type position struct {
		batchID   string
		points    int
		costCents int
	}
	positions := make([]position, 0, len(existing))
	index := make(map[string]int, len(existing))

	for _, allocation := range existing {
		at, seen := index[allocation.BatchID]
		if !seen {
			at = len(positions)
			index[allocation.BatchID] = at
			positions = append(positions, position{batchID: allocation.BatchID})
		}
		positions[at].points += allocation.Points
		positions[at].costCents += allocation.CostCents
	}

	reversals := make([]Allocation, 0, len(positions))
	remaining := points

	for at := len(positions) - 1; at >= 0 && remaining > 0; at-- {
		current := positions[at]
		if current.points <= 0 {
			continue
		}

		giving := remaining
		if giving > current.points {
			giving = current.points
		}

		// Giving the position back whole returns exactly what it was charged, so a full
		// cancellation always nets an issuance to zero however many batches it spanned
		// and however awkward the rate was. A partial one prorates within the position.
		cost := current.costCents
		if giving < current.points {
			cost = proratedCents(current.costCents, giving, current.points)
		}

		reversals = append(reversals, Allocation{
			IssuanceID: issuanceIDOf(existing),
			BatchID:    current.batchID,
			OccurredOn: on,
			Points:     -giving,
			CostCents:  -cost,
		})
		remaining -= giving
	}

	return reversals
}

// issuanceIDOf reads the issuance every slice in the set belongs to. The caller assembles
// the set per issuance, so an empty input simply has no owner to name.
func issuanceIDOf(allocations []Allocation) string {
	if len(allocations) == 0 {
		return ""
	}
	return allocations[0].IssuanceID
}
