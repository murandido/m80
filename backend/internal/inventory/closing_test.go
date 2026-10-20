package inventory

import (
	"testing"

	"github.com/murandido/m-80-backend/internal/data"

	"gorm.io/gorm"
)

// The closing report reads the same figures the settlement is paid on, straight from the
// rows - profit per batch, and what one agency owes the other.
func TestReadClosingReportsProfitAndSettlement(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		// 100 milheiros for R$ 1.000,00 - 10,00 per thousand, sold at 15,00.
		batch := seedBatch(t, tx, f, "2026-08-01", 100000, 100000)
		seedIssuance(t, tx, f, "2026-08-15", 100000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		closing, err := ReadClosing(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"))
		if err != nil {
			t.Fatalf("ReadClosing: %v", err)
		}

		if len(closing.Batches) != 1 || closing.Batches[0].BatchID != batch.ID {
			t.Fatalf("got %d batch lines, want 1 for %s", len(closing.Batches), batch.ID)
		}
		if closing.CostCents != 100000 {
			t.Errorf("cost = %d cents, want 100000", closing.CostCents)
		}
		// 100 milheiros sold at 15,00.
		if closing.RevenueCents != 150000 {
			t.Errorf("revenue = %d cents, want 150000", closing.RevenueCents)
		}
		if closing.ProfitCents != 50000 {
			t.Errorf("profit = %d cents, want 50000", closing.ProfitCents)
		}
		// The funding agency issued it all, so there is nothing to settle.
		if len(closing.Settlements) != 0 {
			t.Errorf("got %v, want no settlement", closing.Settlements)
		}
	})
}

// Points issued against stock nobody has entered a batch for are reported, not hidden -
// and they are the one figure on the report that means something is missing.
func TestReadClosingReportsPendingPoints(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 50000, 75000)
		seedIssuance(t, tx, f, "2026-08-15", 200000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		closing, err := ReadClosing(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"))
		if err != nil {
			t.Fatalf("ReadClosing: %v", err)
		}

		if closing.PendingPoints != 150000 {
			t.Errorf("pending = %d points, want 150000", closing.PendingPoints)
		}
	})
}

// A batch nothing drew on this month has no line: it is not finished, it simply did
// nothing, and its points carry on into the next period.
func TestReadClosingLeavesOutABatchNothingTouched(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		used := seedBatch(t, tx, f, "2026-08-01", 100000, 100000)
		seedBatch(t, tx, f, "2026-08-20", 500000, 700000)
		seedIssuance(t, tx, f, "2026-08-15", 100000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		closing, err := ReadClosing(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"))
		if err != nil {
			t.Fatalf("ReadClosing: %v", err)
		}

		if len(closing.Batches) != 1 || closing.Batches[0].BatchID != used.ID {
			t.Errorf("got %d batch lines, want only the one that was consumed", len(closing.Batches))
		}
	})
}

// A reversal inside the period takes back the revenue it earned as well as the cost, so a
// booking issued and cancelled in the same month contributes nothing to the profit.
func TestReadClosingNetsABookingCancelledInTheSamePeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-01", 500000, 500000)
		issuance := seedIssuance(t, tx, f, "2026-08-15", 100000)

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

		closing, err := ReadClosing(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"))
		if err != nil {
			t.Fatalf("ReadClosing: %v", err)
		}

		if closing.ProfitCents != 0 {
			t.Errorf("profit = %d cents, want 0 - it was issued and cancelled in the period", closing.ProfitCents)
		}
		if closing.CostCents != 0 {
			t.Errorf("cost = %d cents, want 0", closing.CostCents)
		}
	})
}

// Two agencies, one batch: the one that did not fund it owes the points it spent plus its
// share of the profit.
func TestReadClosingSettlesBetweenTwoAgencies(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)

		// The account's batches are funded by the fixture's agency; a second agency issues.
		other := seedAgency(t, tx)
		batch := seedBatch(t, tx, f, "2026-08-01", 200000, 200000)
		if err := tx.Model(&batch).Update("profit_share_percent", 50).Error; err != nil {
			t.Fatalf("setting the split: %v", err)
		}

		issuance := seedIssuance(t, tx, f, "2026-08-15", 100000)
		if err := tx.Model(&issuance).Update("agency_id", other.ID).Error; err != nil {
			t.Fatalf("reassigning the agency: %v", err)
		}

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		closing, err := ReadClosing(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"))
		if err != nil {
			t.Fatalf("ReadClosing: %v", err)
		}

		if len(closing.Settlements) != 1 {
			t.Fatalf("got %d settlements, want 1", len(closing.Settlements))
		}
		settlement := closing.Settlements[0]
		if settlement.AgencyID != other.ID || settlement.ToAgencyID != f.agency.ID {
			t.Errorf("settlement is %s owing %s, want %s owing %s",
				settlement.AgencyID, settlement.ToAgencyID, other.ID, f.agency.ID)
		}
		// 100.000 points at 10,00 cost 100000 cents, sold at 15,00 for 150000: a profit of
		// 50000, half of which stays with the issuer.
		if want := 100000 + 25000; settlement.AmountCents != want {
			t.Errorf("owed = %d cents, want %d", settlement.AmountCents, want)
		}
	})
}

// A single booking big enough to overflow the revenue arithmetic.
//
// The revenue of a slice is points × the sale rate in centavos, and both are integers in
// Postgres: 2.254.000 points at R$ 15,00 the thousand multiply to 3.381.000.000 before the
// division ever happens, which is past what a 32-bit integer holds. The report died with
// "integer out of range" and the screen said only that the closing could not be computed.
//
// It stayed hidden while every batch was small: a booking is charged in slices no larger
// than the batch it draws on, and those never reached the ceiling. The day an account was
// left with no batches at all, the whole booking became one pending row - and the account
// with the largest bookings was exactly the one being repaired.
func TestReadClosingSurvivesABookingLargeEnoughToOverflow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		// No batch: the booking lands as one pending slice, undivided.
		seedIssuance(t, tx, f, "2026-08-15", 2254000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}

		closing, err := ReadClosing(tx, f.account.ID, day("2026-08-01"), day("2026-08-31"))
		if err != nil {
			t.Fatalf("ReadClosing: %v", err)
		}
		if closing.PendingPoints != 2254000 {
			t.Errorf("pending = %d points, want 2254000", closing.PendingPoints)
		}
	})
}

// A booking split across two batches leaves a slice on each, and the batch it drew on
// first lists it among its lines.
func TestReadClosingSplitsABookingAcrossBatches(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		other := seedAgency(t, tx)

		first := seedBatch(t, tx, f, "2026-09-01", 30000, 30000)
		second := seedBatch(t, tx, f, "2026-09-02", 100000, 100000)

		// 40.000 points: 30.000 from the first batch, 10.000 from the second.
		booking := seedIssuance(t, tx, f, "2026-09-10", 40000)
		if err := tx.Model(&booking).Update("agency_id", other.ID).Error; err != nil {
			t.Fatalf("setting the booking: %v", err)
		}

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}
		closing, err := ReadClosing(tx, f.account.ID, day("2026-09-01"), day("2026-09-30"))
		if err != nil {
			t.Fatalf("ReadClosing: %v", err)
		}

		byBatch := map[string]BatchResult{}
		for _, batch := range closing.Batches {
			byBatch[batch.BatchID] = batch
		}
		if got := byBatch[first.ID].ConsumedPoints; got != 30000 {
			t.Errorf("first batch consumed = %d, want 30000", got)
		}
		if got := byBatch[second.ID].ConsumedPoints; got != 10000 {
			t.Errorf("second batch consumed = %d, want 10000", got)
		}
		if lines := byBatch[first.ID].Lines; len(lines) != 1 || lines[0].IssuanceID != booking.ID || lines[0].Points != 30000 {
			t.Errorf("first batch lines = %+v, want the booking's 30.000-point slice", lines)
		}
	})
}

// The period's parts, merged across batches, add up to the settlement - the calculation
// the screen shows has to arrive at the figure that is paid.
func TestReadClosingPartsAddUpToTheSettlement(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx)
		other := seedAgency(t, tx)
		for _, batch := range []data.PointBatch{
			seedBatch(t, tx, f, "2026-09-01", 30000, 30000),
			seedBatch(t, tx, f, "2026-09-02", 100000, 100000),
		} {
			if err := tx.Model(&batch).Update("profit_share_percent", 50).Error; err != nil {
				t.Fatalf("setting the split: %v", err)
			}
		}
		theirs := seedIssuance(t, tx, f, "2026-09-10", 40000)
		if err := tx.Model(&theirs).Update("agency_id", other.ID).Error; err != nil {
			t.Fatalf("setting the booking: %v", err)
		}
		seedIssuance(t, tx, f, "2026-09-11", 20000)

		if err := Recompute(tx, f.account.ID); err != nil {
			t.Fatalf("Recompute: %v", err)
		}
		closing, err := ReadClosing(tx, f.account.ID, day("2026-09-01"), day("2026-09-30"))
		if err != nil {
			t.Fatalf("ReadClosing: %v", err)
		}

		if len(closing.Settlements) != 1 {
			t.Fatalf("settlements = %+v, want one line", closing.Settlements)
		}
		total := closing.Settlements[0]
		if got := signedSum(closing.Parts, total.AgencyID, total.ToAgencyID); got != total.AmountCents {
			t.Errorf("parts add up to %d, want the total %d", got, total.AmountCents)
		}
		if len(closing.Agencies) != 2 {
			t.Errorf("agencies = %+v, want both", closing.Agencies)
		}
	})
}
