package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/sale"
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
	// A sale and its client share an agency, so the issuance moves to the client's.
	var client data.Client
	if err := tx.First(&client, "id = ?", clientID).Error; err != nil || client.AgencyID == nil {
		t.Fatalf("failed to read the client's agency: %v", err)
	}
	// 1000 points at a price per thousand of amountCents is worth amountCents.
	err := tx.Model(&issuance).UpdateColumns(map[string]any{
		"agency_id":             *client.AgencyID,
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
	// The main agency: seedSaleFixture tests put their sales there, and a client can only be
	// linked to a sale of its own agency.
	agencyID := defaultAgency(t, tx).ID
	client := data.Client{Name: "Cliente " + uuid.New().String()[:8], AgencyID: &agencyID}
	create(t, tx, &client)
	return client
}

func callCreateClientPayment(t *testing.T, tx *gorm.DB, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())
	actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])

	req := httptest.NewRequest(http.MethodPost, "/sales/client-payments", strings.NewReader(body))
	req = req.WithContext(asUser(req.Context(), actor))

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
	credit, err := sale.CreditOf(tx, clientID)
	if err != nil {
		t.Fatalf("failed to read credit: %v", err)
	}
	return credit
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

func TestCreateClientPayment_SurplusBecomesCredit(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		only := seedClientSale(t, tx, client.ID, 40000, time.Now().Add(-time.Hour))

		rec := callCreateClientPayment(t, tx, clientPaymentBody(client.ID, 100000))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		var resp clientPaymentEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if got := paidCentsOf(t, tx, only.ID); got != 40000 {
			t.Errorf("sale paid: got %d, want 40000 - never more than it is worth", got)
		}
		if got := creditOf(t, tx, client.ID); got != 60000 {
			t.Errorf("credit: got %d, want 60000", got)
		}
		if resp.Data.ClientPaymentID == "" || resp.Data.AppliedCents != 40000 || resp.Data.CreditCents != 60000 {
			t.Errorf("response: got %+v", resp.Data)
		}
	})
}

// A second payment continues where the first stopped, and one dated before it is replayed
// first: the spread follows when the money came in, not when it was typed.
func TestCreateClientPayment_ReplaysPaymentsInTheOrderTheyCameIn(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		first := seedClientSale(t, tx, client.ID, 10000, base)
		second := seedClientSale(t, tx, client.ID, 10000, base.Add(24*time.Hour))

		later := `{"client_id":"` + client.ID + `","amount_cents":6000,"paid_on":"2026-09-20"}`
		earlier := `{"client_id":"` + client.ID + `","amount_cents":8000,"paid_on":"2026-09-10"}`
		for _, body := range []string{later, earlier} {
			if rec := callCreateClientPayment(t, tx, body); rec.Code != http.StatusCreated {
				t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
			}
		}

		// Replayed as 8000 (10/09) then 6000 (20/09): the first sale takes 8000 + 2000.
		var lines []data.SalePayment
		if err := tx.Where("sale_id = ?", first.ID).Order("paid_on").Find(&lines).Error; err != nil {
			t.Fatalf("failed to read lines: %v", err)
		}
		if len(lines) != 2 || lines[0].AmountCents != 8000 || lines[1].AmountCents != 2000 {
			t.Errorf("first sale lines: got %+v, want 8000 then 2000", lines)
		}
		if got := paidCentsOf(t, tx, second.ID); got != 4000 {
			t.Errorf("second sale paid: got %d, want 4000", got)
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

func callDeleteClientPayment(t *testing.T, tx *gorm.DB, paymentID string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())
	actor := createUser(t, tx, "Operador "+uuid.New().String()[:8])

	req := httptest.NewRequest(http.MethodDelete, "/sales/client-payments/"+paymentID, nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("paymentID", paymentID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	req = req.WithContext(asUser(ctx, actor))

	rec := httptest.NewRecorder()
	handler.DeleteClientPayment(rec, req)
	return rec
}

func recordClientPayment(t *testing.T, tx *gorm.DB, body string) string {
	t.Helper()
	rec := callCreateClientPayment(t, tx, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp clientPaymentEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return resp.Data.ClientPaymentID
}

func isPaid(t *testing.T, tx *gorm.DB, saleID string) bool {
	t.Helper()
	var s data.Sale
	if err := tx.First(&s, "id = ?", saleID).Error; err != nil {
		t.Fatalf("failed to re-read sale: %v", err)
	}
	return s.Paid
}

// The case that motivated the history: an amount typed with a zero too many. Undoing it
// must leave no trace in the sales or the credit, and keep the row to explain itself.
func TestDeleteClientPayment_UndoesAWrongAmount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		only := seedClientSale(t, tx, client.ID, 40000, time.Now().Add(-time.Hour))
		paymentID := recordClientPayment(t, tx, clientPaymentBody(client.ID, 4000000))

		if rec := callDeleteClientPayment(t, tx, paymentID); rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := paidCentsOf(t, tx, only.ID); got != 0 {
			t.Errorf("sale paid: got %d, want 0", got)
		}
		if isPaid(t, tx, only.ID) {
			t.Error("expected the sale's paid cache back to false")
		}
		if got := creditOf(t, tx, client.ID); got != 0 {
			t.Errorf("credit: got %d, want 0", got)
		}

		var payment data.ClientPayment
		if err := tx.First(&payment, "id = ?", paymentID).Error; err != nil {
			t.Fatalf("expected the undone payment to stay: %v", err)
		}
		if payment.DeletedAt == nil || payment.DeletedByID == nil {
			t.Errorf("expected deleted_at and deleted_by_id set, got %+v", payment)
		}

		if rec := callDeleteClientPayment(t, tx, paymentID); rec.Code != http.StatusNotFound {
			t.Errorf("second undo: got %d, want 404", rec.Code)
		}
	})
}

// Undoing a payment in the middle moves the later ones back to where it was: FIFO is a
// property of the whole history, not of the order things were typed and undone.
func TestDeleteClientPayment_LaterPaymentsTakeItsPlace(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		first := seedClientSale(t, tx, client.ID, 10000, base)
		second := seedClientSale(t, tx, client.ID, 10000, base.Add(24*time.Hour))
		third := seedClientSale(t, tx, client.ID, 10000, base.Add(48*time.Hour))

		body := func(day string) string {
			return `{"client_id":"` + client.ID + `","amount_cents":10000,"paid_on":"2026-09-` + day + `"}`
		}
		recordClientPayment(t, tx, body("10"))
		middle := recordClientPayment(t, tx, body("11"))
		recordClientPayment(t, tx, body("12"))

		rec := callDeleteClientPayment(t, tx, middle)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if !isPaid(t, tx, first.ID) || !isPaid(t, tx, second.ID) || isPaid(t, tx, third.ID) {
			t.Errorf("paid: got %v %v %v, want the two oldest paid and the newest open",
				isPaid(t, tx, first.ID), isPaid(t, tx, second.ID), isPaid(t, tx, third.ID))
		}
		var resp struct {
			Data schemas.ClientPaymentUndoResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Data.SaleIDs) != 2 {
			t.Errorf("changed sales: got %v, want the second and the third", resp.Data.SaleIDs)
		}
	})
}

func TestGetClientPayments_ListsUndoneAndWhereTheMoneyIs(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		seedClientSale(t, tx, client.ID, 10000, time.Now().Add(-time.Hour))
		undone := recordClientPayment(t, tx, `{"client_id":"`+client.ID+`","amount_cents":99999,"paid_on":"2026-09-10"}`)
		callDeleteClientPayment(t, tx, undone)
		kept := recordClientPayment(t, tx, `{"client_id":"`+client.ID+`","amount_cents":15000,"paid_on":"2026-09-11"}`)

		handler := NewSaleHandler(tx, events.NewBroker())
		req := withScope(httptest.NewRequest(http.MethodGet, "/sales/client-payments?client_id="+client.ID, nil), access.FullScope())
		rec := httptest.NewRecorder()
		handler.GetClientPayments(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var resp struct {
			Data schemas.ClientPaymentHistoryResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Data.CreditCents != 5000 || len(resp.Data.Payments) != 2 {
			t.Fatalf("history: got %+v", resp.Data)
		}
		first, second := resp.Data.Payments[0], resp.Data.Payments[1]
		if first.ID != undone || first.DeletedAt == nil || first.DeletedByName == nil || len(first.Allocations) != 0 {
			t.Errorf("undone payment: got %+v", first)
		}
		if second.ID != kept || len(second.Allocations) != 1 || second.Allocations[0].AmountCents != 10000 {
			t.Errorf("kept payment: got %+v", second)
		}
	})
}

// Without a client, the history is every client's payments received in the period, newest
// first, each one saying whose it is.
func TestGetClientPayments_AcrossClientsInThePeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		first, second := seedClient(t, tx), seedClient(t, tx)
		seedClientSale(t, tx, first.ID, 10000, time.Now().Add(-time.Hour))
		before := recordClientPayment(t, tx, `{"client_id":"`+first.ID+`","amount_cents":1000,"paid_on":"2026-08-31"}`)
		older := recordClientPayment(t, tx, `{"client_id":"`+first.ID+`","amount_cents":2000,"paid_on":"2026-09-02"}`)
		newer := recordClientPayment(t, tx, `{"client_id":"`+second.ID+`","amount_cents":3000,"paid_on":"2026-09-05"}`)

		call := func(query string) schemas.ClientPaymentHistoryResponse {
			t.Helper()
			handler := NewSaleHandler(tx, events.NewBroker())
			req := withScope(httptest.NewRequest(http.MethodGet, "/sales/client-payments?"+query, nil), access.FullScope())
			rec := httptest.NewRecorder()
			handler.GetClientPayments(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status %d (body: %s)", query, rec.Code, rec.Body.String())
			}
			var resp struct {
				Data schemas.ClientPaymentHistoryResponse `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			return resp.Data
		}
		ours := func(history schemas.ClientPaymentHistoryResponse) []string {
			ids := []string{}
			for _, payment := range history.Payments {
				if payment.ClientID == first.ID || payment.ClientID == second.ID {
					ids = append(ids, payment.ID)
				}
			}
			return ids
		}

		all := call("date_from=2026-09-01&date_to=2026-09-30")
		if got := ours(all); !slices.Equal(got, []string{newer, older}) {
			t.Fatalf("period: got %v, want [%s %s] (and not %s)", got, newer, older, before)
		}
		for _, payment := range all.Payments {
			if payment.ID == newer && payment.ClientName != second.Name {
				t.Errorf("client_name: got %q, want %q", payment.ClientName, second.Name)
			}
		}

		picked := call("client_ids=" + second.ID + "&date_from=2026-09-01&date_to=2026-09-30")
		if len(picked.Payments) != 1 || picked.Payments[0].ID != newer {
			t.Errorf("client_ids: got %+v", picked.Payments)
		}
		// 3000 of the second client's that no sale took.
		if picked.CreditCents != 3000 {
			t.Errorf("credit: got %d, want 3000", picked.CreditCents)
		}

		if none := call("client_ids=none"); len(none.Payments) != 0 || none.CreditCents != 0 {
			t.Errorf("no-client pick: got %+v", none)
		}
	})
}

// A spread line comes back on the next replay, so the per-sale statement may not delete it.
func TestDeleteSalePayment_RefusesASpreadLine(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		only := seedClientSale(t, tx, client.ID, 10000, time.Now().Add(-time.Hour))
		recordClientPayment(t, tx, clientPaymentBody(client.ID, 10000))

		var line data.SalePayment
		if err := tx.First(&line, "sale_id = ?", only.ID).Error; err != nil {
			t.Fatalf("failed to read the line: %v", err)
		}

		handler := NewSaleHandler(tx, events.NewBroker())
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", only.ID)
		routeCtx.URLParams.Add("paymentID", line.ID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		rec := httptest.NewRecorder()
		handler.DeleteSalePayment(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("status: got %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// Sales are born without a client, so linking one is when the client's credit reaches it.
func TestUpdateSale_LinkingASaleSpendsTheClientsCredit(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		recordClientPayment(t, tx, clientPaymentBody(client.ID, 60000))

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

// A paid sale moved to the wrong client and back: the money belongs to whoever paid it,
// so it leaves with the sale's old owner and the new owner's credit takes over.
func TestUpdateSale_MovingAPaidSaleRebalancesBothClients(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		oldOwner := seedClient(t, tx)
		newOwner := seedClient(t, tx)
		base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		moved := seedClientSale(t, tx, oldOwner.ID, 10000, base)
		stays := seedClientSale(t, tx, oldOwner.ID, 10000, base.Add(24*time.Hour))
		recordClientPayment(t, tx, clientPaymentBody(oldOwner.ID, 10000))
		recordClientPayment(t, tx, clientPaymentBody(newOwner.ID, 4000))

		if !isPaid(t, tx, moved.ID) {
			t.Fatal("setup: expected the oldest sale paid by its client")
		}

		if rec := callUpdateSale(t, tx, moved.ID, `{"client_id":"`+newOwner.ID+`"}`); rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		if got := paidCentsOf(t, tx, moved.ID); got != 4000 {
			t.Errorf("moved sale: got %d, want only the new owner's 4000", got)
		}
		if got := paidCentsOf(t, tx, stays.ID); got != 10000 {
			t.Errorf("old owner's other sale: got %d, want the 10000 that left the moved one", got)
		}
		if got := creditOf(t, tx, newOwner.ID); got != 0 {
			t.Errorf("new owner's credit: got %d, want it spent", got)
		}

		// Unlinking gives the new owner's money back to them as credit.
		if rec := callUpdateSale(t, tx, moved.ID, `{"client_id":null}`); rec.Code != http.StatusOK {
			t.Fatalf("unlink status: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if got := paidCentsOf(t, tx, moved.ID); got != 0 {
			t.Errorf("unlinked sale: got %d, want 0", got)
		}
		if got := creditOf(t, tx, newOwner.ID); got != 4000 {
			t.Errorf("new owner's credit after unlink: got %d, want 4000", got)
		}
	})
}

// The switch cannot reopen a sale a client payment covers - the next replay would pay it
// again - but it still works on the same sale's own lines once the payment is undone.
func TestUpdateSale_RefusesToReopenASalePaidByAClientPayment(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		covered := seedClientSale(t, tx, client.ID, 10000, time.Now().Add(-time.Hour))
		paymentID := recordClientPayment(t, tx, `{"client_id":"`+client.ID+`","amount_cents":10000,"paid_on":"2026-09-14"}`)

		rec := callUpdateSale(t, tx, covered.ID, `{"paid":false}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status: got %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := paidCentsOf(t, tx, covered.ID); got != 10000 {
			t.Errorf("sale paid: got %d, want it untouched", got)
		}

		var listed saleSingleResponse
		rec = callUpdateSale(t, tx, covered.ID, `{"note":"x"}`)
		if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if listed.Data.ClientPaymentCents != 10000 || listed.Data.ClientPaymentPaidOn == nil ||
			*listed.Data.ClientPaymentPaidOn != "2026-09-14" {
			t.Errorf("response: got cents %d, day %v", listed.Data.ClientPaymentCents, listed.Data.ClientPaymentPaidOn)
		}

		callDeleteClientPayment(t, tx, paymentID)
		if rec := callUpdateSale(t, tx, covered.ID, `{"paid":true}`); rec.Code != http.StatusOK {
			t.Fatalf("paid status: got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if rec := callUpdateSale(t, tx, covered.ID, `{"paid":false}`); rec.Code != http.StatusOK {
			t.Errorf("switch on the sale's own line: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteClient_RefusesAClientWithPayments(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		recordClientPayment(t, tx, clientPaymentBody(client.ID, 1000))

		handler := NewClientHandler(tx)
		req := httptest.NewRequest(http.MethodDelete, "/clients/"+client.ID, nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", client.ID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		rec := httptest.NewRecorder()
		handler.DeleteClient(rec, req)
		// The last statement: the refused DELETE aborts the test transaction.
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status: got %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// A sale partly paid by the client is where the lump sum ran out: the switch may neither
// settle it (money that never came in) nor clear it (the trace of the money that did). Only
// another client payment moves it.
func TestUpdateSale_RefusesTheSwitchOnAPartlyPaidSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		partial := seedClientSale(t, tx, client.ID, 10000, time.Now().Add(-time.Hour))
		recordClientPayment(t, tx, clientPaymentBody(client.ID, 6000))

		for _, body := range []string{`{"paid":true}`, `{"paid":false}`} {
			if rec := callUpdateSale(t, tx, partial.ID, body); rec.Code != http.StatusConflict {
				t.Errorf("%s: got %d, want 409 (body: %s)", body, rec.Code, rec.Body.String())
			}
		}
		if got := paidCentsOf(t, tx, partial.ID); got != 6000 {
			t.Errorf("paid: got %d, want the client's 6000 untouched", got)
		}

		// The note is not the switch, and stays editable.
		if rec := callUpdateSale(t, tx, partial.ID, `{"note":"falta o resto"}`); rec.Code != http.StatusOK {
			t.Errorf("note: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		recordClientPayment(t, tx, clientPaymentBody(client.ID, 4000))
		if got := paidCentsOf(t, tx, partial.ID); got != 10000 {
			t.Errorf("after the second payment: got %d, want 10000", got)
		}
	})
}

// Cancelling a ticket a client payment covered gives that money back to the client: it
// moves to their next open sale instead of sitting on the cancelled one as a refund due.
func TestCancelIssuance_MovesClientPaymentToTheNextSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		cancelled := seedClientSale(t, tx, client.ID, 10000, base)
		next := seedClientSale(t, tx, client.ID, 10000, base.Add(24*time.Hour))
		recordClientPayment(t, tx, clientPaymentBody(client.ID, 10000))

		var issuance data.Issuance
		if err := tx.First(&issuance, "id = ?", cancelled.IssuanceID).Error; err != nil {
			t.Fatalf("failed to read issuance: %v", err)
		}
		now := time.Now()
		issuance.CancelledAt = &now
		if err := tx.Save(&issuance).Error; err != nil {
			t.Fatalf("failed to cancel: %v", err)
		}
		if err := sale.RecalculateByIssuance(tx, issuance.ID); err != nil {
			t.Fatalf("recalculate: %v", err)
		}

		if got := paidCentsOf(t, tx, cancelled.ID); got != 0 {
			t.Errorf("cancelled sale: got %d, want the client's money gone from it", got)
		}
		if got := paidCentsOf(t, tx, next.ID); got != 10000 {
			t.Errorf("next sale: got %d, want 10000", got)
		}
	})
}
