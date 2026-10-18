package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"gorm.io/gorm"
)

// settleByHand marks a sale paid with the switch, which records a line on the sale itself
// for whatever is missing.
func settleByHand(t *testing.T, tx *gorm.DB, saleID string) {
	t.Helper()
	if rec := callUpdateSale(t, tx, saleID, `{"paid":true}`); rec.Code != http.StatusOK {
		t.Fatalf("switch status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func setPrice(t *testing.T, tx *gorm.DB, saleID, amountCents string) {
	t.Helper()
	if rec := callUpdateSale(t, tx, saleID, `{"amount_cents":`+amountCents+`}`); rec.Code != http.StatusOK {
		t.Fatalf("price status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func lineOnSaleFrom(t *testing.T, tx *gorm.DB, saleID, sourceSaleID string) data.SalePayment {
	t.Helper()
	var line data.SalePayment
	if err := tx.First(&line, "sale_id = ? AND credit_from_sale_id = ?", saleID, sourceSaleID).Error; err != nil {
		t.Fatalf("failed to read the credit line: %v", err)
	}
	return line
}

// The case that started this: two tickets paid by hand, then R$ 69,90 of taxes moved from
// one to the other. The cheaper one's surplus pays the dearer one - nobody owes anything,
// nothing is to be handed back.
func TestSurplus_PaysTheClientsOtherSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
		cheaper := seedClientSale(t, tx, client.ID, 71505, base)
		dearer := seedClientSale(t, tx, client.ID, 79414, base.Add(time.Minute))
		settleByHand(t, tx, cheaper.ID)
		settleByHand(t, tx, dearer.ID)

		setPrice(t, tx, cheaper.ID, "64515")
		setPrice(t, tx, dearer.ID, "86404")

		if got := paidCentsOf(t, tx, cheaper.ID); got != 64515 {
			t.Errorf("cheaper sale: got %d, want its new price 64515", got)
		}
		if got := paidCentsOf(t, tx, dearer.ID); got != 86404 {
			t.Errorf("dearer sale: got %d, want its new price 86404", got)
		}
		if !isPaid(t, tx, cheaper.ID) || !isPaid(t, tx, dearer.ID) {
			t.Error("both sales should be settled")
		}
		if got := creditOf(t, tx, client.ID); got != 0 {
			t.Errorf("credit: got %d, want 0", got)
		}
		if got := lineOnSaleFrom(t, tx, dearer.ID, cheaper.ID).AmountCents; got != 6990 {
			t.Errorf("credit line on the dearer sale: got %d, want 6990", got)
		}

		// The listing counts the carried credit as the client's money, so the switch is
		// locked on the dearer sale and the row says why.
		listed := decodeSale(t, callUpdateSale(t, tx, dearer.ID, `{"note":"x"}`).Body.Bytes()).Data
		if listed.ClientPaymentCents != 6990 || listed.BalanceCents != 0 {
			t.Errorf("dearer row: client payment %d, balance %d", listed.ClientPaymentCents, listed.BalanceCents)
		}
	})
}

// With nothing open the surplus is credit, and the next ticket linked to the client takes it.
func TestSurplus_WithNothingOpenIsCreditForTheNextTicket(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		paid := seedClientSale(t, tx, client.ID, 10000, base)
		settleByHand(t, tx, paid.ID)
		setPrice(t, tx, paid.ID, "7000")

		if got := paidCentsOf(t, tx, paid.ID); got != 7000 {
			t.Errorf("cheaper sale: got %d, want 7000 (no refund due)", got)
		}
		if !isPaid(t, tx, paid.ID) {
			t.Error("the cheaper sale should read as paid, not as a refund")
		}
		if got := creditOf(t, tx, client.ID); got != 3000 {
			t.Fatalf("credit: got %d, want 3000", got)
		}

		next := seedClientSale(t, tx, client.ID, 5000, base.Add(24*time.Hour))
		// The sale is created linked; the replay runs on the next action on the client.
		setPrice(t, tx, next.ID, "5000")
		if got := paidCentsOf(t, tx, next.ID); got != 3000 {
			t.Errorf("next ticket: got %d, want the 3000 of credit", got)
		}
		if got := creditOf(t, tx, client.ID); got != 0 {
			t.Errorf("credit after the next ticket: got %d, want 0", got)
		}
	})
}

// A cancelled ticket without a fee owes nothing, so everything paid on it is the client's
// credit - before, it read as "Devolver".
func TestSurplus_CancelledTicketBecomesCredit(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		cancelled := seedClientSale(t, tx, client.ID, 10000, base)
		open := seedClientSale(t, tx, client.ID, 4000, base.Add(24*time.Hour))
		settleByHand(t, tx, cancelled.ID)

		var issuance data.Issuance
		if err := tx.First(&issuance, "id = ?", cancelled.IssuanceID).Error; err != nil {
			t.Fatalf("failed to read the issuance: %v", err)
		}
		actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])
		if _, err := issuancesvc.Cancel(tx, issuance, issuancesvc.CancelInput{}, actor.ID); err != nil {
			t.Fatalf("cancel: %v", err)
		}

		if got := paidCentsOf(t, tx, cancelled.ID); got != 0 {
			t.Errorf("cancelled sale: got %d, want 0 left on it", got)
		}
		if got := paidCentsOf(t, tx, open.ID); got != 4000 {
			t.Errorf("open sale: got %d, want 4000 from the cancelled one", got)
		}
		if got := creditOf(t, tx, client.ID); got != 6000 {
			t.Errorf("credit: got %d, want 6000", got)
		}
	})
}

// The surplus belongs to whoever owns the sale it is on: moving that sale takes it along.
func TestSurplus_FollowsTheSaleToAnotherClient(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		oldOwner := seedClient(t, tx)
		newOwner := seedClient(t, tx)
		base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		source := seedClientSale(t, tx, oldOwner.ID, 10000, base)
		oldOpen := seedClientSale(t, tx, oldOwner.ID, 2000, base.Add(time.Hour))
		newOpen := seedClientSale(t, tx, newOwner.ID, 5000, base.Add(2*time.Hour))
		settleByHand(t, tx, source.ID)
		setPrice(t, tx, source.ID, "7000")

		if got := paidCentsOf(t, tx, oldOpen.ID); got != 2000 {
			t.Fatalf("setup: old owner's open sale got %d, want 2000", got)
		}

		if rec := callUpdateSale(t, tx, source.ID, `{"client_id":"`+newOwner.ID+`"}`); rec.Code != http.StatusOK {
			t.Fatalf("move status: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		if got := paidCentsOf(t, tx, oldOpen.ID); got != 0 {
			t.Errorf("old owner's sale: got %d, want 0", got)
		}
		if got := creditOf(t, tx, oldOwner.ID); got != 0 {
			t.Errorf("old owner's credit: got %d, want 0", got)
		}
		if got := paidCentsOf(t, tx, newOpen.ID); got != 3000 {
			t.Errorf("new owner's sale: got %d, want 3000", got)
		}
		if got := creditOf(t, tx, newOwner.ID); got != 0 {
			t.Errorf("new owner's credit: got %d, want 0", got)
		}
	})
}

// Deleting the issuance the surplus came from was a mistake undone: the credit it gave
// another ticket goes with it, and that ticket owes again.
func TestSurplus_DeletingTheSourceReopensTheSaleItCovered(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		source := seedClientSale(t, tx, client.ID, 10000, base)
		covered := seedClientSale(t, tx, client.ID, 3000, base.Add(time.Hour))
		settleByHand(t, tx, source.ID)
		setPrice(t, tx, source.ID, "7000")
		if !isPaid(t, tx, covered.ID) {
			t.Fatal("setup: expected the second sale covered by the surplus")
		}

		var issuance data.Issuance
		if err := tx.First(&issuance, "id = ?", source.IssuanceID).Error; err != nil {
			t.Fatalf("failed to read the issuance: %v", err)
		}
		actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])
		if err := issuancesvc.Delete(tx, issuance, actor.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}

		if got := paidCentsOf(t, tx, covered.ID); got != 0 {
			t.Errorf("covered sale: got %d, want 0", got)
		}
		if isPaid(t, tx, covered.ID) {
			t.Error("covered sale should be open again")
		}
	})
}

// A credit line is derived: neither DELETE on the line nor the switch on a sale it alone
// pays can remove it - the next replay would put it back.
func TestSurplus_CreditLinesAreNotEditedByHand(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		source := seedClientSale(t, tx, client.ID, 10000, base)
		covered := seedClientSale(t, tx, client.ID, 3000, base.Add(time.Hour))
		settleByHand(t, tx, source.ID)
		setPrice(t, tx, source.ID, "7000")

		if rec := callUpdateSale(t, tx, covered.ID, `{"paid":false}`); rec.Code != http.StatusConflict {
			t.Errorf("switch: got %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}

		line := lineOnSaleFrom(t, tx, covered.ID, source.ID)
		handler := NewSaleHandler(tx, events.NewBroker())
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", covered.ID)
		routeCtx.URLParams.Add("paymentID", line.ID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		rec := httptest.NewRecorder()
		handler.DeleteSalePayment(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("delete: got %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}
