package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// assertRefusedWith checks a 422 carrying the message the panel shows the operator.
func assertRefusedWith(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	var resp utils.StandardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Message != want {
		t.Errorf("message: got %q, want %q", resp.Message, want)
	}
}

// The production case behind this: operator A has the board panel of an issuance open (its
// row is protected from refreshes), operator B cancels it with a R$ 150 fee, and A - who
// still sees "Incompleta" - clicks "Completa". The PATCH used to clear cancelled_at alone,
// leaving active=false with the refunds in place: the sale went back to charging the full
// price, and neither Cancelar nor Reverter could fix the row from the panel.
func TestUpdateIssuance_StatusOnACancelledIssuanceIsRefused(t *testing.T) {
	for _, status := range []string{"complete", "incomplete"} {
		t.Run(status, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				issuance, user := seedIssuance(t, tx)
				linked := data.Sale{IssuanceID: issuance.ID}
				create(t, tx, &linked)

				if rec := callCancel(tx, issuance.ID, user.ID, `{"cancellation_fee": 15000}`); rec.Code != http.StatusOK {
					t.Fatalf("cancel: got %d (body: %s)", rec.Code, rec.Body.String())
				}

				rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"status":"`+status+`"}`)
				assertRefusedWith(t, rec, "Emissão cancelada: use Reverter cancelamento.")

				var persisted data.Issuance
				if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
					t.Fatalf("failed to re-read issuance: %v", err)
				}
				if persisted.Active || persisted.CancelledAt == nil {
					t.Errorf("issuance half-restored: active=%v cancelled_at=%v", persisted.Active, persisted.CancelledAt)
				}

				amount, err := sale.AmountOf(tx, linked.ID)
				if err != nil {
					t.Fatalf("AmountOf: %v", err)
				}
				if !amount.Cancelled || amount.CancellationFeeCents != 15000 {
					t.Errorf("sale amount: got cancelled=%v fee=%d, want cancelled with the 15000 fee",
						amount.Cancelled, amount.CancellationFeeCents)
				}
			})
		})
	}
}

// status=cancelled used to stamp cancelled_at with active still true, no refund, no audit
// and no canceller. Cancelling has exactly one door, POST /issuances/{id}/cancel. The value
// stays in the schema's oneof so this answers with a message, not a generic 400.
func TestUpdateIssuance_StatusCancelledIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"status":"cancelled"}`)
		assertRefusedWith(t, rec, "Use Cancelar emissão.")

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read issuance: %v", err)
		}
		if !persisted.Active || persisted.CancelledAt != nil {
			t.Errorf("issuance half-cancelled: active=%v cancelled_at=%v", persisted.Active, persisted.CancelledAt)
		}
		assertNoAuditLog(t, tx, issuance.ID, "cancelled")
	})
}

// The board's own status switch on a live issuance keeps working both ways.
func TestUpdateIssuance_StatusOnALiveIssuanceStillToggles(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		for _, step := range []struct {
			status   string
			complete bool
		}{{"complete", true}, {"incomplete", false}} {
			rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"status":"`+step.status+`"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: got %d (body: %s)", step.status, rec.Code, rec.Body.String())
			}
			var persisted data.Issuance
			if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
				t.Fatalf("failed to re-read issuance: %v", err)
			}
			if persisted.Complete != step.complete || !persisted.Active || persisted.CancelledAt != nil {
				t.Errorf("%s: got complete=%v active=%v cancelled_at=%v",
					step.status, persisted.Complete, persisted.Active, persisted.CancelledAt)
			}
		}
	})
}

func assertNoAuditLog(t *testing.T, tx *gorm.DB, issuanceID, action string) {
	t.Helper()
	var count int64
	if err := tx.Model(&data.AuditLog{}).Where("issuance_id = ? AND action = ?", issuanceID, action).Count(&count).Error; err != nil {
		t.Fatalf("failed to count audit logs: %v", err)
	}
	if count != 0 {
		t.Errorf("audit logs %q: got %d, want 0", action, count)
	}
}
