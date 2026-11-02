package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/inventory"
	"gorm.io/gorm"
)

// closingWindow is yesterday through today, on the costing's own calendar. The booking is
// created now and the costing dates it with inventory.DayOf, so the window is built from the
// same function rather than from a timezone this test would have to guess.
func closingWindow() (string, string) {
	today := inventory.DayOf(time.Now())
	return today.AddDate(0, 0, -1).Format(batchDateLayout), today.Format(batchDateLayout)
}

func callGetInventoryClosing(tx *gorm.DB, scope access.AgencyScope, query string) *httptest.ResponseRecorder {
	req := withScope(httptest.NewRequest(http.MethodGet, "/inventory-closing?"+query, nil), scope)
	rec := httptest.NewRecorder()
	NewInventoryClosingHandler(tx).GetInventoryClosing(rec, req)
	return rec
}

func readClosing(t *testing.T, tx *gorm.DB, scope access.AgencyScope, accountIDs []string) schemas.InventoryClosingDataResponse {
	t.Helper()
	from, to := closingWindow()
	rec := callGetInventoryClosing(tx, scope, "account_ids="+strings.Join(accountIDs, ",")+"&from="+from+"&to="+to)
	var envelope schemas.InventoryClosingListResponse
	decodeOK(t, rec, "GetInventoryClosing", &envelope)
	return envelope.Data
}

// From the rows to the report: two accounts, each a 10.000-point booking at R$ 20 the
// thousand (R$ 200) drawn from a batch that cost R$ 150, reported one by one and added up
// underneath. Then the range is closed and the same read says so.
func TestGetInventoryClosing_ReportsEachAccountAndTheirSum(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		cost := 15000
		first := seedInventoryBooking(t, tx, &cost)
		second := seedInventoryBooking(t, tx, &cost)
		accountIDs := []string{first.AccountID, second.AccountID}

		report := readClosing(t, tx, access.FullScope(), accountIDs)
		if len(report.Accounts) != 2 {
			t.Fatalf("accounts: want 2, got %d", len(report.Accounts))
		}
		for index, account := range report.Accounts {
			if account.AccountID != accountIDs[index] {
				t.Errorf("account %d: want %s in request order, got %s", index, accountIDs[index], account.AccountID)
			}
			if account.Closed || account.ClosedPeriodID != nil {
				t.Errorf("account %d: an untouched range came back closed", index)
			}
			if len(account.Batches) != 1 {
				t.Fatalf("account %d: want the one batch that was drawn on, got %d", index, len(account.Batches))
			}
			batch := account.Batches[0]
			if batch.ConsumedPoints != 10000 || batch.ChargedCents != 15000 || batch.RevenueCents != 20000 || batch.ProfitCents != 5000 {
				t.Errorf("account %d batch: want 10000 points, 15000 cost, 20000 revenue, 5000 profit; got %+v", index, batch)
			}
			if batch.AcquiredOn == "" {
				t.Errorf("account %d batch: the label lost its acquisition date", index)
			}
			if account.CostCents != 15000 || account.RevenueCents != 20000 || account.ProfitCents != 5000 || account.PendingPoints != 0 {
				t.Errorf("account %d totals: got cost %d, revenue %d, profit %d, pending %d",
					index, account.CostCents, account.RevenueCents, account.ProfitCents, account.PendingPoints)
			}
			if account.ExpectedBalancePoints != 0 {
				t.Errorf("account %d: 10000 in and 10000 out should leave 0, got %d", index, account.ExpectedBalancePoints)
			}
			// One agency funded and issued, so nobody owes anybody.
			if len(account.Settlements) != 0 {
				t.Errorf("account %d: want no settlement within one agency, got %+v", index, account.Settlements)
			}
		}
		if report.Totals.CostCents != 30000 || report.Totals.RevenueCents != 40000 || report.Totals.ProfitCents != 10000 {
			t.Errorf("totals: want 30000/40000/10000, got %d/%d/%d",
				report.Totals.CostCents, report.Totals.RevenueCents, report.Totals.ProfitCents)
		}

		// Close the first account through today: the same read now names the period, and the
		// figures it froze are the ones it was showing.
		_, to := closingWindow()
		ctx := asUser(context.Background(), createUser(t, tx, "Fechador"))
		period := decodePeriod(t, callClosePeriod(tx, closePeriodRequest(ctx, closePeriodBody(first.AccountID, "2026-01-01", to))))

		report = readClosing(t, tx, access.FullScope(), accountIDs)
		closed, open := report.Accounts[0], report.Accounts[1]
		if !closed.Closed || closed.ClosedPeriodID == nil || *closed.ClosedPeriodID != period.ID {
			t.Errorf("closed account: want closed with period %s, got closed=%v id=%v", period.ID, closed.Closed, closed.ClosedPeriodID)
		}
		if closed.CostCents != 15000 || closed.RevenueCents != 20000 {
			t.Errorf("closed account: the frozen figures moved: cost %d, revenue %d", closed.CostCents, closed.RevenueCents)
		}
		if open.Closed {
			t.Error("the other account came back closed too")
		}
	})
}

// Two agencies on one account: A paid for the batch and B issued against it. The report is
// filtered to the caller's side by scopeClosing (unit-tested in inventory_scope_test.go);
// this is the same filter seen through the route.
func TestGetInventoryClosing_ShowsOnlyTheCallersSide(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		funder := seedNamedAgency(t, tx, "Funder")
		issuer := seedNamedAgency(t, tx, "Issuer")
		outsider := seedNamedAgency(t, tx, "Outsider")

		booking, _ := seedSaleFixture(t, tx, issuer.ID)
		if err := tx.Model(&data.Account{}).Where("id = ?", booking.AccountID).
			Update("owns_point_inventory", true).Error; err != nil {
			t.Fatalf("failed to turn the inventory on: %v", err)
		}
		if err := tx.Model(&booking).Updates(map[string]any{"points": 10000, "value_of_the_thousand": 2000}).Error; err != nil {
			t.Fatalf("failed to size the booking: %v", err)
		}
		batch := data.PointBatch{
			AccountID: booking.AccountID, AcquiredOn: time.Now().AddDate(0, 0, -1), Points: 10000,
			CostCents: 15000, FundingAgencyID: funder.ID, ProfitSharePercent: 50, Opening: true,
		}
		create(t, tx, &batch)
		accountIDs := []string{booking.AccountID}

		full := readClosing(t, tx, access.FullScope(), accountIDs).Accounts[0]
		if len(full.Batches) != 1 || full.CostCents != 15000 {
			t.Fatalf("full scope: want the batch and its 15000 cost, got %d batches, cost %d", len(full.Batches), full.CostCents)
		}
		owes := false
		for _, settlement := range full.Settlements {
			if settlement.AgencyID == issuer.ID && settlement.ToAgencyID == funder.ID && settlement.AmountCents > 0 {
				owes = true
			}
		}
		if !owes {
			t.Errorf("full scope: want the issuer to owe the funder, got %+v", full.Settlements)
		}

		funderView := readClosing(t, tx, access.AgencyScope{IDs: []string{funder.ID}}, accountIDs).Accounts[0]
		if len(funderView.Batches) != 1 || funderView.CostCents != 15000 {
			t.Errorf("funder: want its own batch, got %d batches, cost %d", len(funderView.Batches), funderView.CostCents)
		}

		outsiderView := readClosing(t, tx, access.AgencyScope{IDs: []string{outsider.ID}}, accountIDs)
		account := outsiderView.Accounts[0]
		if len(account.Batches) != 0 || account.CostCents != 0 || account.RevenueCents != 0 || len(account.Settlements) != 0 {
			t.Errorf("outsider: want nothing of the account's costing, got %+v", account)
		}
		if outsiderView.Totals.CostCents != 0 || len(outsiderView.Totals.Settlements) != 0 {
			t.Errorf("outsider totals: want empty, got %+v", outsiderView.Totals)
		}
	})
}

func TestGetInventoryClosing_RefusesBadParameters(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		accountID := uuid.New().String()
		cases := []struct {
			name  string
			query string
		}{
			{"no account_ids", "from=2026-08-01&to=2026-08-31"},
			{"only commas", "account_ids=,,&from=2026-08-01&to=2026-08-31"},
			{"no from", "account_ids=" + accountID + "&to=2026-08-31"},
			{"from in the wrong format", "account_ids=" + accountID + "&from=01/08/2026&to=2026-08-31"},
			{"to in the wrong format", "account_ids=" + accountID + "&from=2026-08-01&to=31/08/2026"},
		}
		for _, testCase := range cases {
			if rec := callGetInventoryClosing(tx, access.FullScope(), testCase.query); rec.Code != http.StatusBadRequest {
				t.Errorf("%s: got %d, want 400 (%s)", testCase.name, rec.Code, rec.Body.String())
			}
		}
	})
}

// Pins current behaviour, not a requirement: an account id nobody has is not refused, it
// comes back as an empty report under that id.
func TestGetInventoryClosing_UnknownAccountIsAnEmptyReport(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		accountID := uuid.New().String()
		report := readClosing(t, tx, access.FullScope(), []string{accountID})
		if len(report.Accounts) != 1 || report.Accounts[0].AccountID != accountID || len(report.Accounts[0].Batches) != 0 {
			t.Errorf("unknown account: want one empty report, got %+v", report.Accounts)
		}
	})
}

// Pins current behaviour, not a requirement: account_ids is not checked to be uuids, so a
// malformed one reaches Postgres and comes back as a 500 rather than a 400. Harmless - the
// route is admin-only and the panel only sends ids it read - but worth knowing. Last
// statement on purpose: the Postgres error aborts the test transaction.
func TestGetInventoryClosing_MalformedAccountIDIsAServerError(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		rec := callGetInventoryClosing(tx, access.FullScope(), "account_ids=not-a-uuid&from=2026-08-01&to=2026-08-31")
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("current behaviour changed: malformed id got %d, was 500 (%s)", rec.Code, rec.Body.String())
		}
	})
}
