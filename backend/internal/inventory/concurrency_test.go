package inventory

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// These tests cannot use runInTx. The race they pin only exists between separate
// transactions that commit - two recomputes inside one rolled-back transaction would run on
// one connection, one after the other, and could never interleave. So they seed committed
// rows on the test database and delete every one of them afterwards.

// seedCommittedAccount creates the fixture plus enough activity for a recompute to take a
// while to persist: several batches and a few dozen bookings, a handful of them refunded.
// A single booking would make the race window too narrow to ever hit.
func seedCommittedAccount(t *testing.T, db *gorm.DB) fixture {
	t.Helper()

	f := seedFixture(t, db)
	t.Cleanup(func() { deleteCommittedAccount(t, db, f) })

	seedBatch(t, db, f, "2026-07-01", 2000000, 2600000)
	seedBatch(t, db, f, "2026-07-20", 2000000, 3000000)
	seedBatch(t, db, f, "2026-08-10", 2000000, 2800000)
	for i := 0; i < 40; i++ {
		issuance := seedIssuance(t, db, f, fmt.Sprintf("2026-08-%02d", 1+i%28), 25000+i*1000)
		if i%7 == 0 {
			refunded := issuance.Points
			if err := db.Model(&issuance).Update("refunded_points", refunded).Error; err != nil {
				t.Fatalf("refunding the seeded issuance: %v", err)
			}
		}
	}
	return f
}

// deleteCommittedAccount removes what seedCommittedAccount and the code under test wrote,
// children first so no foreign key stands in the way.
func deleteCommittedAccount(t *testing.T, db *gorm.DB, f fixture) {
	t.Helper()

	statements := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM point_batch_allocations WHERE issuance_id IN (SELECT id FROM issuances WHERE account_id = ?)`, []any{f.account.ID}},
		{`DELETE FROM inventory_periods WHERE account_id = ?`, []any{f.account.ID}},
		{`DELETE FROM issuances WHERE account_id = ?`, []any{f.account.ID}},
		{`DELETE FROM point_batches WHERE account_id = ?`, []any{f.account.ID}},
		{`DELETE FROM accounts WHERE id = ?`, []any{f.account.ID}},
		{`DELETE FROM people WHERE id = ?`, []any{*f.account.HolderID}},
		{`DELETE FROM point_batch_origins WHERE id = ?`, []any{f.origin.ID}},
		{`DELETE FROM users WHERE id = ?`, []any{f.user.ID}},
		{`DELETE FROM agencies WHERE id = ?`, []any{f.agency.ID}},
		{`DELETE FROM loyalty_programs WHERE id = ?`, []any{f.program.ID}},
		{`DELETE FROM airlines WHERE id = ?`, []any{f.airline.ID}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Errorf("cleaning up the committed fixture (%s): %v", statement.sql, err)
		}
	}
}

func allocationCount(t *testing.T, db *gorm.DB, accountID string) int {
	t.Helper()
	return len(allocationsOf(t, db, accountID))
}

// recomputeConcurrently fires the given number of recomputes on one account at once and
// fails the test on any error. The barrier lines them up so they really overlap instead of
// finishing one by one as the goroutines happen to be scheduled.
func recomputeConcurrently(t *testing.T, db *gorm.DB, accountID string, workers int) {
	t.Helper()

	var start, done sync.WaitGroup
	start.Add(1)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			errs <- Recompute(db, accountID)
		}()
	}
	start.Done()
	done.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Recompute: %v", err)
		}
	}
}

// Two recomputes of the same account at once used to leave both results behind: the second
// DELETE waited on the rows the first one removed, never saw the slices the first one
// inserted, and added its own on top. Every booking then cost double on the supplier
// statement - and inside a closing, for good.
func TestConcurrentRecomputesDoNotDuplicateSlices(t *testing.T) {
	db := setupTestDB(t)
	f := seedCommittedAccount(t, db)

	if err := Recompute(db, f.account.ID); err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	want := allocationCount(t, db, f.account.ID)
	wantCost := sumCost(allocationsOf(t, db, f.account.ID))
	if want == 0 {
		t.Fatal("the fixture produced no allocations, so there is nothing to duplicate")
	}

	// Several rounds, because a single one can happen to serialise on its own.
	for round := 0; round < 5; round++ {
		recomputeConcurrently(t, db, f.account.ID, 8)

		rows := allocationsOf(t, db, f.account.ID)
		if len(rows) != want || sumCost(rows) != wantCost {
			t.Fatalf("round %d: %d slices costing %d cents after concurrent recomputes, want %d costing %d",
				round, len(rows), sumCost(rows), want, wantCost)
		}
	}
}

// A closing racing a recompute must freeze one clean computation. The period's slices are
// never revisited, so a duplicate frozen into it would stay there with nothing to say so.
func TestCloseRacingRecomputesFreezesOneComputation(t *testing.T) {
	db := setupTestDB(t)
	f := seedCommittedAccount(t, db)

	if err := Recompute(db, f.account.ID); err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	want := allocationCount(t, db, f.account.ID)
	wantCost := sumCost(allocationsOf(t, db, f.account.ID))

	var start, done sync.WaitGroup
	start.Add(1)
	errs := make(chan error, 9)
	for i := 0; i < 8; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			errs <- Recompute(db, f.account.ID)
		}()
	}
	done.Add(1)
	go func() {
		defer done.Done()
		start.Wait()
		_, err := Close(db, f.account.ID, day("2026-07-01"), day("2026-08-31"), f.user.ID)
		errs <- err
	}()
	start.Done()
	done.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Close/Recompute: %v", err)
		}
	}

	// One more recompute clears anything left in the open period, so whatever surplus is
	// still there afterwards can only be sitting inside the closed one.
	if err := Recompute(db, f.account.ID); err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	rows := allocationsOf(t, db, f.account.ID)
	if len(rows) != want || sumCost(rows) != wantCost {
		t.Fatalf("%d slices costing %d cents after closing under concurrent recomputes, want %d costing %d",
			len(rows), sumCost(rows), want, wantCost)
	}
}

// Two closings of the same stretch at once - a double click, two tabs - must settle it
// once. Each used to check for an earlier period before either had written one, and both
// went through, leaving two closed periods over the same days.
func TestConcurrentClosesSettleThePeriodOnce(t *testing.T) {
	db := setupTestDB(t)
	f := seedCommittedAccount(t, db)

	const attempts = 4
	var start, done sync.WaitGroup
	start.Add(1)
	errs := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			_, err := Close(db, f.account.ID, day("2026-07-01"), day("2026-08-31"), f.user.ID)
			errs <- err
		}()
	}
	start.Done()
	done.Wait()
	close(errs)

	succeeded := 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrPeriodOutOfOrder):
		default:
			t.Fatalf("concurrent Close: %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("%d closings went through, want exactly 1", succeeded)
	}

	var periods int64
	if err := db.Model(&data.InventoryPeriod{}).Where("account_id = ?", f.account.ID).Count(&periods).Error; err != nil {
		t.Fatalf("counting periods: %v", err)
	}
	if periods != 1 {
		t.Errorf("%d periods recorded, want 1", periods)
	}
}
