package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func callAirlineUpdate(tx *gorm.DB, id, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/airlines/"+id, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	NewAirlineHandler(tx).UpdateAirline(rec, req)
	return rec
}

func seedMappedAirline(t *testing.T, tx *gorm.DB) data.Airline {
	t.Helper()

	traveosID := 42
	colour := "#FF6600"
	airline := data.Airline{
		Name:             "Test Air",
		Code:             "T" + uuid.New().String()[:8],
		TraveosSupported: true,
		TraveosAirlineID: &traveosID,
		BrandColor:       &colour,
	}
	create(t, tx, &airline)
	return airline
}

func TestUpdateAirline_ClearsTheTraveosIDWhenSentAsNull(t *testing.T) {
	// An id typed wrong used to be permanent: null read as "leave alone" and 0 is
	// refused by validation, so no payload could take it back.
	runInTx(t, func(tx *gorm.DB) {
		airline := seedMappedAirline(t, tx)

		body := `{"name":"Test Air","code":"` + airline.Code +
			`","traveos_supported":true,"traveos_airline_id":null,"brand_color":null}`
		if rec := callAirlineUpdate(tx, airline.ID, body); rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var updated data.Airline
		if err := tx.First(&updated, "id = ?", airline.ID).Error; err != nil {
			t.Fatalf("reloading the airline: %v", err)
		}
		if updated.TraveosAirlineID != nil {
			t.Errorf("traveos_airline_id = %d, want nil", *updated.TraveosAirlineID)
		}
		if updated.BrandColor != nil {
			t.Errorf("brand_color = %q, want nil", *updated.BrandColor)
		}
	})
}

func TestUpdateAirline_SetsTheTraveosID(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		airline := seedMappedAirline(t, tx)

		body := `{"name":"Test Air","code":"` + airline.Code + `","traveos_airline_id":77}`
		if rec := callAirlineUpdate(tx, airline.ID, body); rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var updated data.Airline
		if err := tx.First(&updated, "id = ?", airline.ID).Error; err != nil {
			t.Fatalf("reloading the airline: %v", err)
		}
		if updated.TraveosAirlineID == nil || *updated.TraveosAirlineID != 77 {
			t.Errorf("traveos_airline_id = %v, want 77", updated.TraveosAirlineID)
		}
	})
}

func TestUpdateAirline_RefusesZeroAsATraveosID(t *testing.T) {
	// 0 is not a carrier in the vendor's catalogue; clearing is done with null.
	runInTx(t, func(tx *gorm.DB) {
		airline := seedMappedAirline(t, tx)

		body := `{"name":"Test Air","code":"` + airline.Code + `","traveos_airline_id":0}`
		if rec := callAirlineUpdate(tx, airline.ID, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
