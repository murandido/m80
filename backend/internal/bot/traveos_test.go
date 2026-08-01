package bot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/traveos"
	"gorm.io/gorm"
)

// stubTraveos stands in for the vendor. The bot's side of this integration is
// only ever about three questions - did it fetch, was it allowed to skip, did it
// fail - so the stub answers those and nothing else.
type stubTraveos struct {
	ticket *traveos.TicketItem
	data   *traveos.TicketData

	findErr error
	emitErr error

	findCalls   atomic.Int32
	emitCalls   atomic.Int32
	verifyCalls atomic.Int32
}

func (s *stubTraveos) FindByLocator(_ context.Context, _ string) (*traveos.TicketItem, error) {
	s.findCalls.Add(1)
	if s.findErr != nil {
		return nil, s.findErr
	}
	return s.ticket, nil
}

func (s *stubTraveos) Emit(_ context.Context, _ int, _, _, _, _ string) error {
	s.emitCalls.Add(1)
	return s.emitErr
}

func (s *stubTraveos) TicketData(_ context.Context, _ string) (*traveos.TicketData, error) {
	return s.data, nil
}

func (s *stubTraveos) VerifyTicket(_ context.Context, _ int) (*traveos.VerifyTicketResponse, error) {
	s.verifyCalls.Add(1)
	return &traveos.VerifyTicketResponse{Success: true}, nil
}

// workingTraveos answers as if the ticket were already there - the common case
// once the airline has confirmed the booking, and the one that requires no
// emission at all.
func workingTraveos() *stubTraveos {
	departure := time.Date(2026, 8, 1, 14, 24, 0, 0, time.UTC)

	return &stubTraveos{
		ticket: &traveos.TicketItem{ID: 7001, Code: "ticket-code", Locator: "ABC123"},
		data: &traveos.TicketData{
			Emission: traveos.Emission{
				Code:     "ticket-code",
				TripType: "somente_ida",
				Passengers: []traveos.Passenger{
					{FirstName: "FERNANDA CRISTINA MS", LastName: "PRADO", Type: "ADT"},
				},
			},
			Flights: traveos.Directional{
				Outbound: []traveos.Flight{{
					Number:          "AA930",
					Cabin:           "Econômica",
					OriginIATA:      "CNF",
					DestinationIATA: "GRU",
					DepartureAt:     traveos.NaiveTime{Time: departure},
				}},
			},
		},
	}
}

// traveosAirline is an airline the sync is allowed to fetch: supported, and
// mapped to a vendor-side id. Both are required, and the sync refuses distinctly
// when either is missing.
func traveosAirline() data.Airline {
	vendorID := 90
	return data.Airline{TraveosSupported: true, TraveosAirlineID: &vendorID}
}

func TestHandleEmissao_CompletesTheIssuanceFromTraveos(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, traveosAirline())
		vendor := workingTraveos()
		s.Bot.Traveos = vendor

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		var issuance data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&issuance).Error; err != nil {
			t.Fatalf("expected the issuance to have been created: %v", err)
		}
		if !issuance.Complete {
			t.Error("the issuance is still incomplete after a successful traveos fetch")
		}
		if issuance.TraveosSyncedAt == nil {
			t.Error("traveos_synced_at is nil - it is what the ticket page and the pdf are gated on")
		}

		// The whole point of the fetch: the message carries neither a flight date
		// nor a first name, so these two rows can only have come from the vendor.
		assertIssuanceRowCount(t, tx, "issuance_flights", issuance.ID, 1)
		assertIssuanceRowCount(t, tx, "issuance_passengers", issuance.ID, 1)

		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
			t.Fatalf("reactions = %v, want a single 🚀", reactions)
		}

		// Found, so never issued. Emitting a ticket that already exists creates a
		// second real record at the vendor, with nothing on their side to stop it.
		if got := vendor.emitCalls.Load(); got != 0 {
			t.Errorf("emitted %d time(s) for a ticket that was already there, want 0", got)
		}
		// force is false on this path, and verify-ticket belongs to the forced one.
		if got := vendor.verifyCalls.Load(); got != 0 {
			t.Errorf("verify-ticket was called %d time(s) from a message, want 0", got)
		}

		if got := s.Notify.Load(); got != 1 {
			t.Errorf("notifyIssuanceCreated called %d time(s), want 1", got)
		}
		if got := s.Updated.Load(); got != 1 {
			t.Errorf("notifyIssuanceUpdated called %d time(s), want 1 - the panel has to hear about the itinerary", got)
		}
	})
}

func TestHandleEmissao_WarnsAndKeepsThePartialIssuanceWhenTraveosFails(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, traveosAirline())
		vendor := workingTraveos()
		vendor.ticket = nil
		vendor.emitErr = traveos.ErrEmissionFailed
		s.Bot.Traveos = vendor

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		// The partial save is the point: the operator's own numbers are worth
		// keeping even when the vendor gives up nothing, and the panel's refetch
		// button can finish the job later.
		var issuance data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&issuance).Error; err != nil {
			t.Fatalf("the issuance was rolled back by a traveos failure: %v", err)
		}
		if issuance.Complete {
			t.Error("the issuance was marked complete despite the fetch failing")
		}
		if issuance.TraveosSyncedAt != nil {
			t.Error("traveos_synced_at was stamped on an issuance that never got its itinerary")
		}

		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != reactionPartial {
			t.Fatalf("reactions = %v, want a single %s", reactions, reactionPartial)
		}
		// No text. Traveos answers nearly every failure with the same opaque 500,
		// so any reason the bot put in the group would be invented.
		if sends, _ := s.Client.calls(); sends != 1 {
			t.Errorf("the bot sent %d message(s), want 1 - the reaction alone, no explanation", sends)
		}
		if got := s.Updated.Load(); got != 0 {
			t.Errorf("notifyIssuanceUpdated called %d time(s) for a fetch that wrote nothing, want 0", got)
		}
	})
}

func TestHandleEmissao_AnUnsupportedAirlineIsNotAWarning(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		// traveos_supported=false is a decision somebody made in the airline CRUD,
		// so an incomplete issuance is the expected outcome, not a problem.
		s := newEmissaoScenario(t, tx, data.Airline{TraveosSupported: false})
		vendor := workingTraveos()
		s.Bot.Traveos = vendor

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
			t.Fatalf("reactions = %v, want a single 🚀", reactions)
		}
		if got := vendor.findCalls.Load(); got != 0 {
			t.Errorf("the vendor was searched %d time(s) for an unsupported airline, want 0", got)
		}
	})
}

func TestHandleEmissao_ReactsNormallyWithNoTraveosConfigured(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		// A deployment without TRAVEOS_EMAIL/TRAVEOS_PASSWORD. Registering the
		// issuance is the part the group depends on, and it works without them - so
		// the absence must not put a warning on every message the team sends.
		s := newEmissaoScenario(t, tx, traveosAirline())
		s.Bot.Traveos = nil

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
			t.Fatalf("reactions = %v, want a single 🚀", reactions)
		}
	})
}

func TestSyncTraveosReportsACancelledJobAsAFailureWithoutBlamingTheVendor(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, traveosAirline())
		vendor := workingTraveos()
		vendor.findErr = errors.New("context canceled")
		s.Bot.Traveos = vendor

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Cancelled work is not a Traveos problem. It still counts as "not
		// complete", but the log line has to say so - whoever cancelled the job is
		// the one who owes the group an answer.
		if got := s.Bot.syncTraveos(ctx, "test", "does-not-matter", "ABC123", false); got != traveosFailed {
			t.Fatalf("outcome = %v, want traveosFailed", got)
		}
	})
}
