package handlers

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The production case behind this: two tickets paid by the switch, then R$ 69,90 moved from
// one to the other by editing the issuances in the grid. The edit has to reach the sales and
// the client's replay right away - before, both stayed "Pago" on their old prices, with a
// surplus on one and a missing amount on the other that nobody saw.
func TestUpdateIssuance_RepricingReachesTheSalesAndTheClient(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
		cheaper := seedClientSale(t, tx, client.ID, 71505, base)
		dearer := seedClientSale(t, tx, client.ID, 79414, base.Add(time.Minute))
		settleByHand(t, tx, cheaper.ID)
		settleByHand(t, tx, dearer.ID)

		actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])
		// 1000 points, so the rate per thousand is the price.
		for issuanceID, rate := range map[string]int{cheaper.IssuanceID: 64515, dearer.IssuanceID: 86404} {
			rec := callPatchIssuance(tx, issuanceID, actor.ID, `{"value_of_the_thousand":`+strconv.Itoa(rate)+`}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("patch status: got %d (body: %s)", rec.Code, rec.Body.String())
			}
		}

		if got := paidCentsOf(t, tx, cheaper.ID); got != 64515 {
			t.Errorf("cheaper sale: got %d, want 64515", got)
		}
		if got := paidCentsOf(t, tx, dearer.ID); got != 86404 {
			t.Errorf("dearer sale: got %d, want 86404 (its own 79414 plus the 6990 surplus)", got)
		}
		if !isPaid(t, tx, cheaper.ID) || !isPaid(t, tx, dearer.ID) {
			t.Error("both sales should be settled")
		}
		if got := creditOf(t, tx, client.ID); got != 0 {
			t.Errorf("credit: got %d, want 0", got)
		}
	})
}

// "Pago" means the client paid for the ticket, so correcting its price does not reopen it:
// the switch's line follows the new figure. Without a client too, where there is nobody to
// carry money to.
func TestUpdateIssuance_RepricingKeepsAHandPaidSalePaid(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		paid := seedClientSale(t, tx, client.ID, 10000, time.Now().Add(-time.Hour))
		settleByHand(t, tx, paid.ID)
		if rec := callUpdateSale(t, tx, paid.ID, `{"client_id":null}`); rec.Code != http.StatusOK {
			t.Fatalf("unlink status: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])
		if rec := callPatchIssuance(tx, paid.IssuanceID, actor.ID, `{"value_of_the_thousand":12000}`); rec.Code != http.StatusOK {
			t.Fatalf("patch status: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if !isPaid(t, tx, paid.ID) {
			t.Error("a sale paid by hand should stay paid after its price went up")
		}
		if got := paidCentsOf(t, tx, paid.ID); got != 12000 {
			t.Errorf("paid: got %d, want the new price 12000", got)
		}
	})
}

// The same through the amount typed in the payments control, both ways: up is not a new
// debt, down is not credit - and the switch still works on the sale afterwards.
func TestUpdateSale_RepricingKeepsAHandPaidSalePaid(t *testing.T) {
	for _, newPrice := range []int{12000, 8000} {
		t.Run(strconv.Itoa(newPrice), func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				client := seedClient(t, tx)
				paid := seedClientSale(t, tx, client.ID, 10000, time.Now().Add(-time.Hour))
				settleByHand(t, tx, paid.ID)

				setPrice(t, tx, paid.ID, strconv.Itoa(newPrice))

				if !isPaid(t, tx, paid.ID) {
					t.Error("a sale paid by hand should stay paid after its price changed")
				}
				if got := paidCentsOf(t, tx, paid.ID); got != newPrice {
					t.Errorf("paid: got %d, want the new price %d", got, newPrice)
				}
				if got := creditOf(t, tx, client.ID); got != 0 {
					t.Errorf("credit: got %d, want 0", got)
				}
				if rec := callUpdateSale(t, tx, paid.ID, `{"paid":false}`); rec.Code != http.StatusOK {
					t.Errorf("switch off: got %d (body: %s)", rec.Code, rec.Body.String())
				}
			})
		})
	}
}

// A sale paid in part by a client's payment is where their money ran out, so a new price
// reorganizes it instead: dearer, it still owes the difference; cheaper than what it got,
// it is paid and the rest is the client's credit.
func TestUpdateSale_RepricingReorganizesAPartlyPaidSale(t *testing.T) {
	for _, tc := range []struct {
		price  int
		paid   int
		settle bool
		credit int
	}{
		{price: 12000, paid: 6000, settle: false, credit: 0},
		{price: 5000, paid: 5000, settle: true, credit: 1000},
	} {
		t.Run(strconv.Itoa(tc.price), func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				client := seedClient(t, tx)
				partial := seedClientSale(t, tx, client.ID, 10000, time.Now().Add(-time.Hour))
				if rec := callCreateClientPayment(t, tx, clientPaymentBody(client.ID, 6000)); rec.Code != http.StatusCreated {
					t.Fatalf("client payment status: got %d (body: %s)", rec.Code, rec.Body.String())
				}

				setPrice(t, tx, partial.ID, strconv.Itoa(tc.price))

				if got := paidCentsOf(t, tx, partial.ID); got != tc.paid {
					t.Errorf("paid: got %d, want %d", got, tc.paid)
				}
				if got := isPaid(t, tx, partial.ID); got != tc.settle {
					t.Errorf("settled: got %v, want %v", got, tc.settle)
				}
				if got := creditOf(t, tx, client.ID); got != tc.credit {
					t.Errorf("credit: got %d, want %d", got, tc.credit)
				}
			})
		})
	}
}
