// Package ticket renders the passenger ticket for an issuance: one HTML
// document that is both the page the panel shows and, unchanged, the input the
// PDF service renders.
//
// One generator, not two. A ticket that looks one way on screen and another way
// in the customer's inbox is a support call waiting to happen, and keeping two
// templates in step is work nobody would remember to do. The cost is that the
// markup has to live inside WeasyPrint 61.2's subset - see ticket.gohtml.
package ticket

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
)

//go:embed ticket.gohtml
var templateFS embed.FS

//go:embed logos/*.svg
var logoFS embed.FS

// defaultBrand is the blue Traveos' own page falls back to. Reused rather than
// invented so a ticket for an airline nobody has coloured in yet looks like the
// page it was copied from, instead of unstyled.
const defaultBrand = "#224CF0"

var tpl = template.Must(template.New("ticket.gohtml").
	Funcs(template.FuncMap{"pluralize": pluralize}).
	ParseFS(templateFS, "ticket.gohtml"))

// ViewModel is everything the template draws, already formatted. No time.Time,
// no pointers and no database types cross into the template: a nil dereference
// inside html/template surfaces as a half-rendered page, and dates formatted in
// two different places drift apart.
type ViewModel struct {
	Locator        string
	PurchaseNumber string

	AirlineName string
	// AirlineLogo is inline SVG, marked safe because it comes from this
	// package's own embedded files and never from the database.
	AirlineLogo template.HTML
	// Brand is the airline's colour, BrandOn the text colour that stays legible
	// on it - see contrastColour.
	Brand   string
	BrandOn string

	Journeys   []Journey
	Passengers []Passenger
}

// Journey is one direction of the trip. Absent, not empty, on a one-way ticket.
type Journey struct {
	Title    string
	Date     string
	LegLabel string
	Segments []Segment
	// Inbound mirrors the plane icon, which is the only difference between the
	// two headers.
	Inbound bool
}

type Segment struct {
	DepartureTime string
	DepartureDate string
	DepartureName string

	ArrivalTime string
	ArrivalDate string
	ArrivalName string

	OriginCode      string
	DestinationCode string

	FlightNumber string
	Cabin        string
}

type Passenger struct {
	Name  string
	Trips []PassengerTrip
}

// PassengerTrip is one direction's seats and baggage for one passenger.
type PassengerTrip struct {
	Label   string
	Seats   []Seat
	Baggage string
}

type Seat struct {
	Label string
	Value string
	// Assigned is false when the airline never gave us a seat, which the
	// template draws as a grey placeholder rather than an empty cell.
	Assigned bool
}

// Render produces the ticket document. The same string is served as the panel's
// page and handed to WeasyPrint.
func Render(model ViewModel) (string, error) {
	var out bytes.Buffer
	if err := tpl.Execute(&out, model); err != nil {
		return "", fmt.Errorf("ticket: rendering: %w", err)
	}
	return out.String(), nil
}

// FromIssuance builds the view model. The issuance must arrive with Airline,
// Flights and Passengers already loaded, each passenger's join rows included -
// this package runs no queries, so that the caller controls exactly one round
// trip to the database and the template can never trigger a lazy load mid-render.
func FromIssuance(issuance data.Issuance, seats map[string][]data.IssuancePassengerFlight) ViewModel {
	model := ViewModel{
		Locator: issuance.ProgramLocator,
		Brand:   defaultBrand,
	}
	if issuance.AirlineLocator != nil && strings.TrimSpace(*issuance.AirlineLocator) != "" {
		// The airline's own PNR is what the passenger quotes at the counter; the
		// program locator is our internal reference to the loyalty booking.
		model.Locator = strings.TrimSpace(*issuance.AirlineLocator)
	}
	if issuance.LatamPurchaseNumber != nil {
		model.PurchaseNumber = strings.TrimSpace(*issuance.LatamPurchaseNumber)
	}

	if issuance.Airline != nil {
		model.AirlineName = issuance.Airline.Name
		model.AirlineLogo = logoFor(issuance.Airline.Code)
		if issuance.Airline.BrandColor != nil && *issuance.Airline.BrandColor != "" {
			model.Brand = *issuance.Airline.BrandColor
		}
	}
	model.BrandOn = contrastColour(model.Brand)

	outbound, inbound := splitByDirection(issuance.Flights)
	model.Journeys = buildJourneys(outbound, inbound)
	model.Passengers = buildPassengers(issuance.Passengers, outbound, inbound, seats)

	return model
}

// splitByDirection separates the legs and puts each direction in leg order. The
// rows are written in order by the sync, but nothing in the schema guarantees
// the read comes back that way, and a connection shown backwards is worse than
// a slow query.
func splitByDirection(flights []data.IssuanceFlight) (outbound, inbound []data.IssuanceFlight) {
	for _, flight := range flights {
		if flight.Direction == "inbound" {
			inbound = append(inbound, flight)
			continue
		}
		outbound = append(outbound, flight)
	}
	sortByLeg(outbound)
	sortByLeg(inbound)
	return outbound, inbound
}

func sortByLeg(flights []data.IssuanceFlight) {
	for i := 1; i < len(flights); i++ {
		for j := i; j > 0 && flights[j].LegOrder < flights[j-1].LegOrder; j-- {
			flights[j], flights[j-1] = flights[j-1], flights[j]
		}
	}
}

func buildJourneys(outbound, inbound []data.IssuanceFlight) []Journey {
	var journeys []Journey
	for _, spec := range []struct {
		title   string
		legs    []data.IssuanceFlight
		inbound bool
	}{
		{"IDA", outbound, false},
		{"VOLTA", inbound, true},
	} {
		if len(spec.legs) == 0 {
			continue
		}
		journeys = append(journeys, Journey{
			Title:    spec.title,
			Date:     longDate(spec.legs[0].DepartureAt),
			LegLabel: pluralize(len(spec.legs), "trecho", "trechos"),
			Segments: buildSegments(spec.legs),
			Inbound:  spec.inbound,
		})
	}
	return journeys
}

func buildSegments(legs []data.IssuanceFlight) []Segment {
	segments := make([]Segment, 0, len(legs))
	for _, leg := range legs {
		segment := Segment{
			DepartureTime:   leg.DepartureAt.Format("15:04"),
			DepartureDate:   leg.DepartureAt.Format("02/01/2006"),
			OriginCode:      derefOr(leg.OriginIATACode, "---"),
			DestinationCode: derefOr(leg.DestinationIATACode, "---"),
			FlightNumber:    derefOr(leg.FlightNumber, ""),
			Cabin:           derefOr(leg.Cabin, ""),
		}
		segment.DepartureName = airportName(leg.OriginAirport, segment.OriginCode)
		segment.ArrivalName = airportName(leg.DestinationAirport, segment.DestinationCode)

		// arrival_at is nullable: some airlines never gave Traveos one. An empty
		// cell is the honest answer - a computed arrival would be a guess printed
		// on a document the passenger travels with.
		if leg.ArrivalAt != nil {
			segment.ArrivalTime = leg.ArrivalAt.Format("15:04")
			segment.ArrivalDate = leg.ArrivalAt.Format("02/01/2006")
		}
		segments = append(segments, segment)
	}
	return segments
}

// airportName renders "City (IATA)", falling back to the bare code for an
// airport our own table has never seen - which the sync tolerates on purpose,
// rather than dropping the whole itinerary over an unseeded code.
func airportName(airport *data.Airport, code string) string {
	if airport == nil || strings.TrimSpace(airport.Name) == "" {
		return code
	}
	return fmt.Sprintf("%s (%s)", airport.Name, code)
}

// buildPassengers assembles the seat and baggage card for each passenger, one
// panel per direction that exists.
func buildPassengers(
	passengers []data.IssuancePassenger,
	outbound, inbound []data.IssuanceFlight,
	seats map[string][]data.IssuancePassengerFlight,
) []Passenger {
	built := make([]Passenger, 0, len(passengers))

	for _, passenger := range passengers {
		byFlight := indexByFlight(seats[passenger.ID])

		person := Passenger{Name: strings.TrimSpace(passenger.FirstName + " " + passenger.LastName)}
		for _, spec := range []struct {
			label string
			legs  []data.IssuanceFlight
		}{
			{"Ida", outbound},
			{"Volta", inbound},
		} {
			if len(spec.legs) == 0 {
				continue
			}
			person.Trips = append(person.Trips, PassengerTrip{
				Label:   spec.label,
				Seats:   buildSeats(spec.legs, byFlight),
				Baggage: baggageLabel(spec.legs, byFlight),
			})
		}
		built = append(built, person)
	}
	return built
}

func indexByFlight(rows []data.IssuancePassengerFlight) map[string]data.IssuancePassengerFlight {
	byFlight := make(map[string]data.IssuancePassengerFlight, len(rows))
	for _, row := range rows {
		byFlight[row.FlightID] = row
	}
	return byFlight
}

func buildSeats(legs []data.IssuanceFlight, byFlight map[string]data.IssuancePassengerFlight) []Seat {
	built := make([]Seat, 0, len(legs))
	for index, leg := range legs {
		seat := Seat{Label: fmt.Sprintf("T%d", index+1), Value: "--"}
		if row, ok := byFlight[leg.ID]; ok && row.Seat != nil && strings.TrimSpace(*row.Seat) != "" {
			seat.Value = strings.TrimSpace(*row.Seat)
			seat.Assigned = true
		}
		built = append(built, seat)
	}
	return built
}

// baggageLabel is the entitlement for one direction. It is stored per leg but is
// not per leg in the source - Traveos reports baggage as an entitlement for the
// whole direction, so every leg of a direction carries the same number and
// showing it once is what matches the booking.
func baggageLabel(legs []data.IssuanceFlight, byFlight map[string]data.IssuancePassengerFlight) string {
	for _, leg := range legs {
		row, ok := byFlight[leg.ID]
		if ok && row.BaggageQty != nil {
			return pluralize(*row.BaggageQty, "bagagem", "bagagens")
		}
	}
	return pluralize(0, "bagagem", "bagagens")
}

// logoFor returns the airline's inline SVG, or nothing when we have no artwork
// for it - the template then falls back to the airline's name in text.
func logoFor(code string) template.HTML {
	svg, err := logoFS.ReadFile("logos/" + strings.ToUpper(strings.TrimSpace(code)) + ".svg")
	if err != nil {
		return ""
	}
	// #nosec G203 -- the bytes come from this package's own embedded files, not
	// from the database or from any request.
	return template.HTML(svg)
}

// contrastColour picks black or white text for a background, by the WCAG
// relative-luminance formula. Half the real airline colours fail against white -
// GOL's orange and AZUL's light blue among them - so a hardcoded white would
// leave those two buttons unreadable.
func contrastColour(hex string) string {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return "#fafbff"
	}

	channel := func(value int) float64 {
		v := float64(value) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		// The sRGB gamma curve, spelled out rather than approximated: an
		// approximation flips the decision on the colours that sit near the
		// threshold, which is exactly the set this function exists for.
		return math.Pow((v+0.055)/1.055, 2.4)
	}

	luminance := 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
	if (luminance+0.05)/0.05 >= 4.5 {
		return "#1a1a1a"
	}
	return "#fafbff"
}

func parseHex(hex string) (r, g, b int, ok bool) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, 0, 0, false
	}
	return r, g, b, true
}

// weekdays and months are spelled out because Go's time package has no
// localisation, and the ticket is read by Brazilian passengers.
var weekdays = [...]string{
	"Domingo", "Segunda-feira", "Terça-feira", "Quarta-feira",
	"Quinta-feira", "Sexta-feira", "Sábado",
}

func longDate(at time.Time) string {
	return fmt.Sprintf("%s, %s", weekdays[int(at.Weekday())], at.Format("02/01/2006"))
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func derefOr(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return strings.TrimSpace(*value)
}
