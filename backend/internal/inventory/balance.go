package inventory

import (
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// ExpectedBalances is how many points the panel believes each of these accounts still has.
//
// Only accounts that own their point inventory get an answer. Everywhere else there is no
// credit side to the account at all - the points were bought from an administrator and the
// panel never saw them arrive - so any figure would be derived from half a history, which
// is worse than an empty field. Those accounts are simply absent from the result.
//
// The obvious formula, batches minus allocations, is not the one used here. Allocations are
// only rebuilt when somebody opens the closing (see the Recompute call in the inventory
// closing handler), so every issuance recorded since would be missing from them and the
// balance would read high until the next visit. What this does instead never goes stale:
//
//	batches
//	  − what closed periods already took (frozen allocations, which by definition never move)
//	  − points issued after the cut
//	  + points refunded after the cut
//
// The open half comes straight from the issuances table, so a booking made a second ago is
// in it. Three aggregates, each grouped by account, so a list of accounts costs the same
// three queries as one.
func ExpectedBalances(db *gorm.DB, accountIDs []string) (map[string]int, error) {
	if len(accountIDs) == 0 {
		return map[string]int{}, nil
	}

	var owning []string
	err := db.Model(&data.Account{}).
		Where("id IN ? AND owns_point_inventory = TRUE", accountIDs).
		Pluck("id", &owning).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.ExpectedBalances: loading accounts: %w", err)
	}
	if len(owning) == 0 {
		return map[string]int{}, nil
	}

	// The cut is per account: each one freezes its own periods.
	frozenUntil := make(map[string]time.Time, len(owning))
	for _, accountID := range owning {
		cut, err := ClosedThrough(db, accountID)
		if err != nil {
			return nil, err
		}
		frozenUntil[accountID] = cut
	}

	balances := make(map[string]int, len(owning))

	type batchTotal struct {
		AccountID string
		Points    int
	}
	var batches []batchTotal
	err = db.Model(&data.PointBatch{}).
		Select("account_id, COALESCE(SUM(points), 0) AS points").
		Where("account_id IN ?", owning).
		Group("account_id").Scan(&batches).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.ExpectedBalances: summing batches: %w", err)
	}
	for _, total := range batches {
		balances[total.AccountID] = total.Points
	}
	// An account with no batch at all still has an answer - zero - rather than no answer.
	// "Nothing recorded yet" is a balance the operator can act on.
	for _, accountID := range owning {
		if _, ok := balances[accountID]; !ok {
			balances[accountID] = 0
		}
	}

	// What the closed periods took, counting only the slices that a batch actually paid
	// for.
	//
	// The pending ones inside a closed period are points that left the account without a
	// batch to draw on - and there is no such batch in the sum above either, so
	// subtracting them would remove stock the panel never had. That is exactly the shape
	// of an account joining the panel with a past: everything before the cut is closed
	// while there are no batches yet, and its bookings freeze as pending. The cut is the
	// line between the history nobody modelled and what is being tracked from here.
	//
	// After the cut the same shortfall is real news, and the open half below counts every
	// issuance - so an unexplained consumption drives the balance negative rather than
	// disappearing quietly.
	type frozenTotal struct {
		AccountID string
		Points    int
	}
	for accountID, cut := range frozenUntil {
		if cut.IsZero() {
			continue
		}
		var frozen frozenTotal
		err = db.Model(&data.PointBatchAllocation{}).
			Select("COALESCE(SUM(point_batch_allocations.points), 0) AS points").
			Joins("JOIN issuances ON issuances.id = point_batch_allocations.issuance_id").
			Where("issuances.account_id = ? AND point_batch_allocations.occurred_on <= ?", accountID, cut).
			Where("point_batch_allocations.batch_id IS NOT NULL").
			Scan(&frozen).Error
		if err != nil {
			return nil, fmt.Errorf("inventory.ExpectedBalances: summing frozen allocations: %w", err)
		}
		balances[accountID] -= frozen.Points
	}

	// And the open half, read from the issuances themselves so it is current to the second.
	// A cancellation-only row never took points out of the account, and a refund puts them
	// back - the same two rules the timeline follows in loadIssuanceActivity.
	type openTotal struct {
		AccountID string
		Issued    int
		Refunded  int
	}
	for accountID, cut := range frozenUntil {
		var open openTotal
		query := db.Model(&data.Issuance{}).
			Select(`COALESCE(SUM(CASE WHEN cancellation_only THEN 0 ELSE points END), 0) AS issued,
			        COALESCE(SUM(CASE
			            WHEN refunded_points IS NOT NULL THEN refunded_points
			            WHEN cancelled_at IS NOT NULL AND NOT cancellation_only THEN points
			            ELSE 0
			        END), 0) AS refunded`).
			Where("account_id = ?", accountID)
		if !cut.IsZero() {
			// The same boundary the recompute draws, on the UTC day of created_at.
			query = query.Where("DATE(created_at AT TIME ZONE 'UTC') > ?", cut)
		}
		if err := query.Scan(&open).Error; err != nil {
			return nil, fmt.Errorf("inventory.ExpectedBalances: summing issuances: %w", err)
		}
		balances[accountID] += open.Refunded - open.Issued

		adjusted, err := openAdjustedPoints(db, accountID, cut)
		if err != nil {
			return nil, err
		}
		balances[accountID] -= adjusted
	}

	return balances, nil
}

// openAdjustedPoints is what the account's alterations took out of the open period, net of
// what they gave back, the same two rules loadIssuanceActivity follows. Each alteration
// counts on its own day. A booking cancelled without an explicit refund returns its
// alterations' points too (refundedPoints), on the same created_at boundary the refund
// above is read by.
func openAdjustedPoints(db *gorm.DB, accountID string, cut time.Time) (int, error) {
	var taken struct{ Points int }
	query := db.Model(&data.IssuanceAdjustment{}).
		Select("COALESCE(SUM(issuance_adjustments.points), 0) AS points").
		Joins("JOIN issuances ON issuances.id = issuance_adjustments.issuance_id").
		Where("issuances.account_id = ? AND issuances.deleted_at IS NULL", accountID)
	if !cut.IsZero() {
		query = query.Where("issuance_adjustments.occurred_on > ?", cut)
	}
	if err := query.Scan(&taken).Error; err != nil {
		return 0, fmt.Errorf("inventory.ExpectedBalances: summing adjustments: %w", err)
	}

	var returned struct{ Points int }
	query = db.Model(&data.IssuanceAdjustment{}).
		Select("COALESCE(SUM(issuance_adjustments.points), 0) AS points").
		Joins("JOIN issuances ON issuances.id = issuance_adjustments.issuance_id").
		Where("issuances.account_id = ? AND issuances.deleted_at IS NULL", accountID).
		Where("issuances.cancelled_at IS NOT NULL AND issuances.refunded_points IS NULL AND NOT issuances.cancellation_only")
	if !cut.IsZero() {
		query = query.Where("DATE(issuances.created_at AT TIME ZONE 'UTC') > ?", cut)
	}
	if err := query.Scan(&returned).Error; err != nil {
		return 0, fmt.Errorf("inventory.ExpectedBalances: summing cancelled adjustments: %w", err)
	}

	return taken.Points - returned.Points, nil
}
