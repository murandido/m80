package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/inventory"
	"gorm.io/gorm"
)

func TestPointBatches_OnlyTheCallersFundingAgencies(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)
		mine := seedNamedAgency(t, tx, "Mine")
		other := seedNamedAgency(t, tx, "Other")
		scope := access.AgencyScope{IDs: []string{mine.ID}}

		seedBatch := func(agency data.Agency) data.PointBatch {
			batch := data.PointBatch{
				AccountID: issuance.AccountID, AcquiredOn: time.Now(), Points: 1000,
				CostCents: 100, FundingAgencyID: agency.ID, ProfitSharePercent: 50, Opening: true,
			}
			create(t, tx, &batch)
			return batch
		}
		visible := seedBatch(mine)
		hidden := seedBatch(other)
		handler := &PointBatchHandler{DB: tx, Validator: validator.New()}

		req := withScope(httptest.NewRequest(http.MethodGet, "/point-batches?account_id="+issuance.AccountID, nil), scope)
		rec := httptest.NewRecorder()
		handler.GetPointBatches(rec, req)
		var body struct {
			Data []schemas.PointBatchResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body.String())
		}
		if len(body.Data) != 1 || body.Data[0].ID != visible.ID {
			t.Errorf("list: want only %s, got %+v", visible.ID, body.Data)
		}

		req = withScope(withIDs(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"id": hidden.ID}), scope)
		rec = httptest.NewRecorder()
		handler.DeletePointBatch(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("delete a foreign batch: got %d, want 404", rec.Code)
		}

		req = withScope(withIDs(httptest.NewRequest(http.MethodPut, "/",
			strings.NewReader(`{"funding_agency_id": "`+other.ID+`"}`)), map[string]string{"id": visible.ID}), scope)
		rec = httptest.NewRecorder()
		handler.UpdatePointBatch(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("hand a batch to a foreign agency: got %d, want 404 (%s)", rec.Code, rec.Body.String())
		}
	})
}

func TestScopeClosing_KeepsOnlyTheCallersSide(t *testing.T) {
	t.Parallel()
	closing := inventory.Closing{
		Batches: []inventory.BatchResult{
			{
				BatchID: "mine", FundingAgencyID: "A", ChargedCents: 100, RevenueCents: 300,
				Consumers:   []inventory.Consumer{{AgencyID: "A"}, {AgencyID: "B"}},
				Settlements: []inventory.Settlement{{AgencyID: "B", ToAgencyID: "A", AmountCents: 50}},
			},
			{BatchID: "theirs", FundingAgencyID: "B", ChargedCents: 1000, RevenueCents: 5000},
		},
		CostCents: 1100, RevenueCents: 5300, ProfitCents: 4200,
		Settlements: []inventory.Settlement{
			{AgencyID: "B", ToAgencyID: "A", AmountCents: 50},
			{AgencyID: "C", ToAgencyID: "B", AmountCents: 70},
		},
	}

	got := scopeClosing(closing, access.AgencyScope{IDs: []string{"A"}})

	if len(got.Batches) != 1 || got.Batches[0].BatchID != "mine" {
		t.Fatalf("batches: want only the one A paid for, got %+v", got.Batches)
	}
	if len(got.Batches[0].Consumers) != 1 || got.Batches[0].Consumers[0].AgencyID != "A" {
		t.Errorf("consumers: want only A, got %+v", got.Batches[0].Consumers)
	}
	if got.CostCents != 100 || got.RevenueCents != 300 || got.ProfitCents != 200 {
		t.Errorf("totals: want 100/300/200 from the kept batch, got %d/%d/%d", got.CostCents, got.RevenueCents, got.ProfitCents)
	}
	if len(got.Settlements) != 1 || got.Settlements[0].AgencyID != "B" {
		t.Errorf("settlements: want only the one A takes part in, got %+v", got.Settlements)
	}
	if full := scopeClosing(closing, access.FullScope()); len(full.Batches) != 2 || full.CostCents != 1100 {
		t.Error("dev: the closing must come back untouched")
	}
}
