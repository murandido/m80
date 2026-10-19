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

// Without a client there is nobody to carry money to, but sales.paid still has to follow
// the new price: a ticket paid and then made dearer is not settled any more.
func TestUpdateIssuance_RepricingReopensAPaidSale(t *testing.T) {
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
		if isPaid(t, tx, paid.ID) {
			t.Error("a sale whose price went up past what was paid should be open again")
		}
	})
}
