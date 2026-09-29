package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// seedPricedSale builds a sale whose issuance is worth a known amount, so a test can talk
// in reais instead of deriving the figure it is checking.
//
// 20.000 points at R$ 50,00 per thousand = R$ 1.000,00, and no taxes. Round numbers on
// purpose: the rounding rule has its own test in internal/data.
func seedPricedSale(t *testing.T, tx *gorm.DB) data.Sale {
	t.Helper()

	main := defaultAgency(t, tx)
	issuance, sale := seedSaleFixture(t, tx, main.ID)

	err := tx.Model(&issuance).Updates(map[string]any{
		"points":                20000,
		"value_of_the_thousand": 5000,
		"taxes":                 nil,
	}).Error
	if err != nil {
		t.Fatalf("pricing the issuance: %v", err)
	}

	return sale
}

const pricedSaleAmount = 100000 // R$ 1.000,00 in centavos

type salePaymentListResponse struct {
	Data []schemas.SalePaymentResponse `json:"data"`
}

type salePaymentSingleResponse struct {
	Data schemas.SalePaymentResponse `json:"data"`
}

func callCreatePayment(t *testing.T, tx *gorm.DB, saleID, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())
	actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])

	req := httptest.NewRequest(http.MethodPost, "/sales/"+saleID+"/payments", strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", saleID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.CreateSalePayment(rec, req)
	return rec
}

func callDeletePayment(t *testing.T, tx *gorm.DB, saleID, paymentID string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())

	req := httptest.NewRequest(http.MethodDelete, "/sales/"+saleID+"/payments/"+paymentID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", saleID)
	rctx.URLParams.Add("paymentID", paymentID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	handler.DeleteSalePayment(rec, req)
	return rec
}

func callGetPayments(t *testing.T, tx *gorm.DB, saleID string) []schemas.SalePaymentResponse {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())

	req := httptest.NewRequest(http.MethodGet, "/sales/"+saleID+"/payments", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", saleID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	handler.GetSalePayments(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var resp salePaymentListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding the statement: %v", err)
	}
	return resp.Data
}

func paidFlag(t *testing.T, tx *gorm.DB, saleID string) bool {
	t.Helper()
	var sale data.Sale
	if err := tx.First(&sale, "id = ?", saleID).Error; err != nil {
		t.Fatalf("re-reading the sale: %v", err)
	}
	return sale.Paid
}

func payment(amountCents int) string {
	return fmt.Sprintf(`{"amount_cents":%d,"paid_on":"2026-08-14"}`, amountCents)
}

func TestSalePayments_TwoPartsSettleTheSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		sale := seedPricedSale(t, tx)

		// The case the whole feature exists for: R$ 400 now, the rest later. Before this,
		// the sale could only claim all or nothing.
		if rec := callCreatePayment(t, tx, sale.ID, payment(40000)); rec.Code != http.StatusCreated {
			t.Fatalf("first payment: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if paidFlag(t, tx, sale.ID) {
			t.Error("sale reads as paid with R$ 600 still missing")
		}

		if rec := callCreatePayment(t, tx, sale.ID, payment(60000)); rec.Code != http.StatusCreated {
			t.Fatalf("second payment: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if !paidFlag(t, tx, sale.ID) {
			t.Error("sale still reads as open after the payments added up to its value")
		}

		if lines := callGetPayments(t, tx, sale.ID); len(lines) != 2 {
			t.Errorf("statement has %d lines, want 2", len(lines))
		}
	})
}

func TestSalePayments_DeletingAPaymentReopensTheSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		sale := seedPricedSale(t, tx)

		rec := callCreatePayment(t, tx, sale.ID, payment(pricedSaleAmount))
		if rec.Code != http.StatusCreated {
			t.Fatalf("payment: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var created salePaymentSingleResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decoding the payment: %v", err)
		}
		if !paidFlag(t, tx, sale.ID) {
			t.Fatal("sale did not settle on a full payment")
		}

		// A payment that never happened is a typo, not history: removing it has to put the
		// sale back exactly where it was.
		if rec := callDeletePayment(t, tx, sale.ID, created.Data.ID); rec.Code != http.StatusOK {
			t.Fatalf("delete: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if paidFlag(t, tx, sale.ID) {
			t.Error("sale still reads as paid after its only payment was removed")
		}
	})
}

func TestSalePayments_ANegativeEntryHandsMoneyBack(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		sale := seedPricedSale(t, tx)

		if rec := callCreatePayment(t, tx, sale.ID, payment(pricedSaleAmount)); rec.Code != http.StatusCreated {
			t.Fatalf("payment: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		// The refund of a cancelled booking: the money goes back, and the sale is open
		// again because nothing was ultimately received for it.
		if rec := callCreatePayment(t, tx, sale.ID, payment(-pricedSaleAmount)); rec.Code != http.StatusCreated {
			t.Fatalf("refund: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		if paidFlag(t, tx, sale.ID) {
			t.Error("sale reads as paid after everything was handed back")
		}

		sales := callGetSales(t, tx, "")
		for _, row := range sales {
			if row.ID != sale.ID {
				continue
			}
			if row.PaidCents != 0 {
				t.Errorf("paid_cents = %d, want 0 - the two entries cancel out", row.PaidCents)
			}
			if row.BalanceCents != pricedSaleAmount {
				t.Errorf("balance_cents = %d, want %d", row.BalanceCents, pricedSaleAmount)
			}
		}
	})
}

func TestSalePayments_RefusesAMovementOfNothing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		sale := seedPricedSale(t, tx)

		// The database refuses it too, but as a 500 with a Postgres string in it.
		rec := callCreatePayment(t, tx, sale.ID, payment(0))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestSalePayments_RefusesAPaymentFromAnotherSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		mine := seedPricedSale(t, tx)
		theirs := seedPricedSale(t, tx)

		rec := callCreatePayment(t, tx, theirs.ID, payment(10000))
		var created salePaymentSingleResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decoding the payment: %v", err)
		}

		// Scoped by both ids: without that, a guessed id would let one sale's statement be
		// edited through another's.
		if rec := callDeletePayment(t, tx, mine.ID, created.Data.ID); rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404", rec.Code)
		}
	})
}

func TestSalePayments_ListingCarriesTheBalance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		sale := seedPricedSale(t, tx)

		if rec := callCreatePayment(t, tx, sale.ID, payment(40000)); rec.Code != http.StatusCreated {
			t.Fatalf("payment: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		for _, row := range callGetSales(t, tx, "") {
			if row.ID != sale.ID {
				continue
			}
			if row.AmountCents != pricedSaleAmount {
				t.Errorf("amount_cents = %d, want %d", row.AmountCents, pricedSaleAmount)
			}
			if row.PaidCents != 40000 {
				t.Errorf("paid_cents = %d, want 40000", row.PaidCents)
			}
			if row.BalanceCents != 60000 {
				t.Errorf("balance_cents = %d, want 60000 - what the operator is chasing", row.BalanceCents)
			}
			return
		}
		t.Fatal("the sale did not come back in the listing")
	})
}

// Cancelling a paid booking leaves the money where it is and turns the balance around: the
// client no longer owes the price, the agency owes the refund. The sale is not settled
// until that refund is recorded, which is exactly the row that needs somebody to act.
func TestSalePayments_CancellingAPaidIssuanceLeavesARefundToMake(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, sale := seedSaleFixture(t, tx, main.ID)

		err := tx.Model(&issuance).Updates(map[string]any{
			"points":                20000,
			"value_of_the_thousand": 5000,
			"taxes":                 nil,
		}).Error
		if err != nil {
			t.Fatalf("pricing the issuance: %v", err)
		}

		if rec := callCreatePayment(t, tx, sale.ID, payment(pricedSaleAmount)); rec.Code != http.StatusCreated {
			t.Fatalf("payment: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		canceller := createUser(t, tx, "Cancelador "+uuid.New().String()[:8])
		if rec := callCancel(tx, issuance.ID, canceller.ID, ""); rec.Code != http.StatusOK {
			t.Fatalf("cancel: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		if paidFlag(t, tx, sale.ID) {
			t.Error("the cancelled sale reads as settled while the client's money is still in the till")
		}
		// The statement is untouched - nothing was refunded yet.
		if lines := callGetPayments(t, tx, sale.ID); len(lines) != 1 {
			t.Errorf("statement has %d lines, want the original payment still there", len(lines))
		}

		// And the balance now points at the refund, not at a debt the client no longer has.
		for _, row := range callGetSales(t, tx, "") {
			if row.ID != sale.ID {
				continue
			}
			if row.BalanceCents != -pricedSaleAmount {
				t.Errorf("balance_cents = %d, want %d - what has to go back to the client",
					row.BalanceCents, -pricedSaleAmount)
			}
			return
		}
		t.Fatal("the sale did not come back in the listing")
	})
}

// The imperfection this rule exists to remove: a cancelled sale nobody ever paid used to
// read as owing its full price forever, because the price stayed in the balance after the
// ticket it was for had been cancelled. There is nothing to do about it in either
// direction, and the screen has to say so.
func TestSalePayments_ACancelledSaleWithNoMoneyInItIsSettled(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, sale := seedSaleFixture(t, tx, main.ID)

		err := tx.Model(&issuance).Updates(map[string]any{
			"points":                20000,
			"value_of_the_thousand": 5000,
			"taxes":                 nil,
		}).Error
		if err != nil {
			t.Fatalf("pricing the issuance: %v", err)
		}

		canceller := createUser(t, tx, "Cancelador "+uuid.New().String()[:8])
		if rec := callCancel(tx, issuance.ID, canceller.ID, ""); rec.Code != http.StatusOK {
			t.Fatalf("cancel: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		// Nothing ever came in and nothing has to go out.
		for _, row := range callGetSales(t, tx, "") {
			if row.ID != sale.ID {
				continue
			}
			if row.BalanceCents != 0 {
				t.Errorf("balance_cents = %d, want 0 - a cancelled ticket is not a debt", row.BalanceCents)
			}
			return
		}
		t.Fatal("the sale did not come back in the listing")
	})
}

// Refunding a cancelled sale brings it back to nothing pending, which is the other end of
// the same rule.
func TestSalePayments_RefundingACancelledSaleClearsIt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, sale := seedSaleFixture(t, tx, main.ID)

		err := tx.Model(&issuance).Updates(map[string]any{
			"points":                20000,
			"value_of_the_thousand": 5000,
			"taxes":                 nil,
		}).Error
		if err != nil {
			t.Fatalf("pricing the issuance: %v", err)
		}

		if rec := callCreatePayment(t, tx, sale.ID, payment(pricedSaleAmount)); rec.Code != http.StatusCreated {
			t.Fatalf("payment: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		canceller := createUser(t, tx, "Cancelador "+uuid.New().String()[:8])
		if rec := callCancel(tx, issuance.ID, canceller.ID, ""); rec.Code != http.StatusOK {
			t.Fatalf("cancel: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if rec := callCreatePayment(t, tx, sale.ID, payment(-pricedSaleAmount)); rec.Code != http.StatusCreated {
			t.Fatalf("refund: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		if !paidFlag(t, tx, sale.ID) {
			t.Error("the refunded sale still reads as pending; there is nothing left to move on it")
		}
		for _, row := range callGetSales(t, tx, "") {
			if row.ID != sale.ID {
				continue
			}
			if row.BalanceCents != 0 {
				t.Errorf("balance_cents = %d, want 0", row.BalanceCents)
			}
			return
		}
		t.Fatal("the sale did not come back in the listing")
	})
}

// The old switch has to keep working while the panel that replaces it is not published -
// the two sides deploy by independent labels.
func TestUpdateSale_LegacyPaidFlagSettlesAndClearsTheStatement(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		sale := seedPricedSale(t, tx)

		if rec := callUpdateSale(t, tx, sale.ID, `{"paid":true}`); rec.Code != http.StatusOK {
			t.Fatalf("marking paid: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		lines := callGetPayments(t, tx, sale.ID)
		if len(lines) != 1 || lines[0].AmountCents != pricedSaleAmount {
			t.Fatalf("statement = %+v, want one line for the full value", lines)
		}
		if !paidFlag(t, tx, sale.ID) {
			t.Error("sale did not settle")
		}

		if rec := callUpdateSale(t, tx, sale.ID, `{"paid":false}`); rec.Code != http.StatusOK {
			t.Fatalf("marking open: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if lines := callGetPayments(t, tx, sale.ID); len(lines) != 0 {
			t.Errorf("statement = %+v, want it cleared", lines)
		}
		if paidFlag(t, tx, sale.ID) {
			t.Error("sale still reads as paid")
		}
	})
}

func TestUpdateSale_LegacyPaidFlagTopsUpAPartialPayment(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		sale := seedPricedSale(t, tx)

		if rec := callCreatePayment(t, tx, sale.ID, payment(40000)); rec.Code != http.StatusCreated {
			t.Fatalf("payment: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if rec := callUpdateSale(t, tx, sale.ID, `{"paid":true}`); rec.Code != http.StatusOK {
			t.Fatalf("marking paid: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		// The switch means "it is settled now", so it records what was missing - not a
		// second payment for the whole value.
		lines := callGetPayments(t, tx, sale.ID)
		if len(lines) != 2 {
			t.Fatalf("statement has %d lines, want 2", len(lines))
		}
		total := 0
		for _, line := range lines {
			total += line.AmountCents
		}
		if total != pricedSaleAmount {
			t.Errorf("statement totals %d, want exactly the sale's value %d", total, pricedSaleAmount)
		}
	})
}
