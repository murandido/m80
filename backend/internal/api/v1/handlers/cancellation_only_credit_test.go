package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"gorm.io/gorm"
)

// paidOutsideLines counts the lines a cancellation-only estorno wrote on a sale.
func paidOutsideLines(t *testing.T, tx *gorm.DB, saleID string) int64 {
	t.Helper()
	var count int64
	if err := tx.Model(&data.SalePayment{}).
		Where("sale_id = ? AND note = ?", saleID, sale.NotePaidOutsideSystem).
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count the paid-outside lines: %v", err)
	}
	return count
}

// A booking registered only to be estornada was paid by the client outside the panel: the
// cancellation turns that ticket into the client's money, which pays their open sale first
// and leaves the rest as credit.
func TestCancelIssuance_CancellationOnlyBecomesClientCredit(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
		open := seedClientSale(t, tx, client.ID, 4000, base)
		estorno := seedClientSale(t, tx, client.ID, 10000, base.Add(48*time.Hour))
		actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])

		if rec := callCancel(tx, estorno.IssuanceID, actor.ID, `{"cancellation_only":true}`); rec.Code != http.StatusOK {
			t.Fatalf("cancel status: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		if got := paidOutsideLines(t, tx, estorno.ID); got != 1 {
			t.Errorf("paid-outside lines: got %d, want 1", got)
		}
		if got := paidCentsOf(t, tx, open.ID); got != 4000 {
			t.Errorf("open sale: got %d paid, want 4000 from the estorno", got)
		}
		if !isPaid(t, tx, open.ID) {
			t.Error("open sale: want it settled by the estorno")
		}
		if got := creditOf(t, tx, client.ID); got != 6000 {
			t.Errorf("credit: got %d, want 6000", got)
		}
	})
}

// Reverting the cancellation takes the outside payment back, and cancelling again records it
// once - never twice.
func TestUncancelIssuance_RemovesThePaidOutsideLine(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
		open := seedClientSale(t, tx, client.ID, 4000, base)
		estorno := seedClientSale(t, tx, client.ID, 10000, base.Add(48*time.Hour))
		actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])

		if rec := callCancel(tx, estorno.IssuanceID, actor.ID, `{"cancellation_only":true}`); rec.Code != http.StatusOK {
			t.Fatalf("cancel status: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if rec := callUncancel(tx, estorno.IssuanceID, actor.ID); rec.Code != http.StatusOK {
			t.Fatalf("uncancel status: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		if got := paidOutsideLines(t, tx, estorno.ID); got != 0 {
			t.Errorf("after revert, paid-outside lines: got %d, want 0", got)
		}
		if got := paidCentsOf(t, tx, open.ID); got != 0 {
			t.Errorf("after revert, open sale: got %d paid, want 0", got)
		}
		if got := creditOf(t, tx, client.ID); got != 0 {
			t.Errorf("after revert, credit: got %d, want 0", got)
		}

		if rec := callCancel(tx, estorno.IssuanceID, actor.ID, `{"cancellation_only":true}`); rec.Code != http.StatusOK {
			t.Fatalf("second cancel status: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if got := paidOutsideLines(t, tx, estorno.ID); got != 1 {
			t.Errorf("after cancelling again, paid-outside lines: got %d, want 1", got)
		}
		if got := creditOf(t, tx, client.ID); got != 6000 {
			t.Errorf("after cancelling again, credit: got %d, want 6000", got)
		}
	})
}

// A plain cancellation of a booking the panel made claims nothing about money: an unpaid
// ticket stays unpaid and no credit appears.
func TestCancelIssuance_PlainCancellationRecordsNoPayment(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
		open := seedClientSale(t, tx, client.ID, 4000, base)
		cancelled := seedClientSale(t, tx, client.ID, 10000, base.Add(48*time.Hour))
		actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])

		if rec := callCancel(tx, cancelled.IssuanceID, actor.ID, `{}`); rec.Code != http.StatusOK {
			t.Fatalf("cancel status: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		if got := paidCentsOf(t, tx, cancelled.ID); got != 0 {
			t.Errorf("cancelled sale: got %d paid, want 0", got)
		}
		if got := paidCentsOf(t, tx, open.ID); got != 0 {
			t.Errorf("open sale: got %d paid, want 0", got)
		}
		if got := creditOf(t, tx, client.ID); got != 0 {
			t.Errorf("credit: got %d, want 0", got)
		}
	})
}
