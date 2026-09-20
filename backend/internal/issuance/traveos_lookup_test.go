package issuance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/traveos"
	"gorm.io/gorm"
)

func lookupTicketData() *traveos.TicketData {
	departure := time.Date(2026, 10, 22, 8, 30, 0, 0, time.UTC)
	details := &traveos.TicketData{}
	details.Flights.Outbound = []traveos.Flight{
		{OriginIATA: "rbr", DestinationIATA: "BSB", DepartureAt: traveos.NaiveTime{Time: departure}},
		{OriginIATA: "BSB", DestinationIATA: "MAO", DepartureAt: traveos.NaiveTime{Time: departure.Add(3 * time.Hour)}},
	}
	details.Flights.Inbound = []traveos.Flight{
		{OriginIATA: "MAO", DestinationIATA: "RBR", DepartureAt: traveos.NaiveTime{Time: departure.AddDate(0, 0, 7)}},
	}
	details.Emission.Passengers = []traveos.Passenger{{FirstName: "THALYSSON MR", LastName: "BRILHANTE"}}
	return details
}

func TestPreviewTakesTheDestinationFromTheEndOfTheOutbound(t *testing.T) {
	// A round trip ends where it started, so reading the last leg of the whole
	// itinerary would say the passenger flew to their own city.
	preview := previewTicketData(lookupTicketData())

	if preview.OriginIATACode == nil || *preview.OriginIATACode != "RBR" {
		t.Errorf("origin = %v, want RBR", preview.OriginIATACode)
	}
	if preview.DestinationIATACode == nil || *preview.DestinationIATACode != "MAO" {
		t.Errorf("destination = %v, want MAO", preview.DestinationIATACode)
	}
	if preview.FlightDate == nil || preview.FlightDate.Format("2006-01-02") != "2026-10-22" {
		t.Errorf("flight date = %v, want 2026-10-22", preview.FlightDate)
	}
	if preview.MainPassengerName == nil || *preview.MainPassengerName != "THALYSSON BRILHANTE" {
		t.Errorf("passenger = %v, want the courtesy title stripped", preview.MainPassengerName)
	}
}

func TestLookupCacheHandsAnEntryOutOnce(t *testing.T) {
	cache := NewLookupCache()
	id, err := cache.put(lookupEntry{airlineID: "a"})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, ok := cache.take(id); !ok {
		t.Fatal("the first take found nothing")
	}
	if _, ok := cache.take(id); ok {
		t.Error("the same lookup was handed out twice - it could land on a second issuance")
	}
}

func TestLookupCacheDropsExpiredEntries(t *testing.T) {
	cache := NewLookupCache()
	now := time.Now()
	cache.now = func() time.Time { return now }
	id, err := cache.put(lookupEntry{airlineID: "a"})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	cache.now = func() time.Time { return now.Add(lookupTTL + time.Minute) }
	if _, ok := cache.take(id); ok {
		t.Error("an expired lookup was still handed out")
	}
}

func TestLookupFindsBeforeEmittingAndCachesThePayload(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		client := &fakeTraveos{
			findResults: []*traveos.TicketItem{existingTicket()},
			ticketData:  lookupTicketData(),
		}
		cache := NewLookupCache()
		origin := "RBR"

		result, err := Lookup(context.Background(), tx, client, cache, LookupInput{
			AirlineID:      f.Airline.ID,
			ProgramLocator: "ABC123",
			OriginIATACode: &origin,
		})
		if err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if client.emitCalls != 0 {
			t.Errorf("emitted %d times for a ticket already at the vendor", client.emitCalls)
		}
		if result.TicketCode != "code-1" || result.LookupID == "" {
			t.Errorf("result = %+v, want the ticket code and a lookup id", result)
		}
		if _, ok := cache.take(result.LookupID); !ok {
			t.Error("the payload was not kept for the create")
		}
	})
}

func TestLookupRefusesAnAirlineWithoutIntegration(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{TraveosSupported: false})
		client := &fakeTraveos{}

		_, err := Lookup(context.Background(), tx, client, NewLookupCache(), LookupInput{
			AirlineID:      f.Airline.ID,
			ProgramLocator: "ABC123",
		})
		if !errors.Is(err, ErrAirlineNotSupported) {
			t.Fatalf("err = %v, want ErrAirlineNotSupported", err)
		}
		if client.findCalls != 0 {
			t.Errorf("called the vendor %d times for an airline it refuses", client.findCalls)
		}
	})
}

func TestApplyLookupFromTheCacheMakesNoVendorCall(t *testing.T) {
	// The point of the cache: the lookup already paid for the itinerary, and saving the
	// row must not buy it again.
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		cache := NewLookupCache()
		id, err := cache.put(lookupEntry{airlineID: f.Airline.ID, ticket: existingTicket(), details: lookupTicketData()})
		if err != nil {
			t.Fatalf("put: %v", err)
		}
		client := &fakeTraveos{}

		if err := ApplyLookup(context.Background(), tx, client, cache, issuance.ID, id, "code-1"); err != nil {
			t.Fatalf("ApplyLookup: %v", err)
		}
		if calls := client.findCalls + client.emitCalls + client.ticketDataCalls; calls != 0 {
			t.Errorf("made %d vendor calls, want 0", calls)
		}

		var reloaded data.Issuance
		if err := tx.First(&reloaded, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("reload: %v", err)
		}
		if reloaded.TraveosSyncedAt == nil || !reloaded.Complete {
			t.Error("the issuance was not marked as synced and complete")
		}
		assertCount(t, tx, "issuance_flights", "issuance_id = ?", issuance.ID, 3)
	})
}

func TestApplyLookupWithoutTheCacheReadsTheTicketDirectly(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		client := &fakeTraveos{ticketData: lookupTicketData()}

		if err := ApplyLookup(context.Background(), tx, client, NewLookupCache(), issuance.ID, "gone", "code-1"); err != nil {
			t.Fatalf("ApplyLookup: %v", err)
		}
		if client.ticketDataCalls != 1 {
			t.Errorf("read the ticket %d times, want 1", client.ticketDataCalls)
		}
		if client.findCalls != 0 || client.emitCalls != 0 {
			t.Errorf("searched %d and emitted %d times, want neither", client.findCalls, client.emitCalls)
		}
	})
}
