package issuance

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/traveos"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// courtesyTitles are the honorifics some airlines append to the given name.
// AMERICAN puts "MS"/"MR" at the end of `nome` for every passenger; nobody else
// observed does.
var courtesyTitles = map[string]bool{
	"MR": true, "MRS": true, "MS": true, "MISS": true,
	"DR": true, "DRA": true, "SR": true, "SRA": true, "SRTA": true,
}

// applyTraveosData writes the fetched itinerary over whatever the issuance had.
//
// Replace rather than merge, and in one transaction: a resync has to converge on
// what the airline says now, so a leg that no longer exists in the booking must
// disappear from our copy instead of lingering next to its replacement.
func applyTraveosData(
	tx *gorm.DB,
	issuance *data.Issuance,
	airline *data.Airline,
	ticket *traveos.TicketItem,
	details *traveos.TicketData,
) error {
	if err := deleteExistingItinerary(tx, issuance.ID); err != nil {
		return err
	}

	flights, err := insertFlights(tx, issuance.ID, details)
	if err != nil {
		return err
	}

	if err := insertPassengers(tx, issuance.ID, details, flights); err != nil {
		return err
	}

	// This is where real documents enter the system: everywhere else a passenger is just a
	// name. Registering every passenger, not only the main one, because the cap counts
	// people on the booking - a family of four consumes four slots, not one.
	//
	// Idempotent by design, which matters here more than anywhere: the block above deletes
	// and reinserts the passengers on every resync, and a resync must not consume the
	// account's slots a second time.
	if err := registerBeneficiaries(tx, issuance, details); err != nil {
		return err
	}

	applyIssuanceFields(tx, issuance, details, flights)

	// Backfill the brand colour from the ticket when the airline has none yet. The
	// catalog is the tidier source, but this arrives free with data we already
	// fetched, and a ticket with no colour renders in a generic blue nobody chose.
	if airline.BrandColor == nil && ticket.Airline.PrimaryColor != "" {
		colour := strings.ToUpper(strings.TrimSpace(ticket.Airline.PrimaryColor))
		if isHexColour(colour) {
			airline.BrandColor = &colour
			if err := tx.Model(airline).Update("brand_color", colour).Error; err != nil {
				return err
			}
		}
	}

	return tx.Save(issuance).Error
}

// deleteExistingItinerary clears the itinerary tables for this issuance,
// children first: issuance_passenger_flights has no cascade from either parent,
// so deleting passengers before their join rows would leave orphans behind.
// issuance_passenger_baggage is not listed because it does cascade from the
// passenger - it has a single parent, so the database can clean it up itself.
func deleteExistingItinerary(tx *gorm.DB, issuanceID string) error {
	if err := tx.Exec(`
		DELETE FROM issuance_passenger_flights
		WHERE passenger_id IN (SELECT id FROM issuance_passengers WHERE issuance_id = ?)
		   OR flight_id     IN (SELECT id FROM issuance_flights     WHERE issuance_id = ?)`,
		issuanceID, issuanceID).Error; err != nil {
		return err
	}
	if err := tx.Where("issuance_id = ?", issuanceID).Delete(&data.IssuancePassenger{}).Error; err != nil {
		return err
	}
	return tx.Where("issuance_id = ?", issuanceID).Delete(&data.IssuanceFlight{}).Error
}

// insertedFlights indexes the persisted legs by direction, in the order Traveos
// returned them, because that ordering is what the seat arrays are aligned with.
type insertedFlights struct {
	outbound []data.IssuanceFlight
	inbound  []data.IssuanceFlight
}

func (f insertedFlights) all() []data.IssuanceFlight {
	return append(append([]data.IssuanceFlight{}, f.outbound...), f.inbound...)
}

func insertFlights(tx *gorm.DB, issuanceID string, details *traveos.TicketData) (insertedFlights, error) {
	var inserted insertedFlights

	for _, spec := range []struct {
		direction string
		legs      []traveos.Flight
		target    *[]data.IssuanceFlight
	}{
		{"outbound", details.Flights.Outbound, &inserted.outbound},
		{"inbound", details.Flights.Inbound, &inserted.inbound},
	} {
		for index, leg := range spec.legs {
			// departure_at is NOT NULL, and a zero time would write a ticket dated
			// year 1. Better to refuse the whole sync: a flight with no departure is
			// not a flight anyone can travel on.
			if leg.DepartureAt.IsZero() {
				return insertedFlights{}, ErrFlightMissingDeparture
			}

			flight := data.IssuanceFlight{
				IssuanceID:  issuanceID,
				Direction:   spec.direction,
				LegOrder:    index + 1,
				DepartureAt: leg.DepartureAt.Time,
			}
			if number := strings.TrimSpace(leg.Number); number != "" {
				flight.FlightNumber = &number
			}
			if cabin := strings.TrimSpace(leg.Cabin); cabin != "" {
				// Copied verbatim, including the vendor's own broken unicode escapes
				// ("Econu00f4mica"). Rewriting it would be inventing data.
				flight.Cabin = &cabin
			}
			if !leg.ArrivalAt.IsZero() {
				arrival := leg.ArrivalAt.Time
				flight.ArrivalAt = &arrival
			}
			if leg.Duration > 0 {
				duration := int(leg.Duration)
				flight.DurationMin = &duration
			}
			flight.OriginIATACode = resolveAirportCode(tx, firstNonEmpty(leg.OriginDetails.IATA, leg.OriginIATA))
			flight.DestinationIATACode = resolveAirportCode(tx, firstNonEmpty(leg.DestinationDetails.IATA, leg.DestinationIATA))

			if err := tx.Create(&flight).Error; err != nil {
				return insertedFlights{}, err
			}
			*spec.target = append(*spec.target, flight)
		}
	}

	return inserted, nil
}

// resolveAirportCode returns the code only if it exists in our airports table.
//
// Both columns are FKs to airports.iata_code, so an airport we have never seeded
// would abort the insert with a raw 23503 and take the whole itinerary down with
// it. The seed covers 7917 airports and every code the five test tickets
// returned, but "every airport Traveos will ever name" is not a promise anyone
// can make. Losing one foreign key is a far better outcome than losing the
// itinerary, so an unknown code is dropped and logged.
func resolveAirportCode(tx *gorm.DB, code string) *string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil
	}

	var airport data.Airport
	err := tx.Select("iata_code").First(&airport, "iata_code = ?", code).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("traveos: failed to look up an airport", "iata", code, "err", err)
			return nil
		}
		slog.Warn("traveos: airport is not in the airports table, leaving the leg without it",
			"iata", code)
		return nil
	}
	return &code
}

// registerBeneficiaries charges this issuance's passengers against the account's cap.
//
// Traveos is authoritative about who was on the booking, so this replaces whatever the
// issuance had consumed rather than adding to it - see account.ReplaceIssuanceUses for why
// appending would double-count the main passenger on the bot path.
//
// Every passenger, not only the main one: the cap counts people on the booking, so a family
// of four consumes four slots.
//
// The documents are read straight off the Traveos payload rather than back from the rows
// just written: those are encrypted on save, and reading them back would mean decrypting
// what we already have in the clear right here.
func registerBeneficiaries(tx *gorm.DB, issuance *data.Issuance, details *traveos.TicketData) error {
	passengers := make([]account.Passenger, 0, len(details.Emission.Passengers))
	for _, source := range details.Emission.Passengers {
		name := strings.TrimSpace(cleanGivenName(source.FirstName) + " " + strings.TrimSpace(source.LastName))
		passengers = append(passengers, account.Passenger{Name: name, CPF: source.Document})
	}

	// Dated by the issuance, not by the sync: a booking made in December and synced in
	// January belongs to December's window - the program charged the slot when the ticket
	// was issued, not when we got around to reading it.
	return account.ReplaceIssuanceUses(tx, issuance.AccountID, issuance.ID, passengers, issuance.CreatedAt)
}

func insertPassengers(
	tx *gorm.DB,
	issuanceID string,
	details *traveos.TicketData,
	flights insertedFlights,
) error {
	for index, source := range details.Emission.Passengers {
		passenger := data.IssuancePassenger{
			IssuanceID: issuanceID,
			FirstName:  cleanGivenName(source.FirstName),
			LastName:   strings.TrimSpace(source.LastName),
			Type:       normalisePassengerType(source.Type),
			// Position 0 is the main passenger, in the order Traveos listed them.
			Position: index,
		}

		// first_name and last_name are both NOT NULL. A passenger with neither is
		// not something to persist, but one of the two missing is common enough
		// (some airlines return an empty surname) that a placeholder beats
		// refusing the whole ticket.
		if passenger.FirstName == "" && passenger.LastName == "" {
			slog.Warn("traveos: skipping a passenger with no name at all",
				"issuance_id", issuanceID, "position", index)
			continue
		}
		if passenger.FirstName == "" {
			passenger.FirstName = "-"
		}
		if passenger.LastName == "" {
			passenger.LastName = "-"
		}

		// Assigned in the clear: the model's BeforeSave hook encrypts it (AES-256-GCM).
		// Encrypting here as well would double-wrap it and the AfterFind decrypt would
		// hand back ciphertext.
		if source.Document != nil {
			if document := strings.TrimSpace(*source.Document); document != "" {
				passenger.Document = &document
			}
		}

		if err := tx.Create(&passenger).Error; err != nil {
			return err
		}

		if err := insertPassengerFlights(tx, passenger, source, flights); err != nil {
			return err
		}
	}
	return nil
}

// insertPassengerFlights writes the per-passenger seat and baggage rows.
//
// The two sources are shaped differently, and this is the trap the reverse
// engineering write-up originally got wrong:
//
//   - assentos.ida[] IS positionally aligned with the flight legs - LATAM sends
//     [null, null] for two legs, AMERICAN sends ["36D"] for one.
//   - bagagens.ida[] is NOT. Its entries are entitlement classes (carry-on,
//     checked), not legs: GOL sends [1, 0] for a single-leg ticket and AVIANCA
//     sends two objects (10kg, 23kg) for one leg. Indexing it by leg reads the
//     wrong entitlement on a short itinerary and runs off the end on a long one.
//
// So they go to different tables. The seat is taken by index and written per
// leg; the baggage is written once per direction, one row per entitlement, in
// the vendor's own array order. It used to be summed into a single integer per
// leg, which fit the old column but destroyed exactly what the ticket needs to
// show: two classes with different weights are not one number.
func insertPassengerFlights(
	tx *gorm.DB,
	passenger data.IssuancePassenger,
	source traveos.Passenger,
	flights insertedFlights,
) error {
	for _, spec := range []struct {
		direction string
		legs      []data.IssuanceFlight
		seats     []*string
		baggage   []traveos.BaggageAllowance
	}{
		{"outbound", flights.outbound, source.Seats.Outbound, source.Baggage.Outbound},
		{"inbound", flights.inbound, source.Seats.Inbound, source.Baggage.Inbound},
	} {
		for index, leg := range spec.legs {
			link := data.IssuancePassengerFlight{
				PassengerID: passenger.ID,
				FlightID:    leg.ID,
			}
			if index < len(spec.seats) && spec.seats[index] != nil {
				if seat := strings.TrimSpace(*spec.seats[index]); seat != "" {
					link.Seat = &seat
				}
			}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}

		// A direction with no legs has no baggage either - writing rows for it
		// would put a return entitlement on a one-way ticket.
		if len(spec.legs) == 0 {
			continue
		}
		for position, allowance := range spec.baggage {
			row := data.IssuancePassengerBaggage{
				PassengerID: passenger.ID,
				Direction:   spec.direction,
				Position:    position,
				Quantity:    allowance.Quantity,
			}
			if weight := strings.TrimSpace(allowance.Weight); weight != "" {
				row.Weight = &weight
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// applyIssuanceFields fills in the columns the operator's own input never
// carries. Errors here are not fatal: every field it touches is nullable, and a
// missing route is a smaller loss than a rejected itinerary.
func applyIssuanceFields(
	tx *gorm.DB,
	issuance *data.Issuance,
	details *traveos.TicketData,
	flights insertedFlights,
) {
	tripType := deriveTripType(details)
	issuance.TripType = &tripType

	legs := flights.all()
	if len(legs) > 0 {
		if code := legs[0].OriginIATACode; code != nil {
			issuance.OriginIATACode = code
		}
	}

	// The destination is where the outbound ends, not where the itinerary does: on a
	// round trip the last leg lands back at the origin, and the column would claim the
	// trip was to the passenger's own city. Connections still count - it is the last
	// leg of the outbound, not the first. `all()` is only a fallback for an itinerary
	// the vendor returned entirely as inbound legs.
	outbound := flights.outbound
	if len(outbound) == 0 {
		outbound = legs
	}
	if len(outbound) > 0 {
		if code := outbound[len(outbound)-1].DestinationIATACode; code != nil {
			issuance.DestinationIATACode = code
		}
	}

	if len(details.Emission.Passengers) > 0 {
		main := details.Emission.Passengers[0]
		if name := strings.TrimSpace(cleanGivenName(main.FirstName) + " " + strings.TrimSpace(main.LastName)); name != "" {
			issuance.MainPassengerName = &name
		}
	}

	if number := strings.TrimSpace(details.Emission.PurchaseNumber); number != "" {
		issuance.LatamPurchaseNumber = &number
	}

	// Stored verbatim: three mutually incompatible shapes across airlines
	// (assistance slots, paid extras, an empty array) and nothing on our side
	// reads it structurally, so normalising it would be invented work.
	if len(details.Emission.AdditionalServices) > 0 {
		issuance.AdditionalServices = datatypes.JSON(details.Emission.AdditionalServices)
	}

	now := time.Now()
	issuance.TraveosSyncedAt = &now
	// The airline confirmed the itinerary - that is exactly what complete means.
	issuance.Complete = true
}

// deriveTripType maps the vendor's vocabulary onto our trip_type enum.
//
// Note that multi-leg is not a trip type at Traveos: tripType only distinguishes
// one-way from round-trip, and a multi-leg itinerary shows up as
// arguments.multitrechos instead. Our enum has all three, so the flag has to be
// checked separately - and it is checked first, because a multi-leg journey is
// more specific than "one way".
func deriveTripType(details *traveos.TicketData) string {
	if bool(details.Emission.Arguments.MultiLeg) {
		return "multi_leg"
	}
	if len(details.Flights.Inbound) > 0 || details.Emission.TripType == "ida_e_volta" {
		return "round_trip"
	}
	return "one_way"
}

// cleanGivenName strips a trailing courtesy title from the given name.
//
// AMERICAN appends "MS"/"MR" to every passenger's `nome`, which then reads
// "FERNANDA CRISTINA MS" on the ticket. Only the last token of the given name is
// a candidate: that is where the airline puts it, and the surname is never
// touched, because a real name could legitimately be one of these words and
// eating it would be worse than leaving an honorific in.
func cleanGivenName(name string) string {
	tokens := strings.Fields(strings.TrimSpace(name))
	if len(tokens) > 1 {
		last := strings.ToUpper(strings.TrimRight(tokens[len(tokens)-1], "."))
		if courtesyTitles[last] {
			tokens = tokens[:len(tokens)-1]
		}
	}
	return strings.Join(tokens, " ")
}

// normalisePassengerType keeps the value inside the passenger_type enum. The
// vendor uses the same three codes, but it is free text on their side and a
// value outside the enum would abort the insert.
func normalisePassengerType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "CHD":
		return "CHD"
	case "INF":
		return "INF"
	default:
		return "ADT"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isHexColour(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, c := range value[1:] {
		isDigit := c >= '0' && c <= '9'
		isHex := c >= 'A' && c <= 'F'
		if !isDigit && !isHex {
			return false
		}
	}
	return true
}
