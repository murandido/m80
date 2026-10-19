package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

func callGetSalesPage(t *testing.T, tx *gorm.DB, query string) schemas.SalePageResponse {
	t.Helper()
	handler := NewSaleHandler(tx, events.NewBroker())
	req := withScope(httptest.NewRequest(http.MethodGet, "/sales/page?"+query, nil), access.FullScope())
	rec := httptest.NewRecorder()
	handler.GetSalesPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetSalesPage status: got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data schemas.SalePageResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding the page: %v", err)
	}
	return resp.Data
}

// A window is a slice of the full listing: same order, the total of the filter, and the
// "Total devido" over every sale of the one client asked for, not just the window.
func TestGetSalesPageIsAWindowOfTheListing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		base := time.Now().AddDate(0, 0, -1)
		ids := make([]string, 0, 5)
		for i := range 5 {
			s := seedClientSale(t, tx, client.ID, 1_000*(i+1), base.Add(time.Duration(i)*time.Minute))
			ids = append(ids, s.ID)
		}
		filter := "client_ids=" + client.ID

		page := callGetSalesPage(t, tx, filter+"&offset=1&limit=2")
		if page.Total != 5 || page.Offset != 1 || page.Limit != 2 || len(page.Sales) != 2 {
			t.Fatalf("page = total %d offset %d limit %d len %d, want 5/1/2/2", page.Total, page.Offset, page.Limit, len(page.Sales))
		}
		if page.Sales[0].ID != ids[1] || page.Sales[1].ID != ids[2] {
			t.Errorf("window = %s, %s; want the 2nd and 3rd oldest", page.Sales[0].ID, page.Sales[1].ID)
		}
		// 1000 + 2000 + 3000 + 4000 + 5000, none paid.
		if page.OwedCents != 15_000 {
			t.Errorf("owed = %d, want 15000 over every sale of the client", page.OwedCents)
		}

		last := callGetSalesPage(t, tx, filter+"&offset=4&limit="+strconv.Itoa(100))
		if len(last.Sales) != 1 || last.Sales[0].ID != ids[4] {
			t.Errorf("last window = %d sales, want only the newest", len(last.Sales))
		}

		if huge := callGetSalesPage(t, tx, filter+"&limit=100000"); huge.Limit != salePageMaxLimit {
			t.Errorf("limit = %d, want capped at %d", huge.Limit, salePageMaxLimit)
		}
	})
}

// The search finds a sale by what the operator has in hand - here the PNR and the passenger,
// spelled without the accent - and stays inside the other filters: a match of another
// client's does not leak in through the OR of the search.
func TestGetSalesPageSearchesByLocatorAndPassenger(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := seedClient(t, tx)
		other := seedClient(t, tx)
		issuedAt := time.Now().AddDate(0, 0, -1)
		target := seedClientSale(t, tx, client.ID, 1_000, issuedAt)
		seedClientSale(t, tx, client.ID, 2_000, issuedAt)
		elsewhere := seedClientSale(t, tx, other.ID, 3_000, issuedAt)

		// The locator is unique among live issuances, so the other client's one only
		// contains it - which the search still matches.
		locator := "Q" + strings.ToUpper(uuid.New().String()[:5])
		for saleID, saleLocator := range map[string]string{
			target.ID:    locator,
			elsewhere.ID: locator + "X",
		} {
			err := tx.Exec(`UPDATE issuances SET program_locator = ?, main_passenger_name = ?
				WHERE id = (SELECT issuance_id FROM sales WHERE id = ?)`, saleLocator, "João Buscável", saleID).Error
			if err != nil {
				t.Fatalf("failed to name the issuance: %v", err)
			}
		}

		for _, search := range []string{locator, strings.ToLower(locator), "joao buscavel"} {
			page := callGetSalesPage(t, tx, "client_ids="+client.ID+"&q="+url.QueryEscape(search))
			if page.Total != 1 || len(page.Sales) != 1 || page.Sales[0].ID != target.ID {
				t.Errorf("q=%q: total %d, want only the matching sale of the filtered client", search, page.Total)
			}
		}

		if page := callGetSalesPage(t, tx, "client_ids="+client.ID+"&q=ZZZNADA"); page.Total != 0 {
			t.Errorf("a search matching nothing returned %d sales", page.Total)
		}
	})
}
