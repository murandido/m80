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

// A booking from a closed month cancelled after the closing. The closed period froze its
// consumption, and the recompute lands the reversal in the open period, dated by the
// cancellation - so the open half has to count the refund by that day as well, not by the
// booking's created_at, or the points come back on the costing and never on the balance.
func TestExpectedBalanceCountsALateRefundOfAClosedBooking(t *testing.T) {
	cases := []struct {
		name     string
		refunded *int // nil is a full cancellation without an explicit refund
		altered  int  // points an alteration in the closed month added to the booking
		want     int
	}{
		{name: "full cancellation", want: 500000},
		{name: "explicit partial refund", refunded: ptr(20000), want: 470000},
		// A full cancellation returns the alteration's points too (refundedPoints).
		{name: "full cancellation of an altered booking", altered: 10000, want: 500000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				f := seedFixture(t, tx)
				seedBatch(t, tx, f, "2026-08-01", 500000, 700000)
				booking := seedIssuance(t, tx, f, "2026-08-10", 50000)
				if c.altered != 0 {
					seedAdjustment(t, tx, f, booking, "2026-08-12", c.altered, 1500)
				}
				if _, err := Close(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"), f.user.ID); err != nil {
					t.Fatalf("closing August: %v", err)
				}

				updates := map[string]any{"active": false, "cancelled_at": day("2026-09-05")}
				if c.refunded != nil {
					updates["refunded_points"] = *c.refunded
				}
				if err := tx.Model(&booking).Updates(updates).Error; err != nil {
					t.Fatalf("cancelling: %v", err)
				}

				balances, err := ExpectedBalances(tx, []string{f.account.ID})
				if err != nil {
					t.Fatalf("ExpectedBalances failed: %v", err)
				}
				if got := balances[f.account.ID]; got != c.want {
					t.Errorf("balance: got %d, want %d", got, c.want)
				}

				// And the costing agrees: what the batches still hold after the recompute is
				// the same figure.
				if err := Recompute(tx, f.account.ID); err != nil {
					t.Fatalf("Recompute: %v", err)
				}
				backed := 0
				for _, row := range allocationsOf(t, tx, f.account.ID) {
					if row.BatchID != nil {
						backed += row.Points
					}
				}
				if got := 500000 - backed; got != c.want {
					t.Errorf("batches less their slices: got %d, want %d", got, c.want)
				}
			})
		})
	}
}

func ptr(value int) *int { return &value }
