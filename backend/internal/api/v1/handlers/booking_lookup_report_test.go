package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func attemptRow(syncID, airlineID, provider, outcome string, ms int) lookupAttemptRow {
	row := lookupAttemptRow{BookingLookupAttempt: data.BookingLookupAttempt{
		SyncID: syncID, AirlineID: airlineID, IssuanceID: "issuance-" + syncID,
		Provider: provider, Outcome: outcome, DurationMS: ms,
	}, ProgramLocator: "PROG" + syncID}
	if outcome != data.LookupFound {
		message := provider + " said " + outcome
		row.Error = &message
	}
	return row
}

// Three syncs for one airline: the airline answered, the airline was down and Traveos
// caught it, and nobody found the booking. Only the second is a fallback.
func TestBuildLookupReportTellsFallbacksApart(t *testing.T) {
	copa := data.Airline{ID: "copa-id", Code: "COPA", Name: "COPA AIRLINES", DirectBookingEnabled: true}
	quiet := data.Airline{ID: "quiet-id", Code: "QUIET", Name: "QUIET AIR"}
	rows := []lookupAttemptRow{
		attemptRow("3", "copa-id", "copa", data.LookupNotFound, 300),
		attemptRow("3", "copa-id", "traveos", data.LookupNotFound, 900),
		attemptRow("2", "copa-id", "copa", data.LookupUnavailable, 100),
		attemptRow("2", "copa-id", "traveos", data.LookupFound, 2000),
		attemptRow("1", "copa-id", "copa", data.LookupFound, 200),
	}
	rows[2].AirlineLocator = strPtr("AIRLOC")

	report := buildLookupReport([]data.Airline{copa, quiet}, rows,
		func(code string) string {
			if code == "COPA" {
				return "copa"
			}
			return ""
		}, "traveos")

	if len(report) != 1 {
		t.Fatalf("report has %d airlines, want only the one with an integration or attempts", len(report))
	}
	line := report[0]
	if line.Syncs != 3 || line.AnsweredByDirect != 1 || line.AnsweredByTraveos != 1 ||
		line.FellBackToTraveos != 1 || line.Unanswered != 1 {
		t.Errorf("syncs = %+v", line)
	}
	if line.DirectFound != 1 || line.DirectNotFound != 1 || line.DirectUnavailable != 1 || line.DirectAverageMS != 200 {
		t.Errorf("direct attempts = found %d, not found %d, unavailable %d, avg %d",
			line.DirectFound, line.DirectNotFound, line.DirectUnavailable, line.DirectAverageMS)
	}
	if len(line.RecentFailures) != 2 {
		t.Fatalf("recent failures = %+v, want the two direct misses", line.RecentFailures)
	}
	if first := line.RecentFailures[0]; first.Outcome != data.LookupNotFound || first.AnsweredBy != "" || first.Locator != "PROG3" {
		t.Errorf("newest failure = %+v", first)
	}
	if second := line.RecentFailures[1]; second.AnsweredBy != "traveos" || second.Locator != "AIRLOC" {
		t.Errorf("fallback failure = %+v, want Traveos named and the airline locator", second)
	}
}

// The query side: attempts outside the window are left out, and each one is joined to
// its issuance's locator.
func TestGetReportReadsTheWindow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)
		syncID := uuid.New().String()
		for _, attempt := range []data.BookingLookupAttempt{
			{SyncID: syncID, IssuanceID: issuance.ID, AirlineID: issuance.AirlineID, Provider: "traveos", Outcome: data.LookupFound, DurationMS: 50},
			{SyncID: uuid.New().String(), IssuanceID: issuance.ID, AirlineID: issuance.AirlineID, Provider: "traveos",
				Outcome: data.LookupFound, DurationMS: 50, CreatedAt: time.Now().Add(-30 * 24 * time.Hour)},
		} {
			create(t, tx, &attempt)
		}

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/booking-lookups/report?days=7", nil)
		NewBookingLookupReportHandler(tx, booking.NewRegistry(nil)).GetReport(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}

		var envelope struct {
			Data struct {
				Airlines []struct {
					AirlineID string `json:"airline_id"`
					Syncs     int    `json:"syncs"`
				} `json:"airlines"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		found := false
		for _, line := range envelope.Data.Airlines {
			if line.AirlineID == issuance.AirlineID {
				found = true
				if line.Syncs != 1 {
					t.Errorf("syncs = %d, want the one inside the window", line.Syncs)
				}
			}
		}
		if !found {
			t.Error("the airline with an attempt in the window is missing from the report")
		}
	})
}

func TestGetReportRefusesAWindowOutOfRange(t *testing.T) {
	for _, days := range []string{"0", "91", "x"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/booking-lookups/report?days="+days, nil)
		NewBookingLookupReportHandler(nil, booking.NewRegistry(nil)).GetReport(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("days=%s: status = %d, want 400", days, recorder.Code)
		}
	}
}
