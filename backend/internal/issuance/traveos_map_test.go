package issuance

import (
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/traveos"
)

// These exercise the pure mapping decisions, so they run without Postgres -
// unlike the rest of this package's tests. The values are synthetic; the shapes
// are the ones production returns.

func TestCleanGivenNameStripsATrailingCourtesyTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// AMERICAN appends the honorific to every passenger's given name, which is
		// what made the original ticket page read "FERNANDA CRISTINA MS".
		{"american appends MS", "FERNANDA CRISTINA MS", "FERNANDA CRISTINA"},
		{"american appends MR", "RODRIGO MR", "RODRIGO"},
		{"with a full stop", "ISABELA MS.", "ISABELA"},
		{"lowercase", "marcelo mr", "marcelo"},
		{"portuguese honorific", "JOANA SRA", "JOANA"},

		// Nothing to strip.
		{"no title", "FERNANDA CRISTINA", "FERNANDA CRISTINA"},
		{"single token", "RODRIGO", "RODRIGO"},

		// The load-bearing refusal: a name that IS one of these words must survive,
		// because a single-token given name has no honorific to remove and eating it
		// would leave the passenger nameless.
		{"a name that is itself a title", "MS", "MS"},

		{"extra whitespace", "  RODRIGO   MR  ", "RODRIGO"},
		{"empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanGivenName(tc.input); got != tc.want {
				t.Errorf("cleanGivenName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCleanGivenNameNeverTouchesTheSurname(t *testing.T) {
	// Only the given name is cleaned, on purpose. A real surname could legitimately
	// be one of these words, and there is no signal in the data to tell that apart
	// from an honorific - so the surname is left alone and an occasional stray
	// title is accepted as the cheaper mistake.
	if got := cleanGivenName("RODRIGO"); got != "RODRIGO" {
		t.Fatalf("given name was altered: %q", got)
	}
	// Documenting the accepted limitation rather than asserting a behaviour we do
	// not have: a title in the middle is not removed.
	if got := cleanGivenName("MR RODRIGO"); got != "MR RODRIGO" {
		t.Errorf("cleanGivenName(%q) = %q; a leading title is deliberately left in place",
			"MR RODRIGO", got)
	}
}

func TestTotalBaggageSumsTheEntitlementsRatherThanIndexingByLeg(t *testing.T) {
	// The trap: bagagens entries are entitlement classes, not legs. GOL sends
	// [1, 0] for a single-leg ticket, so bagagens[legIndex] would read 1 for leg 1
	// and run off the end on a two-leg itinerary.
	tests := []struct {
		name  string
		input []traveos.BaggageAllowance
		want  int
	}{
		{"gol sends counts", []traveos.BaggageAllowance{{Quantity: 1}, {Quantity: 0}}, 1},
		{
			"avianca sends weighted allowances with no pieces",
			[]traveos.BaggageAllowance{{Quantity: 0, Weight: "10kg"}, {Quantity: 0, Weight: "23kg"}},
			0,
		},
		{
			"two checked pieces plus a carry-on",
			[]traveos.BaggageAllowance{{Quantity: 2}, {Quantity: 1}},
			3,
		},
		{"absent direction", nil, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := totalBaggage(tc.input); got != tc.want {
				t.Errorf("totalBaggage(%v) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestDeriveTripTypeMapsTheVendorVocabulary(t *testing.T) {
	tests := []struct {
		name     string
		tripType string
		inbound  int
		multiLeg bool
		want     string
	}{
		{"one way", "somente_ida", 0, false, "one_way"},
		{"round trip by tripType", "ida_e_volta", 1, false, "round_trip"},
		{
			// Belt and braces: a return leg present without tripType saying so still
			// means round trip, because issuance_flights would carry an inbound row.
			name: "round trip inferred from an inbound leg", tripType: "", inbound: 1, want: "round_trip",
		},
		{
			// Multi-leg is not a trip type at Traveos - tripType only knows one-way
			// and round-trip, and a multi-leg journey shows up in arguments instead.
			name:     "multi leg wins over one way",
			tripType: "somente_ida", multiLeg: true, want: "multi_leg",
		},
		{
			// More specific than round trip, so it wins there too.
			name:     "multi leg wins over round trip",
			tripType: "ida_e_volta", inbound: 1, multiLeg: true, want: "multi_leg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			details := &traveos.TicketData{}
			details.Emission.TripType = tc.tripType
			details.Emission.Arguments.MultiLeg = traveos.FlexibleBool(tc.multiLeg)
			details.Flights.Inbound = make([]traveos.Flight, tc.inbound)

			if got := deriveTripType(details); got != tc.want {
				t.Errorf("deriveTripType() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDeriveTripTypeIgnoresPopulatedBaggageOnAOneWayTicket(t *testing.T) {
	// GOL returns a populated bagagens.volta on a one-way ticket, whose flights
	// object has no volta at all. Only flights.volta proves a return leg exists, so
	// a passenger carrying inbound baggage data must not flip the trip type.
	var details traveos.TicketData
	details.Emission.TripType = "somente_ida"
	details.Emission.Passengers = []traveos.Passenger{{
		Baggage: traveos.Baggage{
			Outbound: []traveos.BaggageAllowance{{Quantity: 1}, {Quantity: 0}},
			Inbound:  []traveos.BaggageAllowance{{Quantity: 1}, {Quantity: 0}},
		},
	}}

	if got := deriveTripType(&details); got != "one_way" {
		t.Fatalf("deriveTripType() = %q, want one_way", got)
	}
}

func TestNormalisePassengerTypeStaysInsideTheEnum(t *testing.T) {
	// passenger_type is a Postgres enum; a value outside it aborts the insert. The
	// vendor uses the same three codes but as free text.
	for input, want := range map[string]string{
		"ADT": "ADT", "CHD": "CHD", "INF": "INF",
		"adt": "ADT", "chd": "CHD",
		" INF ": "INF",
		"":      "ADT",
		// Anything unrecognised falls back to adult rather than failing: the
		// passenger exists either way, and the type is the least important thing
		// about them.
		"SENIOR": "ADT",
	} {
		if got := normalisePassengerType(input); got != want {
			t.Errorf("normalisePassengerType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTraveosLocatorPrefersTheAirlinePNR(t *testing.T) {
	// Traveos scrapes the airline, so the airline's own PNR is the one that
	// identifies the booking there. The program locator is the loyalty program's
	// reference and is only a fallback.
	airlinePNR := "AIR123"
	blank := "   "

	tests := []struct {
		name           string
		programLocator string
		airlineLocator *string
		want           string
	}{
		{"airline locator wins", "PRG456", &airlinePNR, "AIR123"},
		{"falls back to the program locator", "PRG456", nil, "PRG456"},
		{"a blank airline locator is not a locator", "PRG456", &blank, "PRG456"},
		{"whitespace is trimmed", "  PRG456  ", nil, "PRG456"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issuance := data.Issuance{
				ProgramLocator: tc.programLocator,
				AirlineLocator: tc.airlineLocator,
			}
			if got := traveosLocator(issuance); got != tc.want {
				t.Errorf("traveosLocator() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsHexColourRejectsAnythingTheTicketCSSCannotUse(t *testing.T) {
	// The brand colour is backfilled from whatever the vendor sends, and it lands
	// in the ticket's stylesheet.
	for input, want := range map[string]bool{
		"#E30613":  true,
		"#FFFFFF":  true,
		"#ffffff":  false, // normalised to upper case before this is called
		"E30613":   false,
		"#E3061":   false,
		"#E306133": false,
		"":         false,
		"#ZZZZZZ":  false,
		"red":      false,
	} {
		if got := isHexColour(input); got != want {
			t.Errorf("isHexColour(%q) = %v, want %v", input, got, want)
		}
	}
}
