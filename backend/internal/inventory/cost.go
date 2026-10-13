package inventory

import (
	"fmt"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// IssuanceCost is what the FIFO costing charged one issuance, split the way the supplier
// statement reads it: a booking line and, when points came back, a refund line.
//
// They are kept apart rather than netted because the statement shows them as two rows -
// the booking in the month it was issued, the refund in the month it was returned - and a
// single net figure could not be placed on either.
type IssuanceCost struct {
	// ChargedCents is what the consumed slices cost, always positive or zero.
	ChargedCents int
	// RefundedCents is what the reversals gave back, always negative or zero.
	RefundedCents int
	// BookingCovered says every point of the booking was charged to a batch. Only then is
	// ChargedCents the cost: a booking still pending - wholly or in part - has no batch
	// saying what those points cost, and zero would claim they were free. A batch that
	// really cost nothing (a flight accrual) still counts as covered, at zero.
	BookingCovered bool
	// RefundCovered is the same question for the reversals.
	RefundCovered bool
}

// CostByIssuance reads the exact cost of each of the given issuances, keyed by issuance id.
//
// This is the figure the supplier statement has to use on an inventory account, in place
// of points x cost_of_the_thousand. The rate on the issuance is a rounded average of
// slices bought at different prices, so multiplying it back out disagrees with the batches
// by a centavo or so per booking - which over a month becomes a discrepancy nobody can
// trace to a line.
//
// An issuance with no allocations is simply absent from the result: it has not been costed
// yet, which is different from having cost nothing.
func CostByIssuance(db *gorm.DB, issuanceIDs []string) (map[string]IssuanceCost, error) {
	costs := map[string]IssuanceCost{}
	if len(issuanceIDs) == 0 {
		return costs, nil
	}

	type row struct {
		IssuanceID    string
		ChargedCents  int
		RefundedCents int
		BookedSlices  int
		BookedPending int
		RefundSlices  int
		RefundPending int
	}
	var rows []row

	// Split in SQL rather than in Go so the whole statement is one query however many
	// bookings the period holds.
	//
	// An alteration's slices are its own lines on the statement (CostByAdjustment), so the
	// booking side leaves them out. The cancellation is the exception: it gives back an
	// alteration's points too, tagged with that alteration for the closing's sake, and those
	// belong on the refund line - which is why the refund side keeps the reversals of a
	// charge (a positive alteration) and drops only an alteration's own refund.
	err := db.Model(&data.PointBatchAllocation{}).
		Select(`point_batch_allocations.issuance_id,
		        COALESCE(SUM(point_batch_allocations.cost_cents) FILTER (WHERE `+bookingSlice+`), 0) AS charged_cents,
		        COALESCE(SUM(point_batch_allocations.cost_cents) FILTER (WHERE `+cancellationSlice+`), 0) AS refunded_cents,
		        COUNT(*) FILTER (WHERE `+bookingSlice+`) AS booked_slices,
		        COUNT(*) FILTER (WHERE `+bookingSlice+` AND point_batch_allocations.batch_id IS NULL) AS booked_pending,
		        COUNT(*) FILTER (WHERE `+cancellationSlice+`) AS refund_slices,
		        COUNT(*) FILTER (WHERE `+cancellationSlice+` AND point_batch_allocations.batch_id IS NULL) AS refund_pending`).
		Joins("LEFT JOIN issuance_adjustments ON issuance_adjustments.id = point_batch_allocations.adjustment_id").
		Where("point_batch_allocations.issuance_id IN ?", issuanceIDs).
		Group("point_batch_allocations.issuance_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.CostByIssuance: %w", err)
	}

	for _, r := range rows {
		costs[r.IssuanceID] = IssuanceCost{
			ChargedCents:   r.ChargedCents,
			RefundedCents:  r.RefundedCents,
			BookingCovered: r.BookedSlices > 0 && r.BookedPending == 0,
			RefundCovered:  r.RefundSlices > 0 && r.RefundPending == 0,
		}
	}
	return costs, nil
}

// The two halves of an issuance's own slices, as SQL over point_batch_allocations joined to
// issuance_adjustments. See CostByIssuance.
const (
	bookingSlice      = `point_batch_allocations.points >= 0 AND point_batch_allocations.adjustment_id IS NULL`
	cancellationSlice = `point_batch_allocations.points < 0 AND (point_batch_allocations.adjustment_id IS NULL OR issuance_adjustments.points > 0)`
)

// AdjustmentCost is what the FIFO costing charged one alteration: the cost of the miles it
// took, or (negative) of the miles it gave back.
type AdjustmentCost struct {
	CostCents int
	// Covered says a batch priced every point, the same question IssuanceCost answers.
	Covered bool
}

// CostByAdjustment reads the exact cost of each of the given alterations, keyed by id. A
// charge counts only the slices it consumed: the cancellation's reversal of those points
// carries its tag too, and belongs on the issuance's refund line (CostByIssuance).
func CostByAdjustment(db *gorm.DB, adjustmentIDs []string) (map[string]AdjustmentCost, error) {
	costs := map[string]AdjustmentCost{}
	if len(adjustmentIDs) == 0 {
		return costs, nil
	}

	var rows []struct {
		AdjustmentID string
		CostCents    int
		Slices       int
		Pending      int
	}
	own := `(issuance_adjustments.points < 0 OR point_batch_allocations.points >= 0)`
	err := db.Model(&data.PointBatchAllocation{}).
		Select(`point_batch_allocations.adjustment_id,
		        COALESCE(SUM(point_batch_allocations.cost_cents) FILTER (WHERE `+own+`), 0) AS cost_cents,
		        COUNT(*) FILTER (WHERE `+own+`) AS slices,
		        COUNT(*) FILTER (WHERE `+own+` AND point_batch_allocations.batch_id IS NULL) AS pending`).
		Joins("JOIN issuance_adjustments ON issuance_adjustments.id = point_batch_allocations.adjustment_id").
		Where("point_batch_allocations.adjustment_id IN ?", adjustmentIDs).
		Group("point_batch_allocations.adjustment_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.CostByAdjustment: %w", err)
	}

	for _, r := range rows {
		costs[r.AdjustmentID] = AdjustmentCost{CostCents: r.CostCents, Covered: r.Slices > 0 && r.Pending == 0}
	}
	return costs, nil
}

// RateOfTheThousand converts a cost back into the per-thousand rate the statement prints,
// rounded to the centavo.
//
// Display only, and the reason IssuanceCost exists: this rounding is exactly what must not
// happen before the amount owed is computed.
func RateOfTheThousand(costCents, points int) int {
	if points == 0 {
		return 0
	}
	if points < 0 {
		points = -points
	}
	if costCents < 0 {
		costCents = -costCents
	}
	return proratedCents(costCents, 1000, points)
}
