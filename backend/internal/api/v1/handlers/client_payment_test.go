package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

type clientPaymentEnvelope struct {
	Data schemas.ClientPaymentResponse `json:"data"`
}

// seedClientSale creates a sale worth exactly amountCents, linked to the client, issued at
// the given time - the order the lump sum is spent in.
func seedClientSale(t *testing.T, tx *gorm.DB, clientID string, amountCents int, issuedAt time.Time) data.Sale {
	t.Helper()

	issuance, _ := seedIssuance(t, tx)
	// 1000 points at a price per thousand of amountCents is worth amountCents.
	err := tx.Model(&issuance).UpdateColumns(map[string]any{
		"points":                1000,
		"value_of_the_thousand": amountCents,
		"created_at":            issuedAt,
	}).Error
	if err != nil {
		t.Fatalf("failed to price the issuance: %v", err)
	}

	sale := data.Sale{IssuanceID: issuance.ID, ClientID: &clientID}
	create(t, tx, &sale)
	return sale
}

func seedClient(t *testing.T, tx *gorm.DB) data.Client {
	t.Helper()
	client := data.Client{Name: "Cliente " + uuid.New().String()[:8]}
	create(t, tx, &client)
	return client
}

func callCreateClientPayment(t *testing.T, tx *gorm.DB, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())
	actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])

	req := httptest.NewRequest(http.MethodPost, "/sales/client-payments", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, actor))

	rec := httptest.NewRecorder()
	handler.CreateClientPayment(rec, req)
	return rec
}

func clientPaymentBody(clientID string, amountCents int) string {
	return `{"client_id":"` + clientID + `","amount_cents":` + strconv.Itoa(amountCents) + `,"paid_on":"2026-09-14"}`
}

func paidCentsOf(t *testing.T, tx *gorm.DB, saleID string) int {
	t.Helper()
	var total *int
	if err := tx.Model(&data.SalePayment{}).Where("sale_id = ?", saleID).
		Select("SUM(amount_cents)").Scan(&total).Error; err != nil {
		t.Fatalf("failed to sum payments: %v", err)
	}
	if total == nil {
		return 0
	}
	return *total
}

func creditOf(t *testing.T, tx *gorm.DB, clientID string) int {
	t.Helper()
	var total *int
	if err := tx.Model(&data.ClientCredit{}).Where("client_id = ?", clientID).
		Select("SUM(amount_cents)").Scan(&total).Error; err != nil {
		t.Fatalf("failed to sum credit: %v", err)
	}
	if total == nil {
		return 0
	}
	return *total
}

func TestCreateClientPayment_SpreadsOldestFirst(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		// Created out of order on purpose: the issuance date decides, not the insert.
		newest := seedClientSale(t, tx, client.ID, 30000, base.Add(48*time.Hour))
		oldest := seedClientSale(t, tx, client.ID, 100000, base)
		middle := seedClientSale(t, tx, client.ID, 50000, base.Add(24*time.Hour))

		rec := callCreateClientPayment(t, tx, clientPaymentBody(client.ID, 125000))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		var resp clientPaymentEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if got := paidCentsOf(t, tx, oldest.ID); got != 100000 {
			t.Errorf("oldest paid: got %d, want 100000", got)
		}
		if got := paidCentsOf(t, tx, middle.ID); got != 25000 {
			t.Errorf("middle paid: got %d, want the 25000 left over", got)
		}
		if got := paidCentsOf(t, tx, newest.ID); got != 0 {
			t.Errorf("newest paid: got %d, want nothing", got)
		}
		if resp.Data.AppliedCents != 125000 || resp.Data.SettledSales != 1 || resp.Data.CreditCents != 0 {
			t.Errorf("response: got %+v", resp.Data)
		}

		var settled data.Sale
		if err := tx.First(&settled, "id = ?", oldest.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		if !settled.Paid {
			t.Error("expected the settled sale's paid cache to be true")
		}
	})
}

func TestCreateClientPayment_SurplusBecomesCreditSpentOnTheNextLink(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		only := seedClientSale(t, tx, client.ID, 40000, time.Now().Add(-time.Hour))

		rec := callCreateClientPayment(t, tx, clientPaymentBody(client.ID, 100000))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := paidCentsOf(t, tx, only.ID); got != 40000 {
			t.Errorf("sale paid: got %d, want 40000 - never more than it is worth", got)
		}
		if got := creditOf(t, tx, client.ID); got != 60000 {
			t.Fatalf("credit: got %d, want 60000", got)
		}

		// A later sale, born without a client like every sale, picks the credit up when
		// it is linked.
		next, _ := seedSaleFixture(t, tx, defaultAgency(t, tx).ID)
		if err := tx.Model(&next).UpdateColumn("value_of_the_thousand", 25000).Error; err != nil {
			t.Fatalf("failed to price the issuance: %v", err)
		}
		var nextSale data.Sale
		if err := tx.First(&nextSale, "issuance_id = ?", next.ID).Error; err != nil {
			t.Fatalf("failed to read the sale: %v", err)
		}
		if rec := callUpdateSale(t, tx, nextSale.ID, `{"client_id":"`+client.ID+`"}`); rec.Code != http.StatusOK {
			t.Fatalf("link status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		// points 1000 at 25000 per thousand = 25000 cents.
		if got := paidCentsOf(t, tx, nextSale.ID); got != 25000 {
			t.Errorf("linked sale paid from credit: got %d, want 25000", got)
		}
		if got := creditOf(t, tx, client.ID); got != 35000 {
			t.Errorf("credit after the link: got %d, want 35000", got)
		}
	})
}

func TestCreateClientPayment_ExistingCreditIsSpentFirst(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])
		create(t, tx, &data.ClientCredit{ClientID: client.ID, AmountCents: 10000, CreatedByID: actor.ID})
		sale := seedClientSale(t, tx, client.ID, 30000, time.Now().Add(-time.Hour))

		rec := callCreateClientPayment(t, tx, clientPaymentBody(client.ID, 5000))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := paidCentsOf(t, tx, sale.ID); got != 15000 {
			t.Errorf("sale paid: got %d, want 10000 of credit + 5000 new", got)
		}
		if got := creditOf(t, tx, client.ID); got != 0 {
			t.Errorf("credit: got %d, want it spent", got)
		}
	})
}

func TestCreateClientPayment_SkipsCancelledSales(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		cancelled := seedClientSale(t, tx, client.ID, 30000, time.Now().Add(-2*time.Hour))
		live := seedClientSale(t, tx, client.ID, 30000, time.Now().Add(-time.Hour))
		if err := tx.Model(&data.Issuance{}).
			Where("id = (SELECT issuance_id FROM sales WHERE id = ?)", cancelled.ID).
			UpdateColumn("cancelled_at", time.Now()).Error; err != nil {
			t.Fatalf("failed to cancel: %v", err)
		}

		if rec := callCreateClientPayment(t, tx, clientPaymentBody(client.ID, 30000)); rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := paidCentsOf(t, tx, cancelled.ID); got != 0 {
			t.Errorf("cancelled sale paid: got %d, want nothing - a cancelled ticket is no debt", got)
		}
		if got := paidCentsOf(t, tx, live.ID); got != 30000 {
			t.Errorf("live sale paid: got %d, want 30000", got)
		}
	})
}

func TestCreateClientPayment_Rejects(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		cases := []struct {
			name string
			body string
			want int
		}{
			{"zero amount", clientPaymentBody(client.ID, 0), http.StatusBadRequest},
			{"negative amount", clientPaymentBody(client.ID, -100), http.StatusBadRequest},
			{"bad date", `{"client_id":"` + client.ID + `","amount_cents":100,"paid_on":"14/09/2026"}`, http.StatusBadRequest},
			{"unknown client", clientPaymentBody(uuid.New().String(), 100), http.StatusUnprocessableEntity},
		}
		for _, tc := range cases {
			if rec := callCreateClientPayment(t, tx, tc.body); rec.Code != tc.want {
				t.Errorf("%s: got %d, want %d (body: %s)", tc.name, rec.Code, tc.want, rec.Body.String())
			}
		}
	})
}
