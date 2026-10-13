package statement

import (
	"fmt"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// AdjustedPoints is what a booking's alterations moved through the account
// (data.IssuanceAdjustment). Charged is the extra miles the airline debited, and Returned
// the miles it credited back, both as positive numbers.
//
// The airline puts an alteration's debit on the statement under the same locator as the
// booking, so the statement's total for that locator is the booking plus the charges.
// Comparing it against the booking alone reported a ticket that was altered and recorded
// correctly as a figure that differs.
type AdjustedPoints struct {
	Charged  int
	Returned int
}

// LoadAdjustedPoints sums the live alterations of the given issuances, keyed by issuance.
// An issuance with none is absent, which reads as zero on both sides.
func LoadAdjustedPoints(db *gorm.DB, issuanceIDs []string) (map[string]AdjustedPoints, error) {
	totals := map[string]AdjustedPoints{}
	if len(issuanceIDs) == 0 {
		return totals, nil
	}

	var rows []struct {
		IssuanceID string
		Charged    int
		Returned   int
	}
	err := db.Model(&data.IssuanceAdjustment{}).
		Select(`issuance_id,
		        COALESCE(SUM(points) FILTER (WHERE points > 0), 0) AS charged,
		        COALESCE(-SUM(points) FILTER (WHERE points < 0), 0) AS returned`).
		Where("issuance_id IN ?", issuanceIDs).
		Group("issuance_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("statement.LoadAdjustedPoints: %w", err)
	}
	for _, row := range rows {
		totals[row.IssuanceID] = AdjustedPoints{Charged: row.Charged, Returned: row.Returned}
	}
	return totals, nil
}

// RedeemedPoints is what the statement should show leaving the account for this booking:
// the booking itself plus what its alterations charged.
func (b ResolvedBooking) RedeemedPoints() int {
	return b.Issuance.Points + b.Adjusted.Charged
}

// RefundedPoints is what the statement should show coming back: the cancellation's refund
// plus what the alterations returned.
func (b ResolvedBooking) RefundedPoints() int {
	refunded := b.Adjusted.Returned
	if b.Issuance.RefundedPoints != nil {
		refunded += *b.Issuance.RefundedPoints
	}
	return refunded
}
