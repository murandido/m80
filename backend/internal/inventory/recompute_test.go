package inventory

import (
	"fmt"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/businessday"
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

		cancelledAt := instantOn("2026-08-20")
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
		cancelledAt := instantOn("2026-08-20")
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

		cancelledAt := instantOn("2026-09-04")
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
		if !reversal.OccurredOn.Equal(day("2026-09-04")) {
			t.Errorf("reversal dated %s, want the cancellation's 2026-09-04",
				reversal.OccurredOn.Format(time.DateOnly))
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

// A cancellation-only row that carries a refund puts the points back into the queue, so a
// later booking spends them instead of falling short - the costing agreeing with the
// expected balance, which already counts them.
func TestRecomputeReturnsTheRefundOfACancellationOnlyRowToTheQueue(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-09-01", 100000, 150000)
		refund := seedIssuance(t, tx, f, "2026-09-05", 37600)
		if err := tx.Model(&refund).Updates(map[string]any{
			"cancellation_only": true,
			"cancelled_at":      instantOn("2026-09-04"),
			"refunded_points":   37600,
		}).Error; err != nil {
			t.Fatalf("marking the refund: %v", err)
		}
		seedIssuance(t, tx, f, "2026-09-10", 137600)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		rows := allocationsOf(t, tx, f.account.ID)
		for _, row := range rows {
			if row.BatchID == nil {
				t.Fatalf("got a pending slice of %d points while the refund covered the booking", row.Points)
			}
		}
		// 100000 bought + 37600 refunded - 137600 spent.
		if points := sumPoints(rows); points != 137600-37600 {
			t.Errorf("net points = %d, want %d", points, 100000)
		}
		// The batch is spent, so the slices net to exactly what it cost.
		if cost := sumCost(rows); cost != 150000 {
			t.Errorf("net cost = %d cents, want the batch's 150000", cost)
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

// The business runs on São Paulo's calendar, whatever zone the process is in. A booking
// made at 23:30 on the last day of the month is 02:30 UTC of the next one, and when the
// costing read the UTC day it went into the next month's closing - in a different month
// than the supplier report and the sales screen put it in. The same goes for a
// cancellation made that late. This test runs in TZ=UTC (production) and in
// TZ=America/Sao_Paulo (a developer's machine), and must pass in both.
func TestRecomputeCostsALateEveningBookingOnItsSaoPauloDay(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-10-01", 500000, 700000)

		lastEvening := time.Date(2026, 10, 31, 23, 30, 0, 0, businessday.Location)
		late := seedIssuance(t, tx, f, "2026-10-31", 200000)
		if err := tx.Model(&late).UpdateColumn("created_at", lastEvening).Error; err != nil {
			t.Fatalf("stamping the late booking: %v", err)
		}
		// Half an hour after São Paulo midnight, the first day of the next month.
		nextMonth := seedIssuance(t, tx, f, "2026-11-01", 100000)
		if err := tx.Model(&nextMonth).UpdateColumn("created_at",
			time.Date(2026, 11, 1, 0, 30, 0, 0, businessday.Location)).Error; err != nil {
			t.Fatalf("stamping the next month's booking: %v", err)
		}
		// Booked mid-month and cancelled at 23:40 on the 31st: the refund is October's too.
		cancelled := seedIssuance(t, tx, f, "2026-10-10", 50000)
		if err := tx.Model(&cancelled).Updates(map[string]any{
			"active":       false,
			"cancelled_at": time.Date(2026, 10, 31, 23, 40, 0, 0, businessday.Location),
		}).Error; err != nil {
			t.Fatalf("cancelling: %v", err)
		}

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		wantDays := map[string][]string{
			late.ID:      {"2026-10-31"},
			nextMonth.ID: {"2026-11-01"},
			// The booking, then its reversal on the evening it was cancelled.
			cancelled.ID: {"2026-10-10", "2026-10-31"},
		}
		gotDays := map[string][]string{}
		for _, row := range allocationsOf(t, tx, f.account.ID) {
			gotDays[row.IssuanceID] = append(gotDays[row.IssuanceID], row.OccurredOn.Format(time.DateOnly))
		}
		for issuanceID, want := range wantDays {
			if fmt.Sprint(gotDays[issuanceID]) != fmt.Sprint(want) {
				t.Errorf("issuance %s costed on %v, want %v", issuanceID, gotDays[issuanceID], want)
			}
		}

		// Closing October freezes the late booking and the late refund inside it, and the
		// expected balance - which reads the open half straight from created_at - draws
		// the boundary on the same day: nothing is counted twice or lost at the cut.
		closePeriod(t, tx, f, "2026-10-01", "2026-10-31")
		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute after closing: %v", err)
		}
		balances, err := ExpectedBalances(tx, []string{f.account.ID})
		if err != nil {
			t.Fatalf("ExpectedBalances: %v", err)
		}
		if got, want := balances[f.account.ID], 500000-200000-100000; got != want {
			t.Errorf("expected balance after closing October: got %d, want %d", got, want)
		}
	})
}
