package inventory

import (
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The plain case: what was bought, less what was issued.
func TestExpectedBalanceIsWhatWasBoughtLessWhatWasIssued(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-09-01", 100000, 150000)
		seedBatch(t, tx, f, "2026-09-05", 50000, 75000)
		seedIssuance(t, tx, f, "2026-09-06", 30000)

		balances, err := ExpectedBalances(tx, []string{f.account.ID})
		if err != nil {
			t.Fatalf("ExpectedBalances failed: %v", err)
		}
		if got := balances[f.account.ID]; got != 120000 {
			t.Errorf("balance: got %d, want 120000", got)
		}
	})
}

// An account whose points were bought from an administrator has no credit side in the
// panel at all, so it gets no answer rather than a figure derived from half a history.
func TestExpectedBalanceSkipsAccountsWithoutInventory(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		if err := tx.Model(&data.Account{}).Where("id = ?", f.account.ID).
			Update("owns_point_inventory", false).Error; err != nil {
			t.Fatalf("turning the inventory off: %v", err)
		}
		seedBatch(t, tx, f, "2026-09-01", 100000, 150000)

		balances, err := ExpectedBalances(tx, []string{f.account.ID})
		if err != nil {
			t.Fatalf("ExpectedBalances failed: %v", err)
		}
		if _, ok := balances[f.account.ID]; ok {
			t.Errorf("an account without its own inventory must have no expected balance, got %d",
				balances[f.account.ID])
		}
	})
}

// A refund puts the points back, and a cancellation-only row never took any out - the same
// two rules the costing timeline follows.
func TestExpectedBalanceGivesBackWhatWasRefunded(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-09-01", 100000, 150000)

		cancelled := seedIssuance(t, tx, f, "2026-09-02", 40000)
		cancelledAt := day("2026-09-03")
		if err := tx.Model(&cancelled).Updates(map[string]any{
			"cancelled_at":    cancelledAt,
			"refunded_points": 40000,
		}).Error; err != nil {
			t.Fatalf("cancelling: %v", err)
		}

		// A row that exists only to carry an estorno for a booking made outside the panel.
		estorno := seedIssuance(t, tx, f, "2026-09-04", 25000)
		if err := tx.Model(&estorno).Updates(map[string]any{
			"cancelled_at":      cancelledAt,
			"cancellation_only": true,
			"refunded_points":   25000,
		}).Error; err != nil {
			t.Fatalf("recording the estorno: %v", err)
		}

		balances, err := ExpectedBalances(tx, []string{f.account.ID})
		if err != nil {
			t.Fatalf("ExpectedBalances failed: %v", err)
		}
		// 100.000 bought, 40.000 issued and given back, and 25.000 that only ever came in.
		if got := balances[f.account.ID]; got != 125000 {
			t.Errorf("balance: got %d, want 125000", got)
		}
	})
}

// What a closed period took is counted once, from the frozen slices - not again from the
// issuances behind them.
func TestExpectedBalanceCountsAClosedPeriodOnlyOnce(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 100000, 150000)
		seedIssuance(t, tx, f, "2026-08-10", 30000)
		// The real Close, which costs the period before freezing it - the test helper only
		// writes the period row, and the frozen slices are half of what is being checked.
		if _, err := Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID); err != nil {
			t.Fatalf("closing August: %v", err)
		}

		seedIssuance(t, tx, f, "2026-09-02", 20000)

		balances, err := ExpectedBalances(tx, []string{f.account.ID})
		if err != nil {
			t.Fatalf("ExpectedBalances failed: %v", err)
		}
		if got := balances[f.account.ID]; got != 50000 {
			t.Errorf("balance: got %d, want 50000", got)
		}
	})
}

// The starting-point recipe, which is how an account with a past joins the panel: close
// everything before the cut while there are no batches yet, then record what the account
// had. The bookings from before must not eat the opening balance - that is the whole
// reason the cut exists.
func TestExpectedBalanceIgnoresWhatHappenedBeforeTheCut(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedIssuance(t, tx, f, "2026-07-15", 500000)
		seedIssuance(t, tx, f, "2026-08-20", 300000)
		if _, err := Close(tx, f.account.ID, day("2026-01-01"), day("2026-08-31"), f.user.ID); err != nil {
			t.Fatalf("closing the past: %v", err)
		}

		// Recorded after the cut, the way the opening balance is.
		seedBatch(t, tx, f, "2026-09-01", 120000, 180000)
		seedIssuance(t, tx, f, "2026-09-05", 20000)

		balances, err := ExpectedBalances(tx, []string{f.account.ID})
		if err != nil {
			t.Fatalf("ExpectedBalances failed: %v", err)
		}
		if got := balances[f.account.ID]; got != 100000 {
			t.Errorf("balance: got %d, want 100000 - the bookings before the cut must not count", got)
		}
	})
}

// The point of the formula: it reads the issuances directly, so a booking recorded since
// the last costing is already in the number. Allocations are deliberately left stale here.
func TestExpectedBalanceDoesNotWaitForTheCosting(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-09-01", 100000, 150000)
		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("first recompute: %v", err)
		}

		// Recorded after the costing ran, so no allocation exists for it yet.
		seedIssuance(t, tx, f, "2026-09-08", 15000)

		balances, err := ExpectedBalances(tx, []string{f.account.ID})
		if err != nil {
			t.Fatalf("ExpectedBalances failed: %v", err)
		}
		if got := balances[f.account.ID]; got != 85000 {
			t.Errorf("balance: got %d, want 85000 - the booking has to count before the next recompute", got)
		}
	})
}

// An account marked for the inventory with nothing recorded yet answers zero, not silence:
// "nothing bought so far" is a balance somebody can act on.
func TestExpectedBalanceIsZeroWithNoBatches(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)

		balances, err := ExpectedBalances(tx, []string{f.account.ID})
		if err != nil {
			t.Fatalf("ExpectedBalances failed: %v", err)
		}
		got, ok := balances[f.account.ID]
		if !ok || got != 0 {
			t.Errorf("balance: got %d (present=%v), want 0", got, ok)
		}
	})
}

func TestExpectedBalancesWithNoAccounts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		balances, err := ExpectedBalances(tx, nil)
		if err != nil {
			t.Fatalf("ExpectedBalances failed: %v", err)
		}
		if len(balances) != 0 {
			t.Errorf("want an empty map, got %v", balances)
		}
	})
}

// Several accounts in one call must each get exactly what they would get alone. The
// account list asks for a whole page of holders at once, and the figure it shows is the
// one compared against the real balance at the closing - so a query that let one
// account's cut, refunds or alterations leak into another's would move money silently.
// The four accounts cover the shapes that matter: no closed period at all, several closed
// periods (plus one still open, which must not count as a cut), a cut that splits an
// issuance's alterations with a cancellation without refund after it, and an account
// outside the inventory, which gets no answer.
func TestExpectedBalancesAgreeAcrossSeveralAccountsInOneCall(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		cancelIssuance := func(issuance data.Issuance, on string, fields map[string]any) {
			t.Helper()
			fields["cancelled_at"] = day(on)
			if err := tx.Model(&issuance).Updates(fields).Error; err != nil {
				t.Fatalf("cancelling: %v", err)
			}
		}

		// No closed period: everything is open. An archived batch and an archived booking
		// (with an alteration of its own) stay out of the figure.
		noPeriod := seedFixture(t, tx)
		seedBatch(t, tx, noPeriod, "2026-09-01", 100000, 150000)
		archivedBatch := seedBatch(t, tx, noPeriod, "2026-09-02", 50000, 75000)
		if err := tx.Delete(&archivedBatch).Error; err != nil {
			t.Fatalf("archiving the batch: %v", err)
		}
		booked := seedIssuance(t, tx, noPeriod, "2026-09-06", 30000)
		seedAdjustment(t, tx, noPeriod, booked, "2026-09-07", 5000, 1500)
		seedAdjustment(t, tx, noPeriod, booked, "2026-09-08", -2000, 1500)
		archivedIssuance := seedIssuance(t, tx, noPeriod, "2026-09-09", 10000)
		seedAdjustment(t, tx, noPeriod, archivedIssuance, "2026-09-10", 3000, 1500)
		if err := tx.Delete(&archivedIssuance).Error; err != nil {
			t.Fatalf("archiving the issuance: %v", err)
		}

		// Two closed periods, so the cut is the later one, and a period opened after them
		// that has not been settled.
		closedTwice := seedFixture(t, tx)
		seedBatch(t, tx, closedTwice, "2026-07-01", 100000, 150000)
		seedIssuance(t, tx, closedTwice, "2026-07-10", 10000)
		if _, err := Close(tx, closedTwice.account.ID, day("2026-07-01"), day("2026-07-31"), closedTwice.user.ID); err != nil {
			t.Fatalf("closing July: %v", err)
		}
		seedIssuance(t, tx, closedTwice, "2026-08-10", 30000)
		if _, err := Close(tx, closedTwice.account.ID, day("2026-08-01"), day("2026-08-31"), closedTwice.user.ID); err != nil {
			t.Fatalf("closing August: %v", err)
		}
		mustCreate(t, tx, &data.InventoryPeriod{
			AccountID: closedTwice.account.ID,
			StartsOn:  day("2026-09-01"),
			EndsOn:    day("2026-09-30"),
		})
		seedIssuance(t, tx, closedTwice, "2026-09-02", 20000)
		refunded := seedIssuance(t, tx, closedTwice, "2026-09-03", 15000)
		cancelIssuance(refunded, "2026-09-04", map[string]any{"refunded_points": 15000})

		// A mid-month cut that falls between an issuance's two alterations, then a
		// cancellation without an explicit refund (which gives back the booking and its
		// alteration) and an estorno-only row after it.
		splitCut := seedFixture(t, tx)
		seedBatch(t, tx, splitCut, "2026-08-01", 200000, 300000)
		beforeCut := seedIssuance(t, tx, splitCut, "2026-08-05", 40000)
		seedAdjustment(t, tx, splitCut, beforeCut, "2026-08-10", 10000, 1500)
		if _, err := Close(tx, splitCut.account.ID, day("2026-08-01"), day("2026-08-15"), splitCut.user.ID); err != nil {
			t.Fatalf("closing the first half of August: %v", err)
		}
		seedAdjustment(t, tx, splitCut, beforeCut, "2026-08-20", 5000, 1500)
		cancelledWithoutRefund := seedIssuance(t, tx, splitCut, "2026-09-03", 30000)
		seedAdjustment(t, tx, splitCut, cancelledWithoutRefund, "2026-09-04", 6000, 1500)
		cancelIssuance(cancelledWithoutRefund, "2026-09-06", map[string]any{})
		estorno := seedIssuance(t, tx, splitCut, "2026-09-05", 7000)
		cancelIssuance(estorno, "2026-09-06", map[string]any{
			"cancellation_only": true,
			"refunded_points":   7000,
		})

		// Outside the inventory: no answer, whatever it has recorded.
		withoutInventory := seedFixture(t, tx)
		if err := tx.Model(&data.Account{}).Where("id = ?", withoutInventory.account.ID).
			Update("owns_point_inventory", false).Error; err != nil {
			t.Fatalf("turning the inventory off: %v", err)
		}
		seedBatch(t, tx, withoutInventory, "2026-09-01", 100000, 150000)
		seedIssuance(t, tx, withoutInventory, "2026-09-02", 10000)

		want := map[string]int{
			// 100.000 bought, 30.000 issued, +5.000 and -2.000 altered.
			noPeriod.account.ID: 67000,
			// 100.000 bought, 40.000 frozen in July and August, 20.000 issued after; the
			// refunded booking nets to zero.
			closedTwice.account.ID: 40000,
			// 200.000 bought, 50.000 frozen (the booking and its first alteration), 5.000
			// altered after the cut, the cancelled booking and its alteration both given
			// back, and 7.000 that only ever came in.
			splitCut.account.ID: 152000,
		}

		ids := []string{
			noPeriod.account.ID,
			closedTwice.account.ID,
			splitCut.account.ID,
			withoutInventory.account.ID,
		}
		together, err := ExpectedBalances(tx, ids)
		if err != nil {
			t.Fatalf("ExpectedBalances for all four: %v", err)
		}
		if len(together) != len(want) {
			t.Errorf("got %d balances, want %d: %v", len(together), len(want), together)
		}

		for _, id := range ids {
			alone, err := ExpectedBalances(tx, []string{id})
			if err != nil {
				t.Fatalf("ExpectedBalances for %s alone: %v", id, err)
			}
			gotTogether, inTogether := together[id]
			gotAlone, inAlone := alone[id]
			if inTogether != inAlone || gotTogether != gotAlone {
				t.Errorf("account %s: together %d (present=%v), alone %d (present=%v)",
					id, gotTogether, inTogether, gotAlone, inAlone)
			}

			wanted, expected := want[id]
			if inTogether != expected || gotTogether != wanted {
				t.Errorf("account %s: got %d (present=%v), want %d (present=%v)",
					id, gotTogether, inTogether, wanted, expected)
			}
		}
	})
}
