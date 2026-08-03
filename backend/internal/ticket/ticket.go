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
	Funcs(template.FuncMap{"pluralize": pluralize, "dict": dict}).
	ParseFS(templateFS, "ticket.gohtml"))

// dict lets the icon sub-templates take more than one argument, which
// html/template otherwise has no way to express. It exists only so the plane
// silhouette is written once instead of three times, each copy free to drift.
func dict(pairs ...any) map[string]any {
	built := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			continue
		}
		built[key] = pairs[i+1]
	}
	return built
}

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

	// PageHeightMM is the paper the document asks for, in millimetres. See
	// pageHeightMM: the width is fixed at the vendor's own 216mm and only the
	// height moves, so a ticket lands on one page instead of being sliced by a
	// paper size that knows nothing about its contents.
	PageHeightMM int
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
	Label string
	Seats []Seat
	// Inbound mirrors the little plane in the card's rail, the same way
	// Journey.Inbound mirrors the one in the journey header.
	Inbound bool
	Baggage []Allowance
}

// Allowance is one baggage entitlement as the ticket draws it: an icon, a
// quantity and a label underneath saying which bag that is.
type Allowance struct {
	// Label identifies the entitlement. It is the weight when the airline gave
	// one ("10kg"), because a weight is the most specific thing that can be said
	// about a bag; when it did not, it is the kind ("Mão"/"Despachada"), which
	// the array position knows. What it is never allowed to be is a count of
	// pieces - the number beside it already says that, and repeating it leaves
	// the passenger with no idea which of the two bags the row is about.
	Label    string
	Quantity int
	// CarryOn picks the backpack over the suitcase. It is the vendor's own array
	// position, not a deduction from the weight - the first entitlement of a
	// direction is the cabin bag, and guessing from "10kg" would be a guess.
	CarryOn bool
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
func FromIssuance(
	issuance data.Issuance,
	seats map[string][]data.IssuancePassengerFlight,
	baggage map[string][]data.IssuancePassengerBaggage,
) ViewModel {
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
	model.Passengers = buildPassengers(issuance.Passengers, outbound, inbound, seats, baggage)
	model.PageHeightMM = pageHeightMM(model)

	return model
}

// The page is 216mm wide - the width Traveos' own PDF uses - and as tall as the
// content needs. A fixed paper is what put the ticket on three cut pages: A4
// landscape has ~186mm of usable height and the document has never fit in it.
//
// The numbers below are millimetres measured from real renders, not guesses on
// paper, and they are deliberately generous. Overestimating costs white space
// at the bottom, which the vendor's own file has too; underestimating costs the
// second page this whole thing exists to remove. The page-break-inside: avoid
// on the journey and passenger cards means an underestimate degrades into a
// clean break rather than a card sliced in half.
const (
	// pageChromeMM covers the two 12mm page margins, the header, the brand rule
	// and the "Informações" block - everything present on every ticket.
	pageChromeMM = 52
	// journeyMM is one coloured bar plus the flight list's own padding.
	journeyMM = 24
	// legMM is one flight segment card plus the gap under it.
	legMM = 28
	// passengersTitleMM is the "N Passageiros" line, charged once.
	passengersTitleMM = 10
	// passengerMM is one passenger card. Both directions sit side by side inside
	// it, so a round trip costs nothing extra here - what does cost is the seat
	// chips wrapping once a direction has more than two legs, which is what
	// passengerLegWrapMM pays for.
	passengerMM        = 34
	passengerLegWrapMM = 20
	passengerWrapAfter = 2

	// pageStepMM keeps the paper on a round number, so two similar tickets get
	// the same page instead of differing by a millimetre.
	pageStepMM = 20
	// pageMinMM stops a one-leg ticket from coming out as a slip of paper.
	pageMinMM = 160
	// pageMaxMM is where "one page when possible" gives up. A ticket past this
	// is genuinely too much content, and breaking it is the honest answer.
	pageMaxMM = 600
)

func pageHeightMM(model ViewModel) int {
	height := pageChromeMM + len(model.Journeys)*journeyMM

	// The busiest single direction, because that is what makes a passenger's
	// seat chips wrap onto a second row inside a half-width card.
	busiest := 0
	for _, journey := range model.Journeys {
		height += len(journey.Segments) * legMM
		if len(journey.Segments) > busiest {
			busiest = len(journey.Segments)
		}
	}

	if len(model.Passengers) > 0 {
		perPassenger := passengerMM
		if busiest > passengerWrapAfter {
			perPassenger += passengerLegWrapMM
		}
		height += passengersTitleMM + len(model.Passengers)*perPassenger
	}

	// Round up to the step, then clamp.
	height = ((height + pageStepMM - 1) / pageStepMM) * pageStepMM
	if height < pageMinMM {
		return pageMinMM
	}
	if height > pageMaxMM {
		return pageMaxMM
	}
	return height
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
			Cabin:           cleanCabin(derefOr(leg.Cabin, "")),
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
	baggage map[string][]data.IssuancePassengerBaggage,
) []Passenger {
	built := make([]Passenger, 0, len(passengers))

	for _, passenger := range passengers {
		byFlight := indexByFlight(seats[passenger.ID])
		byDirection := indexByDirection(baggage[passenger.ID])

		person := Passenger{Name: strings.TrimSpace(passenger.FirstName + " " + passenger.LastName)}
		for _, spec := range []struct {
			label     string
			direction string
			legs      []data.IssuanceFlight
			inbound   bool
		}{
			{"Ida", "outbound", outbound, false},
			{"Volta", "inbound", inbound, true},
		} {
			if len(spec.legs) == 0 {
				continue
			}
			person.Trips = append(person.Trips, PassengerTrip{
				Label:   spec.label,
				Seats:   buildSeats(spec.legs, byFlight),
				Inbound: spec.inbound,
				Baggage: buildAllowances(byDirection[spec.direction]),
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

func indexByDirection(rows []data.IssuancePassengerBaggage) map[string][]data.IssuancePassengerBaggage {
	byDirection := make(map[string][]data.IssuancePassengerBaggage, 2)
	for _, row := range rows {
		byDirection[row.Direction] = append(byDirection[row.Direction], row)
	}
	// The rows are written in array order but nothing in the schema promises the
	// read comes back that way, and the order is what picks the icons.
	for direction := range byDirection {
		sortByPosition(byDirection[direction])
	}
	return byDirection
}

func sortByPosition(rows []data.IssuancePassengerBaggage) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].Position < rows[j-1].Position; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// buildAllowances turns one direction's entitlements into the cards the ticket
// draws. A quantity of zero is kept on purpose: "23kg: 0" says the airline
// offers checked baggage and this passenger did not buy any, which is what the
// operator is looking for. Dropping it would read as "this fare has no checked
// bag at all".
//
// A passenger with no baggage rows at all - an airline that reported none, or an
// issuance synced before this table existed - falls back to a single "0
// bagagens", which is the label the ticket has always shown. That case is the
// one place a count is the right label: there is no entitlement to name.
func buildAllowances(rows []data.IssuancePassengerBaggage) []Allowance {
	if len(rows) == 0 {
		return []Allowance{{Label: pluralize(0, "bagagem", "bagagens"), CarryOn: true}}
	}

	built := make([]Allowance, 0, len(rows))
	for _, row := range rows {
		allowance := Allowance{Quantity: row.Quantity, CarryOn: row.Position == 0}
		if row.Weight != nil {
			allowance.Label = strings.TrimSpace(*row.Weight)
		}
		if allowance.Label == "" {
			// GOL sends `[1, 0]` - two counts and no weights at all. The kind of
			// bag is still known, because it is the array position, and naming it
			// is the whole point of the line: "1 bagagem" under a backpack icon
			// says nothing the number above it did not already say.
			allowance.Label = "Despachada"
			if allowance.CarryOn {
				allowance.Label = "Mão"
			}
		}
		built = append(built, allowance)
	}
	return built
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
