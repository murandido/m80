package issuance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func identityAirport(code string) *string {
	if code == "" {
		return nil
	}
	return &code
}

func TestPreviewFieldsReadsTheRouteWhereTheOutboundEnds(t *testing.T) {
	// A round trip lands back where it started; the destination is where the outbound
	// ends, the same rule applyIssuanceFields follows.
	departure := time.Date(2026, 10, 22, 8, 30, 0, 0, time.UTC)
	fetched := &booking.Booking{
		Outbound: []booking.Flight{
			{OriginIATA: "GRU", DestinationIATA: "PTY", DepartureAt: departure},
			{OriginIATA: "PTY", DestinationIATA: "MIA", DepartureAt: departure.Add(6 * time.Hour)},
		},
		Inbound:        []booking.Flight{{OriginIATA: "MIA", DestinationIATA: "GRU", DepartureAt: departure.Add(240 * time.Hour)}},
		Passengers:     []booking.Passenger{{FirstName: "FERNANDA MS", LastName: "ASSIS"}},
		PurchaseNumber: " LA123 ",
	}

	preview := previewFields(fetched, identityAirport)

	if preview.OriginIATACode == nil || *preview.OriginIATACode != "GRU" {
		t.Errorf("origin = %v, want GRU", preview.OriginIATACode)
	}
	if preview.DestinationIATACode == nil || *preview.DestinationIATACode != "MIA" {
		t.Errorf("destination = %v, want MIA - where the outbound ends", preview.DestinationIATACode)
	}
	if preview.FlightDate == nil || *preview.FlightDate != "2026-10-22" {
		t.Errorf("flight date = %v, want 2026-10-22", preview.FlightDate)
	}
	if preview.MainPassengerName == nil || *preview.MainPassengerName != "FERNANDA ASSIS" {
		t.Errorf("passenger = %v, want the full name without the courtesy title", preview.MainPassengerName)
	}
	if preview.LatamPurchaseNumber == nil || *preview.LatamPurchaseNumber != "LA123" {
		t.Errorf("purchase number = %v, want LA123", preview.LatamPurchaseNumber)
	}
}

func TestPreviewFieldsLeavesAnUnknownAirportBlank(t *testing.T) {
	// The create that follows would fail on the airport foreign key, so a code the
	// table does not have is dropped rather than carried into the form.
	fetched := &booking.Booking{Outbound: []booking.Flight{
		{OriginIATA: "GRU", DestinationIATA: "XXX", DepartureAt: time.Now()},
	}}
	known := func(code string) *string {
		if code == "GRU" {
			return &code
		}
		return nil
	}

	preview := previewFields(fetched, known)
	if preview.DestinationIATACode != nil {
		t.Errorf("destination = %q, want nil for an airport the table does not have", *preview.DestinationIATACode)
	}
}

func lookupInput(airlineID string) LookupInput {
	origin := "GRU"
	lastName := "ASSIS"
	return LookupInput{AirlineID: airlineID, ProgramLocator: "ABC123", OriginIATACode: &origin, MainPassengerName: &lastName}
}

func TestLookupReadsTheBookingWithoutAnIssuance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		provider := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}

		preview, err := Lookup(context.Background(), tx, booking.NewRegistry(provider), lookupInput(f.Airline.ID))
		if err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if preview.Source != "fake" {
			t.Errorf("source = %q, want the provider that answered", preview.Source)
		}
		if provider.requested.Locator != "ABC123" || provider.requested.LastName != "ASSIS" {
			t.Errorf("asked with %+v, want the operator's locator and surname", provider.requested)
		}

		var count int64
		tx.Model(&data.Issuance{}).Where("program_locator = ?", "ABC123").Count(&count)
		if count != 0 {
			t.Errorf("%d issuances written by a lookup, want none", count)
		}
	})
}

func TestLookupAsksWithTheSurnameTypedForIt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		provider := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}
		input := lookupInput(f.Airline.ID)
		full := "MARIA DA SILVA"
		surname := "SILVA"
		input.MainPassengerName = &full
		input.PassengerLastName = &surname

		if _, err := Lookup(context.Background(), tx, booking.NewRegistry(provider), input); err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if provider.requested.LastName != "SILVA" {
			t.Errorf("lastName = %q, want the surname typed for the lookup", provider.requested.LastName)
		}
	})
}

func TestLookupImportsAtTheVendorWhenOnlyItCanFetch(t *testing.T) {
	// The vendor is a provider like any other here: a booking it does not have yet is
	// imported, and the Sync after the save simply reads it.
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		provider := &fakeProvider{fetchResults: []*booking.Booking{nil, existingBooking()}}

		if _, err := Lookup(context.Background(), tx, booking.NewRegistry(provider), lookupInput(f.Airline.ID)); err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if provider.importCalls != 1 {
			t.Errorf("imported %d times, want 1", provider.importCalls)
		}
	})
}

func TestLookupNeverImportsAfterTheAirlineSaysNo(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, DirectBookingEnabled: true, TraveosAirlineID: &traveosID})
		fallback := &fakeProvider{}
		registry := booking.NewRegistry(fallback)
		registry.Register(f.Airline.Code, &readOnlyProvider{})

		_, err := Lookup(context.Background(), tx, registry, lookupInput(f.Airline.ID))
		if !errors.Is(err, booking.ErrNotFound) {
			t.Fatalf("err = %v, want booking.ErrNotFound", err)
		}
		if fallback.importCalls != 0 {
			t.Errorf("the vendor imported %d times after the airline disowned the booking", fallback.importCalls)
		}
	})
}

func TestLookupRefusesAnAirlineNobodyIntegrated(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})
		provider := &fakeProvider{}

		_, err := Lookup(context.Background(), tx, booking.NewRegistry(provider), lookupInput(f.Airline.ID))
		if !errors.Is(err, ErrAirlineNotSupported) {
			t.Fatalf("err = %v, want ErrAirlineNotSupported", err)
		}
		if provider.fetchCalls != 0 {
			t.Errorf("called the provider %d times for an airline it refuses", provider.fetchCalls)
		}
	})
}
