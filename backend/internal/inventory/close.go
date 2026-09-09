package inventory

import (
	"errors"
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

var (
	// ErrPeriodOutOfOrder is a period that ends on or before one already closed. Periods
	// are settled in order because each one's starting position is what the previous one
	// left behind.
	ErrPeriodOutOfOrder = errors.New("inventory: period ends on or before one already closed")
	// ErrPeriodInverted is a period whose end precedes its start.
	ErrPeriodInverted = errors.New("inventory: period ends before it starts")
)

// Close settles a stretch of an account's costing and freezes it.
//
// Everything up to and including endsOn stops being recomputable: those slices are what
// the profit was split on, and a later correction must not reach back and revise a figure
// somebody has already been paid. From here a cancellation of anything inside the period
// lands as a reversal in the open one instead.
//
// The costing is recomputed first, on purpose. Closing is the last moment the figures can
// still be made to agree with the rows, and freezing a stale computation would preserve
// exactly the wrong thing.
//
// Nothing is carried forward as a new batch, and that is deliberate. The remaining balance
// already lives in the batches themselves: each one keeps its own acquisition date, its own
// rate and the agency that funded it, and the next period simply sees what is left of them.
// Consolidating the remainder into one opening batch - the way the spreadsheet's "RESTO
// ULTIMA" had to, having nowhere else to put it - would flatten three things that matter:
// the different rates, whose money bought them, and a batch's ability to come back to life
// when a refund returns its points. The opening flag stays for the one case that genuinely
// needs it: the starting balance typed in when an account joins the panel.
func Close(db *gorm.DB, accountID string, startsOn, endsOn time.Time, closedByID string) (data.InventoryPeriod, error) {
	var period data.InventoryPeriod

	if endsOn.Before(startsOn) {
		return period, ErrPeriodInverted
	}

	frozenUntil, err := ClosedThrough(db, accountID)
	if err != nil {
		return period, err
	}
	if !frozenUntil.IsZero() && !endsOn.After(frozenUntil) {
		return period, ErrPeriodOutOfOrder
	}

	if err := Recompute(db, accountID); err != nil {
		return period, err
	}

	closedAt := time.Now()
	period = data.InventoryPeriod{
		AccountID:  accountID,
		StartsOn:   startsOn,
		EndsOn:     endsOn,
		ClosedAt:   &closedAt,
		ClosedByID: &closedByID,
	}
	if err := db.Create(&period).Error; err != nil {
		return period, fmt.Errorf("inventory.Close: creating the period: %w", err)
	}

	return period, nil
}

// ErrPeriodNotLast is a reopen aimed at a period that is not the most recent closed one.
var ErrPeriodNotLast = errors.New("inventory: only the last closed period can be reopened")

// Reopen undoes a closing, putting the period back under the recompute.
//
// Only the most recent closed period, and that restriction is the whole design. The periods
// are a chain - each one starts from what the last left in the batches - so reopening one
// from the middle would recompute it while the periods stacked on top still hold figures
// derived from the old result, and nothing would say so. Undoing two months means undoing
// two, newest first, seeing each one go.
//
// Deliberately does not touch the batches' settled_at. Closing and settling are separate
// acts: the closing froze the figures, the settlement recorded that somebody was paid, and
// a closing reopened by mistake does not unpay anyone.
func Reopen(db *gorm.DB, periodID string) error {
	var period data.InventoryPeriod
	if err := db.First(&period, "id = ?", periodID).Error; err != nil {
		return fmt.Errorf("inventory.Reopen: loading the period: %w", err)
	}

	var newer int64
	err := db.Model(&data.InventoryPeriod{}).
		Where("account_id = ? AND closed_at IS NOT NULL AND ends_on > ?", period.AccountID, period.EndsOn).
		Count(&newer).Error
	if err != nil {
		return fmt.Errorf("inventory.Reopen: checking for later periods: %w", err)
	}
	if newer > 0 {
		return ErrPeriodNotLast
	}

	if err := db.Delete(&period).Error; err != nil {
		return fmt.Errorf("inventory.Reopen: deleting the period: %w", err)
	}

	// The slices it had frozen are derived data like any other, and the next recompute
	// rebuilds them - now that nothing stops it from reaching them.
	return Recompute(db, period.AccountID)
}
