package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// periodScopeWorld is an inventory account plus a caller acting on it with a given agency
// scope, for the close/reopen scope rule (overnight court, D10): a restricted admin may close
// or reopen an account only when every live batch on it was funded by one of their agencies.
type periodScopeWorld struct {
	tx      *gorm.DB
	account data.Account
	user    data.User
}

func seedPeriodScopeWorld(t *testing.T, tx *gorm.DB) periodScopeWorld {
	t.Helper()
	account, user := seedAzulStatementTarget(t, tx)
	return periodScopeWorld{tx: tx, account: account, user: user}
}

func (w periodScopeWorld) fundBatch(t *testing.T, agencyID string) data.PointBatch {
	t.Helper()
	batch := data.PointBatch{
		AccountID: w.account.ID, AcquiredOn: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), Points: 1000,
		CostCents: 100, FundingAgencyID: agencyID, ProfitSharePercent: 50, Opening: true,
	}
	create(t, w.tx, &batch)
	return batch
}

// withCaller puts the user and the scope in the request the way RequireAuth would.
func (w periodScopeWorld) withCaller(req *http.Request, scope access.AgencyScope) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, w.user)
	return withScope(req.WithContext(ctx), scope)
}

func (w periodScopeWorld) close(scope access.AgencyScope) *httptest.ResponseRecorder {
	body := `{"account_id":"` + w.account.ID + `","starts_on":"2026-08-01","ends_on":"2026-08-31"}`
	req := w.withCaller(httptest.NewRequest(http.MethodPost, "/inventory-periods", strings.NewReader(body)), scope)
	rec := httptest.NewRecorder()
	NewInventoryPeriodHandler(w.tx).ClosePeriod(rec, req)
	return rec
}

func (w periodScopeWorld) reopen(periodID string, scope access.AgencyScope) *httptest.ResponseRecorder {
	req := withIDs(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"id": periodID})
	rec := httptest.NewRecorder()
	NewInventoryPeriodHandler(w.tx).ReopenPeriod(rec, w.withCaller(req, scope))
	return rec
}

// mustClose closes the period and returns its id, failing the test on anything but 201.
func (w periodScopeWorld) mustClose(t *testing.T, scope access.AgencyScope) string {
	t.Helper()
	rec := w.close(scope)
	if rec.Code != http.StatusCreated {
		t.Fatalf("close: got %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data schemas.InventoryPeriodResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode close: %v (%s)", err, rec.Body.String())
	}
	return body.Data.ID
}

func (w periodScopeWorld) periodCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	if err := w.tx.Model(&data.InventoryPeriod{}).Where("account_id = ?", w.account.ID).Count(&count).Error; err != nil {
		t.Fatalf("count periods: %v", err)
	}
	return count
}

// Replaces TestPeriod_RestrictedAdminCanCloseAndReopenAForeignFundedAccount from PR #298, which
// pinned the behaviour from before the owner's decision: an account with even one live batch
// funded outside the caller's scope is refused with 404 on close and on reopen, and nothing
// is written either way.
func TestPeriod_RestrictedAdminCannotCloseOrReopenAForeignFundedAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		mine := seedNamedAgency(t, tx, "Mine")
		theirs := seedNamedAgency(t, tx, "Theirs")
		restricted := access.AgencyScope{IDs: []string{mine.ID}}

		foreignOnly := seedPeriodScopeWorld(t, tx)
		foreignOnly.fundBatch(t, theirs.ID)

		mixed := seedPeriodScopeWorld(t, tx)
		mixed.fundBatch(t, mine.ID)
		mixed.fundBatch(t, theirs.ID)

		for name, world := range map[string]periodScopeWorld{"foreign only": foreignOnly, "mixed": mixed} {
			if rec := world.close(restricted); rec.Code != http.StatusNotFound {
				t.Errorf("%s: restricted close got %d, want 404 (%s)", name, rec.Code, rec.Body.String())
			} else if !strings.Contains(rec.Body.String(), "financiados por uma agência que você não acessa") {
				t.Errorf("%s: want the Portuguese explanation, got %s", name, rec.Body.String())
			}
			if count := world.periodCount(t); count != 0 {
				t.Errorf("%s: refused close still wrote %d period(s)", name, count)
			}

			// A period somebody with every agency closed stays closed for the restricted admin.
			periodID := world.mustClose(t, access.FullScope())
			if rec := world.reopen(periodID, restricted); rec.Code != http.StatusNotFound {
				t.Errorf("%s: restricted reopen got %d, want 404 (%s)", name, rec.Code, rec.Body.String())
			}
			if count := world.periodCount(t); count != 1 {
				t.Errorf("%s: refused reopen left %d period(s), want the closed one untouched", name, count)
			}
		}

		// No grants at all is "no agency", never "nothing foreign": fail closed even on an
		// account with no batches.
		empty := seedPeriodScopeWorld(t, tx)
		if rec := empty.close(access.AgencyScope{}); rec.Code != http.StatusNotFound {
			t.Errorf("no grants: close got %d, want 404", rec.Code)
		}
	})
}

// The restricted admin keeps doing what they always could on accounts their own agencies paid
// for - including one with no batches yet, which has nothing foreign to freeze. An archived
// batch from another agency no longer counts: the costing ignores it too.
func TestPeriod_RestrictedAdminClosesAndReopensAnOwnFundedAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		mine := seedNamedAgency(t, tx, "Mine")
		theirs := seedNamedAgency(t, tx, "Theirs")
		restricted := access.AgencyScope{IDs: []string{mine.ID}}

		own := seedPeriodScopeWorld(t, tx)
		own.fundBatch(t, mine.ID)
		archived := own.fundBatch(t, theirs.ID)
		if err := tx.Delete(&archived).Error; err != nil {
			t.Fatalf("archive batch: %v", err)
		}

		noBatches := seedPeriodScopeWorld(t, tx)

		for name, world := range map[string]periodScopeWorld{"own": own, "no batches": noBatches} {
			periodID := world.mustClose(t, restricted)
			if rec := world.reopen(periodID, restricted); rec.Code != http.StatusOK {
				t.Errorf("%s: restricted reopen got %d, want 200 (%s)", name, rec.Code, rec.Body.String())
			}
			if count := world.periodCount(t); count != 0 {
				t.Errorf("%s: reopen left %d period(s)", name, count)
			}
		}
	})
}

func TestPeriod_DevClosesAndReopensAMixedFundedAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedPeriodScopeWorld(t, tx)
		world.fundBatch(t, seedNamedAgency(t, tx, "One").ID)
		world.fundBatch(t, seedNamedAgency(t, tx, "Other").ID)

		periodID := world.mustClose(t, access.FullScope())
		if rec := world.reopen(periodID, access.FullScope()); rec.Code != http.StatusOK {
			t.Errorf("dev reopen got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
	})
}
