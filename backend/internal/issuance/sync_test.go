package issuance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/traveos"
	"gorm.io/gorm"
)

// fakeProvider records what the sync asked of the provider. The interesting
// assertions in this file are all about *call counts*: for the one provider that
// has an import, importing the same locator twice creates two real records on the
// vendor's side, with no idempotency key and no server-side duplicate check to
// save us, so "did we import?" is the property worth pinning down.
type fakeProvider struct {
	// fetchResults is consumed one per Fetch call; a nil entry means the provider
	// answered that it has no such booking.
	fetchResults []*booking.Booking
	fetchErr     error
	importErr    error

	fetchCalls  int
	importCalls int
	requested   booking.Request

	// Runs at the start of Fetch, i.e. while the sync is in flight - the only
	// moment from which the syncing marker can be observed.
	duringCall func()
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Fetch(_ context.Context, req booking.Request) (*booking.Booking, error) {
	f.fetchCalls++
	f.requested = req
	if f.duringCall != nil {
		f.duringCall()
	}
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}

	var result *booking.Booking
	if len(f.fetchResults) > 0 {
		result = f.fetchResults[0]
		if len(f.fetchResults) > 1 {
			f.fetchResults = f.fetchResults[1:]
		}
	}
	if result == nil {
		return nil, booking.ErrNotFound
	}
	return result, nil
}

func (f *fakeProvider) EnsureAvailable(_ context.Context, req booking.Request) error {
	f.importCalls++
	f.requested = req
	return f.importErr
}

// readOnlyProvider is a direct integration: it reads the airline and has nothing
// to import, which is the shape every carrier after Traveos has.
type readOnlyProvider struct{ fetchCalls int }

func (r *readOnlyProvider) Name() string { return "read-only" }

func (r *readOnlyProvider) Fetch(context.Context, booking.Request) (*booking.Booking, error) {
	r.fetchCalls++
	return nil, booking.ErrNotFound
}

func testAirline() data.Airline {
	traveosID := 90
	return data.Airline{
		ID:               "airline-1",
		Name:             "AMERICAN AIRLINES",
		Code:             "AA",
		TraveosSupported: true,
		TraveosAirlineID: &traveosID,
	}
}

func testIssuance() data.Issuance {
	origin := "GRU"
	lastName := "PRADO FERREIRA"
	return data.Issuance{
		ID:                "issuance-1",
		ProgramLocator:    "ABC123",
		OriginIATACode:    &origin,
		MainPassengerName: &lastName,
	}
}

func existingBooking() *booking.Booking {
	return &booking.Booking{Locator: "ABC123", TripType: booking.TripOneWay}
}

func TestFetchOrImportDoesNotImportWhenTheBookingIsAlreadyThere(t *testing.T) {
	// The whole reason the search comes first. Without this, a refetch after a
	// failed read would issue a second ticket on the vendor's side, and nothing on
	// their end would stop it.
	provider := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}

	fetched, err := fetchOrImport(context.Background(), provider, testIssuance(), testAirline(), false)
	if err != nil {
		t.Fatalf("fetchOrImport: %v", err)
	}
	if fetched.Locator != "ABC123" {
		t.Errorf("locator = %q, want ABC123", fetched.Locator)
	}
	if provider.importCalls != 0 {
		t.Errorf("imported %d times for a booking that already exists, want 0 - this is a duplicate on the vendor's side",
			provider.importCalls)
	}
	if provider.fetchCalls != 1 {
		t.Errorf("fetched %d times, want 1", provider.fetchCalls)
	}
}

func TestFetchOrImportStopsAtAbsenceForAProviderWithNoImport(t *testing.T) {
	// A direct integration asks the airline itself, so "no such booking" is the
	// airline's own answer and there is nothing left to try.
	provider := &readOnlyProvider{}

	_, err := fetchOrImport(context.Background(), provider, testIssuance(), testAirline(), false)
	if !errors.Is(err, booking.ErrNotFound) {
		t.Fatalf("err = %v, want booking.ErrNotFound", err)
	}
	if provider.fetchCalls != 1 {
		t.Errorf("fetched %d times, want 1 - there is no second attempt without an import", provider.fetchCalls)
	}
}

func TestFetchOrImportNeverImportsForAnImportedIssuance(t *testing.T) {
	// The one that guards real money. An imported issuance describes a booking made
	// on the spreadsheet months ago, so "not at the vendor" means the vendor never
	// saw it - not that it needs issuing. Importing here would buy a second ticket
	// for a flight that has already departed.
	provider := &fakeProvider{}
	issuance := testIssuance()
	issuance.Imported = true

	_, err := fetchOrImport(context.Background(), provider, issuance, testAirline(), false)
	if !errors.Is(err, ErrImportedNotAtVendor) {
		t.Fatalf("err = %v, want ErrImportedNotAtVendor", err)
	}
	if provider.importCalls != 0 {
		t.Errorf("imported %d times for an imported issuance, want 0 - that is a real ticket bought for a past flight",
			provider.importCalls)
	}
	if provider.fetchCalls != 1 {
		t.Errorf("fetched %d times, want 1 - searching is the whole of what an imported row may do",
			provider.fetchCalls)
	}
}

func TestFetchOrImportReadsAnImportedIssuanceTheVendorAlreadyHas(t *testing.T) {
	// The refusal above is about the import, not about the lookup: when the vendor
	// does have the booking, an imported issuance is filled in like any other.
	provider := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}
	issuance := testIssuance()
	issuance.Imported = true

	fetched, err := fetchOrImport(context.Background(), provider, issuance, testAirline(), false)
	if err != nil {
		t.Fatalf("fetchOrImport: %v", err)
	}
	if fetched.Locator != "ABC123" {
		t.Errorf("locator = %q, want ABC123", fetched.Locator)
	}
	if provider.importCalls != 0 {
		t.Errorf("imported %d times, want 0", provider.importCalls)
	}
}

func TestFetchOrImportImportsThenFetchesAgainWhenTheBookingIsAbsent(t *testing.T) {
	// The import response carries no ticket code at all, so the second fetch is the
	// only way to learn what was created.
	provider := &fakeProvider{fetchResults: []*booking.Booking{nil, existingBooking()}}

	fetched, err := fetchOrImport(context.Background(), provider, testIssuance(), testAirline(), false)
	if err != nil {
		t.Fatalf("fetchOrImport: %v", err)
	}
	if fetched.Locator != "ABC123" {
		t.Errorf("locator = %q, want ABC123", fetched.Locator)
	}
	if provider.importCalls != 1 {
		t.Errorf("imported %d times, want exactly 1", provider.importCalls)
	}
	if provider.fetchCalls != 2 {
		t.Errorf("fetched %d times, want 2 (before and after importing)", provider.fetchCalls)
	}
}

func TestFetchOrImportPassesTheAirlineIDAndTheOperatorsFields(t *testing.T) {
	provider := &fakeProvider{fetchResults: []*booking.Booking{nil, existingBooking()}}

	if _, err := fetchOrImport(context.Background(), provider, testIssuance(), testAirline(), false); err != nil {
		t.Fatalf("fetchOrImport: %v", err)
	}

	if provider.requested.VendorAirlineID == nil || *provider.requested.VendorAirlineID != 90 {
		t.Errorf("vendor airline id = %v, want 90 from the airline's traveos_airline_id", provider.requested.VendorAirlineID)
	}
	if provider.requested.AirlineCode != "AA" {
		t.Errorf("airline code = %q, want AA - it is what selects the provider", provider.requested.AirlineCode)
	}
	if provider.requested.Locator != "ABC123" {
		t.Errorf("locator = %q, want ABC123", provider.requested.Locator)
	}
	// Sent verbatim: there is no way to tell a compound surname from a given name
	// plus surname, and stripping the wrong token fails the scrape with the same
	// opaque 500 as everything else.
	if provider.requested.LastName != "PRADO FERREIRA" {
		t.Errorf("lastName = %q, want the operator's value unchanged", provider.requested.LastName)
	}
	if provider.requested.OriginIATA != "GRU" {
		t.Errorf("origin = %q, want GRU", provider.requested.OriginIATA)
	}
}

func TestFetchOrImportPrefersTheAirlineLocator(t *testing.T) {
	provider := &fakeProvider{fetchResults: []*booking.Booking{nil, existingBooking()}}
	issuance := testIssuance()
	airlinePNR := "XYZ789"
	issuance.AirlineLocator = &airlinePNR

	if _, err := fetchOrImport(context.Background(), provider, issuance, testAirline(), false); err != nil {
		t.Fatalf("fetchOrImport: %v", err)
	}
	if provider.requested.Locator != "XYZ789" {
		t.Errorf("locator = %q, want the airline PNR XYZ789 - every provider here reaches the airline, not the program",
			provider.requested.Locator)
	}
}

func TestForceIsPassedToTheProvider(t *testing.T) {
	// What force means is the provider's business - Traveos re-checks its own
	// snapshot against the airline, a direct integration has no snapshot to
	// re-check. The sync's job is only to carry the intent across.
	provider := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}

	if _, err := fetchOrImport(context.Background(), provider, testIssuance(), testAirline(), true); err != nil {
		t.Fatalf("fetchOrImport: %v", err)
	}
	if !provider.requested.Force {
		t.Error("force did not reach the provider")
	}
	if provider.importCalls != 0 {
		t.Error("a forced refetch imported a ticket; force means refresh, never re-issue")
	}
}

func TestFetchOrImportPropagatesAnImportFailureUnclassified(t *testing.T) {
	// Every Traveos import failure but one arrives as the same opaque 500, so the
	// sync hands the vendor's sentinel straight through rather than reinterpreting it.
	provider := &fakeProvider{
		fetchResults: []*booking.Booking{nil},
		importErr:    traveos.ErrEmissionFailed,
	}

	_, err := fetchOrImport(context.Background(), provider, testIssuance(), testAirline(), false)
	if !errors.Is(err, traveos.ErrEmissionFailed) {
		t.Fatalf("err = %v, want the vendor's ErrEmissionFailed", err)
	}
	// And it must not have fetched again after a failure: there is nothing to find.
	if provider.fetchCalls != 1 {
		t.Errorf("fetched %d times after a failed import, want 1", provider.fetchCalls)
	}
}

func TestFetchOrImportFlagsASuccessfulImportThatVanished(t *testing.T) {
	// The vendor said it worked but the booking is not in its own list. A record may
	// well exist on their side, so this gets its own error instead of looking like
	// an ordinary failure that invites an immediate retry.
	provider := &fakeProvider{fetchResults: []*booking.Booking{nil}}

	_, err := fetchOrImport(context.Background(), provider, testIssuance(), testAirline(), false)
	if !errors.Is(err, ErrTicketNotFoundAfterEmission) {
		t.Fatalf("err = %v, want ErrTicketNotFoundAfterEmission", err)
	}
	if errors.Is(err, traveos.ErrEmissionFailed) {
		t.Error("a vanished ticket was reported as a failed import, which would invite a duplicating retry")
	}
}

func TestFetchOrImportRefusesToImportWithoutTheAirlinesRequiredFields(t *testing.T) {
	// The expensive failure this prevents: the import is not idempotent, so a call
	// made with an empty purchase number creates a real record on the vendor's side
	// and then answers the same opaque 500 as everything else.
	provider := &fakeProvider{fetchResults: []*booking.Booking{nil}}
	airline := testAirline()
	airline.RequiresPurchaseNumber = true

	_, err := fetchOrImport(context.Background(), provider, testIssuance(), airline, false)
	if !errors.Is(err, ErrPurchaseNumberRequired) {
		t.Fatalf("err = %v, want ErrPurchaseNumberRequired", err)
	}
	if provider.importCalls != 0 {
		t.Errorf("imported %d times without the purchase number the airline requires, want 0", provider.importCalls)
	}
}

func TestFetchOrImportStillReadsAnExistingBookingWithoutThoseFields(t *testing.T) {
	// The counterpart: those fields exist to *create* the ticket at the vendor. One
	// that is already there is found by locator alone, so refusing here would block a
	// refetch that costs nothing and would have worked.
	provider := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}
	airline := testAirline()
	airline.RequiresPurchaseNumber = true

	fetched, err := fetchOrImport(context.Background(), provider, testIssuance(), airline, false)
	if err != nil {
		t.Fatalf("fetchOrImport: %v", err)
	}
	if fetched.Locator != "ABC123" {
		t.Errorf("locator = %q, want ABC123", fetched.Locator)
	}
}

func TestFetchOrImportRefusesABlankLocatorBeforeTouchingTheProvider(t *testing.T) {
	// Unreachable in practice - program_locator is NOT NULL - but a blank string
	// would make us search for nothing and then issue a ticket with an empty
	// locator, which is a real record on the vendor's side.
	provider := &fakeProvider{}
	issuance := testIssuance()
	issuance.ProgramLocator = "   "

	_, err := fetchOrImport(context.Background(), provider, issuance, testAirline(), false)
	if !errors.Is(err, ErrLocatorMissing) {
		t.Fatalf("err = %v, want ErrLocatorMissing", err)
	}
	if provider.fetchCalls != 0 || provider.importCalls != 0 {
		t.Error("a blank locator reached the provider")
	}
}

// syncingMarkerOf reads the row straight from the database, bypassing whatever the
// sync holds in memory.
func syncingMarkerOf(t *testing.T, tx *gorm.DB, issuanceID string) *time.Time {
	t.Helper()

	var issuance data.Issuance
	if err := tx.First(&issuance, "id = ?", issuanceID).Error; err != nil {
		t.Fatalf("reloading the issuance: %v", err)
	}
	return issuance.TraveosSyncingAt
}

func TestSyncMarksTheRowWhileItRunsAndClearsItOnFailure(t *testing.T) {
	// The marker is what every other panel reads to show the row as busy, so the
	// two properties that matter are that it is set *before* the provider is called
	// and gone afterwards - including when the call blows up, which is the case
	// that would otherwise leave the row locked for everyone.
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		var markedDuringCall bool
		provider := &fakeProvider{
			fetchErr: booking.Unavailable(nil, "the vendor is down"),
			duringCall: func() {
				markedDuringCall = syncingMarkerOf(t, tx, issuance.ID) != nil
			},
		}

		if err := Sync(context.Background(), tx, booking.NewRegistry(provider), issuance.ID, true); err == nil {
			t.Fatal("Sync: expected the provider failure to be reported")
		}

		if !markedDuringCall {
			t.Error("the issuance was not marked as syncing while the provider was being called")
		}
		if marker := syncingMarkerOf(t, tx, issuance.ID); marker != nil {
			t.Errorf("the syncing marker survived a failed sync: %v", marker)
		}
	})
}

func TestSyncDoesNotMarkAnAirlineItRefuses(t *testing.T) {
	// The refusal happens before the provider is involved, so there is nothing to
	// report as busy - and a marker left here would lock the row on every screen
	// for the three minutes it takes to go stale.
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{TraveosSupported: false})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		provider := &fakeProvider{}
		if err := Sync(context.Background(), tx, booking.NewRegistry(provider), issuance.ID, true); !errors.Is(err, ErrAirlineNotSupported) {
			t.Fatalf("Sync: %v, want ErrAirlineNotSupported", err)
		}

		if marker := syncingMarkerOf(t, tx, issuance.ID); marker != nil {
			t.Errorf("an airline that is not integrated marked the row as syncing: %v", marker)
		}
		if provider.fetchCalls != 0 {
			t.Errorf("called the provider %d times for an airline it refuses", provider.fetchCalls)
		}
	})
}

func TestResolveProvidersAsksTheAirlineBeforeTheVendor(t *testing.T) {
	direct := &readOnlyProvider{}
	fallback := &fakeProvider{}
	registry := booking.NewRegistry(fallback)
	registry.Register("AA", direct)

	airline := testAirline()
	airline.DirectBookingEnabled = true

	providers := resolveProviders(registry, airline)
	if len(providers) != 2 {
		t.Fatalf("providers = %d, want the airline and the vendor behind it", len(providers))
	}
	if providers[0] != booking.Provider(direct) {
		t.Error("the vendor was asked before the airline's own integration")
	}
}

func TestResolveProvidersIgnoresARegisteredButUnswitchedIntegration(t *testing.T) {
	// Registered and enabled are two different things: a carrier ships dark and is
	// switched on from the airline screen, so a deploy alone must change nothing.
	registry := booking.NewRegistry(&fakeProvider{})
	registry.Register("AA", &readOnlyProvider{})

	providers := resolveProviders(registry, testAirline()) // DirectBookingEnabled is false
	if len(providers) != 1 {
		t.Fatalf("providers = %d, want only the vendor", len(providers))
	}
}

func TestResolveProvidersDropsTheVendorForAnUnsupportedAirline(t *testing.T) {
	registry := booking.NewRegistry(&fakeProvider{})
	registry.Register("AA", &readOnlyProvider{})

	airline := testAirline()
	airline.DirectBookingEnabled = true
	airline.TraveosSupported = false

	providers := resolveProviders(registry, airline)
	if len(providers) != 1 {
		t.Fatalf("providers = %d, want only the airline's own integration", len(providers))
	}
}

func TestAnUnreachableAirlineFallsBackToTheVendor(t *testing.T) {
	direct := &fakeProvider{fetchErr: booking.Unavailable(nil, "edge returned 403")}
	fallback := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}

	registry := booking.NewRegistry(fallback)
	fetched, source, err := attemptProviders(context.Background(), registry,
		[]booking.Provider{direct, fallback}, testIssuance(), testAirline(), false)
	if err != nil {
		t.Fatalf("attemptProviders: %v", err)
	}
	if fetched.Locator != "ABC123" {
		t.Errorf("locator = %q, want the fallback's answer", fetched.Locator)
	}
	if source != "fake" {
		t.Errorf("source = %q, want the provider that actually answered", source)
	}
	if fallback.fetchCalls != 1 {
		t.Errorf("the fallback was called %d times, want 1", fallback.fetchCalls)
	}
}

func TestAnAbsentBookingDoesNotReachTheVendor(t *testing.T) {
	// The one that guards real money. "The airline says there is no such booking"
	// handed to Traveos is not a second opinion - it is an import, and the import
	// creates a real, non-idempotent ticket for a locator that does not exist.
	direct := &readOnlyProvider{}
	fallback := &fakeProvider{}

	_, _, err := attemptProviders(context.Background(), booking.NewRegistry(fallback),
		[]booking.Provider{direct, fallback}, testIssuance(), testAirline(), false)
	if !errors.Is(err, booking.ErrNotFound) {
		t.Fatalf("err = %v, want booking.ErrNotFound", err)
	}
	if fallback.fetchCalls != 0 || fallback.importCalls != 0 {
		t.Errorf("the vendor was reached after the airline said the booking does not exist: %d fetches, %d imports",
			fallback.fetchCalls, fallback.importCalls)
	}
}

func TestARefusalIsNotRetriedAgainstTheNextProvider(t *testing.T) {
	// An imported row, a missing required field: refusals this layer made on
	// purpose. Asking the next provider would produce the same refusal twice.
	direct := &fakeProvider{}
	fallback := &fakeProvider{}
	issuance := testIssuance()
	issuance.Imported = true

	_, _, err := attemptProviders(context.Background(), booking.NewRegistry(fallback),
		[]booking.Provider{direct, fallback}, issuance, testAirline(), false)
	if !errors.Is(err, ErrImportedNotAtVendor) {
		t.Fatalf("err = %v, want ErrImportedNotAtVendor", err)
	}
	if fallback.fetchCalls != 0 {
		t.Errorf("the refusal was retried against the vendor %d times", fallback.fetchCalls)
	}
}

func TestSyncRecordsWhichProviderAnswered(t *testing.T) {
	// Without this column, an integration that has been silently falling back to
	// the vendor on every lookup is indistinguishable from one that works.
	runInTx(t, func(tx *gorm.DB) {
		traveosID := 90
		f := seedFixture(t, tx, data.Airline{TraveosSupported: true, TraveosAirlineID: &traveosID})
		created, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		provider := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}
		if err := Sync(context.Background(), tx, booking.NewRegistry(provider), created.ID, true); err != nil {
			t.Fatalf("Sync: %v", err)
		}

		var issuance data.Issuance
		if err := tx.First(&issuance, "id = ?", created.ID).Error; err != nil {
			t.Fatalf("reloading the issuance: %v", err)
		}
		if issuance.BookingSource == nil || *issuance.BookingSource != "fake" {
			t.Errorf("booking_source = %v, want the provider that answered", issuance.BookingSource)
		}
	})
}

func TestEveryRealLookupIsReportedToTheWatcher(t *testing.T) {
	// The panel's freshest evidence. Both outcomes are reported, and an airline that
	// answered "no such booking" counts as reached: it is the edge blocking us, not
	// the refusal, that means the carrier is down.
	direct := &fakeProvider{fetchErr: booking.Unavailable(nil, "edge returned 403")}
	fallback := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}

	observed := map[string]bool{}
	registry := booking.NewRegistry(fallback)
	registry.Watch(func(name string, reached bool) { observed[name] = reached })

	// Two providers with the same Name would collapse in the map, so the direct one
	// is given its own identity for this test.
	azul := &namedProvider{name: "azul", inner: direct}
	registry.Register("AZUL", azul)

	if _, _, err := attemptProviders(context.Background(), registry,
		[]booking.Provider{azul, fallback}, testIssuance(), testAirline(), false); err != nil {
		t.Fatalf("attemptProviders: %v", err)
	}

	if reached, ok := observed["azul"]; !ok || reached {
		t.Errorf("the unreachable provider was reported as %v (seen: %v), want reached=false", reached, ok)
	}

	// The fallback is deliberately absent: it has a light of its own in the panel,
	// and reporting it here too would list it a second time under the airlines and
	// make "1 of 3 down" count one dependency twice.
	if _, ok := observed["fake"]; ok {
		t.Error("the fallback was reported as an airline integration")
	}
}

func TestAReachedDirectProviderIsReportedAsHealthy(t *testing.T) {
	answering := &fakeProvider{fetchResults: []*booking.Booking{existingBooking()}}
	azul := &namedProvider{name: "azul", inner: answering}

	observed := map[string]bool{}
	registry := booking.NewRegistry(nil)
	registry.Register("AZUL", azul)
	registry.Watch(func(name string, reached bool) { observed[name] = reached })

	if _, _, err := attemptProviders(context.Background(), registry,
		[]booking.Provider{azul}, testIssuance(), testAirline(), false); err != nil {
		t.Fatalf("attemptProviders: %v", err)
	}

	if reached, ok := observed["azul"]; !ok || !reached {
		t.Errorf("the provider that answered was reported as %v (seen: %v), want reached=true", reached, ok)
	}
}

// namedProvider renames another provider, so a test can tell two of them apart.
type namedProvider struct {
	name  string
	inner booking.Provider
}

func (n *namedProvider) Name() string { return n.name }

func (n *namedProvider) Fetch(ctx context.Context, req booking.Request) (*booking.Booking, error) {
	return n.inner.Fetch(ctx, req)
}
