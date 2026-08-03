package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// These need a live Postgres for the same reason the cancel tests do: what is
// worth asserting is that a replace really replaced, and there is nothing left
// of that once the database is mocked away.

// seedItinerary hangs two outbound legs and two passengers off a seeded
// issuance, which is the smallest shape that can catch a replace writing rows
// onto the wrong passenger.
func seedItinerary(t *testing.T, tx *gorm.DB) (data.Issuance, []data.IssuanceFlight, []data.IssuancePassenger) {
	t.Helper()

	issuance, _ := seedIssuance(t, tx)

	var flights []data.IssuanceFlight
	for legOrder := 1; legOrder <= 2; legOrder++ {
		leg := data.IssuanceFlight{
			IssuanceID:  issuance.ID,
			Direction:   "outbound",
			LegOrder:    legOrder,
			DepartureAt: time.Date(2026, 8, 20, 5, 0, 0, 0, time.UTC),
		}
		create(t, tx, &leg)
		flights = append(flights, leg)
	}

	var passengers []data.IssuancePassenger
	for _, name := range []string{"FERNANDA", "THEO"} {
		passenger := data.IssuancePassenger{
			IssuanceID: issuance.ID,
			FirstName:  name,
			LastName:   "PRADO",
			Position:   len(passengers) + 1,
		}
		create(t, tx, &passenger)
		passengers = append(passengers, passenger)
	}

	return issuance, flights, passengers
}

func callService(tx *gorm.DB, method, path, id, body string, handler func(*IssuanceHandler, http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	h := NewIssuanceHandler(tx, events.NewBroker(), nil)

	req := httptest.NewRequest(method, "/issuances/"+id+path, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	handler(h, rec, req)
	return rec
}

type seatsEnvelope struct {
	Data []schemas.IssuanceSeat `json:"data"`
}

type baggageEnvelope struct {
	Data []schemas.IssuanceBaggage `json:"data"`
}

func TestPutIssuanceSeats_ReplacesTheWholeSet(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, flights, passengers := seedItinerary(t, tx)

		// What is there before: one seat on the first leg, from the sync.
		create(t, tx, &data.IssuancePassengerFlight{
			PassengerID: passengers[0].ID, FlightID: flights[0].ID, Seat: strPtr("12A"),
		})

		body := `{"seats":[
			{"passenger_id":"` + passengers[0].ID + `","flight_id":"` + flights[0].ID + `","seat":"14C"},
			{"passenger_id":"` + passengers[0].ID + `","flight_id":"` + flights[1].ID + `","seat":null},
			{"passenger_id":"` + passengers[1].ID + `","flight_id":"` + flights[0].ID + `","seat":"14D"}
		]}`
		rec := callService(tx, http.MethodPut, "/seats", issuance.ID, body,
			(*IssuanceHandler).PutIssuanceSeats)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		var rows []data.IssuancePassengerFlight
		if err := tx.Where("passenger_id IN ?", []string{passengers[0].ID, passengers[1].ID}).
			Find(&rows).Error; err != nil {
			t.Fatalf("reading the seats back: %v", err)
		}
		// Three rows, not four: the pre-existing one was replaced, not added to.
		if len(rows) != 3 {
			t.Fatalf("seat rows = %d, want the 3 the request carried", len(rows))
		}

		byKey := map[string]*string{}
		for _, row := range rows {
			byKey[row.PassengerID+"|"+row.FlightID] = row.Seat
		}
		if seat := byKey[passengers[0].ID+"|"+flights[0].ID]; seat == nil || *seat != "14C" {
			t.Errorf("the corrected seat did not stick: %v", seat)
		}
		// An explicit null is a real state - several airlines assign at check-in -
		// and has to survive as null rather than as an empty string.
		if seat := byKey[passengers[0].ID+"|"+flights[1].ID]; seat != nil {
			t.Errorf("a null seat came back as %q, want null", *seat)
		}
	})
}

func TestPutIssuanceSeats_FoldsABlankSeatIntoNull(t *testing.T) {
	// The panel sends "" when the operator clears the field. Storing that would
	// make "the airline gave no seat" and "the operator wiped it" two values
	// that render identically.
	runInTx(t, func(tx *gorm.DB) {
		issuance, flights, passengers := seedItinerary(t, tx)

		body := `{"seats":[{"passenger_id":"` + passengers[0].ID +
			`","flight_id":"` + flights[0].ID + `","seat":"   "}]}`
		if rec := callService(tx, http.MethodPut, "/seats", issuance.ID, body,
			(*IssuanceHandler).PutIssuanceSeats); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		var row data.IssuancePassengerFlight
		if err := tx.Where("passenger_id = ?", passengers[0].ID).First(&row).Error; err != nil {
			t.Fatalf("reading the seat back: %v", err)
		}
		if row.Seat != nil {
			t.Errorf("seat = %q, want null", *row.Seat)
		}
	})
}

func TestPutIssuanceSeats_RefusesAPassengerFromAnotherIssuance(t *testing.T) {
	// Without this check a payload could write onto somebody else's booking just
	// by naming its uuid - every authenticated user can reach this route.
	runInTx(t, func(tx *gorm.DB) {
		issuance, flights, _ := seedItinerary(t, tx)
		_, _, strangers := seedItinerary(t, tx)

		body := `{"seats":[{"passenger_id":"` + strangers[0].ID +
			`","flight_id":"` + flights[0].ID + `","seat":"1A"}]}`
		rec := callService(tx, http.MethodPut, "/seats", issuance.ID, body,
			(*IssuanceHandler).PutIssuanceSeats)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
		}

		var count int64
		tx.Model(&data.IssuancePassengerFlight{}).Where("passenger_id = ?", strangers[0].ID).Count(&count)
		if count != 0 {
			t.Errorf("the refused request still wrote %d row(s)", count)
		}
	})
}

func TestGetIssuanceSeats_AnswersTheShapeThePutAccepts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, flights, passengers := seedItinerary(t, tx)
		create(t, tx, &data.IssuancePassengerFlight{
			PassengerID: passengers[0].ID, FlightID: flights[0].ID, Seat: strPtr("12A"),
		})

		rec := callService(tx, http.MethodGet, "/seats", issuance.ID, "",
			(*IssuanceHandler).GetIssuanceSeats)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		var envelope seatsEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decoding the envelope: %v", err)
		}
		if len(envelope.Data) != 1 {
			t.Fatalf("seats = %d, want 1", len(envelope.Data))
		}
		if envelope.Data[0].Seat == nil || *envelope.Data[0].Seat != "12A" {
			t.Errorf("seat = %v, want 12A", envelope.Data[0].Seat)
		}
	})
}

func TestPutIssuanceBaggage_ReplacesAndKeepsThePosition(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _, passengers := seedItinerary(t, tx)

		body := `{"baggage":[
			{"passenger_id":"` + passengers[0].ID + `","direction":"outbound","position":0,"weight":"10kg","quantity":1},
			{"passenger_id":"` + passengers[0].ID + `","direction":"outbound","position":1,"weight":"23kg","quantity":2}
		]}`
		if rec := callService(tx, http.MethodPut, "/baggage", issuance.ID, body,
			(*IssuanceHandler).PutIssuanceBaggage); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		var rows []data.IssuancePassengerBaggage
		if err := tx.Where("passenger_id = ?", passengers[0].ID).
			Order("position asc").Find(&rows).Error; err != nil {
			t.Fatalf("reading the baggage back: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("baggage rows = %d, want 2", len(rows))
		}
		// The position is the panel's to send, not the server's to renumber: it
		// is what picks the icon on the ticket.
		if rows[0].Position != 0 || rows[1].Position != 1 {
			t.Errorf("positions = %d/%d, want 0/1", rows[0].Position, rows[1].Position)
		}
		if rows[1].Quantity != 2 || rows[1].Weight == nil || *rows[1].Weight != "23kg" {
			t.Errorf("second allowance = %+v, want 2 x 23kg", rows[1])
		}
	})
}

func TestPutIssuanceBaggage_RefusesTwoAllowancesInTheSameSlot(t *testing.T) {
	// The table has UNIQUE (passenger_id, direction, position). Catching it here
	// answers 422 with a sentence; letting the constraint fire answers 500 with a
	// Postgres error string.
	runInTx(t, func(tx *gorm.DB) {
		issuance, _, passengers := seedItinerary(t, tx)

		body := `{"baggage":[
			{"passenger_id":"` + passengers[0].ID + `","direction":"outbound","position":0,"weight":"10kg","quantity":1},
			{"passenger_id":"` + passengers[0].ID + `","direction":"outbound","position":0,"weight":"23kg","quantity":1}
		]}`
		rec := callService(tx, http.MethodPut, "/baggage", issuance.ID, body,
			(*IssuanceHandler).PutIssuanceBaggage)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestPutIssuanceBaggage_KeepsAnEntitlementWithNoPieces(t *testing.T) {
	// "23kg: 0" is information - the airline offers it and the passenger did not
	// buy any - so a zero must not be dropped on the way in either.
	runInTx(t, func(tx *gorm.DB) {
		issuance, _, passengers := seedItinerary(t, tx)

		body := `{"baggage":[{"passenger_id":"` + passengers[0].ID +
			`","direction":"outbound","position":0,"weight":"23kg","quantity":0}]}`
		if rec := callService(tx, http.MethodPut, "/baggage", issuance.ID, body,
			(*IssuanceHandler).PutIssuanceBaggage); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		var rows []data.IssuancePassengerBaggage
		tx.Where("passenger_id = ?", passengers[0].ID).Find(&rows)
		if len(rows) != 1 || rows[0].Quantity != 0 {
			t.Errorf("rows = %+v, want a single allowance with quantity 0", rows)
		}
	})
}

func TestGetIssuanceBaggage_AnswersTheShapeThePutAccepts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _, passengers := seedItinerary(t, tx)
		create(t, tx, &data.IssuancePassengerBaggage{
			PassengerID: passengers[0].ID, Direction: "outbound", Position: 0,
			Weight: strPtr("10kg"), Quantity: 1,
		})

		rec := callService(tx, http.MethodGet, "/baggage", issuance.ID, "",
			(*IssuanceHandler).GetIssuanceBaggage)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		var envelope baggageEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decoding the envelope: %v", err)
		}
		if len(envelope.Data) != 1 || envelope.Data[0].Direction != "outbound" {
			t.Fatalf("baggage = %+v, want one outbound allowance", envelope.Data)
		}
	})
}

func TestIssuanceServices_NotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		missing := "00000000-0000-4000-8000-000000000000"
		rec := callService(tx, http.MethodGet, "/seats", missing, "",
			(*IssuanceHandler).GetIssuanceSeats)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

func strPtr(value string) *string { return &value }
