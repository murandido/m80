package inventory

import (
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// tagTotals nets an issuance's slices per alteration tag, which is what the closing prices.
func tagTotals(allocations []Allocation) (points, cost map[string]int) {
	points, cost = map[string]int{}, map[string]int{}
	for _, allocation := range allocations {
		points[allocation.AdjustmentID] += allocation.Points
		cost[allocation.AdjustmentID] += allocation.CostCents
	}
	return points, cost
}

// An alteration that charged more miles consumes from the queue like a booking, on its own
// day, and its slices carry the alteration so the closing can price them at its rate.
func TestAllocateCostsAnAlterationOnItsOwnDay(t *testing.T) {
	t.Parallel()

	batch := Batch{ID: "lote", AcquiredOn: day("2026-08-01"), Points: 100000, CostCents: 100000}
	events := []Event{
		{IssuanceID: "abc", OccurredOn: day("2026-08-10"), Points: 50000},
		{IssuanceID: "abc", OccurredOn: day("2026-08-20"), Points: 10000, Sequence: 1, AdjustmentID: "extra"},
	}

	allocations := Allocate([]Batch{batch}, events, nil)

	if len(allocations) != 2 {
		t.Fatalf("got %d slices, want 2", len(allocations))
	}
	extra := allocations[1]
	if extra.AdjustmentID != "extra" || extra.Points != 10000 || extra.CostCents != 10000 || !extra.OccurredOn.Equal(day("2026-08-20")) {
		t.Errorf("alteration slice = %+v, want 10000 points costing 10000 on 2026-08-20 tagged extra", extra)
	}
}

// The case the tags exist for. A booking, an alteration charging more, an alteration
// giving some back, then a full cancellation. Every tag has to net to zero except the
// refund's, because what the refund gave back was spent from the booking - so the booking
// keeps exactly the refund's points, and the closing prices each side at its own rate.
func TestAllocateNetsAlterationsAndACancellationPerTag(t *testing.T) {
	t.Parallel()

	older := Batch{ID: "older", AcquiredOn: day("2026-08-01"), Points: 40000, CostCents: 40000}
	newer := Batch{ID: "newer", AcquiredOn: day("2026-08-02"), Points: 100000, CostCents: 150000}
	events := []Event{
		{IssuanceID: "abc", OccurredOn: day("2026-08-10"), Points: 50000},
		{IssuanceID: "abc", OccurredOn: day("2026-08-12"), Points: 10000, Sequence: 1, AdjustmentID: "extra"},
		{IssuanceID: "abc", OccurredOn: day("2026-08-14"), Points: -5000, Sequence: 2, AdjustmentID: "refund"},
		// What refundedPoints works out for a cancelled booking: 50000 + 10000 - 5000.
		{IssuanceID: "abc", OccurredOn: day("2026-08-20"), Points: -55000, Sequence: 3},
	}

	allocations := Allocate([]Batch{older, newer}, events, nil)

	if total := totalPoints(allocations); total != 0 {
		t.Errorf("net points = %d, want 0", total)
	}
	if total := totalCost(allocations); total != 0 {
		t.Errorf("net cost = %d, want 0", total)
	}

	points, _ := tagTotals(allocations)
	if points["extra"] != 0 {
		t.Errorf("alteration nets %d points, want 0: the cancellation gives it back at its own rate", points["extra"])
	}
	if points["refund"] != -5000 || points[""] != 5000 {
		t.Errorf("refund nets %d and booking %d, want -5000 and 5000", points["refund"], points[""])
	}

	// The refund draws from the booking's own slices, newest first: the booking's last
	// 5000 points came from the newer batch, at 15,00 the thousand.
	for _, allocation := range allocations {
		if allocation.AdjustmentID == "refund" && (allocation.BatchID != "newer" || allocation.CostCents != -7500) {
			t.Errorf("refund slice = %+v, want -7500 from newer", allocation)
		}
	}
}

// Prices the alterations from the database end to end: one charged more and was passed on
// to the client, one was absorbed, and one gave miles back that the agency kept.
func TestReadClosingPricesAlterationsAtTheirOwnRate(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		// 200 milheiros for R$ 2.000,00 - 10,00 per thousand.
		seedBatch(t, tx, f, "2026-08-01", 200000, 200000)
		// 100 milheiros sold at 15,00.
		issuance := seedIssuance(t, tx, f, "2026-08-10", 100000)

		seedAdjustment(t, tx, f, issuance, "2026-08-12", 10000, 2000) // passed on at 20,00
		seedAdjustment(t, tx, f, issuance, "2026-08-13", 5000, 0)     // absorbed
		seedAdjustment(t, tx, f, issuance, "2026-08-14", -4000, 0)    // refund kept

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}
		closing, err := ReadClosing(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"))
		if err != nil {
			t.Fatalf("ReadClosing: %v", err)
		}

		// 100k + 10k + 5k - 4k = 111k at 10,00.
		if closing.CostCents != 111000 {
			t.Errorf("cost = %d, want 111000", closing.CostCents)
		}
		// 100k at 15,00 + 10k at 20,00 + nothing for the absorbed and the kept refund.
		if closing.RevenueCents != 170000 {
			t.Errorf("revenue = %d, want 170000", closing.RevenueCents)
		}
	})
}

// A cancelled booking with no explicit refund gives back what its alterations charged too.
func TestRecomputeCancellationReturnsTheAlterationsPoints(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 200000, 200000)
		issuance := seedIssuance(t, tx, f, "2026-08-10", 100000)
		seedAdjustment(t, tx, f, issuance, "2026-08-12", 10000, 0)

		cancelledAt := instantOn("2026-08-20")
		if err := tx.Model(&issuance).Updates(map[string]any{"cancelled_at": cancelledAt, "active": false}).Error; err != nil {
			t.Fatalf("cancelling: %v", err)
		}

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		net := 0
		for _, row := range allocationsOf(t, tx, f.account.ID) {
			net += row.Points
		}
		if net != 0 {
			t.Errorf("net points after the cancellation = %d, want 0", net)
		}

		balances, err := ExpectedBalances(tx, []string{f.account.ID})
		if err != nil {
			t.Fatalf("ExpectedBalances: %v", err)
		}
		if balances[f.account.ID] != 200000 {
			t.Errorf("expected balance = %d, want the whole batch back", balances[f.account.ID])
		}
	})
}

func seedAdjustment(t *testing.T, tx *gorm.DB, f fixture, issuance data.Issuance, occurredOn string, points, valueOfTheThousand int) data.IssuanceAdjustment {
	t.Helper()

	adjustment := data.IssuanceAdjustment{
		IssuanceID:         issuance.ID,
		OccurredOn:         day(occurredOn),
		Points:             points,
		ValueOfTheThousand: valueOfTheThousand,
		Reason:             "trecho alterado",
		CreatedByID:        f.user.ID,
		// created_at orders same-day alterations; a fixed instant keeps it stable.
		CreatedAt: day(occurredOn).Add(time.Hour),
	}
	mustCreate(t, tx, &adjustment)
	return adjustment
}
