package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// These reuse the Postgres harness declared in issuances_cancel_test.go (setupTestDB,
// runInTx, create, createUser, seedIssuance): the sales handlers are pure SQL joins with
// no service layer to mock, so a real database is the only way to test them.

// defaultAgency returns the agency flagged is_default, creating it when the database has
// none. It is never allowed to promote a second one: `idx_agencies_is_default` is a
// partial unique index, so flipping the flag on a new row would violate it.
func defaultAgency(t *testing.T, tx *gorm.DB) data.Agency {
	t.Helper()

	var agency data.Agency
	err := tx.Where("is_default = ?", true).First(&agency).Error
	if err == nil {
		return agency
	}

	agency = data.Agency{Name: "Agência Principal " + uuid.New().String()[:8], IsDefault: true}
	create(t, tx, &agency)
	return agency
}

// seedSaleFixture builds an issuance in the given agency plus its sale, mirroring what
// CreateIssuance does. Returns both so tests can assert on either side.
func seedSaleFixture(t *testing.T, tx *gorm.DB, agencyID string) (data.Issuance, data.Sale) {
	t.Helper()

	issuance, _ := seedIssuance(t, tx)
	if err := tx.Model(&issuance).Update("agency_id", agencyID).Error; err != nil {
		t.Fatalf("failed to move issuance to agency %s: %v", agencyID, err)
	}
	issuance.AgencyID = agencyID

	sale := data.Sale{IssuanceID: issuance.ID}
	create(t, tx, &sale)

	return issuance, sale
}

type saleListResponse struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Data    []schemas.SaleResponse `json:"data"`
}

type saleSingleResponse struct {
	Success bool                 `json:"success"`
	Message string               `json:"message"`
	Data    schemas.SaleResponse `json:"data"`
}

type supplierListResponse struct {
	Success bool                              `json:"success"`
	Message string                            `json:"message"`
	Data    []schemas.SupplierBalanceResponse `json:"data"`
}

func callGetSales(t *testing.T, tx *gorm.DB, query string) []schemas.SaleResponse {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())

	req := httptest.NewRequest(http.MethodGet, "/sales?"+query, nil)
	rec := httptest.NewRecorder()
	handler.GetSales(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetSales status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp saleListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode sales response: %v", err)
	}
	return resp.Data
}

func findSale(sales []schemas.SaleResponse, id string) *schemas.SaleResponse {
	for i := range sales {
		if sales[i].ID == id {
			return &sales[i]
		}
	}
	return nil
}

// callCreateIssuance drives CreateIssuance with the authenticated user injected the same
// way the middleware chain would.
func callCreateIssuance(tx *gorm.DB, userID, body string) *httptest.ResponseRecorder {
	handler := NewIssuanceHandler(tx, events.NewBroker(), nil)

	req := httptest.NewRequest(http.MethodPost, "/issuances", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, data.User{ID: userID, Role: "admin"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.CreateIssuance(rec, req)
	return rec
}

func TestCreateIssuance_CreatesOpenSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		// seedIssuance gives us a valid FK chain; the issuance it creates is discarded,
		// only its account/program are reused for the payload.
		seed, user := seedIssuance(t, tx)
		defaultAgency(t, tx)

		body := `{
			"program_locator": "LOC` + uuid.New().String()[:6] + `",
			"loyalty_program_id": "` + seed.LoyaltyProgramID + `",
			"account_id": "` + seed.AccountID + `",
			"points": 10000,
			"value_of_the_thousand": 2000
		}`
		rec := callCreateIssuance(tx, user.ID, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}

		var resp cancelResponse // same envelope shape: {success, message, data: IssuanceResponse}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		var sale data.Sale
		if err := tx.First(&sale, "issuance_id = ?", resp.Data.ID).Error; err != nil {
			t.Fatalf("expected a sale created alongside the issuance: %v", err)
		}
		if sale.ClientID != nil {
			t.Errorf("client_id: got %v, want nil", *sale.ClientID)
		}
		if sale.Paid {
			t.Error("expected the new sale to be unpaid")
		}
	})
}

func TestDeleteIssuance_DeletesSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		sale := data.Sale{IssuanceID: issuance.ID, Paid: true}
		create(t, tx, &sale)

		handler := NewIssuanceHandler(tx, events.NewBroker(), nil)
		req := httptest.NewRequest(http.MethodDelete, "/issuances/"+issuance.ID, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", issuance.ID)
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, middleware.UserContextKey, data.User{ID: user.ID, Role: "admin"})
		rec := httptest.NewRecorder()
		handler.DeleteIssuance(rec, req.WithContext(ctx))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		// The sale row is gone, not just unpaid: a deleted issuance is a mistake being
		// undone, so there is nothing left to charge anyone for.
		var persisted data.Sale
		err := tx.First(&persisted, "issuance_id = ?", issuance.ID).Error
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Errorf("expected the sale to be deleted, got err=%v sale=%+v", err, persisted)
		}
	})
}

func TestGetSales_OnlyMainAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		partner := data.Agency{Name: "Agência Parceira " + uuid.New().String()[:8]}
		create(t, tx, &partner)

		_, mainSale := seedSaleFixture(t, tx, main.ID)
		_, partnerSale := seedSaleFixture(t, tx, partner.ID)

		sales := callGetSales(t, tx, "")

		if findSale(sales, mainSale.ID) == nil {
			t.Error("expected the main agency's sale in the listing")
		}
		if findSale(sales, partnerSale.ID) != nil {
			t.Error("a partner agency's sale must never appear in the listing")
		}
	})
}

func TestGetSales_ExposesCancelledAt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)

		_, activeSale := seedSaleFixture(t, tx, main.ID)
		cancelledIssuance, cancelledSale := seedSaleFixture(t, tx, main.ID)

		if err := tx.Model(&cancelledIssuance).Updates(map[string]any{
			"active":       false,
			"cancelled_at": time.Now(),
		}).Error; err != nil {
			t.Fatalf("failed to cancel issuance: %v", err)
		}

		sales := callGetSales(t, tx, "")

		active := findSale(sales, activeSale.ID)
		if active == nil {
			t.Fatal("active sale missing from the listing")
		}
		if active.CancelledAt != nil {
			t.Errorf("expected cancelled_at nil on an active sale, got %v", active.CancelledAt)
		}

		cancelled := findSale(sales, cancelledSale.ID)
		if cancelled == nil {
			t.Fatal("cancelled sale missing from the listing")
		}
		if cancelled.CancelledAt == nil {
			t.Error("expected cancelled_at set on the reopened sale")
		}
	})
}

func TestGetSales_HidesSalesOfDeletedIssuances(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		deletedIssuance, orphanSale := seedSaleFixture(t, tx, main.ID)

		// Soft-deleting the issuance directly (not through DeleteIssuance) leaves the sale
		// behind, exactly like rows created before deletion started removing them. The
		// listing must filter them out on its own - the join is hand-written, so GORM's
		// soft-delete scope does not apply.
		if err := tx.Delete(&deletedIssuance).Error; err != nil {
			t.Fatalf("failed to soft-delete issuance: %v", err)
		}

		if findSale(callGetSales(t, tx, ""), orphanSale.ID) != nil {
			t.Error("a sale whose issuance was deleted must not appear in the listing")
		}
	})
}

func TestGetSales_FiltersByAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		partner := data.Agency{Name: "Agência Parceira " + uuid.New().String()[:8]}
		create(t, tx, &partner)

		_, mainSale := seedSaleFixture(t, tx, main.ID)
		_, partnerSale := seedSaleFixture(t, tx, partner.ID)

		// Asked for explicitly, a partner agency's sales are reachable - the default is a
		// convenience, not a wall.
		sales := callGetSales(t, tx, "agency_id="+partner.ID)
		if findSale(sales, partnerSale.ID) == nil {
			t.Error("expected the partner agency's sale when agency_id asks for it")
		}
		if findSale(sales, mainSale.ID) != nil {
			t.Error("the main agency's sale leaked into an explicit partner filter")
		}
	})
}

func TestGetSales_FiltersByIssuanceDimensions(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		wanted, wantedSale := seedSaleFixture(t, tx, main.ID)
		_, otherSale := seedSaleFixture(t, tx, main.ID)

		client := data.Client{Name: "Cliente " + uuid.New().String()[:8]}
		create(t, tx, &client)
		if err := tx.Model(&data.Sale{}).Where("id = ?", wantedSale.ID).
			Update("client_id", client.ID).Error; err != nil {
			t.Fatalf("failed to link client: %v", err)
		}

		for _, tc := range []struct{ name, query string }{
			{"loyalty_program_id", "loyalty_program_id=" + wanted.LoyaltyProgramID},
			{"account_id", "account_id=" + wanted.AccountID},
			{"issuer_id", "issuer_id=" + wanted.IssuerID},
			{"client_id", "client_id=" + client.ID},
		} {
			sales := callGetSales(t, tx, tc.query)
			if findSale(sales, wantedSale.ID) == nil {
				t.Errorf("%s: expected the matching sale", tc.name)
			}
			if findSale(sales, otherSale.ID) != nil {
				t.Errorf("%s: a non-matching sale leaked through", tc.name)
			}
		}
	})
}

func TestGetSales_ComputesAmount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, sale := seedSaleFixture(t, tx, main.ID)

		// 10.000 pontos a R$ 20,00/milheiro = R$ 200,00, mais R$ 35,50 de taxas.
		taxes := 3550
		if err := tx.Model(&issuance).Updates(map[string]any{
			"points":                10000,
			"value_of_the_thousand": 2000,
			"taxes":                 taxes,
		}).Error; err != nil {
			t.Fatalf("failed to set issuance amounts: %v", err)
		}

		got := findSale(callGetSales(t, tx, ""), sale.ID)
		if got == nil {
			t.Fatal("sale missing from the listing")
		}
		if got.AmountCents != 23550 {
			t.Errorf("amount_cents: got %d, want 23550", got.AmountCents)
		}
	})
}

func TestGetSales_FiltersByPaid(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		_, openSale := seedSaleFixture(t, tx, main.ID)
		_, paidSale := seedSaleFixture(t, tx, main.ID)

		if err := tx.Model(&data.Sale{}).Where("id = ?", paidSale.ID).Update("paid", true).Error; err != nil {
			t.Fatalf("failed to mark sale as paid: %v", err)
		}

		open := callGetSales(t, tx, "paid=false")
		if findSale(open, openSale.ID) == nil {
			t.Error("expected the open sale under paid=false")
		}
		if findSale(open, paidSale.ID) != nil {
			t.Error("paid sale leaked into paid=false")
		}

		paid := callGetSales(t, tx, "paid=true")
		if findSale(paid, paidSale.ID) == nil {
			t.Error("expected the paid sale under paid=true")
		}
		if findSale(paid, openSale.ID) != nil {
			t.Error("open sale leaked into paid=true")
		}
	})
}

func callUpdateSale(tx *gorm.DB, id, body string) *httptest.ResponseRecorder {
	handler := NewSaleHandler(tx, events.NewBroker())

	req := httptest.NewRequest(http.MethodPatch, "/sales/"+id, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	handler.UpdateSale(rec, req)
	return rec
}

func TestUpdateSale_LinksClientAndMarksPaid(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		_, sale := seedSaleFixture(t, tx, main.ID)

		client := data.Client{Name: "Cliente " + uuid.New().String()[:8]}
		create(t, tx, &client)

		rec := callUpdateSale(tx, sale.ID, `{"client_id":"`+client.ID+`","paid":true}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var resp saleSingleResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Data.ClientID == nil || *resp.Data.ClientID != client.ID {
			t.Errorf("client_id: got %v, want %q", resp.Data.ClientID, client.ID)
		}
		if resp.Data.ClientName == nil || *resp.Data.ClientName != client.Name {
			t.Errorf("client_name: got %v, want %q", resp.Data.ClientName, client.Name)
		}
		if !resp.Data.Paid {
			t.Error("expected paid=true in the response")
		}

		var persisted data.Sale
		if err := tx.First(&persisted, "id = ?", sale.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		if persisted.ClientID == nil || *persisted.ClientID != client.ID || !persisted.Paid {
			t.Errorf("persisted sale: client_id=%v paid=%v", persisted.ClientID, persisted.Paid)
		}
	})
}

func TestUpdateSale_NullClientUnlinks(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		_, sale := seedSaleFixture(t, tx, main.ID)

		client := data.Client{Name: "Cliente " + uuid.New().String()[:8]}
		create(t, tx, &client)
		if err := tx.Model(&data.Sale{}).Where("id = ?", sale.ID).Update("client_id", client.ID).Error; err != nil {
			t.Fatalf("failed to pre-link client: %v", err)
		}

		if rec := callUpdateSale(tx, sale.ID, `{"client_id":null}`); rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var persisted data.Sale
		if err := tx.First(&persisted, "id = ?", sale.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		if persisted.ClientID != nil {
			t.Errorf("expected client_id cleared, got %q", *persisted.ClientID)
		}
	})
}

func TestUpdateSale_OmittedClientKeepsLink(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		_, sale := seedSaleFixture(t, tx, main.ID)

		client := data.Client{Name: "Cliente " + uuid.New().String()[:8]}
		create(t, tx, &client)
		if err := tx.Model(&data.Sale{}).Where("id = ?", sale.ID).Update("client_id", client.ID).Error; err != nil {
			t.Fatalf("failed to pre-link client: %v", err)
		}

		// Only paid is sent: the absent client_id must not be read as "unlink".
		if rec := callUpdateSale(tx, sale.ID, `{"paid":true}`); rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var persisted data.Sale
		if err := tx.First(&persisted, "id = ?", sale.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		if persisted.ClientID == nil || *persisted.ClientID != client.ID {
			t.Errorf("expected the client link untouched, got %v", persisted.ClientID)
		}
	})
}

func TestUpdateSale_UnknownClientIsUnprocessable(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		_, sale := seedSaleFixture(t, tx, main.ID)

		rec := callUpdateSale(tx, sale.ID, `{"client_id":"`+uuid.New().String()+`"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateSale_NotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		defaultAgency(t, tx)

		rec := callUpdateSale(tx, uuid.New().String(), `{"paid":true}`)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateSale_EmptyPayloadIsBadRequest(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		_, sale := seedSaleFixture(t, tx, main.ID)

		rec := callUpdateSale(tx, sale.ID, `{}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// Marking a sale as paid is invisible to the issuance routes, so this event is the only
// thing that tells the other open panels about it. The broker is driven directly rather
// than through the SSE handler: Broadcast is synchronous, so a buffered subscriber already
// has the event by the time UpdateSale returns.
func TestUpdateSale_BroadcastsSaleUpdated(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		_, sale := seedSaleFixture(t, tx, main.ID)

		broker := events.NewBroker()
		subscriber := broker.Subscribe()
		defer broker.Unsubscribe(subscriber)

		handler := NewSaleHandler(tx, broker)
		req := httptest.NewRequest(http.MethodPatch, "/sales/"+sale.ID, strings.NewReader(`{"paid":true}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", sale.ID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateSale(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		select {
		case event := <-subscriber:
			if event.Type != eventSaleUpdated {
				t.Fatalf("event type: got %q, want %q", event.Type, eventSaleUpdated)
			}
			payload, ok := event.Payload.(map[string]string)
			if !ok || payload["id"] != sale.ID {
				t.Fatalf("event payload: got %#v, want the sale id %q", event.Payload, sale.ID)
			}
		default:
			t.Fatal("no event was broadcast")
		}
	})
}

// A failed update must not reach the stream: a panel told to refetch would find the sale
// exactly as it was, and the next real change would look like it had already been applied.
func TestUpdateSale_DoesNotBroadcastOnFailure(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		broker := events.NewBroker()
		subscriber := broker.Subscribe()
		defer broker.Unsubscribe(subscriber)

		handler := NewSaleHandler(tx, broker)
		unknownID := uuid.New().String()
		req := httptest.NewRequest(http.MethodPatch, "/sales/"+unknownID, strings.NewReader(`{"paid":true}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", unknownID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateSale(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}

		select {
		case event := <-subscriber:
			t.Fatalf("unexpected event broadcast: %#v", event)
		default:
		}
	})
}

// callGetSupplierBalances drives the handler directly. It takes no role: the admin gate
// lives on the route (RequireAdmin), not in the handler - see the router test below.
func callGetSupplierBalances(t *testing.T, tx *gorm.DB, query string) []schemas.SupplierBalanceResponse {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())

	req := httptest.NewRequest(http.MethodGet, "/sales/suppliers?"+query, nil)
	rec := httptest.NewRecorder()
	handler.GetSupplierBalances(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetSupplierBalances status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp supplierListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode supplier response: %v", err)
	}
	return resp.Data
}

func findBalance(balances []schemas.SupplierBalanceResponse, accountID string) *schemas.SupplierBalanceResponse {
	for i := range balances {
		if balances[i].AccountID == accountID {
			return &balances[i]
		}
	}
	return nil
}

// spDay renders a day offset from now in the listing timezone, matching how the handler
// resolves date_from/date_to.
func spDay(t *testing.T, offsetDays int) string {
	t.Helper()
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).AddDate(0, 0, offsetDays).Format("2006-01-02")
}

func TestGetSupplierBalances_NetsRefundsInTheSamePeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, _ := seedSaleFixture(t, tx, main.ID)

		// 10.000 pontos emitidos hoje, 4.000 devolvidos por um cancelamento hoje.
		if err := tx.Model(&issuance).Updates(map[string]any{
			"points":                10000,
			"value_of_the_thousand": 2000,
			"active":                false,
			"cancelled_at":          time.Now(),
			"refunded_points":       4000,
		}).Error; err != nil {
			t.Fatalf("failed to set up the cancelled issuance: %v", err)
		}

		today := spDay(t, 0)
		balances := callGetSupplierBalances(t, tx, "date_from="+today+"&date_to="+today)

		got := findBalance(balances, issuance.AccountID)
		if got == nil {
			t.Fatal("account missing from the supplier balances")
		}
		if got.IssuedPoints != 10000 {
			t.Errorf("issued_points: got %d, want 10000", got.IssuedPoints)
		}
		if got.RefundedPoints != 4000 {
			t.Errorf("refunded_points: got %d, want 4000", got.RefundedPoints)
		}
		if got.NetPoints != 6000 {
			t.Errorf("net_points: got %d, want 6000", got.NetPoints)
		}
	})
}

// The statement is the un-aggregated version of the totals, so it has to agree with them
// line by line. An issuance created and cancelled inside the same window produces two
// lines - the booking and the credit - and they have to net to the same figure the totals
// report.
func TestGetSupplierBalances_StatementMatchesTheTotals(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, _ := seedSaleFixture(t, tx, main.ID)

		if err := tx.Model(&issuance).Updates(map[string]any{
			"points":                10000,
			"value_of_the_thousand": 2000,
			"taxes":                 8400,
			"cost_of_the_thousand":  1650,
			"active":                false,
			"cancelled_at":          time.Now(),
			"refunded_points":       4000,
			"refunded_taxes":        3000,
		}).Error; err != nil {
			t.Fatalf("failed to set up the cancelled issuance: %v", err)
		}

		today := spDay(t, 0)
		got := findBalance(callGetSupplierBalances(t, tx, "date_from="+today+"&date_to="+today), issuance.AccountID)
		if got == nil {
			t.Fatal("account missing from the supplier balances")
		}
		if len(got.Issuances) != 2 {
			t.Fatalf("statement lines: got %d, want 2 (%+v)", len(got.Issuances), got.Issuances)
		}

		issued, refund := got.Issuances[0], got.Issuances[1]
		if issued.Kind != "issued" || refund.Kind != "refund" {
			t.Fatalf("expected the booking before the credit, got %q then %q", issued.Kind, refund.Kind)
		}
		if issued.Points != 10000 {
			t.Errorf("issued points: got %d, want 10000", issued.Points)
		}
		if issued.Taxes == nil || *issued.Taxes != 8400 {
			t.Errorf("issued taxes: got %v, want 8400", issued.Taxes)
		}
		// Signed at the source so the panel only ever sums.
		if refund.Points != -4000 {
			t.Errorf("refund points: got %d, want -4000", refund.Points)
		}
		if refund.Taxes == nil || *refund.Taxes != -3000 {
			t.Errorf("refund taxes: got %v, want -3000", refund.Taxes)
		}
		if issued.CostOfTheThousand == nil || *issued.CostOfTheThousand != 1650 {
			t.Errorf("cost_of_the_thousand: got %v, want 1650", issued.CostOfTheThousand)
		}
		if issued.ProgramLocator != issuance.ProgramLocator {
			t.Errorf("program_locator: got %q, want %q", issued.ProgramLocator, issuance.ProgramLocator)
		}
		if issued.IssuanceID != issuance.ID || refund.IssuanceID != issuance.ID {
			t.Error("both lines must point back at the issuance that produced them")
		}

		// The whole point of the statement: summing it reproduces the totals beside it.
		sum := issued.Points + refund.Points
		if sum != got.NetPoints {
			t.Errorf("statement sums to %d but net_points is %d", sum, got.NetPoints)
		}
	})
}

// A booking made before the window and cancelled inside it is money to be clawed back in
// the window of the cancellation, not of the booking - so the only line is the credit.
func TestGetSupplierBalances_StatementCreditsACancellationOnItsOwnDay(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, _ := seedSaleFixture(t, tx, main.ID)

		if err := tx.Model(&issuance).Updates(map[string]any{
			"points":                10000,
			"value_of_the_thousand": 2000,
			"cost_of_the_thousand":  1650,
			"created_at":            time.Now().AddDate(0, 0, -7),
			"active":                false,
			"cancelled_at":          time.Now(),
			"refunded_points":       10000,
		}).Error; err != nil {
			t.Fatalf("failed to set up the cancelled issuance: %v", err)
		}

		today := spDay(t, 0)
		got := findBalance(callGetSupplierBalances(t, tx, "date_from="+today+"&date_to="+today), issuance.AccountID)
		if got == nil {
			t.Fatal("account missing from the supplier balances")
		}
		if len(got.Issuances) != 1 {
			t.Fatalf("statement lines: got %d, want 1 (%+v)", len(got.Issuances), got.Issuances)
		}
		if got.Issuances[0].Kind != "refund" || got.Issuances[0].Points != -10000 {
			t.Errorf("got %s line of %d points, want a refund of -10000",
				got.Issuances[0].Kind, got.Issuances[0].Points)
		}
		if got.NetPoints != -10000 {
			t.Errorf("net_points: got %d, want -10000 - the agency is owed miles back", got.NetPoints)
		}
	})
}

func TestGetSupplierBalances_IgnoresCancellationOutsideThePeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, _ := seedSaleFixture(t, tx, main.ID)

		// Emitida ontem, cancelada hoje: no filtro de ontem só a emissão conta.
		yesterday := time.Now().AddDate(0, 0, -1)
		if err := tx.Model(&issuance).Updates(map[string]any{
			"points":                10000,
			"value_of_the_thousand": 2000,
			"created_at":            yesterday,
			"active":                false,
			"cancelled_at":          time.Now(),
			"refunded_points":       4000,
		}).Error; err != nil {
			t.Fatalf("failed to set up the cancelled issuance: %v", err)
		}

		day := spDay(t, -1)
		balances := callGetSupplierBalances(t, tx, "date_from="+day+"&date_to="+day)

		got := findBalance(balances, issuance.AccountID)
		if got == nil {
			t.Fatal("account missing from the supplier balances")
		}
		if got.IssuedPoints != 10000 || got.RefundedPoints != 0 || got.NetPoints != 10000 {
			t.Errorf("got issued=%d refunded=%d net=%d, want 10000/0/10000",
				got.IssuedPoints, got.RefundedPoints, got.NetPoints)
		}
	})
}

func TestGetSupplierBalances_ExcludesDeletedIssuances(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, _ := seedSaleFixture(t, tx, main.ID)

		if err := tx.Delete(&issuance).Error; err != nil {
			t.Fatalf("failed to soft-delete issuance: %v", err)
		}

		today := spDay(t, 0)
		balances := callGetSupplierBalances(t, tx, "date_from="+today+"&date_to="+today)

		if findBalance(balances, issuance.AccountID) != nil {
			t.Error("a deleted issuance must not create a debt with the account's administrator")
		}
	})
}

func TestGetSupplierBalances_OnlyMainAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		partner := data.Agency{Name: "Agência Parceira " + uuid.New().String()[:8]}
		create(t, tx, &partner)

		mainIssuance, _ := seedSaleFixture(t, tx, main.ID)
		partnerIssuance, _ := seedSaleFixture(t, tx, partner.ID)

		today := spDay(t, 0)
		balances := callGetSupplierBalances(t, tx, "date_from="+today+"&date_to="+today)

		if findBalance(balances, mainIssuance.AccountID) == nil {
			t.Error("expected the main agency's account in the balances")
		}
		if findBalance(balances, partnerIssuance.AccountID) != nil {
			t.Error("a partner agency's account must never appear in the balances")
		}
	})
}

func TestGetSupplierBalances_FiltersByAgencyProgramAndAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		partner := data.Agency{Name: "Agência Parceira " + uuid.New().String()[:8]}
		create(t, tx, &partner)

		wanted, _ := seedSaleFixture(t, tx, main.ID)
		other, _ := seedSaleFixture(t, tx, main.ID)
		partnerIssuance, _ := seedSaleFixture(t, tx, partner.ID)

		today := spDay(t, 0)
		period := "date_from=" + today + "&date_to=" + today

		// Asked for explicitly, the partner agency's accounts are reachable.
		partnerBalances := callGetSupplierBalances(t, tx, period+"&agency_id="+partner.ID)
		if findBalance(partnerBalances, partnerIssuance.AccountID) == nil {
			t.Error("expected the partner agency's account when agency_id asks for it")
		}

		for _, tc := range []struct{ name, query string }{
			{"loyalty_program_id", period + "&loyalty_program_id=" + wanted.LoyaltyProgramID},
			{"account_id", period + "&account_id=" + wanted.AccountID},
		} {
			balances := callGetSupplierBalances(t, tx, tc.query)
			if findBalance(balances, wanted.AccountID) == nil {
				t.Errorf("%s: expected the matching account", tc.name)
			}
			if findBalance(balances, other.AccountID) != nil {
				t.Errorf("%s: a non-matching account leaked through", tc.name)
			}
		}
	})
}

func callSetSupplierCost(t *testing.T, tx *gorm.DB, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())

	req := httptest.NewRequest(http.MethodPost, "/sales/suppliers/issuances", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.UpdateSupplierIssuances(rec, req)
	return rec
}

func taxesIncludedInCost(t *testing.T, tx *gorm.DB, issuanceID string) bool {
	t.Helper()
	var issuance data.Issuance
	if err := tx.First(&issuance, "id = ?", issuanceID).Error; err != nil {
		t.Fatalf("failed to re-read issuance %s: %v", issuanceID, err)
	}
	return issuance.TaxesIncludedInCost
}

func costOfTheThousand(t *testing.T, tx *gorm.DB, issuanceID string) *int {
	t.Helper()
	var issuance data.Issuance
	if err := tx.First(&issuance, "id = ?", issuanceID).Error; err != nil {
		t.Fatalf("failed to re-read issuance %s: %v", issuanceID, err)
	}
	return issuance.CostOfTheThousand
}

func TestSetSupplierCost_WritesOnlyTheListedIssuances(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		first, _ := seedSaleFixture(t, tx, main.ID)
		second, _ := seedSaleFixture(t, tx, main.ID)
		untouched, _ := seedSaleFixture(t, tx, main.ID)

		rec := callSetSupplierCost(t, tx, `{"issuance_ids":["`+first.ID+`","`+second.ID+`"],"cost_of_the_thousand":1650}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var resp struct {
			Data schemas.UpdateSupplierIssuancesResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Data.Updated != 2 {
			t.Errorf("updated: got %d, want 2", resp.Data.Updated)
		}

		for _, id := range []string{first.ID, second.ID} {
			got := costOfTheThousand(t, tx, id)
			if got == nil || *got != 1650 {
				t.Errorf("issuance %s: cost got %v, want 1650", id, got)
			}
		}
		// The endpoint takes ids and never a filter precisely so this can be asserted:
		// nothing outside the list the operator confirmed is written.
		if got := costOfTheThousand(t, tx, untouched.ID); got != nil {
			t.Errorf("an issuance outside the list was written: got %v, want nil", got)
		}
	})
}

// Null is a real value here: it puts the issuance back to "cost not agreed yet", which is
// what makes the payment summary refuse to generate again.
func TestSetSupplierCost_NullClearsTheCost(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, _ := seedSaleFixture(t, tx, main.ID)

		if rec := callSetSupplierCost(t, tx, `{"issuance_ids":["`+issuance.ID+`"],"cost_of_the_thousand":1650}`); rec.Code != http.StatusOK {
			t.Fatalf("seeding the cost failed: %d (%s)", rec.Code, rec.Body.String())
		}
		if rec := callSetSupplierCost(t, tx, `{"issuance_ids":["`+issuance.ID+`"],"cost_of_the_thousand":null}`); rec.Code != http.StatusOK {
			t.Fatalf("clearing the cost failed: %d (%s)", rec.Code, rec.Body.String())
		}

		if got := costOfTheThousand(t, tx, issuance.ID); got != nil {
			t.Errorf("cost after clearing: got %v, want nil", got)
		}
	})
}

// The tax buttons send no cost_of_the_thousand at all, and that has to leave the rates
// alone. Absent and explicitly null look the same to a *int, which is why the handler
// decodes the body twice - and this is what would catch that coming undone: without it, one
// press of "incluir taxas em todas" would wipe every rate the operator had entered.
func TestUpdateSupplierIssuances_OmittedCostIsNotAClearedCost(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, _ := seedSaleFixture(t, tx, main.ID)

		if rec := callSetSupplierCost(t, tx, `{"issuance_ids":["`+issuance.ID+`"],"cost_of_the_thousand":1650}`); rec.Code != http.StatusOK {
			t.Fatalf("seeding the cost failed: %d (%s)", rec.Code, rec.Body.String())
		}

		// Only the tax flag is sent.
		if rec := callSetSupplierCost(t, tx, `{"issuance_ids":["`+issuance.ID+`"],"taxes_included_in_cost":true}`); rec.Code != http.StatusOK {
			t.Fatalf("setting the tax flag failed: %d (%s)", rec.Code, rec.Body.String())
		}

		if got := costOfTheThousand(t, tx, issuance.ID); got == nil || *got != 1650 {
			t.Errorf("the rate was touched by a tax-only write: got %v, want 1650", got)
		}
		if !taxesIncludedInCost(t, tx, issuance.ID) {
			t.Error("expected the taxes to be marked as included")
		}

		// And the inverse: setting only the rate leaves the tax flag alone.
		if rec := callSetSupplierCost(t, tx, `{"issuance_ids":["`+issuance.ID+`"],"cost_of_the_thousand":1800}`); rec.Code != http.StatusOK {
			t.Fatalf("updating the rate failed: %d (%s)", rec.Code, rec.Body.String())
		}
		if !taxesIncludedInCost(t, tx, issuance.ID) {
			t.Error("the tax flag was cleared by a rate-only write")
		}
	})
}

// An UPDATE with no WHERE must not be reachable through this endpoint, whatever the body
// says. Both of these would otherwise rewrite the cost of every issuance in the database.
func TestSetSupplierCost_RejectsAnEmptyOrMissingIDList(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, _ := seedSaleFixture(t, tx, main.ID)

		for _, tc := range []struct{ name, body string }{
			{"empty list", `{"issuance_ids":[],"cost_of_the_thousand":1650}`},
			{"no fields", `{"issuance_ids":["` + issuance.ID + `"]}`},
			{"absent list", `{"cost_of_the_thousand":1650}`},
			{"not a uuid", `{"issuance_ids":["nope"],"cost_of_the_thousand":1650}`},
			{"negative cost", `{"issuance_ids":["` + issuance.ID + `"],"cost_of_the_thousand":-1}`},
		} {
			rec := callSetSupplierCost(t, tx, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: status got %d, want 400 (body: %s)", tc.name, rec.Code, rec.Body.String())
			}
		}

		if got := costOfTheThousand(t, tx, issuance.ID); got != nil {
			t.Errorf("a rejected request still wrote: got %v, want nil", got)
		}
	})
}

// agency_ids is the suppliers view's multi-valued agency choice. The three cases below are
// the whole contract: a list narrows to exactly that list, "all" drops the agency filter
// entirely, and an absent param still means the main agency - the last one being what keeps
// every existing bookmark and sticky-filter cookie behaving as it did.
func TestGetSupplierBalances_FiltersByMultipleAgencies(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		partnerA := data.Agency{Name: "Parceira A " + uuid.New().String()[:8]}
		create(t, tx, &partnerA)
		partnerB := data.Agency{Name: "Parceira B " + uuid.New().String()[:8]}
		create(t, tx, &partnerB)

		mainIssuance, _ := seedSaleFixture(t, tx, main.ID)
		issuanceA, _ := seedSaleFixture(t, tx, partnerA.ID)
		issuanceB, _ := seedSaleFixture(t, tx, partnerB.ID)

		today := spDay(t, 0)
		period := "date_from=" + today + "&date_to=" + today

		twoOfThree := callGetSupplierBalances(t, tx, period+"&agency_ids="+main.ID+","+partnerA.ID)
		if findBalance(twoOfThree, mainIssuance.AccountID) == nil {
			t.Error("agency_ids: expected the main agency's account")
		}
		if findBalance(twoOfThree, issuanceA.AccountID) == nil {
			t.Error("agency_ids: expected partner A's account")
		}
		if findBalance(twoOfThree, issuanceB.AccountID) != nil {
			t.Error("agency_ids: partner B was not asked for and must not appear")
		}

		all := callGetSupplierBalances(t, tx, period+"&agency_ids=all")
		for name, accountID := range map[string]string{
			"main":     mainIssuance.AccountID,
			"partnerA": issuanceA.AccountID,
			"partnerB": issuanceB.AccountID,
		} {
			if findBalance(all, accountID) == nil {
				t.Errorf("agency_ids=all: expected %s's account", name)
			}
		}

		// The param being absent is not the same as "none selected": it still has to
		// fall back to the main agency.
		absent := callGetSupplierBalances(t, tx, period)
		if findBalance(absent, issuanceA.AccountID) != nil {
			t.Error("without agency_ids the query must stay on the main agency")
		}
	})
}

func TestGetSupplierBalances_FiltersByAdministrator(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)

		administrator := data.Person{Name: "Administradora " + uuid.New().String()[:8]}
		create(t, tx, &administrator)

		administered, _ := seedSaleFixture(t, tx, main.ID)
		if err := tx.Model(&data.Account{}).Where("id = ?", administered.AccountID).
			Update("administrator_id", administrator.ID).Error; err != nil {
			t.Fatalf("failed to set the account administrator: %v", err)
		}
		other, _ := seedSaleFixture(t, tx, main.ID)

		today := spDay(t, 0)
		balances := callGetSupplierBalances(t, tx,
			"date_from="+today+"&date_to="+today+"&administrator_id="+administrator.ID)

		match := findBalance(balances, administered.AccountID)
		if match == nil {
			t.Fatal("expected the administered account")
		}
		if match.AdministratorName == nil || *match.AdministratorName != administrator.Name {
			t.Errorf("administrator_name: got %v, want %q", match.AdministratorName, administrator.Name)
		}
		if findBalance(balances, other.AccountID) != nil {
			t.Error("an account with a different administrator leaked through")
		}
	})
}

// The suppliers view is the one surface that never sees the issuance's alias, so it has
// to reach the account's own. Before the account_nicknames sub-select existed it fell
// through to the raw 36-character UUID for any account identified only by an alias.
func TestGetSupplierBalances_LabelsAccountByNicknameAndNeverByFullUUID(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		issuance, _ := seedSaleFixture(t, tx, main.ID)

		today := spDay(t, 0)
		period := "date_from=" + today + "&date_to=" + today

		// No holder, no login and no alias: the label degrades to a short id rather
		// than dumping the whole UUID into the table.
		bare := findBalance(callGetSupplierBalances(t, tx, period), issuance.AccountID)
		if bare == nil {
			t.Fatal("account missing from the supplier balances")
		}
		if bare.AccountLabel == issuance.AccountID {
			t.Error("account_label is the raw UUID; it should be truncated when nothing else identifies the account")
		}
		if want := issuance.AccountID[:8]; bare.AccountLabel != want {
			t.Errorf("account_label: got %q, want %q", bare.AccountLabel, want)
		}

		// An alias registered on the account itself is enough to name the row.
		nickname := data.AccountNickname{
			AccountID:        issuance.AccountID,
			LoyaltyProgramID: issuance.LoyaltyProgramID,
			Nickname:         "conta-do-joao-" + uuid.New().String()[:8],
		}
		create(t, tx, &nickname)

		named := findBalance(callGetSupplierBalances(t, tx, period), issuance.AccountID)
		if named == nil {
			t.Fatal("account missing from the supplier balances after adding a nickname")
		}
		if named.AccountLabel != nickname.Nickname {
			t.Errorf("account_label: got %q, want the account's nickname %q", named.AccountLabel, nickname.Nickname)
		}
	})
}
