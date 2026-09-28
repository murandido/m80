package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/booking"
)

// TestSyncErrorTellsTheOperatorWhatToCheck pins that a booking nobody found reads as
// something the operator can fix, not as the generic "the vendor failed" 502.
func TestSyncErrorTellsTheOperatorWhatToCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantText   string
	}{
		{"not found", booking.NotFound("gol has no booking ABC123 from GRU"), http.StatusUnprocessableEntity, "Confira o localizador, a origem e o sobrenome"},
		{"unreachable", booking.Unavailable(nil, "edge returned 403"), http.StatusBadGateway, "Não consegui puxar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/issuances/id/traveos-sync", nil)

			(&IssuanceHandler{}).respondToTraveosSyncError(recorder, request, "id", tt.err)

			if recorder.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			var body struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			if !strings.Contains(body.Message, tt.wantText) {
				t.Errorf("message = %q, want it to contain %q", body.Message, tt.wantText)
			}
		})
	}
}
