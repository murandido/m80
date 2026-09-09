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
	}
	var rows []row

	// Split in SQL rather than in Go so the whole statement is one query however many
	// bookings the period holds.
	err := db.Model(&data.PointBatchAllocation{}).
		Select(`issuance_id,
		        COALESCE(SUM(CASE WHEN points >= 0 THEN cost_cents ELSE 0 END), 0) AS charged_cents,
		        COALESCE(SUM(CASE WHEN points <  0 THEN cost_cents ELSE 0 END), 0) AS refunded_cents`).
		Where("issuance_id IN ?", issuanceIDs).
		Group("issuance_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.CostByIssuance: %w", err)
	}

	for _, r := range rows {
		costs[r.IssuanceID] = IssuanceCost{ChargedCents: r.ChargedCents, RefundedCents: r.RefundedCents}
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
