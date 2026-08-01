package bot

import (
	"context"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/traveos"
	"gorm.io/gorm"
)

func TestMovedATraveosInput(t *testing.T) {
	// Pure: no Postgres needed. What decides a retry is exactly what the audit log
	// is about to record, so the two can never drift apart.
	tests := []struct {
		name    string
		changed map[string]any
		want    bool
	}{
		{"origin", map[string]any{"origin_iata_code": "GRU"}, true},
		{"surname", map[string]any{"main_passenger_name": "PRADO"}, true},
		{"purchase number", map[string]any{"latam_purchase_number": "123"}, true},
		// Both of these change what is sent to the vendor too: the airline PNR is
		// the locator Traveos scrapes with, and the airline decides which
		// vendor-side airline id is used at all.
		{"airline locator", map[string]any{"airline_locator": "XYZ789"}, true},
		{"airline", map[string]any{"airline_id": "some-uuid"}, true},

		// A wrong price is worth correcting and has nothing to do with the fetch;
		// re-asking the vendor because of it would be a real emission for nothing.
		{"points only", map[string]any{"points": 31000}, false},
		{"observation only", map[string]any{"observation": "ACORDO"}, false},
		{"nothing", map[string]any{}, false},

		{"one of several", map[string]any{"points": 31000, "origin_iata_code": "GRU"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			details := map[string]any{"before": map[string]any{}, "after": tc.changed}
			if got := movedATraveosInput(details); got != tc.want {
				t.Errorf("movedATraveosInput(%v) = %v, want %v", tc.changed, got, tc.want)
			}
		})
	}
}

func TestHandleAlteracao_RetriesTraveosWhenTheCorrectionFixesItsInput(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, traveosAirline())
		// Without this the correction's tie-break (airport, then observation
		// preset, then account alias) reads a bare "GRU" as an account nickname and
		// the test would be correcting a completely different column.
		seedAirport(t, tx, "GRU", "Guarulhos")

		// First pass fails, exactly as a wrong surname would.
		failing := workingTraveos()
		failing.ticket = nil
		failing.emitErr = traveos.ErrEmissionFailed
		s.Bot.Traveos = failing
		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		// The operator retypes the origin. That is the input the vendor is asked
		// with, so it earns a second attempt.
		working := workingTraveos()
		s.Bot.Traveos = working
		s.Bot.handleAlteracao(context.Background(), s.eventWithID("correction"), correctionText(s, "GRU"), time.Now())

		if got := working.findCalls.Load(); got != 1 {
			t.Fatalf("the vendor was searched %d time(s) after the correction, want 1", got)
		}

		var issuance data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&issuance).Error; err != nil {
			t.Fatalf("issuance: %v", err)
		}
		if !issuance.Complete {
			t.Error("the retry did not complete the issuance")
		}
	})
}

func TestHandleAlteracao_DoesNotRetryTraveosForAnUnrelatedField(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, traveosAirline())

		failing := workingTraveos()
		failing.ticket = nil
		failing.emitErr = traveos.ErrEmissionFailed
		s.Bot.Traveos = failing
		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		// A corrected price says nothing about why the fetch failed, and asking the
		// vendor again could mean issuing a real ticket for nothing.
		working := workingTraveos()
		s.Bot.Traveos = working
		s.Bot.handleAlteracao(context.Background(), s.eventWithID("correction"),
			"ALTERACAO - PL"+s.Suffix+"\n30k * 16,5 + 189,22 tx = 684,22", time.Now())

		if got := working.findCalls.Load(); got != 0 {
			t.Fatalf("the vendor was searched %d time(s) for a price correction, want 0", got)
		}
	})
}

// correctionText is an ALTERACAO carrying only a route line, which is the shape
// the retry rule is about.
func correctionText(s emissaoScenario, origin string) string {
	return "ALTERACAO - PL" + s.Suffix + "\n" + origin
}
