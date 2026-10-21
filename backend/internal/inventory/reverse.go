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
	return reverseAgainst(existing, points, on, "")
}

// reverseAgainst is the body Allocate calls inside its pass, where `existing` is what the
// issuance is holding at that point in the timeline rather than its final state.
//
// adjustmentID is set when the return is an alteration giving miles back
// (data.IssuanceAdjustment with negative points), and empty for a cancellation.
//
// The two are tagged differently because the closing prices a slice by its tag. A
// cancellation undoes what was consumed, so each return inherits the tag of the slice it
// gives back: points an alteration charged return at that alteration's rate, and the
// booking's at the booking's. That is what keeps a full cancellation netting revenue to
// zero when the booking and an alteration were sold at different rates, or the alteration
// was absorbed. An alteration's refund is priced at its own rate, because what the client
// gets back is part of that alteration. So it carries its own tag, and it draws from the
// booking's own points first: an airline returning miles on a change is making the booking
// cheaper.
func reverseAgainst(existing []Allocation, points int, on time.Time, adjustmentID string) []Allocation {
	if points <= 0 {
		return nil
	}

	// A refund-adjustment slice only ever gives points back, so a tag with no positive
	// slice is one of those, and it nets against the booking rather than standing as a
	// position of its own. A tag the alteration charged under always has its consumption
	// slices in `existing`, ahead of any return.
	consumedUnder := map[string]bool{}
	for _, allocation := range existing {
		if allocation.Points > 0 {
			consumedUnder[allocation.AdjustmentID] = true
		}
	}
	nettingTag := func(tag string) string {
		if tag != "" && !consumedUnder[tag] {
			return ""
		}
		return tag
	}

	// Net each (batch, tag) out to what it is currently owed, keeping first-consumption
	// order so "most recently consumed" means what it says. Pending slices (empty BatchID)
	// net together under their own key and, being last in, come back first.
	type positionKey struct {
		batchID string
		tag     string
	}
	type position struct {
		positionKey
		points    int
		costCents int
	}
	positions := make([]position, 0, len(existing))
	index := make(map[positionKey]int, len(existing))

	for _, allocation := range existing {
		key := positionKey{batchID: allocation.BatchID, tag: nettingTag(allocation.AdjustmentID)}
		at, seen := index[key]
		if !seen {
			at = len(positions)
			index[key] = at
			positions = append(positions, position{positionKey: key})
		}
		positions[at].points += allocation.Points
		positions[at].costCents += allocation.CostCents
	}

	// The order positions are given back in: newest first. An alteration's refund takes
	// the booking's positions before any other.
	order := make([]int, 0, len(positions))
	for at := len(positions) - 1; at >= 0; at-- {
		if adjustmentID == "" || positions[at].tag == "" {
			order = append(order, at)
		}
	}
	if adjustmentID != "" {
		for at := len(positions) - 1; at >= 0; at-- {
			if positions[at].tag != "" {
				order = append(order, at)
			}
		}
	}

	reversals := make([]Allocation, 0, len(positions))
	remaining := points

	for _, at := range order {
		if remaining <= 0 {
			break
		}
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

		tag := current.tag
		if adjustmentID != "" {
			tag = adjustmentID
		}
		reversals = append(reversals, Allocation{
			IssuanceID:   issuanceIDOf(existing),
			BatchID:      current.batchID,
			OccurredOn:   on,
			Points:       -giving,
			CostCents:    -cost,
			AdjustmentID: tag,
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
