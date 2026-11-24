package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func callUpdatePointBatch(tx *gorm.DB, batchID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/point-batches/"+batchID, strings.NewReader(body))
	req = withScope(withIDs(req, map[string]string{"id": batchID}), access.FullScope())
	rec := httptest.NewRecorder()
	NewPointBatchHandler(tx).UpdatePointBatch(rec, req)
	return rec
}

// A batch dated inside a closed period is never consumed - the recompute does not replay a
// closed period - so it would sit in the stock forever, costing nothing. Same guard as an
// issuance backdated into a closed month.
func TestPointBatchWritesRefuseAClosedPeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, origin, agency := seedStatementTarget(t, tx)
		closer := createUser(t, tx, "closer")
		seedClosedPeriod(t, tx, account.ID, closer.ID, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))

		batchBody := func(day string) string {
			return fmt.Sprintf(`{"account_id":"%s","origin_id":"%s","acquired_on":"%s","points":1000,"cost_cents":1500,"funding_agency_id":"%s"}`,
				account.ID, origin.ID, day, agency.ID)
		}

		if rec := callCreatePointBatch(tx, access.FullScope(), batchBody("2026-08-31")); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("create on the last closed day: got %d, want 422 (%s)", rec.Code, rec.Body.String())
		}

		rec := callCreatePointBatch(tx, access.FullScope(), batchBody("2026-09-01"))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create on the first open day: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}
		open := decodePointBatch(t, rec)

		if rec := callUpdatePointBatch(tx, open.ID, `{"acquired_on":"2026-08-15"}`); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("moving an open batch into the closed period: got %d, want 422", rec.Code)
		}

		// A batch already inside the period (recorded before it was closed) keeps its
		// costing frozen: its points cannot change, but its note still can.
		closedAt := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
		inside := data.PointBatch{
			AccountID: account.ID, OriginID: &origin.ID, AcquiredOn: closedAt, Points: 2000,
			CostCents: 3000, FundingAgencyID: agency.ID, ProfitSharePercent: 50,
		}
		create(t, tx, &inside)

		if rec := callUpdatePointBatch(tx, inside.ID, `{"points":2500}`); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("changing the points of a closed batch: got %d, want 422", rec.Code)
		}
		if rec := callUpdatePointBatch(tx, inside.ID, `{"acquired_on":"2026-09-02"}`); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("moving a closed batch out of its period: got %d, want 422", rec.Code)
		}
		if rec := callUpdatePointBatch(tx, inside.ID, `{"note":"conferido"}`); rec.Code != http.StatusOK {
			t.Errorf("editing only the note of a closed batch: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
	})
}
