package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

type pointBatchEnvelope struct {
	Data schemas.PointBatchResponse `json:"data"`
}

func callCreatePointBatch(tx *gorm.DB, scope access.AgencyScope, body string) *httptest.ResponseRecorder {
	req := withScope(httptest.NewRequest(http.MethodPost, "/point-batches", strings.NewReader(body)), scope)
	rec := httptest.NewRecorder()
	NewPointBatchHandler(tx).CreatePointBatch(rec, req)
	return rec
}

func callSettlePointBatch(tx *gorm.DB, scope access.AgencyScope, batchID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/point-batches/"+batchID+"/settlement", strings.NewReader(body))
	req = withScope(withIDs(req, map[string]string{"id": batchID}), scope)
	rec := httptest.NewRecorder()
	NewPointBatchHandler(tx).SettlePointBatch(rec, req)
	return rec
}

func decodePointBatch(t *testing.T, rec *httptest.ResponseRecorder) schemas.PointBatchResponse {
	t.Helper()
	var envelope pointBatchEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode the batch: %v (%s)", err, rec.Body.String())
	}
	return envelope.Data
}

func TestCreatePointBatch_WritesTheBatchWithTheAccountsShare(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, origin, agency := seedStatementTarget(t, tx)

		body := fmt.Sprintf(`{"account_id":"%s","origin_id":"%s","acquired_on":"2026-08-21","points":632700,"cost_cents":894172,"funding_agency_id":"%s","note":"Livelo agosto"}`,
			account.ID, origin.ID, agency.ID)
		rec := callCreatePointBatch(tx, access.FullScope(), body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}
		batch := decodePointBatch(t, rec)
		if batch.AccountID != account.ID || batch.AcquiredOn != "2026-08-21" || batch.Points != 632700 || batch.CostCents != 894172 {
			t.Errorf("batch: got %+v", batch)
		}
		// No percent in the request: the account's default is copied onto the batch.
		if batch.ProfitSharePercent != 50 {
			t.Errorf("profit share: want the account's 50, got %d", batch.ProfitSharePercent)
		}
		if batch.Origin == nil || batch.Origin.ID != origin.ID || batch.FundingAgency == nil || batch.FundingAgency.ID != agency.ID {
			t.Errorf("preloads: want origin and agency on the response, got %+v / %+v", batch.Origin, batch.FundingAgency)
		}
		if batch.SettledAt != nil {
			t.Error("a new batch came back settled")
		}

		var stored data.PointBatch
		if err := tx.First(&stored, "id = ?", batch.ID).Error; err != nil {
			t.Fatalf("the batch was not written: %v", err)
		}
		if stored.Note == nil || *stored.Note != "Livelo agosto" {
			t.Errorf("note: got %v", stored.Note)
		}

		// An explicit percent wins, and an opening balance needs no origin.
		body = fmt.Sprintf(`{"account_id":"%s","acquired_on":"2026-08-01","points":1000,"cost_cents":0,"funding_agency_id":"%s","profit_share_percent":30,"opening":true}`,
			account.ID, agency.ID)
		rec = callCreatePointBatch(tx, access.FullScope(), body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("opening batch: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}
		opening := decodePointBatch(t, rec)
		if opening.ProfitSharePercent != 30 || !opening.Opening || opening.OriginID != nil {
			t.Errorf("opening batch: got %+v", opening)
		}
	})
}

func TestCreatePointBatch_RefusesBadInput(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, origin, agency := seedStatementTarget(t, tx)
		plain := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: account.LoyaltyProgramID}
		create(t, tx, &plain)
		foreign := seedNamedAgency(t, tx, "Foreign")
		restricted := access.AgencyScope{IDs: []string{agency.ID}}

		bodyFor := func(accountID, originField, acquiredOn, fundingAgencyID string) string {
			return fmt.Sprintf(`{"account_id":"%s",%s"acquired_on":"%s","points":1000,"cost_cents":100,"funding_agency_id":"%s"}`,
				accountID, originField, acquiredOn, fundingAgencyID)
		}
		withOrigin := `"origin_id":"` + origin.ID + `",`

		cases := []struct {
			name  string
			scope access.AgencyScope
			body  string
			want  int
		}{
			{"broken JSON", access.FullScope(), `{"account_id":`, http.StatusBadRequest},
			{"zero points", access.FullScope(), fmt.Sprintf(`{"account_id":"%s","origin_id":"%s","acquired_on":"2026-08-01","points":0,"funding_agency_id":"%s"}`, account.ID, origin.ID, agency.ID), http.StatusBadRequest},
			{"percent over 100", access.FullScope(), fmt.Sprintf(`{"account_id":"%s","origin_id":"%s","acquired_on":"2026-08-01","points":10,"funding_agency_id":"%s","profit_share_percent":101}`, account.ID, origin.ID, agency.ID), http.StatusBadRequest},
			{"no origin and not an opening", access.FullScope(), bodyFor(account.ID, "", "2026-08-01", agency.ID), http.StatusUnprocessableEntity},
			{"funding agency outside the scope", restricted, bodyFor(account.ID, withOrigin, "2026-08-01", foreign.ID), http.StatusNotFound},
			{"date in the wrong format", access.FullScope(), bodyFor(account.ID, withOrigin, "21/08/2026", agency.ID), http.StatusBadRequest},
			{"unknown account", access.FullScope(), bodyFor(uuid.New().String(), withOrigin, "2026-08-01", agency.ID), http.StatusNotFound},
			{"account without its own inventory", access.FullScope(), bodyFor(plain.ID, withOrigin, "2026-08-01", agency.ID), http.StatusUnprocessableEntity},
		}
		for _, testCase := range cases {
			rec := callCreatePointBatch(tx, testCase.scope, testCase.body)
			if rec.Code != testCase.want {
				t.Errorf("%s: got %d, want %d (%s)", testCase.name, rec.Code, testCase.want, rec.Body.String())
			}
		}

		var count int64
		tx.Model(&data.PointBatch{}).Where("account_id IN ?", []string{account.ID, plain.ID}).Count(&count)
		if count != 0 {
			t.Errorf("a refused create wrote %d batches", count)
		}

		// A caller restricted to the funding agency can still record a batch for it.
		rec := callCreatePointBatch(tx, restricted, bodyFor(account.ID, withOrigin, "2026-08-01", agency.ID))
		if rec.Code != http.StatusCreated {
			t.Errorf("restricted caller, own agency: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}
	})
}

// An origin id that names nothing is a foreign-key violation, answered as a sentence.
// Kept in its own test and as the last statement: the violation aborts the surrounding test
// transaction, so nothing can run on it afterwards.
func TestCreatePointBatch_UnknownOriginIsUnprocessable(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, _, agency := seedStatementTarget(t, tx)
		body := fmt.Sprintf(`{"account_id":"%s","origin_id":"%s","acquired_on":"2026-08-01","points":1000,"cost_cents":100,"funding_agency_id":"%s"}`,
			account.ID, uuid.New().String(), agency.ID)
		rec := callCreatePointBatch(tx, access.FullScope(), body)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("unknown origin: got %d, want 422 (%s)", rec.Code, rec.Body.String())
		}
	})
}

func TestSettlePointBatch_MarksAndUnmarksTheSettlement(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, _, agency := seedStatementTarget(t, tx)
		batch := data.PointBatch{
			AccountID: account.ID, AcquiredOn: day(t, "2026-08-05"), Points: 1000,
			CostCents: 100, FundingAgencyID: agency.ID, ProfitSharePercent: 50, Opening: true,
		}
		create(t, tx, &batch)
		scope := access.AgencyScope{IDs: []string{agency.ID}}

		rec := callSettlePointBatch(tx, scope, batch.ID, `{"settled":true}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("settle: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if response := decodePointBatch(t, rec); response.SettledAt == nil {
			t.Error("settle: the response does not carry settled_at")
		}
		var stored data.PointBatch
		tx.First(&stored, "id = ?", batch.ID)
		if stored.SettledAt == nil {
			t.Fatal("settle: settled_at was not written")
		}

		rec = callSettlePointBatch(tx, scope, batch.ID, `{"settled":false}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("unsettle: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if response := decodePointBatch(t, rec); response.SettledAt != nil {
			t.Error("unsettle: the response still carries settled_at")
		}
		// A fresh struct: scanning into the one above would keep its old settled_at.
		var cleared data.PointBatch
		tx.First(&cleared, "id = ?", batch.ID)
		if cleared.SettledAt != nil {
			t.Error("unsettle: settled_at was not cleared")
		}
	})
}

func TestSettlePointBatch_RefusesWhatTheCallerCannotReach(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, _, agency := seedStatementTarget(t, tx)
		mine := seedNamedAgency(t, tx, "Mine")
		batch := data.PointBatch{
			AccountID: account.ID, AcquiredOn: day(t, "2026-08-05"), Points: 1000,
			CostCents: 100, FundingAgencyID: agency.ID, ProfitSharePercent: 50, Opening: true,
		}
		create(t, tx, &batch)

		// A batch another agency paid for answers like a missing one, and stays unsettled.
		rec := callSettlePointBatch(tx, access.AgencyScope{IDs: []string{mine.ID}}, batch.ID, `{"settled":true}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("foreign batch: got %d, want 404 (%s)", rec.Code, rec.Body.String())
		}
		var stored data.PointBatch
		tx.First(&stored, "id = ?", batch.ID)
		if stored.SettledAt != nil {
			t.Error("foreign batch: settled_at was written anyway")
		}

		if rec := callSettlePointBatch(tx, access.FullScope(), uuid.New().String(), `{"settled":true}`); rec.Code != http.StatusNotFound {
			t.Errorf("unknown batch: got %d, want 404 (%s)", rec.Code, rec.Body.String())
		}
		if rec := callSettlePointBatch(tx, access.FullScope(), batch.ID, `{"settled":`); rec.Code != http.StatusBadRequest {
			t.Errorf("broken JSON: got %d, want 400 (%s)", rec.Code, rec.Body.String())
		}

		// An archived batch is gone for the settlement too.
		tx.Delete(&batch)
		if rec := callSettlePointBatch(tx, access.FullScope(), batch.ID, `{"settled":true}`); rec.Code != http.StatusNotFound {
			t.Errorf("archived batch: got %d, want 404 (%s)", rec.Code, rec.Body.String())
		}
	})
}
