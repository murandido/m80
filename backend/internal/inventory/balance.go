package inventory

import (
	"fmt"

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
// in it.
//
// Every figure is one aggregate grouped by account, with each account's own cut joined in
// from the periods, so a page of accounts costs the same six queries as one. It used to
// loop over the accounts with four or five queries each, which is what made the account
// list slow to search: the list asks for every account on the page at once.
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

	// The cut is per account: each one freezes its own periods. MAX(ends_on) over the closed
	// ones is the same date ClosedThrough reads one account at a time. It is a function rather
	// than a value because each query below embeds it as its own subquery; an account with no
	// closed period is simply absent from it, which every query reads as "no cut".
	closedCuts := func() *gorm.DB {
		return db.Model(&data.InventoryPeriod{}).
			Select("account_id, MAX(ends_on) AS cut").
			Where("closed_at IS NOT NULL AND account_id IN ?", owning).
			Group("account_id")
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
	type accountTotal struct {
		AccountID string
		Points    int
	}
	// The inner join leaves out the accounts with no cut, which have nothing frozen.
	var frozen []accountTotal
	err = db.Model(&data.PointBatchAllocation{}).
		Select("issuances.account_id, COALESCE(SUM(point_batch_allocations.points), 0) AS points").
		Joins("JOIN issuances ON issuances.id = point_batch_allocations.issuance_id").
		Joins("JOIN (?) AS cuts ON cuts.account_id = issuances.account_id", closedCuts()).
		Where("point_batch_allocations.occurred_on <= cuts.cut").
		Where("point_batch_allocations.batch_id IS NOT NULL").
		Group("issuances.account_id").
		Scan(&frozen).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.ExpectedBalances: summing frozen allocations: %w", err)
	}
	for _, total := range frozen {
		balances[total.AccountID] -= total.Points
	}

	// And the open half, read from the issuances themselves so it is current to the second.
	// A cancellation-only row never took points out of the account, and a refund puts them
	// back - the same two rules the timeline follows in loadIssuanceActivity.
	//
	// The boundary is the same one the recompute draws, on the UTC day of created_at; an
	// account without a cut (no row in cuts) counts every issuance.
	type openTotal struct {
		AccountID string
		Issued    int
		Refunded  int
	}
	var open []openTotal
	err = db.Model(&data.Issuance{}).
		Select(`issuances.account_id,
		        COALESCE(SUM(CASE WHEN issuances.cancellation_only THEN 0 ELSE issuances.points END), 0) AS issued,
		        COALESCE(SUM(CASE
		            WHEN issuances.refunded_points IS NOT NULL THEN issuances.refunded_points
		            WHEN issuances.cancelled_at IS NOT NULL AND NOT issuances.cancellation_only THEN issuances.points
		            ELSE 0
		        END), 0) AS refunded`).
		Joins("LEFT JOIN (?) AS cuts ON cuts.account_id = issuances.account_id", closedCuts()).
		Where("issuances.account_id IN ?", owning).
		Where("cuts.cut IS NULL OR DATE(issuances.created_at AT TIME ZONE 'UTC') > cuts.cut").
		Group("issuances.account_id").
		Scan(&open).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.ExpectedBalances: summing issuances: %w", err)
	}
	for _, total := range open {
		balances[total.AccountID] += total.Refunded - total.Issued
	}

	adjusted, err := openAdjustedPoints(db, owning, closedCuts)
	if err != nil {
		return nil, err
	}
	for accountID, points := range adjusted {
		balances[accountID] -= points
	}

	return balances, nil
}

// openAdjustedPoints is what each account's alterations took out of the open period, net
// of what they gave back, the same two rules loadIssuanceActivity follows. Each alteration
// counts on its own day. A booking cancelled without an explicit refund returns its
// alterations' points too (refundedPoints), on the same created_at boundary the refund
// above is read by. Accounts with nothing to report are absent from the result.
func openAdjustedPoints(db *gorm.DB, accountIDs []string, closedCuts func() *gorm.DB) (map[string]int, error) {
	type accountTotal struct {
		AccountID string
		Points    int
	}

	var taken []accountTotal
	err := db.Model(&data.IssuanceAdjustment{}).
		Select("issuances.account_id, COALESCE(SUM(issuance_adjustments.points), 0) AS points").
		Joins("JOIN issuances ON issuances.id = issuance_adjustments.issuance_id").
		Joins("LEFT JOIN (?) AS cuts ON cuts.account_id = issuances.account_id", closedCuts()).
		Where("issuances.account_id IN ? AND issuances.deleted_at IS NULL", accountIDs).
		Where("cuts.cut IS NULL OR issuance_adjustments.occurred_on > cuts.cut").
		Group("issuances.account_id").
		Scan(&taken).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.ExpectedBalances: summing adjustments: %w", err)
	}

	var returned []accountTotal
	err = db.Model(&data.IssuanceAdjustment{}).
		Select("issuances.account_id, COALESCE(SUM(issuance_adjustments.points), 0) AS points").
		Joins("JOIN issuances ON issuances.id = issuance_adjustments.issuance_id").
		Joins("LEFT JOIN (?) AS cuts ON cuts.account_id = issuances.account_id", closedCuts()).
		Where("issuances.account_id IN ? AND issuances.deleted_at IS NULL", accountIDs).
		Where("issuances.cancelled_at IS NOT NULL AND issuances.refunded_points IS NULL AND NOT issuances.cancellation_only").
		Where("cuts.cut IS NULL OR DATE(issuances.created_at AT TIME ZONE 'UTC') > cuts.cut").
		Group("issuances.account_id").
		Scan(&returned).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.ExpectedBalances: summing cancelled adjustments: %w", err)
	}

	net := make(map[string]int, len(taken))
	for _, total := range taken {
		net[total.AccountID] += total.Points
	}
	for _, total := range returned {
		net[total.AccountID] -= total.Points
	}
	return net, nil
}
