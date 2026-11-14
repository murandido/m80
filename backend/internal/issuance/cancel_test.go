package issuance

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func TestCancel_MarksInactiveAndRecordsRefund(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		points, taxes, fee := 800, 1500, 500
		cancelled, err := Cancel(tx, issuance, CancelInput{
			RefundedPoints:  &points,
			RefundedTaxes:   &taxes,
			CancellationFee: &fee,
		}, f.User.ID)
		if err != nil {
			t.Fatalf("Cancel failed: %v", err)
		}

		if cancelled.Active {
			t.Error("expected active=false")
		}
		if cancelled.CancelledAt == nil {
			t.Error("expected cancelled_at to be set")
		}
		if cancelled.CancelledByID == nil || *cancelled.CancelledByID != f.User.ID {
			t.Errorf("cancelled_by_id = %v, want %q", cancelled.CancelledByID, f.User.ID)
		}
		if cancelled.RefundedPoints == nil || *cancelled.RefundedPoints != 800 {
			t.Errorf("refunded_points = %v, want 800", cancelled.RefundedPoints)
		}

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read issuance: %v", err)
		}
		if persisted.Active {
			t.Error("persisted issuance still active")
		}

		assertCount(t, tx, "audit_logs", "issuance_id = ? AND action = 'cancelled'", issuance.ID, 1)
	})
}

func TestCancel_AlreadyCancelledIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if _, err := Cancel(tx, issuance, CancelInput{}, f.User.ID); err != nil {
			t.Fatalf("first Cancel failed: %v", err)
		}

		// The freshly re-read (now inactive) row, exactly as findIssuanceToCancel and
		// the panel's own handler both pass it. The stale-struct case is the next test.
		var fresh data.Issuance
		if err := tx.First(&fresh, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read: %v", err)
		}
		_, err = Cancel(tx, fresh, CancelInput{}, f.User.ID)
		if !errors.Is(err, ErrIssuanceAlreadyCancelled) {
			t.Fatalf("err = %v, want ErrIssuanceAlreadyCancelled", err)
		}
	})
}

// The bot's CANCELAMENTO and the panel both look the issuance up before cancelling, and a
// cancel that commits in between leaves them holding a struct that still says active.
// Cancel used to trust that flag and Save the whole row: a second "cancelled" audit line,
// and the second caller's refunds replacing the first's. The conditional write refuses it.
func TestCancel_AStaleLookupIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})
		stale, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		fee := 15000
		if _, err := Cancel(tx, stale, CancelInput{CancellationFee: &fee}, f.User.ID); err != nil {
			t.Fatalf("first Cancel failed: %v", err)
		}

		points := stale.Points
		_, err = Cancel(tx, stale, CancelInput{RefundedPoints: &points}, f.User.ID)
		if !errors.Is(err, ErrIssuanceAlreadyCancelled) {
			t.Fatalf("err = %v, want ErrIssuanceAlreadyCancelled", err)
		}

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", stale.ID).Error; err != nil {
			t.Fatalf("failed to re-read: %v", err)
		}
		if persisted.CancellationFee == nil || *persisted.CancellationFee != fee || persisted.RefundedPoints != nil {
			t.Errorf("the second cancel rewrote the refunds: fee=%v points=%v", persisted.CancellationFee, persisted.RefundedPoints)
		}
		assertCount(t, tx, "audit_logs", "issuance_id = ? AND action = 'cancelled'", stale.ID, 1)
	})
}

// Cancelling used to reopen the sale, which made sense while paid was a switch. It is now
// the sum of what the client actually handed over, and a cancelled booking does not take
// that money back out of the till - returning it is a negative entry on the statement,
// made when it happens. What flags the pending refund is cancelled_at, which the sales
// screen already renders as the "reaberta por cancelamento" badge.
func TestCancel_LeavesAPaidSaleAlone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if err := tx.Model(&data.Sale{}).Where("issuance_id = ?", issuance.ID).Update("paid", true).Error; err != nil {
			t.Fatalf("failed to mark sale paid: %v", err)
		}

		if _, err := Cancel(tx, issuance, CancelInput{}, f.User.ID); err != nil {
			t.Fatalf("Cancel failed: %v", err)
		}

		var sale data.Sale
		if err := tx.First(&sale, "issuance_id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		if !sale.Paid {
			t.Error("the sale was reopened by the cancellation; the money the client paid is still in the till")
		}

		// And the audit entry no longer claims to have reset anything.
		var log data.AuditLog
		if err := tx.Where("issuance_id = ? AND action = 'cancelled'", issuance.ID).First(&log).Error; err != nil {
			t.Fatalf("failed to read audit log: %v", err)
		}
		var details map[string]any
		if err := json.Unmarshal(log.Details, &details); err != nil {
			t.Fatalf("failed to decode details: %v", err)
		}
		if _, ok := details["sale_paid_reset"]; ok {
			t.Errorf("details still carry sale_paid_reset: %v", details)
		}
	})
}

func TestCancel_NoRefundLeavesFieldsNil(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		cancelled, err := Cancel(tx, issuance, CancelInput{}, f.User.ID)
		if err != nil {
			t.Fatalf("Cancel failed: %v", err)
		}
		if cancelled.RefundedPoints != nil || cancelled.RefundedTaxes != nil || cancelled.CancellationFee != nil {
			t.Error("expected all refund fields to stay nil")
		}
	})
}

// The spreadsheet importer cancels an estorno on the day of the sheet tab it was written
// on, not on the day somebody clicked Importar. Both the column and the audit details have
// to carry that date: a fix that reached only one of them would leave the trail disagreeing
// with the row it describes, and nothing would say which one to believe.
func TestCancel_UsesTheGivenCancelledAt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		sheetDay := time.Date(2025, 11, 4, 15, 0, 0, 0, time.UTC)
		points := 800
		cancelled, err := Cancel(tx, issuance, CancelInput{
			RefundedPoints: &points,
			CancelledAt:    &sheetDay,
		}, f.User.ID)
		if err != nil {
			t.Fatalf("Cancel failed: %v", err)
		}

		if cancelled.CancelledAt == nil || !cancelled.CancelledAt.Equal(sheetDay) {
			t.Errorf("cancelled_at = %v, want %v", cancelled.CancelledAt, sheetDay)
		}

		var log data.AuditLog
		if err := tx.Where("issuance_id = ? AND action = ?", issuance.ID, auditActionCancelled).
			First(&log).Error; err != nil {
			t.Fatalf("loading the audit log: %v", err)
		}
		var details struct {
			CancelledAt time.Time `json:"cancelled_at"`
		}
		if err := json.Unmarshal(log.Details, &details); err != nil {
			t.Fatalf("parsing the audit details: %v", err)
		}
		if !details.CancelledAt.Equal(sheetDay) {
			t.Errorf("audit details cancelled_at = %v, want %v", details.CancelledAt, sheetDay)
		}
	})
}

// The panel and the bot cancel things as they happen, so leaving the field out has to keep
// stamping the clock. This is the guard that says the seam above changed nothing for them.
func TestCancel_WithoutACancelledAtStampsNow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		before := time.Now().Add(-time.Minute)
		cancelled, err := Cancel(tx, issuance, CancelInput{}, f.User.ID)
		if err != nil {
			t.Fatalf("Cancel failed: %v", err)
		}
		after := time.Now().Add(time.Minute)

		if cancelled.CancelledAt == nil {
			t.Fatal("expected cancelled_at to be set")
		}
		if cancelled.CancelledAt.Before(before) || cancelled.CancelledAt.After(after) {
			t.Errorf("cancelled_at = %v, want roughly now", cancelled.CancelledAt)
		}
	})
}
