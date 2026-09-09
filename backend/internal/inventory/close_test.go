package inventory

import (
	"errors"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// Closing freezes the period: what was computed at the moment of closing is what the
// settlement was based on, and it has to survive every later recompute.
func TestCloseFreezesWhatWasComputed(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		seedIssuance(t, tx, f, "2026-08-15", 200000)

		if _, err := Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID); err != nil {
			t.Fatalf("Close: %v", err)
		}

		settled := allocationsOf(t, tx, f.account.ID)
		if len(settled) != 1 {
			t.Fatalf("got %d allocations, want 1", len(settled))
		}
		settledCost := settled[0].CostCents

		// A cheaper batch backdated AHEAD of the one already in the queue: with the month
		// open the FIFO would take this one first and the booking would cost almost
		// nothing. Dated after the existing batch it would change nothing either way, and
		// the test would pass without the freezing doing any work.
		seedBatch(t, tx, f, "2026-07-15", 500000, 1000)
		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		after := allocationsOf(t, tx, f.account.ID)
		if len(after) != 1 || after[0].CostCents != settledCost {
			t.Errorf("the settled costing changed: %d cents became %v", settledCost, after)
		}
	})
}

// Closing recomputes first: it is the last moment the figures can be made to agree with
// the rows, and freezing a stale computation would preserve the wrong thing.
func TestCloseRecomputesBeforeFreezing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		batch := seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		seedIssuance(t, tx, f, "2026-08-15", 200000)

		// Nothing has ever been computed on this account - Close has to do it itself.
		if rows := allocationsOf(t, tx, f.account.ID); len(rows) != 0 {
			t.Fatalf("expected no allocations before closing, got %d", len(rows))
		}

		if _, err := Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID); err != nil {
			t.Fatalf("Close: %v", err)
		}

		rows := allocationsOf(t, tx, f.account.ID)
		if len(rows) != 1 {
			t.Fatalf("got %d allocations after closing, want 1", len(rows))
		}
		if rows[0].BatchID == nil || *rows[0].BatchID != batch.ID {
			t.Errorf("slice priced against %v, want %s", rows[0].BatchID, batch.ID)
		}
	})
}

// The remainder carries forward on its own, still divided into the batches that hold it -
// each with its own rate and its own funding agency. The next period's first booking is
// priced exactly as it would have been without the closing.
func TestCloseCarriesTheRemainderForwardInItsOwnBatches(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		cheap := seedBatch(t, tx, f, "2026-08-01", 300000, 300000)
		dear := seedBatch(t, tx, f, "2026-08-05", 300000, 600000)
		seedIssuance(t, tx, f, "2026-08-15", 200000)

		if _, err := Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID); err != nil {
			t.Fatalf("Close: %v", err)
		}

		// September takes the 100.000 left of the cheap batch and then bites into the dear
		// one - the same queue August was working through, not a consolidated average.
		seedIssuance(t, tx, f, "2026-09-02", 150000)
		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		rows := allocationsOf(t, tx, f.account.ID)
		if len(rows) != 3 {
			t.Fatalf("got %d allocations, want 3 (August's one plus September's two)", len(rows))
		}

		september := rows[1:]
		if september[0].BatchID == nil || *september[0].BatchID != cheap.ID || september[0].Points != 100000 {
			t.Errorf("first September slice = %v / %d, want the rest of the cheap batch (100000)",
				september[0].BatchID, september[0].Points)
		}
		// 100.000 points at 1,00 per thousand: the cheap batch's own rate, not an average.
		if september[0].CostCents != 100000 {
			t.Errorf("first September slice cost %d cents, want 100000", september[0].CostCents)
		}
		if september[1].BatchID == nil || *september[1].BatchID != dear.ID || september[1].Points != 50000 {
			t.Errorf("second September slice = %v / %d, want the dear batch (50000)",
				september[1].BatchID, september[1].Points)
		}
		// 50.000 at 2,00 per thousand.
		if september[1].CostCents != 100000 {
			t.Errorf("second September slice cost %d cents, want 100000", september[1].CostCents)
		}
	})
}

// Periods settle in order, because each one's starting position is what the last left.
func TestCloseRefusesAPeriodThatEndsBeforeOneAlreadyClosed(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 500000, 700000)

		if _, err := Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID); err != nil {
			t.Fatalf("closing August: %v", err)
		}

		_, err := Close(tx, f.account.ID, day("2026-07-01"), day("2026-07-31"), f.user.ID)
		if !errors.Is(err, ErrPeriodOutOfOrder) {
			t.Errorf("closing July after August returned %v, want ErrPeriodOutOfOrder", err)
		}

		_, err = Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID)
		if !errors.Is(err, ErrPeriodOutOfOrder) {
			t.Errorf("closing August twice returned %v, want ErrPeriodOutOfOrder", err)
		}
	})
}

func TestCloseRefusesAnInvertedPeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)

		_, err := Close(tx, f.account.ID, day("2026-08-31"), day("2026-08-01"), f.user.ID)
		if !errors.Is(err, ErrPeriodInverted) {
			t.Errorf("got %v, want ErrPeriodInverted", err)
		}
	})
}

// Reopening undoes a closing: the period stops being frozen and the costing follows the
// rows again.
func TestReopenPutsThePeriodBackUnderTheRecompute(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		seedIssuance(t, tx, f, "2026-08-15", 200000)

		period, err := Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID)
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
		settled := allocationsOf(t, tx, f.account.ID)[0].CostCents

		// Ahead of the batch already in the queue, so reopening visibly changes the
		// costing - see the note in TestCloseFreezesWhatWasComputed.
		seedBatch(t, tx, f, "2026-07-15", 500000, 1000)

		if err := Reopen(tx, period.ID); err != nil {
			t.Fatalf("Reopen: %v", err)
		}

		after := allocationsOf(t, tx, f.account.ID)
		if len(after) == 0 {
			t.Fatal("no allocations after reopening")
		}
		if after[0].CostCents == settled {
			t.Errorf("costing unchanged at %d cents after reopening; the backdated batch should have been seen", settled)
		}
	})
}

// Periods are a chain, so only the top of the stack comes off. Reopening one from the
// middle would recompute it while the periods above still hold figures derived from the
// old result.
func TestReopenRefusesAPeriodThatIsNotTheLast(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-07-01", 900000, 900000)

		august, err := Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID)
		if err != nil {
			t.Fatalf("closing August: %v", err)
		}
		if _, err := Close(tx, f.account.ID, day("2026-09-01"), day("2026-09-30"), f.user.ID); err != nil {
			t.Fatalf("closing September: %v", err)
		}

		if err := Reopen(tx, august.ID); !errors.Is(err, ErrPeriodNotLast) {
			t.Errorf("reopening August under September returned %v, want ErrPeriodNotLast", err)
		}
	})
}

// Closing and settling are separate acts, so undoing one must not undo the other: a
// settlement records that somebody was paid, and a closing reopened by mistake does not
// unpay them.
func TestReopenLeavesTheSettlementMarkAlone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		batch := seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
		seedIssuance(t, tx, f, "2026-08-15", 200000)

		period, err := Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID)
		if err != nil {
			t.Fatalf("Close: %v", err)
		}

		settledAt := time.Now()
		if err := tx.Model(&batch).Update("settled_at", settledAt).Error; err != nil {
			t.Fatalf("settling: %v", err)
		}

		if err := Reopen(tx, period.ID); err != nil {
			t.Fatalf("Reopen: %v", err)
		}

		var after data.PointBatch
		if err := tx.First(&after, "id = ?", batch.ID).Error; err != nil {
			t.Fatalf("reloading the batch: %v", err)
		}
		if after.SettledAt == nil {
			t.Error("the settlement mark was cleared by reopening")
		}
	})
}
