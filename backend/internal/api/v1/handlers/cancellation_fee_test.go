package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The client always pays the cancellation fee. A cancelled sale nobody paid owes the fee,
// shows on the day of the cancellation, and a lump sum from the client covers it - keeping
// the fee and spending the rest elsewhere.
func TestCancellationFeeIsOwedByTheClient(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		old := time.Now().AddDate(0, 0, -10)
		cancelledSale := seedClientSale(t, tx, client.ID, 50_000, old)
		openSale := seedClientSale(t, tx, client.ID, 20_000, old.Add(time.Hour))

		var issuance data.Issuance
		if err := tx.First(&issuance, "id = ?", cancelledSale.IssuanceID).Error; err != nil {
			t.Fatalf("reading the issuance: %v", err)
		}
		if rec := callCancel(tx, issuance.ID, issuance.IssuerID, `{"refunded_points":1000,"cancellation_fee":15000}`); rec.Code != http.StatusOK {
			t.Fatalf("cancel status = %d (body: %s)", rec.Code, rec.Body.String())
		}

		today := spDay(t, 0)
		listed := findSale(callGetSales(t, tx, "date_from="+today+"&date_to="+today), cancelledSale.ID)
		if listed == nil {
			t.Fatalf("the cancelled sale with a fee is not on the cancellation day")
		}
		if listed.BalanceCents != 15_000 || listed.CancellationFeeCents != 15_000 {
			t.Errorf("cancelled sale balance %d fee %d, want 15000 / 15000", listed.BalanceCents, listed.CancellationFeeCents)
		}

		// R$ 300: the fee takes 150, the open sale the other 150.
		if rec := callCreateClientPayment(t, tx, clientPaymentBody(client.ID, 30_000)); rec.Code != http.StatusCreated {
			t.Fatalf("client payment status = %d (body: %s)", rec.Code, rec.Body.String())
		}
		if got := paidCentsOf(t, tx, cancelledSale.ID); got != 15_000 {
			t.Errorf("paid on the cancelled sale = %d, want the fee 15000", got)
		}
		if got := paidCentsOf(t, tx, openSale.ID); got != 15_000 {
			t.Errorf("paid on the open sale = %d, want 15000", got)
		}

		// A cancellation with nothing to do - no fee, nothing paid - stays off that day.
		quiet := seedClientSale(t, tx, client.ID, 10_000, old)
		if rec := callCancel(tx, quiet.IssuanceID, issuance.IssuerID, `{}`); rec.Code != http.StatusOK {
			t.Fatalf("cancel status = %d", rec.Code)
		}
		if findSale(callGetSales(t, tx, "date_from="+today+"&date_to="+today), quiet.ID) != nil {
			t.Error("a cancellation with nothing to collect or give back showed on the cancellation day")
		}
	})
}
