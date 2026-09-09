package inventory

import (
	"testing"
	"time"
)

// mjf6hcSlices is the two-batch split from allocate_test.go, reused as the input every
// reversal case starts from: 360 milheiros costing R$ 5.042,45 across two rates.
func mjf6hcSlices() []Allocation {
	return []Allocation{
		{IssuanceID: "MJF6HC", BatchID: "resto-ultima", OccurredOn: day("2026-08-05"), Points: 108202, CostCents: 162303},
		{IssuanceID: "MJF6HC", BatchID: "esfera", OccurredOn: day("2026-08-05"), Points: 251798, CostCents: 341942},
	}
}

// A full cancellation has to net the issuance to zero, however many batches it spanned and
// however awkward the rates were. Anything left over is profit or cost still being claimed
// on a booking that no longer exists.
func TestReverseGivesBackEverythingOnAFullCancellation(t *testing.T) {
	t.Parallel()

	slices := mjf6hcSlices()
	reversals := Reverse(slices, 360000, day("2026-09-04"))

	if points := totalPoints(reversals); points != -360000 {
		t.Errorf("returned %d points, want -360000", points)
	}
	if cost := totalCost(reversals); cost != -504245 {
		t.Errorf("returned %d cents, want -504245", cost)
	}

	net := append(append([]Allocation{}, slices...), reversals...)
	if points := totalPoints(net); points != 0 {
		t.Errorf("issuance nets to %d points, want 0", points)
	}
	if cost := totalCost(net); cost != 0 {
		t.Errorf("issuance nets to %d cents, want 0", cost)
	}
}

// Last consumed, first returned. Simple to state at a reconciliation, and it avoids
// splitting a partial refund proportionally across batches at two different rates.
func TestReverseReturnsTheMostRecentlyConsumedBatchFirst(t *testing.T) {
	t.Parallel()

	reversals := Reverse(mjf6hcSlices(), 100000, day("2026-09-04"))

	if len(reversals) != 1 {
		t.Fatalf("got %d reversals, want 1 - it fits inside the last batch consumed", len(reversals))
	}
	if reversals[0].BatchID != "esfera" {
		t.Errorf("returned to %s, want esfera - the batch consumed last", reversals[0].BatchID)
	}
	if reversals[0].Points != -100000 {
		t.Errorf("returned %d points, want -100000", reversals[0].Points)
	}
	// 100.000 of the 251.798 points esfera supplied, at that slice's own rate.
	if want := -proratedCents(341942, 100000, 251798); reversals[0].CostCents != want {
		t.Errorf("returned %d cents, want %d", reversals[0].CostCents, want)
	}
}

// The reversal is dated by the refund, not by the booking, so it falls in the open period
// while the issuance it reverses stays in the closed one.
func TestReverseDatesTheReturnByTheRefundNotTheBooking(t *testing.T) {
	t.Parallel()

	refundedOn := day("2026-09-04")
	reversals := Reverse(mjf6hcSlices(), 360000, refundedOn)

	for _, reversal := range reversals {
		if !reversal.OccurredOn.Equal(refundedOn) {
			t.Errorf("reversal dated %s, want the refund's %s",
				reversal.OccurredOn.Format(time.DateOnly), refundedOn.Format(time.DateOnly))
		}
	}
}

// The airline refunds a booking in instalments - MM8LVJ came back as 26.000 and then
// 11.600 points. Each call sees the earlier reversals, so the two together give back
// exactly what one call for the total would have.
func TestReverseAccumulatesAcrossPartialRefunds(t *testing.T) {
	t.Parallel()

	slices := mjf6hcSlices()

	first := Reverse(slices, 26000, day("2026-09-04"))
	afterFirst := append(append([]Allocation{}, slices...), first...)
	second := Reverse(afterFirst, 11600, day("2026-09-06"))

	returnedPoints := totalPoints(first) + totalPoints(second)
	if returnedPoints != -37600 {
		t.Errorf("two instalments returned %d points, want -37600", returnedPoints)
	}

	// Both instalments came out of esfera, so the total returned has to match one
	// reversal of 37.600 taken from that slice directly.
	want := -proratedCents(341942, 37600, 251798)
	if got := totalCost(first) + totalCost(second); got != want {
		t.Errorf("two instalments returned %d cents, want %d", got, want)
	}
}

// A refund that spans more than the last batch has to walk backwards into the one before,
// still returning each batch's points at that batch's own cost.
func TestReverseWalksBackIntoTheEarlierBatch(t *testing.T) {
	t.Parallel()

	reversals := Reverse(mjf6hcSlices(), 300000, day("2026-09-04"))

	if len(reversals) != 2 {
		t.Fatalf("got %d reversals, want 2", len(reversals))
	}
	if reversals[0].BatchID != "esfera" || reversals[0].Points != -251798 {
		t.Errorf("first reversal = %s %d, want esfera -251798 (the whole slice)",
			reversals[0].BatchID, reversals[0].Points)
	}
	// Returned whole, so esfera gets back exactly what it charged.
	if reversals[0].CostCents != -341942 {
		t.Errorf("esfera got back %d cents, want -341942", reversals[0].CostCents)
	}
	if reversals[1].BatchID != "resto-ultima" || reversals[1].Points != -48202 {
		t.Errorf("second reversal = %s %d, want resto-ultima -48202",
			reversals[1].BatchID, reversals[1].Points)
	}
}

// A shortfall that was never charged to anything is the first thing to go: it was the last
// slice in, and giving it back costs nothing because it never cost anything.
func TestReverseReturnsAPendingSliceBeforeTouchingABatch(t *testing.T) {
	t.Parallel()

	slices := []Allocation{
		{IssuanceID: "ZFLSGB", BatchID: "small", OccurredOn: day("2026-09-02"), Points: 50000, CostCents: 75000},
		{IssuanceID: "ZFLSGB", OccurredOn: day("2026-09-02"), Points: 150000, CostCents: 0},
	}

	reversals := Reverse(slices, 150000, day("2026-09-05"))

	if len(reversals) != 1 {
		t.Fatalf("got %d reversals, want 1", len(reversals))
	}
	if !reversals[0].Pending() {
		t.Errorf("returned to %s, want the pending slice first", reversals[0].BatchID)
	}
	if reversals[0].CostCents != 0 {
		t.Errorf("pending slice returned %d cents, want 0", reversals[0].CostCents)
	}
}

// The airline decides how much comes back. A refund larger than what we recorded means our
// figure was wrong, not that the refund is invalid - so it gives back all there is.
func TestReverseStopsAtWhatWasActuallyConsumed(t *testing.T) {
	t.Parallel()

	slices := mjf6hcSlices()
	reversals := Reverse(slices, 500000, day("2026-09-04"))

	if points := totalPoints(reversals); points != -360000 {
		t.Errorf("returned %d points, want -360000 - all there was", points)
	}
}

// Nothing to give back is not an error, and must not write an empty reversal.
func TestReverseIgnoresANonPositiveRequest(t *testing.T) {
	t.Parallel()

	if reversals := Reverse(mjf6hcSlices(), 0, day("2026-09-04")); reversals != nil {
		t.Errorf("got %v, want no reversals", reversals)
	}
}

// A fully reversed issuance has nothing left to give, so a second cancellation is a no-op
// rather than a way to mint points back into a batch.
func TestReverseGivesNothingBackTwice(t *testing.T) {
	t.Parallel()

	slices := mjf6hcSlices()
	first := Reverse(slices, 360000, day("2026-09-04"))
	afterFirst := append(append([]Allocation{}, slices...), first...)

	if second := Reverse(afterFirst, 360000, day("2026-09-10")); len(second) != 0 {
		t.Errorf("got %v on a second full reversal, want nothing left to return", second)
	}
}
