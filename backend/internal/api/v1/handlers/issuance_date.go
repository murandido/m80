package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/inventory"
	"gorm.io/gorm"
)

// issuanceDateLayout is how the panel sends the day an issuance or a cancellation happened
// on. A plain date: the operator is saying which day it belongs to, not what time it was.
const issuanceDateLayout = "2006-01-02"

// parseIssuanceDay reads that date as a local calendar day.
//
// The same zone every listing already resolves date_from/date_to in (see parseDateRange):
// "03/09" has to mean the operator's third of September on every screen, or the row lands
// in one month on the grid and in another on the closing.
func parseIssuanceDay(value string) (time.Time, error) {
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		loc = time.UTC
	}
	day, err := time.ParseInLocation(issuanceDateLayout, value, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q, expected AAAA-MM-DD", value)
	}
	return day, nil
}

// localDayOf is a timestamp as the operator's calendar day, the form the panel edits.
func localDayOf(moment time.Time) string {
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		loc = time.UTC
	}
	return moment.In(loc).Format(issuanceDateLayout)
}

// rebaseToDay moves a timestamp onto another calendar day, keeping the time of day.
//
// Keeping the clock is what preserves the order of two rows recorded in the same session:
// the costing breaks a tie between same-day events by created_at
// (internal/inventory/recompute.go), so flattening every backdated row to one instant
// would leave their FIFO order to whatever the database happened to return.
//
// The exception is the late evening. The costing reads the *UTC* day of created_at
// (inventory.DayOf), so a row kept at 22:30 in São Paulo would land on the following day
// there - the operator picks the 3rd and the batch of the 4th gets charged. When the clock
// would cross that boundary, midday is used instead: it is the same calendar day in both
// zones, which is the whole point of the correction.
func rebaseToDay(moment, day time.Time) time.Time {
	// Read in the target zone, not as it comes out of the database. created_at arrives in
	// UTC, and taking its hour as if it were local moved the clock by the offset on every
	// correction - three hours per edit, compounding.
	local := moment.In(day.Location())
	rebased := time.Date(
		day.Year(), day.Month(), day.Day(),
		local.Hour(), local.Minute(), local.Second(), local.Nanosecond(),
		day.Location(),
	)
	if rebased.UTC().Day() != rebased.Day() {
		return time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, day.Location())
	}
	return rebased
}

// ErrClosedInventoryPeriod is a date that falls inside a period whose costing was frozen.
var ErrClosedInventoryPeriod = errors.New("the inventory period covering that date is closed")

// refuseClosedPeriod reports the last closed day when any of these dates falls inside a
// frozen period on the account, and the zero time when they are all free to be recorded.
//
// Without this the mistake is silent, which is what makes it worth a check. The recompute
// only replays events after the last closed day (loadIssuanceActivity's frozenUntil), so a
// booking backdated into a closed month is never costed at all: its points stay in the
// stock, the issuance shows no cost, and nothing on any screen says why.
//
// Only accounts that own their point inventory have anything frozen; everywhere else a
// date is just a date.
func refuseClosedPeriod(db *gorm.DB, accountID string, days ...time.Time) (time.Time, error) {
	var account data.Account
	if err := db.Select("id", "owns_point_inventory").
		First(&account, "id = ?", accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Not this check's business: the account is validated where it is set, and
			// answering "closed" for a missing account would be the wrong sentence.
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	if !account.OwnsPointInventory {
		return time.Time{}, nil
	}

	frozenUntil, err := inventory.ClosedThrough(db, accountID)
	if err != nil {
		return time.Time{}, err
	}
	if frozenUntil.IsZero() {
		return time.Time{}, nil
	}

	for _, day := range days {
		if day.IsZero() {
			continue
		}
		// The same comparison the recompute makes, on the same UTC day, so the guard and
		// the costing can never disagree about which side of the boundary a date is on.
		if !inventory.DayOf(day).After(frozenUntil) {
			return frozenUntil, nil
		}
	}
	return time.Time{}, nil
}

// recordIssuanceDateChange writes the move to the audit log.
//
// The date is the one field an edit rewrites rather than adds to: created_at is also the
// row's own record of when it was typed. That record survives in the "created" entry,
// whose timestamp nobody touches, and this says who moved the day and from where.
//
// Ordinary cell edits are not audited - the grid autosaves on every keystroke's worth of
// change and would bury the log - so this is deliberately the exception.
func recordIssuanceDateChange(tx *gorm.DB, r *http.Request, issuanceID string, from, to time.Time) error {
	actor, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	if !ok {
		return nil
	}

	// The same before/after shape the account merge writes, so whatever reads the history
	// reads both without a second case.
	details, err := json.Marshal(map[string]any{
		"before": map[string]any{"issued_on": localDayOf(from)},
		"after":  map[string]any{"issued_on": localDayOf(to)},
		"reason": "issuance_date_correction",
	})
	if err != nil {
		return err
	}

	return tx.Create(&data.AuditLog{
		IssuanceID:  issuanceID,
		ChangedByID: actor.ID,
		Action:      auditActionUpdated,
		Details:     details,
	}).Error
}

// closedPeriodMessage is what the operator reads, and it names the way out.
func closedPeriodMessage(frozenUntil time.Time) string {
	return fmt.Sprintf(
		"O período até %s está fechado nessa conta. Reabra o período para lançar nessa data.",
		frozenUntil.Format("02/01/2006"),
	)
}
