package issuance

import (
	"encoding/json"
	"errors"
	"testing"

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

		// Cancel trusts the Active flag on the struct it's handed - it never
		// re-queries - so the guard only bites when the caller passes back the
		// freshly re-read (now inactive) row, exactly as findIssuanceToCancel and
		// the panel's own handler both do right before calling it.
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
