package booking

import (
	"context"
	"sort"
	"strings"
)

// Request is everything any provider is allowed to ask for in order to find a
// booking. It is the union of what the observed endpoints need, not the
// intersection: the Azul endpoint keys on locator plus origin, GOL wants all
// three of locator, origin and surname, LATAM takes the purchase number and the
// surname and ignores the origin entirely.
//
// Every provider therefore reads only the fields it needs and refuses early -
// with its own error - when one of them is empty. Nothing validates this
// centrally, because "required" is a fact about a carrier, not about the type.
type Request struct {
	// AirlineCode is our own airlines.code, uppercase. It is what selected the
	// provider, and it is passed along so one provider can serve several carriers
	// - the GOL endpoint answers for bookings sold as Smiles too.
	AirlineCode string

	Locator        string
	OriginIATA     string
	LastName       string
	PurchaseNumber string

	// VendorAirlineID is the provider's own numeric id for this carrier, for the
	// one provider that has such a thing: Traveos routes by an integer from its
	// own catalog, mirrored in airlines.traveos_airline_id. It is nil for every
	// direct integration, which reaches the airline by talking to it.
	VendorAirlineID *int

	// Force is the operator pressing "refetch" on purpose, and it means "the
	// flight changed", not "this failed, try again". A provider that keeps a
	// snapshot of its own - again, only Traveos - re-checks it against the
	// airline first instead of answering from that snapshot.
	Force bool
}

// Provider is one source of bookings: the Traveos adapter, or one airline's own
// endpoint.
//
// Fetch is read-only by contract. Traveos has a write side - its emission
// imports a booking into the vendor so their scraper can read it - and that
// stays inside the Traveos adapter rather than becoming a method here, because
// no direct integration has an equivalent and a port whose second method only
// one implementation can honour is not a port.
//
// Fetch returns ErrNotFound or ErrUnavailable for every failure that is not a
// programming error; see errors.go for why that split carries the design.
type Provider interface {
	// Name identifies the provider in logs and in issuances.booking_source. It is
	// short and stable ("traveos", "azul"): it is written to a column and read
	// back to answer "was the direct integration actually carrying this?".
	Name() string

	Fetch(ctx context.Context, req Request) (*Booking, error)
}

// Importer is the other optional half, and it exists for exactly one provider.
//
// Traveos does not read the airline on demand: a booking has to be imported into
// the vendor first, so their scraper can go and fetch it, and only then can it be
// read back. Every direct integration asks the airline itself and has nothing to
// import, so this is deliberately not a method on Provider - a port whose second
// method only one implementation can honour is not a port, and when Traveos is
// finally switched off this interface and its one caller disappear together.
//
// EnsureAvailable is not idempotent for Traveos and the caller is responsible
// for only reaching it after a Fetch came back ErrNotFound: issuing the same
// locator twice creates two records on the vendor's side, with no idempotency
// key to fall back on.
type Importer interface {
	EnsureAvailable(ctx context.Context, req Request) error
}

// Prober is the optional half of a provider: a cheap call that says whether the
// carrier is answering right now, without a real booking in hand.
//
// It is separate from Provider because Traveos does not need it - it has a
// catalog endpoint the health package already probes - while a direct
// integration has no such thing. The implementations send a deliberately
// invalid locator and let the airline reject it: a rejection is a healthy
// airline, and it is the one probe available that costs nobody a real lookup.
//
// A Probe returns nil or ErrNotFound when the carrier answered, and
// ErrUnavailable when it did not.
type Prober interface {
	Probe(ctx context.Context) error
}

// Registry is the boot-time wiring of which provider serves which airline, plus
// the fallback that catches everything else.
//
// It is populated once in cmd/api and cmd/bot and only read afterwards, so it
// carries no lock: a map that is never written after startup is safe for
// concurrent reads, and a mutex here would suggest a mutability that does not
// exist.
type Registry struct {
	direct   map[string]Provider
	fallback Provider
	observer func(providerName string, reached bool)

	// directNames is the set of names in direct, which is not the same as its
	// keys: those are airline codes, and one provider can serve several of them -
	// the GOL endpoint answers for bookings sold as Smiles too.
	directNames map[string]bool
}

// NewRegistry returns a registry whose fallback is used whenever no direct
// provider is registered for an airline, or whenever a direct one could not be
// reached. fallback may be nil - a server started without Traveos credentials is
// a supported state, and so is one where Traveos has been switched off for good.
func NewRegistry(fallback Provider) *Registry {
	return &Registry{direct: map[string]Provider{}, directNames: map[string]bool{}, fallback: fallback}
}

// Register wires a direct provider to an airline code. Calling it twice for the
// same code replaces the first, which only happens by mistake at boot.
func (r *Registry) Register(airlineCode string, provider Provider) {
	r.direct[normaliseCode(airlineCode)] = provider
	r.directNames[provider.Name()] = true
}

// Direct returns the provider registered for an airline, if any.
//
// Being registered is not the same as being switched on: airlines.
// direct_booking_enabled is what decides whether the sync actually tries it, and
// that check belongs to the caller. Keeping them apart is what lets a carrier be
// deployed dark and enabled later from the panel, with no release in between.
func (r *Registry) Direct(airlineCode string) (Provider, bool) {
	provider, ok := r.direct[normaliseCode(airlineCode)]
	return provider, ok
}

// Fallback returns the provider used when no direct one applies. It may be nil.
func (r *Registry) Fallback() Provider { return r.fallback }

// Watch installs a callback that every real lookup reports its outcome to.
//
// It is how the panel's lights stay honest without a ticker fast enough to be
// rude: a carrier that is being used all day is answering real questions every few
// minutes, and those are a better signal than a synthetic probe on a slow loop.
// Installed once at boot, like everything else here, and optional - a registry
// nobody watches simply reports to nobody.
func (r *Registry) Watch(observer func(providerName string, reached bool)) {
	r.observer = observer
}

// Observed reports one real lookup's outcome to whoever is watching.
//
// Only the direct integrations are reported. The fallback has a light of its own
// in the panel and a probe of its own in cmd/api, so feeding it in here too would
// put it in the airlines list a second time and make "2 of 3 down" count one
// dependency twice.
//
// reached is derived here rather than by the caller so that "the airline said no"
// keeps counting as a healthy airline in exactly one place.
func (r *Registry) Observed(providerName string, err error) {
	if r.observer == nil || !r.directNames[providerName] {
		return
	}
	r.observer(providerName, Reached(err))
}

// Empty reports whether this server can fetch an itinerary at all: no vendor
// credentials and no direct integration wired.
//
// It is a supported state, not a broken one - the panel boots, issuances are
// registered, and only the routes that need an itinerary refuse. Worth asking
// distinctly, because "nobody configured this" and "this airline is not
// integrated" send an operator to two different places.
func (r *Registry) Empty() bool { return r.fallback == nil && len(r.direct) == 0 }

// Codes lists the airline codes that have a direct provider, sorted so the
// panel's list and the monitor's goroutines come up in a stable order.
func (r *Registry) Codes() []string {
	codes := make([]string, 0, len(r.direct))
	for code := range r.direct {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func normaliseCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
