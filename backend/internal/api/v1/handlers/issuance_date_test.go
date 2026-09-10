package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// saoPaulo is the zone the panel's dates are written in, and the one every listing
// resolves date_from/date_to in.
func saoPaulo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		t.Fatalf("tzdata is missing from the test image: %v", err)
	}
	return loc
}

// Two rows backdated to the same day have to keep the order they were recorded in: the
// costing breaks a same-day tie by created_at, so flattening the clock would leave their
// FIFO order to the database.
func TestRebaseToDayKeepsTheClock(t *testing.T) {
	t.Parallel()
	loc := saoPaulo(t)

	moment := time.Date(2026, 9, 9, 10, 32, 41, 0, loc)
	day := time.Date(2026, 9, 3, 0, 0, 0, 0, loc)

	moved := rebaseToDay(moment, day)

	if moved.Format("2006-01-02 15:04:05") != "2026-09-03 10:32:41" {
		t.Errorf("got %s, want the 3rd at the same clock", moved.Format("2006-01-02 15:04:05"))
	}
}

// The costing reads the UTC day of created_at, so a late-evening clock kept as it is would
// put the issuance on the following day there - the operator picks the 3rd and the batch of
// the 4th gets charged. Midday is the same calendar day in both zones.
func TestRebaseToDayStaysOnTheChosenDayInUTC(t *testing.T) {
	t.Parallel()
	loc := saoPaulo(t)

	moment := time.Date(2026, 9, 9, 22, 30, 0, 0, loc)
	day := time.Date(2026, 9, 3, 0, 0, 0, 0, loc)

	moved := rebaseToDay(moment, day)

	if moved.UTC().Format("2006-01-02") != "2026-09-03" {
		t.Errorf("UTC day: got %s, want 2026-09-03", moved.UTC().Format("2006-01-02"))
	}
	if moved.Format("2006-01-02") != "2026-09-03" {
		t.Errorf("local day: got %s, want 2026-09-03", moved.Format("2006-01-02"))
	}
}

// created_at comes out of the database in UTC, so the clock has to be read in the target
// zone before it is carried over. Reading it raw moved the time by the offset on every
// correction, compounding with each edit.
func TestRebaseToDayReadsTheClockInTheTargetZone(t *testing.T) {
	t.Parallel()
	loc := saoPaulo(t)

	// 09:00:04 in São Paulo, as the database hands it back.
	moment := time.Date(2026, 8, 1, 12, 0, 4, 0, time.UTC)
	day := time.Date(2026, 9, 3, 0, 0, 0, 0, loc)

	moved := rebaseToDay(moment, day)

	if got := moved.In(loc).Format("2006-01-02 15:04:05"); got != "2026-09-03 09:00:04" {
		t.Errorf("got %s, want the 3rd at 09:00:04 local", got)
	}
}

func callUpdate(tx *gorm.DB, id, userID, body string) *httptest.ResponseRecorder {
	handler := NewIssuanceHandler(tx, events.NewBroker(), nil)

	req := httptest.NewRequest(http.MethodPatch, "/issuances/"+id, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, middleware.UserContextKey, data.User{ID: userID, Role: "admin"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.UpdateIssuance(rec, req)
	return rec
}

// The correction the whole slice exists for: a booking found on a statement and recorded
// afterwards has to end up on the day it happened.
func TestUpdateIssuanceMovesTheDay(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		// Stamped rather than left at now(): the clock this test asserts on has to be one
		// that survives the move, and a row seeded late at night would not. Past 21:00 in
		// São Paulo the same clock falls on the next UTC day, and rebaseToDay drops to
		// midday on purpose so the day the operator picked is the day the costing reads.
		// Left to the wall clock, this test passed by day and failed by night.
		before := time.Date(2026, 8, 1, 9, 0, 4, 0, saoPaulo(t))
		if err := tx.Model(&issuance).UpdateColumn("created_at", before).Error; err != nil {
			t.Fatalf("stamping the issuance: %v", err)
		}

		rec := callUpdate(tx, issuance.ID, user.ID, `{"issued_on": "2026-09-03"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var response struct {
			Data struct {
				IssuedOn string `json:"issued_on"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode the response: %v", err)
		}
		if response.Data.IssuedOn != "2026-09-03" {
			t.Errorf("issued_on in the response: got %q, want 2026-09-03", response.Data.IssuedOn)
		}

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read the issuance: %v", err)
		}
		if got := persisted.CreatedAt.In(saoPaulo(t)).Format("2006-01-02"); got != "2026-09-03" {
			t.Errorf("persisted day: got %s, want 2026-09-03", got)
		}
		// The clock is the row's original one, which is what keeps two rows moved to the
		// same day in the order they were recorded - and it must not drift by the zone
		// offset, which is what reading it raw out of the database used to do.
		if persisted.CreatedAt.In(saoPaulo(t)).Format("15:04:05") != before.In(saoPaulo(t)).Format("15:04:05") {
			t.Errorf("the clock moved: was %s, now %s",
				before.In(saoPaulo(t)).Format("15:04:05"),
				persisted.CreatedAt.In(saoPaulo(t)).Format("15:04:05"))
		}

		// The date is the one field an edit rewrites rather than adds to, so the move is
		// audited even though ordinary cell edits are not.
		var logs int64
		if err := tx.Model(&data.AuditLog{}).
			Where("issuance_id = ? AND action = ?", issuance.ID, auditActionUpdated).
			Count(&logs).Error; err != nil {
			t.Fatalf("failed to count the audit entries: %v", err)
		}
		if logs != 1 {
			t.Errorf("audit entries for the move: got %d, want 1", logs)
		}
	})
}

// seedClosedPeriod marks the issuance's account as owning its point inventory and freezes
// everything up to endsOn, the way Close does.
func seedClosedPeriod(t *testing.T, tx *gorm.DB, accountID, closedByID string, endsOn time.Time) {
	t.Helper()

	if err := tx.Model(&data.Account{}).Where("id = ?", accountID).
		Update("owns_point_inventory", true).Error; err != nil {
		t.Fatalf("failed to put the account on the inventory: %v", err)
	}
	closedAt := time.Now()
	period := data.InventoryPeriod{
		AccountID:  accountID,
		StartsOn:   endsOn.AddDate(0, 0, -30),
		EndsOn:     endsOn,
		ClosedAt:   &closedAt,
		ClosedByID: &closedByID,
	}
	create(t, tx, &period)
}

// Backdating into a closed period is the silent mistake this guard exists for: the
// recompute only replays events after the last closed day, so the booking would never be
// costed at all and nothing on any screen would say why.
func TestUpdateIssuanceRefusesADayInsideAClosedPeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		seedClosedPeriod(t, tx, issuance.AccountID, user.ID, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))

		rec := callUpdate(tx, issuance.ID, user.ID, `{"issued_on": "2026-08-20"}`)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "31/08/2026") {
			t.Errorf("the message has to name the closed date, got: %s", rec.Body.String())
		}

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read the issuance: %v", err)
		}
		if persisted.CreatedAt.In(saoPaulo(t)).Format("2006-01-02") == "2026-08-20" {
			t.Error("the issuance was moved into the closed period anyway")
		}
	})
}

// A date after the last closed day is an ordinary correction, even on an account with a
// frozen period behind it.
func TestUpdateIssuanceAllowsADayAfterTheClosedPeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		seedClosedPeriod(t, tx, issuance.AccountID, user.ID, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))

		rec := callUpdate(tx, issuance.ID, user.ID, `{"issued_on": "2026-09-03"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// Nothing is frozen on an account that does not own its point inventory, so there is
// nothing to protect and any day is a valid answer.
func TestUpdateIssuanceLeavesAccountsWithoutInventoryAlone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		rec := callUpdate(tx, issuance.ID, user.ID, `{"issued_on": "2026-01-05"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read the issuance: %v", err)
		}
		if got := persisted.CreatedAt.In(saoPaulo(t)).Format("2006-01-02"); got != "2026-01-05" {
			t.Errorf("persisted day: got %s, want 2026-01-05", got)
		}
	})
}

// A cancellation recorded a week late still has to date its refund in the week it
// happened: cancelled_at is what puts the points back on the costing's timeline.
func TestCancelIssuanceTakesTheDayItHappened(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		rec := callCancel(tx, issuance.ID, user.ID, `{"refunded_points": 1000, "cancelled_on": "2026-09-05"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read the issuance: %v", err)
		}
		if persisted.CancelledAt == nil {
			t.Fatal("the issuance was not cancelled")
		}
		if got := persisted.CancelledAt.In(saoPaulo(t)).Format("2006-01-02"); got != "2026-09-05" {
			t.Errorf("cancelled on: got %s, want 2026-09-05", got)
		}
	})
}

// The refund is an event on the same timeline, so it gets the same guard.
func TestCancelIssuanceRefusesADayInsideAClosedPeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		seedClosedPeriod(t, tx, issuance.AccountID, user.ID, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))

		rec := callCancel(tx, issuance.ID, user.ID, `{"cancelled_on": "2026-08-20"}`)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read the issuance: %v", err)
		}
		if persisted.CancelledAt != nil {
			t.Error("the issuance was cancelled into the closed period anyway")
		}
	})
}
