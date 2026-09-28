package bot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/traveos"
	"gorm.io/gorm"
)

// stubTraveos stands in for the vendor, as a booking provider that also imports -
// the shape only Traveos has. The bot's side of this integration is only ever
// about three questions - did it fetch, was it allowed to skip, did it fail - so
// the stub answers those and nothing else.
type stubTraveos struct {
	result *booking.Booking

	fetchErr  error
	importErr error

	fetchCalls  atomic.Int32
	importCalls atomic.Int32
	forced      atomic.Bool
}

func (s *stubTraveos) Name() string { return "traveos" }

func (s *stubTraveos) Fetch(_ context.Context, req booking.Request) (*booking.Booking, error) {
	s.fetchCalls.Add(1)
	s.forced.Store(req.Force)
	if s.fetchErr != nil {
		return nil, s.fetchErr
	}
	if s.result == nil {
		return nil, booking.ErrNotFound
	}
	return s.result, nil
}

func (s *stubTraveos) EnsureAvailable(context.Context, booking.Request) error {
	s.importCalls.Add(1)
	return s.importErr
}

// workingTraveos answers as if the ticket were already there - the common case
// once the airline has confirmed the booking, and the one that requires no
// emission at all.
// assertFetchThen pins the reaction sequence around a Traveos fetch: the
// hourglass while the vendor is being asked, then the outcome replacing it.
//
// The order is the whole property. ⏳ left as the last reaction is the failure
// this guards against - the group would be looking at a message that claims the
// bot is still working on something it finished, which is exactly the confusion
// the hourglass was added to remove.
func assertFetchThen(t *testing.T, reactions []string, want string) {
	t.Helper()

	if len(reactions) != 2 {
		t.Fatalf("reactions = %v, want exactly [%s %s]", reactions, reactionFetching, want)
	}
	if reactions[0] != reactionFetching {
		t.Errorf("first reaction = %q, want %s while the vendor is being asked", reactions[0], reactionFetching)
	}
	if reactions[1] != want {
		t.Errorf("final reaction = %q, want %q", reactions[1], want)
	}
}

func workingTraveos() *stubTraveos {
	departure := time.Date(2026, 8, 1, 14, 24, 0, 0, time.UTC)

	return &stubTraveos{
		result: &booking.Booking{
			Locator:  "ABC123",
			TripType: booking.TripOneWay,
			Passengers: []booking.Passenger{
				{FirstName: "FERNANDA CRISTINA MS", LastName: "PRADO", Type: booking.PassengerAdult},
			},
			Outbound: []booking.Flight{{
				Number:          "AA930",
				Cabin:           "Econômica",
				OriginIATA:      "CNF",
				DestinationIATA: "GRU",
				DepartureAt:     departure,
			}},
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
		s.Bot.Bookings = booking.NewRegistry(vendor)

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

		assertFetchThen(t, s.Client.reactions(), "🚀")

		// Found, so never issued. Emitting a ticket that already exists creates a
		// second real record at the vendor, with nothing on their side to stop it.
		if got := vendor.importCalls.Load(); got != 0 {
			t.Errorf("emitted %d time(s) for a ticket that was already there, want 0", got)
		}
		// force is false on this path: a message is a first fetch, never the
		// operator asking for a stale itinerary to be refreshed.
		if vendor.forced.Load() {
			t.Error("a fetch triggered by a message asked the provider to force a refresh")
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
		vendor.result = nil
		vendor.importErr = traveos.ErrEmissionFailed
		s.Bot.Bookings = booking.NewRegistry(vendor)

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

		assertFetchThen(t, s.Client.reactions(), reactionPartial)
		// No text. Traveos answers nearly every failure with the same opaque 500,
		// so any reason the bot put in the group would be invented. Two sends, both
		// reactions: the hourglass and the outcome that replaces it.
		if sends, _ := s.Client.calls(); sends != 2 {
			t.Errorf("the bot sent %d message(s), want 2 reactions and no explanation", sends)
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
		s.Bot.Bookings = booking.NewRegistry(vendor)

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		assertFetchThen(t, s.Client.reactions(), "🚀")
		if got := vendor.fetchCalls.Load(); got != 0 {
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
		s.Bot.Bookings = nil

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		// No hourglass here, unlike every other path: it announces a vendor call,
		// and there is no vendor to call. Claiming otherwise would show the group a
		// fetch that never happened.
		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
			t.Fatalf("reactions = %v, want a single 🚀 with no vendor configured", reactions)
		}
	})
}

func TestSyncTraveosReportsACancelledJobAsAFailureWithoutBlamingTheVendor(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, traveosAirline())
		vendor := workingTraveos()
		vendor.fetchErr = errors.New("context canceled")
		s.Bot.Bookings = booking.NewRegistry(vendor)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Cancelled work is not a Traveos problem. It still counts as "not
		// complete", but the log line has to say so - whoever cancelled the job is
		// the one who owes the group an answer.
		if got := s.Bot.syncTraveos(ctx, nil, "test", "does-not-matter", "ABC123", false); got != traveosFailed {
			t.Fatalf("outcome = %v, want traveosFailed", got)
		}
	})
}
