package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

func TestGetSales_AgencyFilterStaysInScope(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		mine := seedNamedAgency(t, tx, "Mine")
		other := seedNamedAgency(t, tx, "Other")
		scope := access.AgencyScope{IDs: []string{mine.ID}}

		call := func(query string, scope access.AgencyScope) int {
			req := withScope(httptest.NewRequest(http.MethodGet, "/sales?"+query, nil), scope)
			rec := httptest.NewRecorder()
			NewSaleHandler(tx, events.NewBroker()).GetSales(rec, req)
			return rec.Code
		}

		if code := call("agency_id="+other.ID, scope); code != http.StatusNotFound {
			t.Errorf("foreign agency: got %d, want 404", code)
		}
		if code := call("agency_id="+mine.ID, scope); code != http.StatusOK {
			t.Errorf("own agency: got %d, want 200", code)
		}
		if code := call("", scope); code != http.StatusOK {
			t.Errorf("no agency asked: got %d, want 200 (falls back to the caller's own)", code)
		}
		if code := call("", access.AgencyScope{}); code != http.StatusForbidden {
			t.Errorf("no grants: got %d, want 403", code)
		}
	})
}
