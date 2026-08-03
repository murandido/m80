package ticket

import (
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
)

// The whole package is pure - it runs no queries - so these run without Postgres
// and actually mean something in `go test ./...`.

func strptr(value string) *string { return &value }

func departure(day, hour int) time.Time {
	return time.Date(2026, 8, day, hour, 24, 0, 0, time.UTC)
}

// roundTrip is a two-leg outbound plus a one-leg inbound, with one passenger who
// has a seat on the first leg and none on the second - which is the mix that
// exercises every conditional in the template at once.
func roundTrip() (
	data.Issuance,
	map[string][]data.IssuancePassengerFlight,
	map[string][]data.IssuancePassengerBaggage,
) {
	arrival := departure(20, 8)
	colour := "#ff5e00"

	issuance := data.Issuance{
		ID:             "issuance-1",
		ProgramLocator: "PLOCAT",
		AirlineLocator: strptr("ABC123"),
		Airline: &data.Airline{
			Name:       "GOL",
			Code:       "GOL",
			BrandColor: &colour,
		},
		Flights: []data.IssuanceFlight{
			{
				ID: "leg-2", Direction: "outbound", LegOrder: 2,
				DepartureAt: departure(20, 13), ArrivalAt: &arrival,
				OriginIATACode: strptr("GRU"), DestinationIATACode: strptr("GIG"),
				FlightNumber: strptr("G31234"), Cabin: strptr("Econômica"),
			},
			{
				ID: "leg-1", Direction: "outbound", LegOrder: 1,
				DepartureAt:    departure(20, 5),
				OriginIATACode: strptr("CNF"), DestinationIATACode: strptr("GRU"),
				OriginAirport: &data.Airport{IATACode: "CNF", Name: "Confins"},
			},
			{
				ID: "leg-3", Direction: "inbound", LegOrder: 1,
				DepartureAt:    departure(26, 19),
				OriginIATACode: strptr("GIG"), DestinationIATACode: strptr("CNF"),
			},
		},
		Passengers: []data.IssuancePassenger{
			{ID: "pax-1", FirstName: "FERNANDA", LastName: "PRADO", Position: 1},
		},
	}

	seats := map[string][]data.IssuancePassengerFlight{
		"pax-1": {
			{PassengerID: "pax-1", FlightID: "leg-1", Seat: strptr("12A")},
			{PassengerID: "pax-1", FlightID: "leg-2"},
			{PassengerID: "pax-1", FlightID: "leg-3", Seat: strptr("3C")},
		},
	}

	// The shape AVIANCA and TAP send: two entitlement classes for the whole
	// direction, the second one unbought. The return has only the cabin bag,
	// which is what a one-sided fare looks like.
	baggage := map[string][]data.IssuancePassengerBaggage{
		"pax-1": {
			{PassengerID: "pax-1", Direction: "outbound", Position: 0, Weight: strptr("10kg"), Quantity: 1},
			{PassengerID: "pax-1", Direction: "outbound", Position: 1, Weight: strptr("23kg"), Quantity: 0},
			{PassengerID: "pax-1", Direction: "inbound", Position: 0, Weight: strptr("10kg"), Quantity: 1},
		},
	}
	return issuance, seats, baggage
}

func TestFromIssuanceSplitsAndOrdersTheJourneys(t *testing.T) {
	model := FromIssuance(roundTrip())

	if len(model.Journeys) != 2 {
		t.Fatalf("journeys = %d, want 2", len(model.Journeys))
	}
	if model.Journeys[0].Title != "IDA" || model.Journeys[1].Title != "VOLTA" {
		t.Fatalf("titles = %q/%q, want IDA/VOLTA", model.Journeys[0].Title, model.Journeys[1].Title)
	}

	// leg-2 is first in the slice and second in the itinerary. Nothing in the
	// schema promises the read comes back in leg order, and a connection shown
	// backwards is a ticket that sends someone to the wrong airport.
	outbound := model.Journeys[0].Segments
	if len(outbound) != 2 {
		t.Fatalf("outbound segments = %d, want 2", len(outbound))
	}
	if outbound[0].OriginCode != "CNF" {
		t.Errorf("first leg departs %q, want CNF - the legs came back out of order", outbound[0].OriginCode)
	}
	if outbound[0].DepartureTime != "05:24" {
		t.Errorf("departure time = %q, want 05:24", outbound[0].DepartureTime)
	}
	if model.Journeys[0].LegLabel != "2 trechos" {
		t.Errorf("leg label = %q, want \"2 trechos\"", model.Journeys[0].LegLabel)
	}
	if model.Journeys[1].LegLabel != "1 trecho" {
		t.Errorf("inbound leg label = %q, want the singular", model.Journeys[1].LegLabel)
	}
}

func TestFromIssuanceLeavesTheArrivalBlankWhenTheAirlineNeverGaveOne(t *testing.T) {
	// arrival_at is nullable and several airlines never fill it. A computed
	// arrival would be a guess printed on a document someone travels with.
	model := FromIssuance(roundTrip())

	if got := model.Journeys[0].Segments[0].ArrivalTime; got != "" {
		t.Errorf("arrival time = %q, want empty", got)
	}
	if got := model.Journeys[0].Segments[1].ArrivalTime; got != "08:24" {
		t.Errorf("arrival time of the leg that has one = %q, want 08:24", got)
	}
}

func TestFromIssuancePrefersTheAirlineLocator(t *testing.T) {
	// It is the airline's PNR the passenger quotes at the counter; the program
	// locator is our reference to the loyalty booking.
	model := FromIssuance(roundTrip())
	if model.Locator != "ABC123" {
		t.Errorf("locator = %q, want the airline PNR ABC123", model.Locator)
	}

	issuance, seats, baggage := roundTrip()
	issuance.AirlineLocator = nil
	if got := FromIssuance(issuance, seats, baggage).Locator; got != "PLOCAT" {
		t.Errorf("locator = %q, want the program locator as fallback", got)
	}
}

func TestFromIssuanceNamesTheAirportWhenWeHaveItAndFallsBackToTheCode(t *testing.T) {
	// The sync deliberately keeps an itinerary whose IATA code our airports table
	// has never seen, rather than losing the whole thing to a foreign key - so
	// the ticket has to survive a nil airport.
	model := FromIssuance(roundTrip())

	if got := model.Journeys[0].Segments[0].DepartureName; got != "Confins (CNF)" {
		t.Errorf("named airport = %q, want \"Confins (CNF)\"", got)
	}
	if got := model.Journeys[0].Segments[0].ArrivalName; got != "GRU" {
		t.Errorf("unseeded airport = %q, want the bare code GRU", got)
	}
}

func TestFromIssuanceBuildsOneSeatPerLegAndMarksTheMissingOnes(t *testing.T) {
	model := FromIssuance(roundTrip())

	trips := model.Passengers[0].Trips
	if len(trips) != 2 {
		t.Fatalf("passenger trips = %d, want one per direction", len(trips))
	}

	seats := trips[0].Seats
	if len(seats) != 2 {
		t.Fatalf("outbound seats = %d, want one per leg", len(seats))
	}
	if seats[0].Label != "T1" || seats[0].Value != "12A" || !seats[0].Assigned {
		t.Errorf("first seat = %+v, want T1/12A assigned", seats[0])
	}
	// An airline that assigns at check-in gives us nothing here, and a blank cell
	// reads as a rendering bug - so it is an explicit placeholder instead.
	if seats[1].Assigned || seats[1].Value != "--" {
		t.Errorf("unassigned seat = %+v, want the -- placeholder", seats[1])
	}
}

func TestFromIssuanceKeepsEachBaggageClassApart(t *testing.T) {
	// Two entitlement classes for one direction are not one number. Summing them
	// - which is what the old baggage_qty column could hold - threw the weights
	// away and merged "one cabin bag" with "no checked bag" into "1".
	model := FromIssuance(roundTrip())

	outbound := model.Passengers[0].Trips[0].Baggage
	if len(outbound) != 2 {
		t.Fatalf("outbound allowances = %d, want one per class", len(outbound))
	}
	if outbound[0] != (Allowance{Label: "10kg", Quantity: 1, CarryOn: true}) {
		t.Errorf("first allowance = %+v, want the 10kg carry-on", outbound[0])
	}
	// Quantity zero is information - the airline offers it and the passenger did
	// not buy it - so it is drawn, not dropped.
	if outbound[1] != (Allowance{Label: "23kg", Quantity: 0}) {
		t.Errorf("second allowance = %+v, want the unbought 23kg", outbound[1])
	}

	if inbound := model.Passengers[0].Trips[1].Baggage; len(inbound) != 1 {
		t.Errorf("inbound allowances = %d, want the one class that direction has", len(inbound))
	}
}

func TestFromIssuanceMarksOnlyTheFirstAllowanceAsCarryOn(t *testing.T) {
	// The icon comes from the vendor's array position, never from the weight.
	// A fare whose first class is heavy is still the cabin bag of that fare.
	issuance, seats, baggage := roundTrip()
	baggage["pax-1"] = []data.IssuancePassengerBaggage{
		{PassengerID: "pax-1", Direction: "outbound", Position: 1, Weight: strptr("23kg"), Quantity: 2},
		{PassengerID: "pax-1", Direction: "outbound", Position: 0, Weight: strptr("32kg"), Quantity: 1},
	}

	outbound := FromIssuance(issuance, seats, baggage).Passengers[0].Trips[0].Baggage
	if len(outbound) != 2 {
		t.Fatalf("allowances = %d, want 2", len(outbound))
	}
	// Read back in position order even though they arrived reversed.
	if !outbound[0].CarryOn || outbound[0].Label != "32kg" {
		t.Errorf("first allowance = %+v, want position 0 as the carry-on", outbound[0])
	}
	if outbound[1].CarryOn {
		t.Errorf("second allowance = %+v, want it drawn as checked", outbound[1])
	}
}

// GOL sends two bare counts and no weight at all. "1 bagagem" under a backpack
// icon repeats the number beside it and names nothing, so the label falls back
// to the kind of bag - which the array position knows.
func TestFromIssuanceNamesTheKindWhenTheAirlineSentNoWeight(t *testing.T) {
	issuance, seats, baggage := roundTrip()
	baggage["pax-1"] = []data.IssuancePassengerBaggage{
		{PassengerID: "pax-1", Direction: "outbound", Position: 0, Quantity: 1},
		{PassengerID: "pax-1", Direction: "outbound", Position: 1, Quantity: 0},
	}

	outbound := FromIssuance(issuance, seats, baggage).Passengers[0].Trips[0].Baggage
	if len(outbound) != 2 {
		t.Fatalf("allowances = %d, want 2", len(outbound))
	}
	if outbound[0].Label != "Mão" {
		t.Errorf("first label = %q, want the carry-on named", outbound[0].Label)
	}
	if outbound[1].Label != "Despachada" {
		t.Errorf("second label = %q, want the checked bag named", outbound[1].Label)
	}
}

func TestFromIssuanceFallsBackWhenTheAirlineReportedNoBaggage(t *testing.T) {
	// Issuances synced before this table existed have no rows at all. An empty
	// baggage column reads as a rendering bug, so they get the label the ticket
	// has always shown.
	issuance, seats, _ := roundTrip()

	trips := FromIssuance(issuance, seats, nil).Passengers[0].Trips
	for _, trip := range trips {
		if len(trip.Baggage) != 1 || trip.Baggage[0].Quantity != 0 || trip.Baggage[0].Label != "0 bagagens" {
			t.Errorf("%s baggage = %+v, want a single empty allowance", trip.Label, trip.Baggage)
		}
	}
}

func TestFromIssuanceOmitsTheReturnOnAOneWayTicket(t *testing.T) {
	issuance, seats, baggage := roundTrip()
	issuance.Flights = issuance.Flights[:2] // both outbound legs, no inbound

	model := FromIssuance(issuance, seats, baggage)
	if len(model.Journeys) != 1 {
		t.Fatalf("journeys = %d, want 1", len(model.Journeys))
	}
	if len(model.Passengers[0].Trips) != 1 {
		t.Errorf("passenger trips = %d, want 1 - there is no return to show", len(model.Passengers[0].Trips))
	}
}

func TestContrastColour(t *testing.T) {
	// Half the real airline colours fail against white text, so a hardcoded
	// white would leave GOL's orange and AZUL's light blue unreadable.
	tests := []struct {
		name string
		hex  string
		want string
	}{
		{"latam navy", "#10004f", "#fafbff"},
		{"american blue", "#005A9C", "#fafbff"},
		{"gol orange", "#ff5e00", "#1a1a1a"},
		{"azul light blue", "#54b1e3", "#1a1a1a"},
		{"traveos default", "#224CF0", "#fafbff"},

		// Garbage in the database must not produce black-on-black.
		{"not a colour", "chartreuse", "#fafbff"},
		{"empty", "", "#fafbff"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := contrastColour(tc.hex); got != tc.want {
				t.Errorf("contrastColour(%q) = %q, want %q", tc.hex, got, tc.want)
			}
		})
	}
}

func TestRenderProducesTheDocumentAndEscapesOperatorText(t *testing.T) {
	issuance, seats, baggage := roundTrip()
	// Passenger names are typed by an operator, so the template has to escape
	// them. html/template does it by construction; this pins that the field is
	// not accidentally passed through as template.HTML one day.
	issuance.Passengers[0].LastName = `PRADO<script>alert(1)</script>`

	document, err := Render(FromIssuance(issuance, seats, baggage))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	for _, want := range []string{"ABC123", "IDA", "VOLTA", "Confins (CNF)", "12A", "10kg", "23kg"} {
		if !strings.Contains(document, want) {
			t.Errorf("the document does not mention %q", want)
		}
	}
	if strings.Contains(document, "<script>") {
		t.Error("operator text reached the document unescaped")
	}

	// The three WeasyPrint 61.2 traps. Each of them fails silently: a grid
	// degrades to stacked blocks, and an unparseable colour renders as black.
	for _, forbidden := range []string{"display: grid", "display:grid", "oklch(", "color-mix(", "@media"} {
		if strings.Contains(document, forbidden) {
			t.Errorf("the stylesheet uses %q, which WeasyPrint 61.2 does not support", forbidden)
		}
	}
}

func TestRenderUsesTheAirlineLogoAndFallsBackToItsName(t *testing.T) {
	document, err := Render(FromIssuance(roundTrip()))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(document, "<svg") {
		t.Error("the GOL logo was not embedded")
	}

	issuance, seats, baggage := roundTrip()
	issuance.Airline.Code = "XX" // an airline we have no artwork for
	document, err = Render(FromIssuance(issuance, seats, baggage))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(document, "GOL") {
		t.Error("an airline with no logo lost its name too")
	}
}

func TestFromIssuanceSurvivesAnIssuanceWithNoAirlineLoaded(t *testing.T) {
	// Defensive rather than reachable through the handler, which preloads it.
	// The alternative is a nil dereference halfway through a rendered page.
	issuance, seats, baggage := roundTrip()
	issuance.Airline = nil

	model := FromIssuance(issuance, seats, baggage)
	if model.Brand != defaultBrand {
		t.Errorf("brand = %q, want the default %q", model.Brand, defaultBrand)
	}
	if _, err := Render(model); err != nil {
		t.Fatalf("Render: %v", err)
	}
}
