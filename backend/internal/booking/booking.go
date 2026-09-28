// Package booking is the vendor-neutral shape of an airline reservation, plus
// the port every source of one has to implement.
//
// It exists because the M-80 is moving off Traveos and onto the airlines' own
// endpoints, one carrier at a time. Without a shape of our own, every carrier
// added would mean another branch inside internal/issuance, and Traveos'
// vocabulary - "ida"/"volta", `tripType: "somente_ida"`, a numeric airlineId -
// would keep leaking into code that has nothing to do with that vendor.
//
// # What this type deliberately does not carry
//
// The fields here are the ones the passenger's ticket needs, which is the only
// thing the itinerary is fetched for: internal/ticket.ViewModel is the list, and
// it wants the locator, the legs, and per passenger the name, the seat per leg
// and the baggage allowances. Fares, payments, contacts, ticket numbers, tax
// breakdowns and loyalty tiers all come back from most of these endpoints and
// none of them are modelled, because nothing on our side reads them and
// persisting a passenger's payment details to render a boarding document would
// be collecting data we have no use for.
//
// Document is the one exception, and it is not for the PDF: it is what
// issuance.registerBeneficiaries charges against the account's beneficiary cap.
package booking

import (
	"encoding/json"
	"time"
)

// Trip types, matching the trip_type enum in the database. They are here rather
// than as free strings because every provider has to map its own vocabulary onto
// exactly these three, and a typo in a mapper should not reach a CHECK
// constraint at insert time.
const (
	TripOneWay    = "one_way"
	TripRoundTrip = "round_trip"
	TripMultiLeg  = "multi_leg"
)

// Passenger types, matching the passenger_type enum. Same reasoning.
const (
	PassengerAdult  = "ADT"
	PassengerChild  = "CHD"
	PassengerInfant = "INF"
)

// Booking is one reservation as every provider must describe it.
//
// The outbound/inbound split is part of the type rather than a `Direction` field
// on each leg because issuance_flights.direction is NOT NULL and the ordering
// within each direction is load-bearing - it is what the seat arrays are aligned
// with. A provider that cannot tell the two apart puts everything in Outbound.
type Booking struct {
	// Locator is the airline's own PNR. PurchaseNumber is the order number some
	// carriers key their reservation on instead (LATAM), and is empty elsewhere.
	Locator        string
	PurchaseNumber string

	// TripType is one of the Trip* constants.
	TripType string

	Outbound []Flight
	Inbound  []Flight

	Passengers []Passenger

	// BrandColor is an optional "#RRGGBB" the provider happened to know. It only
	// ever backfills an airline that has none yet; it never overwrites one.
	BrandColor string

	// AdditionalServices is stored verbatim in issuances.additional_services.
	// Every carrier shapes it differently - assistance slots, paid extras, an
	// empty array - and nothing reads it structurally, so normalising it here
	// would be inventing a schema none of them agreed to.
	AdditionalServices json.RawMessage
}

// Flight is one leg.
type Flight struct {
	Number string

	// Cabin is free text and arrives dirty across every vendor observed
	// ("Economy", "Econômica", and Traveos' own broken escape "Econu00f4mica").
	// It is displayed, never switched on.
	Cabin string

	OriginIATA      string
	DestinationIATA string

	// DepartureAt is required: issuance_flights.departure_at is NOT NULL, and a
	// leg nobody can say the departure of is not a leg anyone can travel on.
	DepartureAt time.Time

	ArrivalAt   *time.Time
	DurationMin *int
}

// Passenger is one person on the booking.
//
// Seats and Baggage are split by direction and shaped differently on purpose,
// which is the trap worth restating here because every new provider walks into
// it: Seats IS positionally aligned with the legs of its direction (one entry
// per leg, nil where unassigned), and Baggage is NOT - its entries are
// entitlement classes (carry-on, checked), so a one-leg ticket can carry two
// allowances and a three-leg ticket one.
type Passenger struct {
	FirstName string
	LastName  string

	// Type is one of the Passenger* constants.
	Type string

	// Document is the CPF, in the clear. It is nil whenever the provider did not
	// return one or returned it masked - several airline endpoints mask it for an
	// anonymous caller, and a masked document is worse than no document, because
	// the beneficiary cap would key on a value that is not the person's.
	Document *string

	SeatsOutbound []*string
	SeatsInbound  []*string

	BaggageOutbound []Allowance
	BaggageInbound  []Allowance
}

// Allowance is one baggage entitlement, in the provider's own array order.
//
// Weight stays a string because it is a label, not a measurement: carriers send
// "23kg", "10 kg" and "" for the same concept, and parsing it into a number
// would force a unit onto values that never declared one.
type Allowance struct {
	Quantity int
	Weight   string
}
