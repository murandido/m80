package statement

import (
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// LoadPanel reads what the panel holds for one account over the statement's own period.
//
// The period is the statement's, not a filter the operator chose: comparing a month's
// export against every booking the account ever had would report the whole history as
// missing. The caller derives it from the file - see PeriodOf.
//
// Bookings are read by created_at, the same date the costing and the supplier report use,
// so all three agree about which month a booking is in. A cancelled booking is still read:
// its points left the account and the statement says so, and dropping it would report a
// real redemption as missing.
func LoadPanel(db *gorm.DB, accountID string, from, to time.Time) ([]PanelBooking, []PanelBatch, error) {
	var issuances []data.Issuance
	err := db.Where("account_id = ?", accountID).
		Where("created_at >= ? AND created_at < ?", from, to.AddDate(0, 0, 1)).
		Order("created_at ASC").
		Find(&issuances).Error
	if err != nil {
		return nil, nil, fmt.Errorf("statement.LoadPanel: loading issuances: %w", err)
	}

	bookings := make([]PanelBooking, 0, len(issuances))
	for _, issuance := range issuances {
		// A cancellation-only row never took points out of the account - it exists to
		// carry an estorno for a booking the panel never had. Counting its points would
		// invent a redemption the statement cannot possibly show.
		if issuance.CancellationOnly {
			continue
		}
		booking := PanelBooking{
			IssuanceID: issuance.ID,
			Locator:    issuance.ProgramLocator,
			IssuedOn:   issuance.CreatedAt,
			Points:     issuance.Points,
		}
		if issuance.RefundedPoints != nil {
			booking.RefundedPoints = *issuance.RefundedPoints
		}
		bookings = append(bookings, booking)
	}

	var rows []data.PointBatch
	err = db.Where("account_id = ?", accountID).
		Where("acquired_on >= ? AND acquired_on <= ?", from, to).
		// An opening balance is a remainder carried forward by a closing, not something
		// that ever appeared on a statement. Comparing it would always report it missing.
		Where("opening = FALSE").
		Order("acquired_on ASC").
		Find(&rows).Error
	if err != nil {
		return nil, nil, fmt.Errorf("statement.LoadPanel: loading batches: %w", err)
	}

	batches := make([]PanelBatch, 0, len(rows))
	for _, row := range rows {
		batches = append(batches, PanelBatch{
			BatchID:    row.ID,
			AcquiredOn: row.AcquiredOn,
			Points:     row.Points,
		})
	}

	return bookings, batches, nil
}

// PeriodOf is the span a statement covers: its earliest and latest movement.
//
// Taken from the file rather than asked for, so the comparison can never be run over a
// window the statement does not describe - which would report every booking outside it as
// missing from a file that never claimed to have it.
func PeriodOf(statement Statement) (from, to time.Time, ok bool) {
	if len(statement.Movements) == 0 {
		return time.Time{}, time.Time{}, false
	}
	from, to = statement.Movements[0].OccurredOn, statement.Movements[0].OccurredOn
	for _, movement := range statement.Movements[1:] {
		if movement.OccurredOn.Before(from) {
			from = movement.OccurredOn
		}
		if movement.OccurredOn.After(to) {
			to = movement.OccurredOn
		}
	}
	return from, to, true
}
