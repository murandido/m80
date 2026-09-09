package inventory

import (
	"testing"
	"time"
)

func day(value string) time.Time {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(err)
	}
	return parsed
}

// The case the whole table exists for, taken from the August 2026 spreadsheet.
//
// MJF6HC is 360 milheiros that did not fit in what was left of the carried-forward
// balance, so the sheet wrote it as two lines at two different rates. Both figures below
// are the spreadsheet's own, to the centavo:
//
//	-108,202 @ 15,000000 -> R$ 1.623,03   (RESTO ULTIMA)
//	-251,798 @ 13,580006 -> R$ 3.419,42   (ESFERA)
//
// If this test ever has to be "adjusted" to keep passing, the costing changed meaning.
func TestAllocateSplitsAcrossBatchesAtTheSpreadsheetsFigures(t *testing.T) {
	t.Parallel()

	// RESTO ULTIMA: 1.126,202 milheiros for R$ 16.893,03 - a flat 15,00 per thousand.
	carried := Batch{ID: "resto-ultima", AcquiredOn: day("2026-07-31"), Points: 1126202, CostCents: 1689303}
	// ESFERA: 651,2 milheiros for R$ 8.843,30 - 13,580006 per thousand.
	esfera := Batch{ID: "esfera", AcquiredOn: day("2026-08-05"), Points: 651200, CostCents: 884330}

	// Everything issued before MJF6HC, as one booking: what the sheet had already spent
	// out of the carried balance by the time it got there.
	earlier := Event{IssuanceID: "earlier", OccurredOn: day("2026-08-04"), Points: 1018000}
	mjf6hc := Event{IssuanceID: "MJF6HC", OccurredOn: day("2026-08-05"), Points: 360000, Sequence: 1}

	allocations := Allocate([]Batch{carried, esfera}, []Event{earlier, mjf6hc}, nil)

	split := allocationsFor(allocations, "MJF6HC")
	if len(split) != 2 {
		t.Fatalf("MJF6HC produced %d slices, want 2", len(split))
	}

	if split[0].BatchID != "resto-ultima" || split[0].Points != 108202 || split[0].CostCents != 162303 {
		t.Errorf("first slice = %s %d points %d cents, want resto-ultima 108202 points 162303 cents",
			split[0].BatchID, split[0].Points, split[0].CostCents)
	}
	if split[1].BatchID != "esfera" || split[1].Points != 251798 || split[1].CostCents != 341942 {
		t.Errorf("second slice = %s %d points %d cents, want esfera 251798 points 341942 cents",
			split[1].BatchID, split[1].Points, split[1].CostCents)
	}

	// 5.042,45 total. Averaging the rate to a whole centavo per thousand and multiplying
	// back would give 5.043,60 - the R$ 1,15 of drift this table exists to prevent.
	if total := totalCost(split); total != 504245 {
		t.Errorf("MJF6HC cost = %d cents, want 504245", total)
	}
}

// A fully consumed batch must have charged exactly what it cost, however many bookings it
// was consumed by. This is the property that per-slice rounding would break.
func TestAllocateChargesAFullyConsumedBatchItsExactCost(t *testing.T) {
	t.Parallel()

	// A rate that does not divide evenly: 8.941,72 over 632,7 milheiros.
	livelo := Batch{ID: "livelo", AcquiredOn: day("2026-08-21"), Points: 632700, CostCents: 894172}

	// Twelve awkward bookings that between them take the batch to exactly zero.
	sizes := []int{37600, 15400, 56327, 38358, 87617, 64383, 152000, 40000, 15200, 44800, 50400, 30615}
	consumptions := make([]Event, 0, len(sizes))
	spent := 0
	for i, size := range sizes {
		consumptions = append(consumptions, Event{
			IssuanceID: string(rune('a' + i)),
			OccurredOn: day("2026-08-22"),
			Points:     size,
			Sequence:   i,
		})
		spent += size
	}
	if spent != livelo.Points {
		t.Fatalf("test setup spends %d of %d points; it must consume the batch exactly", spent, livelo.Points)
	}

	allocations := Allocate([]Batch{livelo}, consumptions, nil)

	if total := totalCost(allocations); total != livelo.CostCents {
		t.Errorf("slices charged %d cents, want the batch's exact %d", total, livelo.CostCents)
	}
}

// Clube Azul and flight accrual put points in the account for nothing. Zero is a real
// cost, not a missing one, and the whole sale counts as profit.
func TestAllocateHandlesAZeroCostBatch(t *testing.T) {
	t.Parallel()

	club := Batch{ID: "clube-azul", AcquiredOn: day("2026-08-10"), Points: 21000, CostCents: 0}
	booking := Event{IssuanceID: "AUW7ER", OccurredOn: day("2026-08-12"), Points: 21000}

	allocations := Allocate([]Batch{club}, []Event{booking}, nil)

	if len(allocations) != 1 {
		t.Fatalf("got %d slices, want 1", len(allocations))
	}
	if allocations[0].BatchID != "clube-azul" || allocations[0].CostCents != 0 {
		t.Errorf("slice = %s at %d cents, want clube-azul at 0", allocations[0].BatchID, allocations[0].CostCents)
	}
}

// The bot writes bookings in real time; the purchase is entered whenever it is entered.
// A shortfall has to be reported, never refused.
func TestAllocateLeavesAShortfallPendingInsteadOfRefusing(t *testing.T) {
	t.Parallel()

	small := Batch{ID: "small", AcquiredOn: day("2026-09-01"), Points: 50000, CostCents: 75000}
	booking := Event{IssuanceID: "ZFLSGB", OccurredOn: day("2026-09-02"), Points: 200000}

	allocations := Allocate([]Batch{small}, []Event{booking}, nil)

	if len(allocations) != 2 {
		t.Fatalf("got %d slices, want 2 (one covered, one pending)", len(allocations))
	}
	if allocations[0].Pending() {
		t.Error("first slice should have been covered by the batch")
	}
	if !allocations[1].Pending() {
		t.Fatal("second slice should be pending")
	}
	if allocations[1].Points != 150000 {
		t.Errorf("pending points = %d, want 150000", allocations[1].Points)
	}
	// Zero because nobody has said what these cost yet, which is not the same as free.
	if allocations[1].CostCents != 0 {
		t.Errorf("pending cost = %d, want 0", allocations[1].CostCents)
	}
}

// Entering the missing batch is the whole fix - no rework, because nothing was written
// against the pending state in the first place.
func TestAllocateResolvesAPendingSliceOnceTheBatchIsEntered(t *testing.T) {
	t.Parallel()

	small := Batch{ID: "small", AcquiredOn: day("2026-09-01"), Points: 50000, CostCents: 75000}
	late := Batch{ID: "late", AcquiredOn: day("2026-09-01"), Points: 300000, CostCents: 420000, Sequence: 1}
	booking := Event{IssuanceID: "ZFLSGB", OccurredOn: day("2026-09-02"), Points: 200000}

	allocations := Allocate([]Batch{small, late}, []Event{booking}, nil)

	for _, allocation := range allocations {
		if allocation.Pending() {
			t.Fatalf("still pending after the late batch was entered: %d points", allocation.Points)
		}
	}
	if points := totalPoints(allocations); points != 200000 {
		t.Errorf("allocated %d points, want 200000", points)
	}
}

// Oldest batch first, whatever order the rows arrive in.
func TestAllocateConsumesTheOldestBatchFirst(t *testing.T) {
	t.Parallel()

	newer := Batch{ID: "newer", AcquiredOn: day("2026-08-20"), Points: 100000, CostCents: 150000}
	older := Batch{ID: "older", AcquiredOn: day("2026-08-01"), Points: 100000, CostCents: 130000}
	booking := Event{IssuanceID: "XM7L2J", OccurredOn: day("2026-08-25"), Points: 100000}

	allocations := Allocate([]Batch{newer, older}, []Event{booking}, nil)

	if len(allocations) != 1 || allocations[0].BatchID != "older" {
		t.Fatalf("consumed %v, want the older batch first", allocations)
	}
}

// Same-day bookings queue by Sequence, which is created_at in the panel. Without it the
// costing would depend on the order the database happened to return rows in.
func TestAllocateBreaksASameDayTieBySequence(t *testing.T) {
	t.Parallel()

	cheap := Batch{ID: "cheap", AcquiredOn: day("2026-08-01"), Points: 100000, CostCents: 100000}
	dear := Batch{ID: "dear", AcquiredOn: day("2026-08-02"), Points: 100000, CostCents: 200000}

	second := Event{IssuanceID: "second", OccurredOn: day("2026-08-10"), Points: 100000, Sequence: 2}
	first := Event{IssuanceID: "first", OccurredOn: day("2026-08-10"), Points: 100000, Sequence: 1}

	allocations := Allocate([]Batch{cheap, dear}, []Event{second, first}, nil)

	if got := allocationsFor(allocations, "first")[0].BatchID; got != "cheap" {
		t.Errorf("the earlier booking took %s, want cheap", got)
	}
	if got := allocationsFor(allocations, "second")[0].BatchID; got != "dear" {
		t.Errorf("the later booking took %s, want dear", got)
	}
}

// Recomputing is only safe if it is deterministic: the same inputs, in any row order,
// have to produce the same costing.
func TestAllocateIsDeterministicRegardlessOfInputOrder(t *testing.T) {
	t.Parallel()

	batches := []Batch{
		{ID: "b1", AcquiredOn: day("2026-08-01"), Points: 200000, CostCents: 280000},
		{ID: "b2", AcquiredOn: day("2026-08-15"), Points: 300000, CostCents: 405000},
		{ID: "b3", AcquiredOn: day("2026-08-15"), Points: 150000, CostCents: 210000, Sequence: 1},
	}
	consumptions := []Event{
		{IssuanceID: "i1", OccurredOn: day("2026-08-10"), Points: 120000},
		{IssuanceID: "i2", OccurredOn: day("2026-08-20"), Points: 250000, Sequence: 1},
		{IssuanceID: "i3", OccurredOn: day("2026-08-20"), Points: 180000, Sequence: 2},
	}

	forward := Allocate(batches, consumptions, nil)

	reversedBatches := []Batch{batches[2], batches[1], batches[0]}
	reversedConsumptions := []Event{consumptions[2], consumptions[1], consumptions[0]}
	backward := Allocate(reversedBatches, reversedConsumptions, nil)

	if len(forward) != len(backward) {
		t.Fatalf("slice counts differ: %d vs %d", len(forward), len(backward))
	}
	for i := range forward {
		if forward[i] != backward[i] {
			t.Errorf("slice %d differs:\n  %+v\n  %+v", i, forward[i], backward[i])
		}
	}
}

func allocationsFor(allocations []Allocation, issuanceID string) []Allocation {
	var found []Allocation
	for _, allocation := range allocations {
		if allocation.IssuanceID == issuanceID {
			found = append(found, allocation)
		}
	}
	return found
}

func totalCost(allocations []Allocation) int {
	total := 0
	for _, allocation := range allocations {
		total += allocation.CostCents
	}
	return total
}

func totalPoints(allocations []Allocation) int {
	total := 0
	for _, allocation := range allocations {
		total += allocation.Points
	}
	return total
}

// A batch half spent in a closed period offers only what it has left, and prices that
// remainder as the continuation of the same batch rather than as a fresh one.
func TestAllocateStartsFromWhatClosedPeriodsAlreadyTook(t *testing.T) {
	t.Parallel()

	// The carried balance from the MJF6HC case, already 1.018.000 points into a closed
	// August at the 1.527,00 those points were charged.
	carried := Batch{
		ID: "resto-ultima", AcquiredOn: day("2026-07-31"),
		Points: 1126202, CostCents: 1689303,
		ConsumedPoints: 1018000, ChargedCents: 1527000,
	}
	esfera := Batch{ID: "esfera", AcquiredOn: day("2026-08-05"), Points: 651200, CostCents: 884330}

	mjf6hc := Event{IssuanceID: "MJF6HC", OccurredOn: day("2026-09-05"), Points: 360000}
	allocations := Allocate([]Batch{carried, esfera}, []Event{mjf6hc}, nil)

	// Identical to the split computed in the same period, which is the point: closing a
	// month must not change what the next booking costs.
	if len(allocations) != 2 {
		t.Fatalf("got %d slices, want 2", len(allocations))
	}
	if allocations[0].Points != 108202 || allocations[0].CostCents != 162303 {
		t.Errorf("first slice = %d points %d cents, want 108202 / 162303",
			allocations[0].Points, allocations[0].CostCents)
	}
	if allocations[1].Points != 251798 || allocations[1].CostCents != 341942 {
		t.Errorf("second slice = %d points %d cents, want 251798 / 341942",
			allocations[1].Points, allocations[1].CostCents)
	}
}

// A batch emptied in a closed period, then handed points back by a reversal, rejoins the
// queue at its own date - ahead of anything acquired since.
func TestAllocateLetsAnExhaustedBatchComeBackToLife(t *testing.T) {
	t.Parallel()

	// Fully consumed in August, then given 100.000 points back by a September refund:
	// the net closed consumption is 100.000 short of the batch.
	revived := Batch{
		ID: "revived", AcquiredOn: day("2026-08-01"),
		Points: 500000, CostCents: 700000,
		ConsumedPoints: 400000, ChargedCents: 560000,
	}
	newer := Batch{ID: "newer", AcquiredOn: day("2026-09-01"), Points: 500000, CostCents: 900000}

	booking := Event{IssuanceID: "next", OccurredOn: day("2026-09-10"), Points: 100000}
	allocations := Allocate([]Batch{revived, newer}, []Event{booking}, nil)

	if len(allocations) != 1 {
		t.Fatalf("got %d slices, want 1", len(allocations))
	}
	if allocations[0].BatchID != "revived" {
		t.Errorf("took from %s, want the revived batch - same points, same place in the queue",
			allocations[0].BatchID)
	}
	// The returned points cost what they always cost: 700000 - 560000 for the last fifth.
	if allocations[0].CostCents != 140000 {
		t.Errorf("cost = %d cents, want 140000", allocations[0].CostCents)
	}
}

// Points refunded in the middle of the month are stock again for the bookings that follow.
//
// This is what one pass buys. Allocating everything and only then applying the reversals
// leaves the later booking reported as a shortfall while the points are sitting in the
// account - which is exactly what the Marlon spreadsheet import turned up: 84,983 milheiros
// of phantom shortfall that no amount of entering batches would clear.
func TestAllocateReturnsRefundedPointsToTheQueueForLaterBookings(t *testing.T) {
	t.Parallel()

	// Only enough stock for one of the two bookings at a time.
	batch := Batch{ID: "only", AcquiredOn: day("2026-08-01"), Points: 100000, CostCents: 150000}

	events := []Event{
		{IssuanceID: "first", OccurredOn: day("2026-08-10"), Points: 100000, Sequence: 1},
		// Cancelled on the 16th, so the points are free again.
		{IssuanceID: "first", OccurredOn: day("2026-08-16"), Points: -100000, Sequence: 2},
		{IssuanceID: "second", OccurredOn: day("2026-08-18"), Points: 100000, Sequence: 3},
	}

	allocations := Allocate([]Batch{batch}, events, nil)

	for _, allocation := range allocations {
		if allocation.Pending() {
			t.Fatalf("%s reported a shortfall of %d points while the stock was back in the account",
				allocation.IssuanceID, allocation.Points)
		}
	}

	// The second booking is charged what the first one was: the same points, at the same
	// rate, from the same batch.
	second := allocationsFor(allocations, "second")
	if len(second) != 1 || second[0].BatchID != "only" {
		t.Fatalf("second booking = %+v, want one slice from the batch", second)
	}
	if second[0].CostCents != 150000 {
		t.Errorf("second booking cost %d cents, want the batch's whole 150000", second[0].CostCents)
	}
}

// The queue rewinds only as far as the batch that got its points back - a newer batch
// already in use is not disturbed.
func TestAllocateRewindsOnlyToTheBatchThatWasRefunded(t *testing.T) {
	t.Parallel()

	older := Batch{ID: "older", AcquiredOn: day("2026-08-01"), Points: 100000, CostCents: 100000}
	newer := Batch{ID: "newer", AcquiredOn: day("2026-08-05"), Points: 100000, CostCents: 200000}

	events := []Event{
		{IssuanceID: "a", OccurredOn: day("2026-08-10"), Points: 100000, Sequence: 1}, // takes older
		{IssuanceID: "b", OccurredOn: day("2026-08-11"), Points: 50000, Sequence: 2},  // takes newer
		{IssuanceID: "a", OccurredOn: day("2026-08-12"), Points: -100000, Sequence: 3},
		{IssuanceID: "c", OccurredOn: day("2026-08-13"), Points: 100000, Sequence: 4},
	}

	allocations := Allocate([]Batch{older, newer}, events, nil)

	// c takes the refunded older points first - they are older than what newer holds.
	c := allocationsFor(allocations, "c")
	if len(c) != 1 || c[0].BatchID != "older" || c[0].CostCents != 100000 {
		t.Fatalf("c = %+v, want the whole refunded older batch at 100000 cents", c)
	}
	// b keeps what it took, untouched by the rewind.
	b := allocationsFor(allocations, "b")
	if len(b) != 1 || b[0].BatchID != "newer" {
		t.Fatalf("b = %+v, want its newer slice left alone", b)
	}
}
