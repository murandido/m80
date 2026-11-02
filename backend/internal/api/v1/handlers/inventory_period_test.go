package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

type inventoryPeriodEnvelope struct {
	Data schemas.InventoryPeriodResponse `json:"data"`
}

// asScopedUser is asUser with a scope of the test's choosing, for the cases that are about
// what a caller restricted to some agencies can reach.
func asScopedUser(ctx context.Context, user data.User, scope access.AgencyScope) context.Context {
	ctx = context.WithValue(ctx, middleware.UserContextKey, user)
	return context.WithValue(ctx, middleware.AgencyScopeContextKey, scope)
}

func closePeriodRequest(ctx context.Context, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/inventory-periods", strings.NewReader(body))
	return req.WithContext(ctx)
}

func closePeriodBody(accountID, startsOn, endsOn string) string {
	return `{"account_id":"` + accountID + `","starts_on":"` + startsOn + `","ends_on":"` + endsOn + `"}`
}

func callClosePeriod(tx *gorm.DB, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	NewInventoryPeriodHandler(tx).ClosePeriod(rec, req)
	return rec
}

func callReopenPeriod(tx *gorm.DB, ctx context.Context, periodID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/inventory-periods/"+periodID, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	NewInventoryPeriodHandler(tx).ReopenPeriod(rec, withIDs(req, map[string]string{"id": periodID}))
	return rec
}

func decodePeriod(t *testing.T, rec *httptest.ResponseRecorder) schemas.InventoryPeriodResponse {
	t.Helper()
	var envelope inventoryPeriodEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode the period: %v (%s)", err, rec.Body.String())
	}
	return envelope.Data
}

func TestClosePeriod_FreezesTheRangeAndRecordsWhoClosedIt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, _, _ := seedStatementTarget(t, tx)
		user := createUser(t, tx, "Fechador")
		ctx := asUser(context.Background(), user)

		rec := callClosePeriod(tx, closePeriodRequest(ctx, closePeriodBody(account.ID, "2026-08-01", "2026-08-31")))
		if rec.Code != http.StatusCreated {
			t.Fatalf("close: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}
		period := decodePeriod(t, rec)
		if period.AccountID != account.ID || period.StartsOn != "2026-08-01" || period.EndsOn != "2026-08-31" {
			t.Errorf("period: got %+v", period)
		}
		if period.ClosedAt == nil {
			t.Error("closed_at: want it set")
		}
		if period.ClosedByID == nil || *period.ClosedByID != user.ID {
			t.Errorf("closed_by_id: want %s, got %v", user.ID, period.ClosedByID)
		}

		var stored data.InventoryPeriod
		if err := tx.First(&stored, "id = ?", period.ID).Error; err != nil {
			t.Fatalf("the period was not written: %v", err)
		}

		// Periods are a chain: one that ends on or before the last closed day is refused.
		rec = callClosePeriod(tx, closePeriodRequest(ctx, closePeriodBody(account.ID, "2026-08-15", "2026-08-31")))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("overlapping close: got %d, want 422 (%s)", rec.Code, rec.Body.String())
		}

		rec = callClosePeriod(tx, closePeriodRequest(ctx, closePeriodBody(account.ID, "2026-09-01", "2026-09-30")))
		if rec.Code != http.StatusCreated {
			t.Errorf("the next month: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}
	})
}

func TestClosePeriod_RefusesBadInput(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, _, _ := seedStatementTarget(t, tx)
		user := createUser(t, tx, "Fechador")
		ctx := asUser(context.Background(), user)

		plain := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: account.LoyaltyProgramID}
		create(t, tx, &plain)

		cases := []struct {
			name string
			ctx  context.Context
			body string
			want int
		}{
			{"no user in the context", context.Background(), closePeriodBody(account.ID, "2026-08-01", "2026-08-31"), http.StatusUnauthorized},
			{"broken JSON", ctx, `{"account_id":`, http.StatusBadRequest},
			{"account_id that is not a uuid", ctx, closePeriodBody("nope", "2026-08-01", "2026-08-31"), http.StatusBadRequest},
			{"missing ends_on", ctx, `{"account_id":"` + account.ID + `","starts_on":"2026-08-01"}`, http.StatusBadRequest},
			{"start in the wrong format", ctx, closePeriodBody(account.ID, "01/08/2026", "2026-08-31"), http.StatusBadRequest},
			{"end in the wrong format", ctx, closePeriodBody(account.ID, "2026-08-01", "31/08/2026"), http.StatusBadRequest},
			{"unknown account", ctx, closePeriodBody(uuid.New().String(), "2026-08-01", "2026-08-31"), http.StatusNotFound},
			{"account without its own inventory", ctx, closePeriodBody(plain.ID, "2026-08-01", "2026-08-31"), http.StatusUnprocessableEntity},
			{"end before start", ctx, closePeriodBody(account.ID, "2026-08-31", "2026-08-01"), http.StatusUnprocessableEntity},
		}
		for _, testCase := range cases {
			rec := callClosePeriod(tx, closePeriodRequest(testCase.ctx, testCase.body))
			if rec.Code != testCase.want {
				t.Errorf("%s: got %d, want %d (%s)", testCase.name, rec.Code, testCase.want, rec.Body.String())
			}
		}

		var count int64
		tx.Model(&data.InventoryPeriod{}).Where("account_id IN ?", []string{account.ID, plain.ID}).Count(&count)
		if count != 0 {
			t.Errorf("a refused close wrote %d periods", count)
		}
	})
}

func TestReopenPeriod_OnlyTheMostRecentComesOff(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, _, _ := seedStatementTarget(t, tx)
		user := createUser(t, tx, "Fechador")
		ctx := asUser(context.Background(), user)

		august := decodePeriod(t, callClosePeriod(tx, closePeriodRequest(ctx, closePeriodBody(account.ID, "2026-08-01", "2026-08-31"))))
		september := decodePeriod(t, callClosePeriod(tx, closePeriodRequest(ctx, closePeriodBody(account.ID, "2026-09-01", "2026-09-30"))))

		if rec := callReopenPeriod(tx, ctx, august.ID); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("reopen from the middle: got %d, want 422 (%s)", rec.Code, rec.Body.String())
		}
		if rec := callReopenPeriod(tx, ctx, uuid.New().String()); rec.Code != http.StatusNotFound {
			t.Errorf("unknown period: got %d, want 404 (%s)", rec.Code, rec.Body.String())
		}

		if rec := callReopenPeriod(tx, ctx, september.ID); rec.Code != http.StatusOK {
			t.Fatalf("reopen the newest: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		var remaining []data.InventoryPeriod
		tx.Where("account_id = ?", account.ID).Find(&remaining)
		if len(remaining) != 1 || remaining[0].ID != august.ID {
			t.Fatalf("after reopening September: want only August left, got %+v", remaining)
		}

		// With September gone August is the newest again, and so it can come off too.
		if rec := callReopenPeriod(tx, ctx, august.ID); rec.Code != http.StatusOK {
			t.Errorf("reopen August next: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
	})
}

// Documents the CURRENT behaviour only, and is not a statement that it is right: whether an
// admin restricted to agency A may freeze or unfreeze an account whose batches agency B paid
// for is an open question for the owner (overnight court, D10). Neither ClosePeriod nor
// ReopenPeriod reads the agency scope today - the account belongs to no agency, so there is
// nothing obvious to check it against. If the owner decides otherwise, this test is the one
// to turn around.
func TestPeriod_RestrictedAdminCanCloseAndReopenAForeignFundedAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, _, _ := seedStatementTarget(t, tx)
		mine := seedNamedAgency(t, tx, "Mine")
		theirs := seedNamedAgency(t, tx, "Theirs")
		batch := data.PointBatch{
			AccountID: account.ID, AcquiredOn: day(t, "2026-08-05"), Points: 1000,
			CostCents: 100, FundingAgencyID: theirs.ID, ProfitSharePercent: 50, Opening: true,
		}
		create(t, tx, &batch)

		user := createUser(t, tx, "Restrito")
		ctx := asScopedUser(context.Background(), user, access.AgencyScope{IDs: []string{mine.ID}})

		rec := callClosePeriod(tx, closePeriodRequest(ctx, closePeriodBody(account.ID, "2026-08-01", "2026-08-31")))
		if rec.Code != http.StatusCreated {
			t.Fatalf("current behaviour changed: restricted close got %d, was 201 (%s)", rec.Code, rec.Body.String())
		}
		period := decodePeriod(t, rec)

		if rec := callReopenPeriod(tx, ctx, period.ID); rec.Code != http.StatusOK {
			t.Errorf("current behaviour changed: restricted reopen got %d, was 200 (%s)", rec.Code, rec.Body.String())
		}
	})
}

// The settlement mark on a batch survives a reopen: closing froze figures, settling recorded
// a payment, and undoing the first does not undo the second (see inventory.Reopen).
func TestReopenPeriod_KeepsTheBatchSettlement(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, _, agency := seedStatementTarget(t, tx)
		settledAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
		batch := data.PointBatch{
			AccountID: account.ID, AcquiredOn: day(t, "2026-08-05"), Points: 1000,
			CostCents: 100, FundingAgencyID: agency.ID, ProfitSharePercent: 50, Opening: true,
			SettledAt: &settledAt,
		}
		create(t, tx, &batch)
		ctx := asUser(context.Background(), createUser(t, tx, "Fechador"))

		period := decodePeriod(t, callClosePeriod(tx, closePeriodRequest(ctx, closePeriodBody(account.ID, "2026-08-01", "2026-08-31"))))
		if rec := callReopenPeriod(tx, ctx, period.ID); rec.Code != http.StatusOK {
			t.Fatalf("reopen: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}

		var reloaded data.PointBatch
		if err := tx.First(&reloaded, "id = ?", batch.ID).Error; err != nil {
			t.Fatalf("reload batch: %v", err)
		}
		if reloaded.SettledAt == nil {
			t.Error("reopening the period cleared the batch's settled_at")
		}
	})
}
