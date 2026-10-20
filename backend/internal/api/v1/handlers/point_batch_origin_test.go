package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

type pointBatchOriginEnvelope struct {
	Data schemas.PointBatchOriginResponse `json:"data"`
}

type pointBatchOriginListEnvelope struct {
	Data []schemas.PointBatchOriginResponse `json:"data"`
}

func callCreateOrigin(tx *gorm.DB, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	NewPointBatchOriginHandler(tx).CreatePointBatchOrigin(rec,
		httptest.NewRequest(http.MethodPost, "/point-batch-origins", strings.NewReader(body)))
	return rec
}

func callUpdateOrigin(tx *gorm.DB, originID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/point-batch-origins/"+originID, strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewPointBatchOriginHandler(tx).UpdatePointBatchOrigin(rec, withIDs(req, map[string]string{"id": originID}))
	return rec
}

func callDeleteOrigin(tx *gorm.DB, originID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/point-batch-origins/"+originID, nil)
	rec := httptest.NewRecorder()
	NewPointBatchOriginHandler(tx).DeletePointBatchOrigin(rec, withIDs(req, map[string]string{"id": originID}))
	return rec
}

func listOrigins(t *testing.T, tx *gorm.DB, query string) []schemas.PointBatchOriginResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	NewPointBatchOriginHandler(tx).GetPointBatchOrigins(rec, httptest.NewRequest(http.MethodGet, "/point-batch-origins"+query, nil))
	var envelope pointBatchOriginListEnvelope
	decodeOK(t, rec, "GetPointBatchOrigins", &envelope)
	return envelope.Data
}

func decodeOrigin(t *testing.T, rec *httptest.ResponseRecorder) schemas.PointBatchOriginResponse {
	t.Helper()
	var envelope pointBatchOriginEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode the origin: %v (%s)", err, rec.Body.String())
	}
	return envelope.Data
}

func originPosition(origins []schemas.PointBatchOriginResponse, id string) int {
	for index, origin := range origins {
		if origin.ID == id {
			return index
		}
	}
	return -1
}

func TestPointBatchOrigins_CreateListAndRetire(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		suffix := uuid.New().String()[:8]

		rec := callCreateOrigin(tx, `{"name":"Zeta `+suffix+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}
		retired := decodeOrigin(t, rec)
		if !retired.Active || retired.Name != "Zeta "+suffix {
			t.Errorf("create: a new origin starts active under its name, got %+v", retired)
		}
		live := decodeOrigin(t, callCreateOrigin(tx, `{"name":"Alfa `+suffix+`"}`))

		if rec := callCreateOrigin(tx, `{"name":""}`); rec.Code != http.StatusBadRequest {
			t.Errorf("empty name: got %d, want 400", rec.Code)
		}
		if rec := callCreateOrigin(tx, `{"name":`); rec.Code != http.StatusBadRequest {
			t.Errorf("broken JSON: got %d, want 400", rec.Code)
		}

		rec = callUpdateOrigin(tx, retired.ID, `{"active":false}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("deactivate: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if response := decodeOrigin(t, rec); response.Active {
			t.Errorf("deactivate: the response still says active: %+v", response)
		}

		// The picker asks for active only; the management screen gets both, inactive last.
		picker := listOrigins(t, tx, "?active=true")
		if originPosition(picker, retired.ID) != -1 || originPosition(picker, live.ID) == -1 {
			t.Errorf("active=true: want the live origin and not the retired one, got %+v", picker)
		}
		all := listOrigins(t, tx, "")
		livePosition, retiredPosition := originPosition(all, live.ID), originPosition(all, retired.ID)
		if livePosition == -1 || retiredPosition == -1 || retiredPosition < livePosition {
			t.Errorf("full list: want both, the inactive one after the active one (live %d, retired %d)", livePosition, retiredPosition)
		}

		rec = callUpdateOrigin(tx, live.ID, `{"name":"Alfa renomeada `+suffix+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("rename: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if response := decodeOrigin(t, rec); response.Name != "Alfa renomeada "+suffix {
			t.Errorf("rename: response name %q", response.Name)
		}

		if rec := callUpdateOrigin(tx, uuid.New().String(), `{"active":false}`); rec.Code != http.StatusNotFound {
			t.Errorf("update an unknown origin: got %d, want 404", rec.Code)
		}
	})
}

func TestPointBatchOrigins_DeleteOnlyWhatNoBatchUses(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		unused := decodeOrigin(t, callCreateOrigin(tx, `{"name":"Sem uso `+uuid.New().String()[:8]+`"}`))

		if rec := callDeleteOrigin(tx, unused.ID); rec.Code != http.StatusOK {
			t.Fatalf("delete an unused origin: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		var count int64
		tx.Model(&data.PointBatchOrigin{}).Where("id = ?", unused.ID).Count(&count)
		if count != 0 {
			t.Error("the deleted origin is still there")
		}
		if rec := callDeleteOrigin(tx, unused.ID); rec.Code != http.StatusNotFound {
			t.Errorf("delete it again: got %d, want 404", rec.Code)
		}
	})
}

// The two refusals below come from Postgres (a unique and a foreign-key violation), and a
// violation aborts the test's surrounding transaction. Each therefore gets its own test and
// is the last statement in it.

func TestPointBatchOrigins_DuplicateNameIsUnprocessable(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		name := "Duplicada " + uuid.New().String()[:8]
		if rec := callCreateOrigin(tx, `{"name":"`+name+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("first create: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}
		if rec := callCreateOrigin(tx, `{"name":"`+name+`"}`); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("same name again: got %d, want 422 (%s)", rec.Code, rec.Body.String())
		}
	})
}

func TestPointBatchOrigins_OriginInUseCannotBeDeleted(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, origin, agency := seedStatementTarget(t, tx)
		batch := data.PointBatch{
			AccountID: account.ID, OriginID: &origin.ID, AcquiredOn: day(t, "2026-08-05"), Points: 1000,
			CostCents: 100, FundingAgencyID: agency.ID, ProfitSharePercent: 50,
		}
		create(t, tx, &batch)

		if rec := callDeleteOrigin(tx, origin.ID); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("delete an origin a batch uses: got %d, want 422 (%s)", rec.Code, rec.Body.String())
		}
	})
}
