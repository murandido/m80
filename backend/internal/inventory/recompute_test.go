package inventory

import (
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The plain case: two batches, one booking that spans both, priced from the database.
func TestRecomputeSplitsABookingAcrossTwoBatches(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		older := seedBatch(t, tx, f, "2026-08-01", 100000, 150000)
		newer := seedBatch(t, tx, f, "2026-08-10", 300000, 405000)
		issuance := seedIssuance(t, tx, f, "2026-08-15", 250000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		rows := allocationsOf(t, tx, f.account.ID)
		if len(rows) != 2 {
			t.Fatalf("got %d allocations, want 2", len(rows))
		}
		if rows[0].BatchID == nil || *rows[0].BatchID != older.ID || rows[0].Points != 100000 {
			t.Errorf("first slice = %v / %d points, want the older batch and 100000", rows[0].BatchID, rows[0].Points)
		}
		if rows[1].BatchID == nil || *rows[1].BatchID != newer.ID || rows[1].Points != 150000 {
			t.Errorf("second slice = %v / %d points, want the newer batch and 150000", rows[1].BatchID, rows[1].Points)
		}
		for _, row := range rows {
			if row.IssuanceID != issuance.ID {
				t.Errorf("slice belongs to %s, want %s", row.IssuanceID, issuance.ID)
			}
		}
	})
}

// Running it again has to produce the same rows, not double them. This is what makes the
// recompute safe to call from anywhere.
func TestRecomputeIsIdempotent(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		seedIssuance(t, tx, f, "2026-08-15", 250000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("first Recompute: %v", err)
		}
		first := allocationsOf(t, tx, f.account.ID)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("second Recompute: %v", err)
		}
		second := allocationsOf(t, tx, f.account.ID)

		if len(first) != len(second) {
			t.Fatalf("allocation count changed on recompute: %d then %d", len(first), len(second))
		}
		if sumPoints(first) != sumPoints(second) || sumCost(first) != sumCost(second) {
			t.Errorf("totals changed on recompute: %d/%d then %d/%d",
				sumPoints(first), sumCost(first), sumPoints(second), sumCost(second))
		}
	})
}

// A batch entered after the booking that already spent it resolves the shortfall on the
// next recompute, with no rework - the reason the costing is rebuilt rather than appended
// to.
func TestRecomputeResolvesAShortfallWhenTheLateBatchArrives(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 50000, 75000)
		seedIssuance(t, tx, f, "2026-08-15", 200000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		pending := 0
		for _, row := range allocationsOf(t, tx, f.account.ID) {
			if row.BatchID == nil {
				pending += row.Points
			}
		}
		if pending != 150000 {
			t.Fatalf("pending points = %d, want 150000", pending)
		}

		// The purchase is finally recorded.
		seedBatch(t, tx, f, "2026-08-02", 300000, 420000)
		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute after the late batch: %v", err)
		}

		for _, row := range allocationsOf(t, tx, f.account.ID) {
			if row.BatchID == nil {
				t.Errorf("still pending after the batch was entered: %d points", row.Points)
			}
		}
	})
}

// Archiving a batch takes it out of the queue, and the bookings it covered go back to
// being priced against what remains.
func TestRecomputeIgnoresAnArchivedBatch(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		mistake := seedBatch(t, tx, f, "2026-08-01", 200000, 300000)
		good := seedBatch(t, tx, f, "2026-08-02", 200000, 260000)
		seedIssuance(t, tx, f, "2026-08-15", 150000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}
		if got := allocationsOf(t, tx, f.account.ID); got[0].BatchID == nil || *got[0].BatchID != mistake.ID {
			t.Fatalf("expected the first batch to be consumed before it was archived")
		}

		if err := tx.Delete(&mistake).Error; err != nil {
			t.Fatalf("archiving the batch: %v", err)
		}
		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute after archiving: %v", err)
		}

		rows := allocationsOf(t, tx, f.account.ID)
		if len(rows) != 1 || rows[0].BatchID == nil || *rows[0].BatchID != good.ID {
			t.Errorf("after archiving, the booking is priced against %v, want the remaining batch %s",
				rows[0].BatchID, good.ID)
		}
	})
}

// A cancelled booking with no explicit refunded_points gives everything back - what the
// spreadsheet did when it zeroed the row and marked it CANCELADA.
func TestRecomputeReturnsThePointsOfACancelledBooking(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		issuance := seedIssuance(t, tx, f, "2026-08-15", 200000)

		cancelledAt := day("2026-08-20")
		if err := tx.Model(&issuance).Updates(map[string]any{
			"active":       false,
			"cancelled_at": cancelledAt,
		}).Error; err != nil {
			t.Fatalf("cancelling: %v", err)
		}

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		rows := allocationsOf(t, tx, f.account.ID)
		if points := sumPoints(rows); points != 0 {
			t.Errorf("cancelled booking nets to %d points, want 0", points)
		}
		if cost := sumCost(rows); cost != 0 {
			t.Errorf("cancelled booking nets to %d cents, want 0", cost)
		}
	})
}

// A partial refund gives back only what the airline returned, at the cost those points
// carried.
func TestRecomputeHonoursAPartialRefund(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		issuance := seedIssuance(t, tx, f, "2026-08-15", 200000)

		refunded := 37600
		cancelledAt := day("2026-08-20")
		if err := tx.Model(&issuance).Updates(map[string]any{
			"cancelled_at":    cancelledAt,
			"refunded_points": refunded,
		}).Error; err != nil {
			t.Fatalf("refunding: %v", err)
		}

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		rows := allocationsOf(t, tx, f.account.ID)
		if points := sumPoints(rows); points != 200000-refunded {
			t.Errorf("net points = %d, want %d", points, 200000-refunded)
		}
	})
}

// The reason closing exists: a settled month must survive a recompute untouched.
func TestRecomputeLeavesAClosedPeriodAlone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		seedIssuance(t, tx, f, "2026-08-15", 200000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}
		settled := allocationsOf(t, tx, f.account.ID)
		if len(settled) != 1 {
			t.Fatalf("got %d allocations, want 1", len(settled))
		}
		settledID, settledCost := settled[0].ID, settled[0].CostCents

		closePeriod(t, tx, f, "2026-08-01", "2026-08-31")

		// A batch entered retroactively into the closed month would have changed the
		// costing if the month were still open. It must not.
		seedBatch(t, tx, f, "2026-08-02", 500000, 100000)
		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute after closing: %v", err)
		}

		after := allocationsOf(t, tx, f.account.ID)
		if len(after) != 1 {
			t.Fatalf("got %d allocations after closing, want the 1 settled one", len(after))
		}
		if after[0].ID != settledID {
			t.Errorf("the settled slice was rewritten: %s became %s", settledID, after[0].ID)
		}
		if after[0].CostCents != settledCost {
			t.Errorf("the settled cost changed from %d to %d", settledCost, after[0].CostCents)
		}
	})
}

// The month is closed, then the booking is cancelled. The closed month keeps its profit
// and its settlement; the return lands in the open period, at the cost the points had.
func TestRecomputeReversesIntoTheOpenPeriodAfterClosing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		batch := seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		issuance := seedIssuance(t, tx, f, "2026-08-15", 200000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}
		closePeriod(t, tx, f, "2026-08-01", "2026-08-31")

		cancelledAt := day("2026-09-04")
		if err := tx.Model(&issuance).Updates(map[string]any{
			"active":       false,
			"cancelled_at": cancelledAt,
		}).Error; err != nil {
			t.Fatalf("cancelling: %v", err)
		}

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute after the late cancellation: %v", err)
		}

		rows := allocationsOf(t, tx, f.account.ID)
		if len(rows) != 2 {
			t.Fatalf("got %d allocations, want the settled one plus a reversal", len(rows))
		}

		reversal := rows[1]
		if !reversal.OccurredOn.Equal(cancelledAt) {
			t.Errorf("reversal dated %s, want the cancellation's %s",
				reversal.OccurredOn.Format(time.DateOnly), cancelledAt.Format(time.DateOnly))
		}
		if reversal.Points != -200000 {
			t.Errorf("reversal returned %d points, want -200000", reversal.Points)
		}
		// Back to the batch it came from, at the cost it was charged - not as free points.
		if reversal.BatchID == nil || *reversal.BatchID != batch.ID {
			t.Errorf("reversal went to %v, want the original batch %s", reversal.BatchID, batch.ID)
		}
		if reversal.CostCents != -rows[0].CostCents {
			t.Errorf("reversal gave back %d cents, want %d", reversal.CostCents, -rows[0].CostCents)
		}
		// The closed month's own figure is untouched.
		if rows[0].Points != 200000 {
			t.Errorf("the settled slice changed to %d points", rows[0].Points)
		}
	})
}

// A cancellation-only row carries an estorno for a booking the panel never had. It never
// took points out, so it must not be costed as a consumption.
func TestRecomputeSkipsACancellationOnlyRow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		issuance := seedIssuance(t, tx, f, "2026-08-15", 200000)

		if err := tx.Model(&issuance).Update("cancellation_only", true).Error; err != nil {
			t.Fatalf("marking cancellation-only: %v", err)
		}

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		if rows := allocationsOf(t, tx, f.account.ID); len(rows) != 0 {
			t.Errorf("got %d allocations for a cancellation-only row, want none", len(rows))
		}
	})
}

// closePeriod settles a stretch of the account's costing.
func closePeriod(t *testing.T, tx *gorm.DB, f fixture, startsOn, endsOn string) data.InventoryPeriod {
	t.Helper()

	closedAt := time.Now()
	period := data.InventoryPeriod{
		AccountID:  f.account.ID,
		StartsOn:   day(startsOn),
		EndsOn:     day(endsOn),
		ClosedAt:   &closedAt,
		ClosedByID: &f.user.ID,
	}
	mustCreate(t, tx, &period)
	return period
}
