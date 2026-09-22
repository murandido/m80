package issuance

import (
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The pure mapping decisions here run without Postgres - unlike the rest of this
// package's tests. What writes rows (the seat/baggage split) needs the database,
// and takes the same skip-if-unreachable path as create_test.go. The values are
// synthetic; the shapes are the ones production returns.

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

// The trap this replaces: bagagens entries are entitlement classes, not legs.
// They used to be summed into one integer per leg, which fit the old column but
// merged two different classes - "one 10kg cabin bag" and "no 23kg checked bag"
// - into the single number "1". One row per class is what lets the ticket draw
// them apart.
func TestInsertPassengerFlightsKeepsOneRowPerBaggageClass(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		passenger, flights := seedItinerary(t, tx)

		seat := "12A"
		source := booking.Passenger{}
		source.SeatsOutbound = []*string{&seat, nil}
		source.BaggageOutbound = []booking.Allowance{
			{Quantity: 1, Weight: "10kg"},
			{Quantity: 0, Weight: "23kg"},
		}
		source.BaggageInbound = []booking.Allowance{{Quantity: 1, Weight: "10kg"}}

		if err := insertPassengerFlights(tx, passenger, source, flights); err != nil {
			t.Fatalf("insertPassengerFlights: %v", err)
		}

		var rows []data.IssuancePassengerBaggage
		if err := tx.Where("passenger_id = ?", passenger.ID).
			Order("direction asc, position asc").Find(&rows).Error; err != nil {
			t.Fatalf("reading the baggage back: %v", err)
		}

		// Only the outbound: the itinerary is one-way, and a direction with no
		// legs must not get a return entitlement.
		if len(rows) != 2 {
			t.Fatalf("baggage rows = %d, want one per outbound class and nothing for the absent return", len(rows))
		}
		if rows[0].Position != 0 || rows[0].Quantity != 1 || rows[0].Weight == nil || *rows[0].Weight != "10kg" {
			t.Errorf("first row = %+v, want position 0, 1 piece, 10kg", rows[0])
		}
		// Zero is kept: the class exists and the passenger did not buy it.
		if rows[1].Position != 1 || rows[1].Quantity != 0 || rows[1].Weight == nil || *rows[1].Weight != "23kg" {
			t.Errorf("second row = %+v, want position 1, 0 pieces, 23kg", rows[1])
		}

		// The seats still go per leg, which is the half of the split that was
		// always right.
		var seats []data.IssuancePassengerFlight
		if err := tx.Where("passenger_id = ?", passenger.ID).Find(&seats).Error; err != nil {
			t.Fatalf("reading the seats back: %v", err)
		}
		if len(seats) != len(flights.outbound) {
			t.Errorf("seat rows = %d, want one per leg (%d)", len(seats), len(flights.outbound))
		}
	})
}

// GOL sends a bare count with no weight at all. That is a real shape, not
// missing data, and it has to survive as a row with a null weight rather than
// being dropped for having nothing to print underneath the number.
func TestInsertPassengerFlightsStoresACountWithNoWeight(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		passenger, flights := seedItinerary(t, tx)

		source := booking.Passenger{}
		source.BaggageOutbound = []booking.Allowance{{Quantity: 1}, {Quantity: 0}}

		if err := insertPassengerFlights(tx, passenger, source, flights); err != nil {
			t.Fatalf("insertPassengerFlights: %v", err)
		}

		var rows []data.IssuancePassengerBaggage
		if err := tx.Where("passenger_id = ?", passenger.ID).Order("position asc").Find(&rows).Error; err != nil {
			t.Fatalf("reading the baggage back: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("baggage rows = %d, want both counts kept", len(rows))
		}
		for _, row := range rows {
			if row.Weight != nil {
				t.Errorf("row at position %d has weight %q, want null", row.Position, *row.Weight)
			}
		}
	})
}

// seedItinerary builds the FK chain insertPassengerFlights needs: an issuance
// with two outbound legs and one passenger on it.
func seedItinerary(t *testing.T, tx *gorm.DB) (data.IssuancePassenger, insertedFlights) {
	t.Helper()

	f := seedFixture(t, tx, data.Airline{})
	issuance, err := Create(tx, f.input(), f.User.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var flights insertedFlights
	for legOrder := 1; legOrder <= 2; legOrder++ {
		leg := data.IssuanceFlight{
			IssuanceID:  issuance.ID,
			Direction:   "outbound",
			LegOrder:    legOrder,
			DepartureAt: time.Date(2026, 8, 20, 5, 0, 0, 0, time.UTC),
		}
		create(t, tx, &leg)
		flights.outbound = append(flights.outbound, leg)
	}

	passenger := data.IssuancePassenger{
		IssuanceID: issuance.ID,
		FirstName:  "FERNANDA",
		LastName:   "PRADO",
		Position:   1,
	}
	create(t, tx, &passenger)

	return passenger, flights
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

func TestBookingLocatorPrefersTheAirlinePNR(t *testing.T) {
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
			if got := bookingLocator(issuance); got != tc.want {
				t.Errorf("bookingLocator() = %q, want %q", got, tc.want)
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

func TestApplyIssuanceFieldsUsesTheOutboundArrivalAsTheDestination(t *testing.T) {
	// The bug this covers: on a round trip the last leg of the itinerary lands back
	// at the origin, so reading the itinerary's last arrival made every round trip
	// look like a trip to the passenger's own city.
	iata := func(code string) *string { return &code }

	tests := []struct {
		name     string
		flights  insertedFlights
		wantFrom string
		wantTo   string
	}{
		{
			name: "one way",
			flights: insertedFlights{
				outbound: []data.IssuanceFlight{
					{OriginIATACode: iata("CNF"), DestinationIATACode: iata("GRU")},
				},
			},
			wantFrom: "CNF", wantTo: "GRU",
		},
		{
			name: "round trip lands back home on the last leg",
			flights: insertedFlights{
				outbound: []data.IssuanceFlight{
					{OriginIATACode: iata("CNF"), DestinationIATACode: iata("LIS")},
				},
				inbound: []data.IssuanceFlight{
					{OriginIATACode: iata("LIS"), DestinationIATACode: iata("CNF")},
				},
			},
			wantFrom: "CNF", wantTo: "LIS",
		},
		{
			// A connection is still part of the outbound: the destination is where
			// the passenger stops, not where they change planes.
			name: "round trip with a connection on the way out",
			flights: insertedFlights{
				outbound: []data.IssuanceFlight{
					{OriginIATACode: iata("CNF"), DestinationIATACode: iata("GRU")},
					{OriginIATACode: iata("GRU"), DestinationIATACode: iata("LIS")},
				},
				inbound: []data.IssuanceFlight{
					{OriginIATACode: iata("LIS"), DestinationIATACode: iata("GRU")},
					{OriginIATACode: iata("GRU"), DestinationIATACode: iata("CNF")},
				},
			},
			wantFrom: "CNF", wantTo: "LIS",
		},
		{
			// Nothing marked outbound is not a reason to leave the columns empty.
			name: "inbound only falls back to the whole itinerary",
			flights: insertedFlights{
				inbound: []data.IssuanceFlight{
					{OriginIATACode: iata("LIS"), DestinationIATACode: iata("CNF")},
				},
			},
			wantFrom: "LIS", wantTo: "CNF",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var issuance data.Issuance
			var fetched booking.Booking

			applyIssuanceFields(&issuance, &fetched, tc.flights, "traveos")

			if issuance.OriginIATACode == nil || *issuance.OriginIATACode != tc.wantFrom {
				t.Errorf("origin = %v, want %q", issuance.OriginIATACode, tc.wantFrom)
			}
			if issuance.DestinationIATACode == nil || *issuance.DestinationIATACode != tc.wantTo {
				t.Errorf("destination = %v, want %q", issuance.DestinationIATACode, tc.wantTo)
			}
		})
	}
}

func TestNormaliseForStorageUpperCasesNamesAndCabins(t *testing.T) {
	// Azul answers with mixed case, and disagrees with itself inside one booking -
	// both spellings would otherwise land on the same ticket. Traveos already sends
	// capitals, so this is a no-op there.
	fetched := &booking.Booking{
		Outbound: []booking.Flight{{Cabin: "Economy"}},
		Inbound:  []booking.Flight{{Cabin: "Economy"}},
		Passengers: []booking.Passenger{
			{FirstName: "Lia Regina", LastName: "de Brito Moura"},
			{FirstName: "DEUSDEDIT CASTELO BRANCO DE", LastName: "ANDRADE"},
		},
	}

	normaliseForStorage(fetched)

	if fetched.Outbound[0].Cabin != "ECONOMY" || fetched.Inbound[0].Cabin != "ECONOMY" {
		t.Errorf("cabins = %q / %q", fetched.Outbound[0].Cabin, fetched.Inbound[0].Cabin)
	}
	if fetched.Passengers[0].FirstName != "LIA REGINA" || fetched.Passengers[0].LastName != "DE BRITO MOURA" {
		t.Errorf("first passenger = %q %q", fetched.Passengers[0].FirstName, fetched.Passengers[0].LastName)
	}
	if fetched.Passengers[1].LastName != "ANDRADE" {
		t.Errorf("second passenger was changed: %q", fetched.Passengers[1].LastName)
	}
}

func TestNormaliseForStorageChangesCaseOnly(t *testing.T) {
	// The spelling is the airline's, including Traveos' own broken unicode escape.
	// Rewriting that would be inventing data; the ticket fixes it at display time.
	fetched := &booking.Booking{Outbound: []booking.Flight{{Cabin: "Econu00f4mica"}}}

	normaliseForStorage(fetched)

	if fetched.Outbound[0].Cabin != "ECONU00F4MICA" {
		t.Errorf("cabin = %q, want the same word in capitals", fetched.Outbound[0].Cabin)
	}
}
