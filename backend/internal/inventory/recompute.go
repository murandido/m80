package inventory

import (
	"errors"
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// Recompute rebuilds one account's costing from its batches and its issuances.
//
// Everything in the open period is thrown away and derived again, which is what keeps the
// costing honest while reality arrives out of order - a booking corrected, a batch entered
// a fortnight after the issuance that spent it, a refund landing on a month-old booking.
// Nothing here is incremental, so nothing can drift.
//
// Closed periods are the exception and are never touched. Their slices stay exactly as
// they were settled, and the batches they consumed carry that into the queue as their
// starting position, so reopening a settled figure is not something this function can do
// even by accident.
//
// Safe to call as often as anything wants: the result is a function of the current rows.
func Recompute(db *gorm.DB, accountID string) error {
	frozenUntil, err := closedThrough(db, accountID)
	if err != nil {
		return err
	}

	batches, err := loadBatches(db, accountID, frozenUntil)
	if err != nil {
		return err
	}

	events, err := loadIssuanceActivity(db, accountID, frozenUntil)
	if err != nil {
		return err
	}

	// The slices a closed period settled, so a refund on a booking from a frozen month
	// knows which batches to give back to.
	frozenByIssuance, err := loadFrozenAllocations(db, accountID, frozenUntil)
	if err != nil {
		return err
	}

	allocations := Allocate(batches, events, frozenByIssuance)

	return persist(db, accountID, frozenUntil, allocations)
}

// closedThrough is the last day already settled on this account, or the zero time when
// nothing has been closed yet. Everything strictly after it is open and recomputable.
func closedThrough(db *gorm.DB, accountID string) (time.Time, error) {
	var period data.InventoryPeriod
	err := db.Where("account_id = ? AND closed_at IS NOT NULL", accountID).
		Order("ends_on DESC").First(&period).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("inventory.Recompute: loading closed periods: %w", err)
	}
	return period.EndsOn, nil
}

// loadBatches reads the queue, each batch already carrying what closed periods took from
// it. Archived batches are left out by the soft-delete scope, which is what makes deleting
// a mistaken batch effective: the next recompute re-queues everything it had covered.
func loadBatches(db *gorm.DB, accountID string, frozenUntil time.Time) ([]Batch, error) {
	var rows []data.PointBatch
	err := db.Where("account_id = ?", accountID).
		Order("acquired_on ASC").Order("created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.Recompute: loading batches: %w", err)
	}

	// One query for every batch's frozen position, rather than one per batch.
	type frozenTotal struct {
		BatchID   string
		Points    int
		CostCents int
	}
	var totals []frozenTotal
	if !frozenUntil.IsZero() {
		// Every column is qualified: point_batches carries its own points and cost_cents,
		// so a bare SUM(points) here is ambiguous rather than merely unclear.
		err = db.Model(&data.PointBatchAllocation{}).
			Select(`point_batch_allocations.batch_id AS batch_id,
			        COALESCE(SUM(point_batch_allocations.points), 0) AS points,
			        COALESCE(SUM(point_batch_allocations.cost_cents), 0) AS cost_cents`).
			Joins("JOIN point_batches ON point_batches.id = point_batch_allocations.batch_id").
			Where("point_batches.account_id = ? AND point_batch_allocations.occurred_on <= ?", accountID, frozenUntil).
			Group("point_batch_allocations.batch_id").Scan(&totals).Error
		if err != nil {
			return nil, fmt.Errorf("inventory.Recompute: loading frozen batch totals: %w", err)
		}
	}
	frozen := make(map[string]frozenTotal, len(totals))
	for _, total := range totals {
		frozen[total.BatchID] = total
	}

	batches := make([]Batch, 0, len(rows))
	for i, row := range rows {
		batch := Batch{
			ID:         row.ID,
			AcquiredOn: row.AcquiredOn,
			Points:     row.Points,
			CostCents:  row.CostCents,
			Sequence:   i,
		}
		if total, ok := frozen[row.ID]; ok {
			batch.ConsumedPoints = total.Points
			batch.ChargedCents = total.CostCents
		}
		batches = append(batches, batch)
	}
	return batches, nil
}

// loadIssuanceActivity turns the account's issuances into the timeline of events: a
// booking taking points, and a refund giving them back.
//
// Both go into one list because they belong to one timeline - points refunded on the 16th
// are stock again on the 18th. Sequence keeps same-day events in the order they happened,
// and puts a booking ahead of a refund dated the same day only when its created_at says so.
//
// created_at is the date, the same one the supplier report and the sales screen use, so the
// costing and the payment summary never disagree about which month a booking is in.
func loadIssuanceActivity(db *gorm.DB, accountID string, frozenUntil time.Time) ([]Event, error) {
	var rows []data.Issuance
	err := db.Where("account_id = ?", accountID).
		Order("created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.Recompute: loading issuances: %w", err)
	}

	events := make([]Event, 0, len(rows))

	for i, row := range rows {
		issuedOn := dateOf(row.CreatedAt)

		// A cancellation-only row never took points out of the account - it exists to
		// carry an estorno for a booking the panel never had (see Issuance.CancellationOnly).
		// Costing it as a consumption would invent stock that was never spent.
		if !row.CancellationOnly && issuedOn.After(frozenUntil) {
			events = append(events, Event{
				IssuanceID: row.ID,
				OccurredOn: issuedOn,
				Points:     row.Points,
				Sequence:   i,
			})
		}

		returned := refundedPoints(row)
		if returned <= 0 {
			continue
		}

		// Dated by the cancellation, which is what puts a reversal in the open period
		// even when the booking it reverses sits in a closed one.
		returnedOn := issuedOn
		if row.CancelledAt != nil {
			returnedOn = dateOf(*row.CancelledAt)
		}
		if returnedOn.After(frozenUntil) {
			// len(rows) offsets the refund past every booking of the same day, so a
			// same-day cancel-and-rebook takes the points back before spending them.
			events = append(events, Event{
				IssuanceID: row.ID,
				OccurredOn: returnedOn,
				Points:     -returned,
				Sequence:   len(rows) + i,
			})
		}
	}

	return events, nil
}

// refundedPoints is how many points an issuance gave back.
//
// An explicit refunded_points wins, because somebody typed what the airline actually
// returned. Without one, a cancelled booking is taken to have returned all of it - which
// is what the spreadsheet did when it zeroed the row and marked it CANCELADA.
func refundedPoints(issuance data.Issuance) int {
	if issuance.RefundedPoints != nil {
		return *issuance.RefundedPoints
	}
	if issuance.CancelledAt != nil && !issuance.CancellationOnly {
		return issuance.Points
	}
	return 0
}

// loadFrozenAllocations reads the slices a closed period settled, keyed by issuance, so a
// refund arriving now can see what that booking had already been charged.
func loadFrozenAllocations(db *gorm.DB, accountID string, frozenUntil time.Time) (map[string][]Allocation, error) {
	byIssuance := map[string][]Allocation{}
	if frozenUntil.IsZero() {
		return byIssuance, nil
	}

	var rows []data.PointBatchAllocation
	err := db.Joins("JOIN issuances ON issuances.id = point_batch_allocations.issuance_id").
		Where("issuances.account_id = ? AND point_batch_allocations.occurred_on <= ?", accountID, frozenUntil).
		Order("point_batch_allocations.occurred_on ASC").
		Order("point_batch_allocations.created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.Recompute: loading frozen allocations: %w", err)
	}

	for _, row := range rows {
		batchID := ""
		if row.BatchID != nil {
			batchID = *row.BatchID
		}
		byIssuance[row.IssuanceID] = append(byIssuance[row.IssuanceID], Allocation{
			IssuanceID: row.IssuanceID,
			BatchID:    batchID,
			OccurredOn: row.OccurredOn,
			Points:     row.Points,
			CostCents:  row.CostCents,
		})
	}
	return byIssuance, nil
}

// persist swaps the open period's slices for the ones just computed, in one transaction so
// no reader ever sees the account half-costed.
func persist(db *gorm.DB, accountID string, frozenUntil time.Time, allocations []Allocation) error {
	return db.Transaction(func(tx *gorm.DB) error {
		deletion := tx.Where(
			"issuance_id IN (SELECT id FROM issuances WHERE account_id = ?)", accountID,
		)
		if !frozenUntil.IsZero() {
			deletion = deletion.Where("occurred_on > ?", frozenUntil)
		}
		if err := deletion.Delete(&data.PointBatchAllocation{}).Error; err != nil {
			return fmt.Errorf("inventory.Recompute: clearing open allocations: %w", err)
		}

		if len(allocations) == 0 {
			return nil
		}

		rows := make([]data.PointBatchAllocation, 0, len(allocations))
		for _, allocation := range allocations {
			row := data.PointBatchAllocation{
				IssuanceID: allocation.IssuanceID,
				OccurredOn: allocation.OccurredOn,
				Points:     allocation.Points,
				CostCents:  allocation.CostCents,
			}
			if allocation.BatchID != "" {
				batchID := allocation.BatchID
				row.BatchID = &batchID
			}
			rows = append(rows, row)
		}

		if err := tx.CreateInBatches(&rows, 200).Error; err != nil {
			return fmt.Errorf("inventory.Recompute: writing allocations: %w", err)
		}
		return nil
	})
}

// dateOf drops the clock from a timestamp. The queue is ordered by day - a booking's
// created_at is UTC in the database, and letting the time of day into the comparison would
// make the same booking fall on either side of a period boundary depending on the reader.
func dateOf(moment time.Time) time.Time {
	return time.Date(moment.Year(), moment.Month(), moment.Day(), 0, 0, 0, 0, time.UTC)
}
