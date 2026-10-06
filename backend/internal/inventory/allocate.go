// Package inventory prices issuances against the batches of points that paid for them.
//
// It replaces the hand-kept "CONTROLE LIMINAR" spreadsheet, whose right-hand tabs did this
// by hand every week: walk the bookings in date order, charge each one to the oldest batch
// with points left, split it across two batches when it does not fit, and close the month
// with a profit per batch and a settlement between the agencies.
//
// This file is the pure half - slices in, slices out, no database. The costing is worth
// keeping testable on its own because it is the part that has to agree with a spreadsheet
// to the centavo.
package inventory

import (
	"sort"
	"time"
)

// Batch is one acquisition of points, as the queue sees it.
type Batch struct {
	ID         string
	AcquiredOn time.Time
	Points     int
	CostCents  int
	// Sequence breaks a tie between two batches acquired on the same day, so the queue
	// is stable rather than dependent on the order rows came back in.
	Sequence int
	// ConsumedPoints and ChargedCents are what this batch already gave out in periods
	// that are closed, and which a recompute may not revisit. The queue starts from
	// them, so a batch closed at half empty offers only the half it has left.
	//
	// They are net figures: a reversal that landed in a closed period gave points back,
	// and the batch offers those again. That is how an exhausted batch comes back to
	// life - at its own acquisition date, ahead of anything newer, because they are the
	// same points they always were.
	ConsumedPoints int
	ChargedCents   int
}

// Event is one thing happening to the stock on a day: a booking taking points, or a refund
// giving them back.
//
// The two are one type, in one ordered list, because they share a timeline. A refund on the
// 16th puts points back that a booking on the 18th can spend, and separating the two passes
// - allocate everything, then reverse - loses exactly that: the later booking finds no
// stock and is reported as a shortfall while the points are sitting in the account.
type Event struct {
	IssuanceID string
	OccurredOn time.Time
	// Points is positive when the booking takes stock and negative when a refund returns
	// it. A return goes back to the batches this issuance drew on, newest slice first.
	Points int
	// Sequence breaks a tie between two events on the same day. The panel has a created_at
	// timestamp for this, which the spreadsheet never had - so the panel will order
	// same-day bookings better than the spreadsheet did, and the first month closed here
	// will differ from the old one by a few centavos. That is the costing being more
	// correct, not less.
	Sequence int
	// WithoutBooking marks a refund for a booking the panel never had (a cancellation-only
	// row). There is no slice to give back to, so the points rejoin the queue at the batch
	// it is on that day instead - otherwise they would vanish from the costing while the
	// account, and the expected balance, has them back.
	WithoutBooking bool
}

// Allocation is one slice: this many of an issuance's points, paid for by this batch, at
// this cost. BatchID is empty when no batch covered them.
type Allocation struct {
	IssuanceID string
	BatchID    string
	OccurredOn time.Time
	Points     int
	CostCents  int
}

// Pending reports whether this slice found no batch to charge to.
func (a Allocation) Pending() bool { return a.BatchID == "" }

// Allocate walks the account's events in date order, charging each booking to the oldest
// batch with points remaining and giving refunded points back to the batches they came
// from.
//
// Everything is recomputed from scratch rather than appended to, and that is the central
// design decision here. Reality arrives out of order: an issuance is corrected, cancelled
// weeks later, imported retroactively; a batch is entered long after the booking that
// already spent it. An incremental FIFO gets the first retroactive cancellation wrong and
// stays wrong in silence - the totals still add up, only the cost of each line lies.
//
// Bookings and refunds share one pass because they share a timeline. Points returned on the
// 16th are stock again on the 17th, and the queue rewinds to the batch that got them back
// so the next booking finds them - at that batch's own rate, ahead of anything newer. A
// refund whose booking the panel never had goes back to the batch the queue is on that
// day, at that batch's rate: the nearest thing to "where it came from" when nobody recorded
// where that was.
//
// A booking that outruns the available points still produces an Allocation, with an empty
// BatchID. The costing never blocks an issuance: the bot writes bookings in real time, and
// a shortfall is something to report, not something to refuse. If it refused, the first
// workaround anyone found would be to turn the account's inventory off.
//
// `prior` carries the slices a closed period already settled, keyed by issuance, so a
// refund on a booking from a frozen month knows which batches to give back to. It may be
// nil when nothing is frozen.
func Allocate(batches []Batch, events []Event, prior map[string][]Allocation) []Allocation {
	queue := append([]Batch(nil), batches...)
	sort.SliceStable(queue, func(i, j int) bool {
		if !queue[i].AcquiredOn.Equal(queue[j].AcquiredOn) {
			return queue[i].AcquiredOn.Before(queue[j].AcquiredOn)
		}
		return queue[i].Sequence < queue[j].Sequence
	})

	timeline := append([]Event(nil), events...)
	sort.SliceStable(timeline, func(i, j int) bool {
		if !timeline[i].OccurredOn.Equal(timeline[j].OccurredOn) {
			return timeline[i].OccurredOn.Before(timeline[j].OccurredOn)
		}
		return timeline[i].Sequence < timeline[j].Sequence
	})

	// consumed and charged track each batch's running totals, seeded with whatever closed
	// periods already took. Charging from the running total is what keeps the slices of a
	// fully consumed batch adding back up to its cost exactly - see sliceCostCents.
	consumed := make([]int, len(queue))
	charged := make([]int, len(queue))
	at := make(map[string]int, len(queue))
	for i, batch := range queue {
		consumed[i] = batch.ConsumedPoints
		charged[i] = batch.ChargedCents
		at[batch.ID] = i
	}

	// What each issuance is holding right now, so a refund knows what to give back. Seeded
	// from the frozen slices, then kept up to date as this pass allocates and reverses.
	held := map[string][]Allocation{}
	for issuanceID, slices := range prior {
		held[issuanceID] = append([]Allocation(nil), slices...)
	}

	allocations := make([]Allocation, 0, len(timeline))
	head := 0

	for _, event := range timeline {
		if event.Points < 0 && event.WithoutBooking {
			allocation := Allocation{
				IssuanceID: event.IssuanceID,
				OccurredOn: event.OccurredOn,
				Points:     event.Points,
			}
			// With no batch at all the return stays pending, and nets against whatever
			// shortfall the account already has.
			if len(queue) > 0 {
				// head only ever sits past the end once every batch is exhausted; the newest
				// batch then takes the points, and the queue steps back onto it.
				if head >= len(queue) {
					head = len(queue) - 1
				}
				batch := queue[head]
				cost := proratedCents(batch.CostCents, -event.Points, batch.Points)
				consumed[head] += event.Points
				charged[head] -= cost
				allocation.BatchID = batch.ID
				allocation.CostCents = -cost
			}
			allocations = append(allocations, allocation)
			held[event.IssuanceID] = append(held[event.IssuanceID], allocation)
			continue
		}

		if event.Points < 0 {
			returns := reverseAgainst(held[event.IssuanceID], -event.Points, event.OccurredOn)
			for _, ret := range returns {
				allocations = append(allocations, ret)
				held[event.IssuanceID] = append(held[event.IssuanceID], ret)
				if ret.Pending() {
					continue
				}
				i, ok := at[ret.BatchID]
				if !ok {
					continue
				}
				// Points and cost come back as negatives, so adding them is the undo.
				consumed[i] += ret.Points
				charged[i] += ret.CostCents
				// Rewind: this batch has stock again, and it sits ahead of everything
				// the queue has moved past.
				if i < head {
					head = i
				}
			}
			continue
		}

		remaining := event.Points

		for remaining > 0 && head < len(queue) {
			batch := queue[head]
			available := batch.Points - consumed[head]
			if available <= 0 {
				head++
				continue
			}

			slice := remaining
			if slice > available {
				slice = available
			}

			cost := sliceCostCents(batch.CostCents, batch.Points, consumed[head], slice, charged[head])
			consumed[head] += slice
			charged[head] += cost
			remaining -= slice

			allocation := Allocation{
				IssuanceID: event.IssuanceID,
				BatchID:    batch.ID,
				OccurredOn: event.OccurredOn,
				Points:     slice,
				CostCents:  cost,
			}
			allocations = append(allocations, allocation)
			held[event.IssuanceID] = append(held[event.IssuanceID], allocation)
		}

		// Whatever the queue could not cover. Cost zero, because nobody has said yet what
		// these points cost - which is different from them having been free.
		if remaining > 0 {
			allocation := Allocation{
				IssuanceID: event.IssuanceID,
				OccurredOn: event.OccurredOn,
				Points:     remaining,
				CostCents:  0,
			}
			allocations = append(allocations, allocation)
			held[event.IssuanceID] = append(held[event.IssuanceID], allocation)
		}
	}

	return allocations
}

// sliceCostCents is what one slice of a batch costs, derived from the batch's running
// total rather than from a per-point rate.
//
// Pricing a slice as `slice * cost / points` rounds every slice independently, so a batch
// consumed in ten bookings can end up having charged a few centavos more or less than it
// actually cost, with no line to blame it on. Taking the difference between two cumulative
// figures makes the errors cancel: the last slice of a fully consumed batch absorbs the
// remainder, and the slices always sum to CostCents exactly.
func sliceCostCents(batchCost, batchPoints, alreadyConsumed, slice, alreadyCharged int) int {
	if batchPoints <= 0 {
		return 0
	}
	return proratedCents(batchCost, alreadyConsumed+slice, batchPoints) - alreadyCharged
}

// proratedCents computes cost * part / whole in integer arithmetic, rounded half up.
//
// int64 throughout because the product overflows a 32-bit int long before the inputs do:
// a 1.126.202-point batch costing R$ 16.893,03 already multiplies out past two billion.
func proratedCents(cost, part, whole int) int {
	if whole <= 0 {
		return 0
	}
	numerator := int64(cost)*int64(part) + int64(whole)/2
	return int(numerator / int64(whole))
}
