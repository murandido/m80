package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"gorm.io/gorm"
)

func decodeSale(t *testing.T, body []byte) saleSingleResponse {
	t.Helper()
	var resp saleSingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decoding the sale: %v (body: %s)", err, body)
	}
	return resp
}

// The amount to receive is typed as a total and kept as the difference from the computed
// price: the difference survives a later change to the rate, a discount works the same way,
// and typing the computed price back clears it.
func TestSaleAmountToReceiveIsKeptAsADifference(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		target := seedClientSale(t, tx, client.ID, 110_000, time.Now())

		rec := callUpdateSale(t, tx, target.ID, `{"amount_cents":130000}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
		}
		got := decodeSale(t, rec.Body.Bytes()).Data
		if got.AmountCents != 130_000 || got.PriceAdjustmentCents != 20_000 || got.ComputedAmountCents != 110_000 || got.BalanceCents != 130_000 {
			t.Errorf("sale = amount %d adjustment %d computed %d balance %d, want 130000/20000/110000/130000",
				got.AmountCents, got.PriceAdjustmentCents, got.ComputedAmountCents, got.BalanceCents)
		}

		// The rate moves: 1000 points at 1200,00 is 120000, and the +200 rides on top.
		if err := tx.Model(&data.Issuance{}).Where("id = ?", target.IssuanceID).
			Update("value_of_the_thousand", 120_000).Error; err != nil {
			t.Fatalf("changing the rate: %v", err)
		}
		amount, err := sale.AmountOf(tx, target.ID)
		if err != nil || amount.Cents != 140_000 {
			t.Errorf("amount after the rate changed = %d (%v), want 140000", amount.Cents, err)
		}

		// A discount, and then back to the computed price.
		got = decodeSale(t, callUpdateSale(t, tx, target.ID, `{"amount_cents":100000}`).Body.Bytes()).Data
		if got.AmountCents != 100_000 || got.PriceAdjustmentCents != -20_000 {
			t.Errorf("discounted sale = %d / %d, want 100000 / -20000", got.AmountCents, got.PriceAdjustmentCents)
		}
		got = decodeSale(t, callUpdateSale(t, tx, target.ID, `{"amount_cents":120000}`).Body.Bytes()).Data
		if got.PriceAdjustmentCents != 0 {
			t.Errorf("adjustment after typing the computed price = %d, want 0", got.PriceAdjustmentCents)
		}

		if rec := callUpdateSale(t, tx, target.ID, `{"amount_cents":-1}`); rec.Code != http.StatusBadRequest {
			t.Errorf("negative amount: status %d, want 400", rec.Code)
		}
	})
}

// The client's lump sum covers the amount typed, not the computed one.
func TestClientPaymentCoversTheAdjustedAmount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		target := seedClientSale(t, tx, client.ID, 110_000, time.Now())
		if rec := callUpdateSale(t, tx, target.ID, `{"amount_cents":130000}`); rec.Code != http.StatusOK {
			t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
		}

		if rec := callCreateClientPayment(t, tx, clientPaymentBody(client.ID, 130_000)); rec.Code != http.StatusCreated {
			t.Fatalf("client payment status = %d (body: %s)", rec.Code, rec.Body.String())
		}
		if got := paidCentsOf(t, tx, target.ID); got != 130_000 {
			t.Errorf("paid = %d, want the adjusted 130000", got)
		}
		if credit := creditOf(t, tx, client.ID); credit != 0 {
			t.Errorf("credit = %d, want 0", credit)
		}
	})
}

// A cancelled sale's amount is its cancellation fee, and is not typed.
func TestCancelledSaleAmountCannotBeTyped(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		target := seedClientSale(t, tx, client.ID, 110_000, time.Now())
		if err := tx.Model(&data.Issuance{}).Where("id = ?", target.IssuanceID).
			Updates(map[string]any{"active": false, "cancelled_at": time.Now()}).Error; err != nil {
			t.Fatalf("cancelling: %v", err)
		}
		if rec := callUpdateSale(t, tx, target.ID, `{"amount_cents":130000}`); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want 422", rec.Code)
		}
	})
}
