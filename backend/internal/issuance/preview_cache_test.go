package issuance

import (
	"context"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// testClock is a hand-moved clock, so a test can cross the TTL without sleeping.
type testClock struct{ current time.Time }

func (c *testClock) now() time.Time { return c.current }

func newTestPreviewCache() (*previewBookingCache, *testClock) {
	clock := &testClock{current: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}
	return newPreviewBookingCache(previewBookingTTL, clock.now), clock
}

func TestPreviewCacheIsTakenOnlyOnce(t *testing.T) {
	// Take-once is what keeps a later "refetch" press going to the provider: the
	// preview's answer serves the Sync right after the save, and nothing after it.
	cache, _ := newTestPreviewCache()
	cache.put([]string{"key"}, existingBooking(), "fake", 42)

	entry, ok := cache.take("key")
	if !ok || entry.source != "fake" || entry.durationMS != 42 || entry.booking.Locator != "ABC123" {
		t.Fatalf("take = %+v, %v; want the stored booking", entry, ok)
	}
	if _, ok := cache.take("key"); ok {
		t.Error("the same entry was taken twice")
	}
}

func TestPreviewCacheExpires(t *testing.T) {
	cache, clock := newTestPreviewCache()
	cache.put([]string{"fresh", "stale"}, existingBooking(), "fake", 0)

	clock.current = clock.current.Add(previewBookingTTL - time.Second)
	if _, ok := cache.take("fresh"); !ok {
		t.Fatal("an entry inside the window was not returned")
	}

	cache.put([]string{"stale"}, existingBooking(), "fake", 0)
	clock.current = clock.current.Add(previewBookingTTL)
	if _, ok := cache.take("stale"); ok {
		t.Error("an entry past its TTL was returned")
	}
	if len(cache.entries) != 0 {
		t.Errorf("%d entries left after taking an expired one, want none", len(cache.entries))
	}
}

func TestPreviewCacheSweepsExpiredEntriesOnPut(t *testing.T) {
	// Drafts that are never saved never get taken; the map must not grow with them.
	cache, clock := newTestPreviewCache()
	cache.put([]string{"abandoned"}, existingBooking(), "fake", 0)
	clock.current = clock.current.Add(previewBookingTTL)
	cache.put([]string{"new"}, existingBooking(), "fake", 0)

	if _, ok := cache.entries["abandoned"]; ok {
		t.Error("an expired entry survived the next put")
	}
}

func TestTakingOneAliasRemovesTheOthers(t *testing.T) {
	// The preview is stored under the key it was asked with and under the one the
	// saved row will have; applying it through both would write the itinerary twice.
	cache, _ := newTestPreviewCache()
	cache.put([]string{"asked", "saved"}, existingBooking(), "fake", 0)

	if _, ok := cache.take("saved"); !ok {
		t.Fatal("the saved-row alias was not found")
	}
	if _, ok := cache.take("asked"); ok {
		t.Error("the other alias still answered after the entry was taken")
	}
}

func TestANewerPreviewReplacesTheOlderOneWithItsAliases(t *testing.T) {
	cache, _ := newTestPreviewCache()
	older := existingBooking()
	older.TripType = booking.TripRoundTrip
	cache.put([]string{"asked", "old-alias"}, older, "old", 0)
	cache.put([]string{"asked"}, existingBooking(), "new", 0)

	if _, ok := cache.take("old-alias"); ok {
		t.Error("the older preview's alias outlived its replacement")
	}
	if entry, ok := cache.take("asked"); !ok || entry.source != "new" {
		t.Errorf("take = %+v, %v; want the newer preview", entry, ok)
	}
}

func TestTheCachedBookingIsDetachedFromTheProvidersCopy(t *testing.T) {
	// applyBooking upper-cases names and cabins in place; that must not reach a
	// booking the provider may still be holding.
	original := &booking.Booking{
		Locator:    "ABC123",
		Outbound:   []booking.Flight{{Cabin: "economy"}},
		Passengers: []booking.Passenger{{FirstName: "maria", LastName: "silva"}},
	}
	cache, _ := newTestPreviewCache()
	cache.put([]string{"key"}, original, "fake", 0)

	entry, _ := cache.take("key")
	normaliseForStorage(entry.booking)

	if original.Outbound[0].Cabin != "economy" || original.Passengers[0].FirstName != "maria" {
		t.Errorf("normalising the cached booking rewrote the provider's: %+v", original)
	}
}

func TestPreviewKeyNormalisesWhatTheOperatorTyped(t *testing.T) {
	airlineLocator := " xyz789 "
	origin := "gru"
	surname := " da  silva "
	typed := data.Issuance{
		ProgramLocator:    " abc123",
		AirlineLocator:    &airlineLocator,
		OriginIATACode:    &origin,
		PassengerLastName: &surname,
	}

	cleanAirlineLocator := "XYZ789"
	cleanOrigin := "GRU"
	cleanSurname := "DA SILVA"
	saved := data.Issuance{
		ProgramLocator:    "ABC123",
		AirlineLocator:    &cleanAirlineLocator,
		OriginIATACode:    &cleanOrigin,
		PassengerLastName: &cleanSurname,
	}

	if previewKey("airline", typed) != previewKey("airline", saved) {
		t.Error("case and whitespace changed the key")
	}
	if previewKey("airline", saved) == previewKey("other-airline", saved) {
		t.Error("two airlines share a key")
	}

	otherOrigin := "CGH"
	saved.OriginIATACode = &otherOrigin
	if previewKey("airline", typed) == previewKey("airline", saved) {
		t.Error("a different origin produced the same key")
	}
}

func TestPreviewKeyUsesTheSurnameTheProviderIsAskedWith(t *testing.T) {
	// BookingLastName: the surname typed for the lookup wins over the passenger name,
	// and a blank one falls back to it - the same rule the request is built with.
	name := "SILVA"
	blank := "  "
	withName := data.Issuance{ProgramLocator: "ABC123", MainPassengerName: &name}
	withBlankSurname := data.Issuance{ProgramLocator: "ABC123", MainPassengerName: &name, PassengerLastName: &blank}

	if previewKey("airline", withName) != previewKey("airline", withBlankSurname) {
		t.Error("a blank surname did not fall back to the passenger name")
	}
}

func TestPreviewKeysCoverTheRowAsThePanelSavesIt(t *testing.T) {
	// The panel copies the preview's full name, origin and purchase number into the
	// draft, so the saved row is asked with different fields than the preview was.
	surname := "SILVA"
	asked := data.Issuance{ProgramLocator: "ABC123", MainPassengerName: &surname}

	fullName := "MARIA SILVA"
	origin := "GRU"
	purchase := "LA123"
	keys := previewKeys("airline", asked, &LookupPreview{
		MainPassengerName:   &fullName,
		OriginIATACode:      &origin,
		LatamPurchaseNumber: &purchase,
	})

	saved := data.Issuance{
		ProgramLocator:      "ABC123",
		MainPassengerName:   &fullName,
		OriginIATACode:      &origin,
		LatamPurchaseNumber: &purchase,
	}
	if keys[0] != previewKey("airline", asked) || keys[1] != previewKey("airline", saved) {
		t.Errorf("keys = %q, want the asked row's and the saved row's", keys)
	}
}

// The DB-backed half: the provider is asked once across preview and save, and the
// sync still leaves the same trail it would have left by asking.

func TestSyncReusesTheBookingThePreviewFetched(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		provider := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}
		registry := booking.NewRegistry(provider)
		cache, _ := newTestPreviewCache()

		input := f.input()
		if _, err := lookup(context.Background(), tx, registry,
			LookupInput{AirlineID: f.Airline.ID, ProgramLocator: input.ProgramLocator}, cache); err != nil {
			t.Fatalf("lookup: %v", err)
		}
		created, err := Create(tx, input, f.User.ID)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		// force=true, as the panel's sweep sends it.
		if err := syncIssuance(context.Background(), tx, registry, created.ID, true, cache); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if provider.fetchCalls != 1 {
			t.Errorf("the provider was asked %d times across preview and sync, want 1", provider.fetchCalls)
		}

		var issuance data.Issuance
		if err := tx.First(&issuance, "id = ?", created.ID).Error; err != nil {
			t.Fatalf("reloading the issuance: %v", err)
		}
		if issuance.BookingSource == nil || *issuance.BookingSource != "fake" {
			t.Errorf("booking_source = %v, want the provider the preview asked", issuance.BookingSource)
		}
		if issuance.TraveosSyncedAt == nil || issuance.TripType == nil || *issuance.TripType != booking.TripOneWay {
			t.Errorf("synced_at = %v, trip_type = %v: the reused booking was not applied",
				issuance.TraveosSyncedAt, issuance.TripType)
		}

		var attempts []data.BookingLookupAttempt
		if err := tx.Where("issuance_id = ?", created.ID).Find(&attempts).Error; err != nil {
			t.Fatalf("reading the attempts: %v", err)
		}
		if len(attempts) != 1 || attempts[0].Provider != "fake" || attempts[0].Outcome != data.LookupFound ||
			attempts[0].SyncID == "" {
			t.Errorf("attempts = %+v, want one found attempt for the provider that answered", attempts)
		}

		// Taken, not read: a refetch afterwards goes to the provider.
		if err := syncIssuance(context.Background(), tx, registry, created.ID, true, cache); err != nil {
			t.Fatalf("second Sync: %v", err)
		}
		if provider.fetchCalls != 2 {
			t.Errorf("a refetch after the reuse asked the provider %d times in total, want 2", provider.fetchCalls)
		}
	})
}

func TestSyncAsksAgainOnceThePreviewExpired(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		provider := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}
		registry := booking.NewRegistry(provider)
		cache, clock := newTestPreviewCache()

		input := f.input()
		if _, err := lookup(context.Background(), tx, registry,
			LookupInput{AirlineID: f.Airline.ID, ProgramLocator: input.ProgramLocator}, cache); err != nil {
			t.Fatalf("lookup: %v", err)
		}
		created, err := Create(tx, input, f.User.ID)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		clock.current = clock.current.Add(previewBookingTTL + time.Minute)
		if err := syncIssuance(context.Background(), tx, registry, created.ID, true, cache); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if provider.fetchCalls != 2 {
			t.Errorf("the provider was asked %d times, want 2 - the preview had expired", provider.fetchCalls)
		}
	})
}

func TestAFailedPreviewIsNotCached(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		provider := &fakeProvider{fetchErr: booking.Unavailable(nil, "the vendor is down")}
		cache, _ := newTestPreviewCache()

		if _, err := lookup(context.Background(), tx, booking.NewRegistry(provider),
			LookupInput{AirlineID: f.Airline.ID, ProgramLocator: "ABC123"}, cache); err == nil {
			t.Fatal("lookup: expected the provider failure")
		}
		if len(cache.entries) != 0 {
			t.Errorf("%d entries cached from a failed preview, want none", len(cache.entries))
		}
	})
}
