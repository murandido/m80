package issuance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// ErrLookupAirlineNotFound is the lookup's own "no such airline": unlike Sync, which
// reads the airline off a saved issuance, the lookup is handed the id by the panel.
var ErrLookupAirlineNotFound = errors.New("airline not found")

// LookupInput is what the panel knows about a booking before the issuance exists:
// the fields a provider is asked with, and nothing else.
type LookupInput struct {
	AirlineID         string
	ProgramLocator    string
	AirlineLocator    *string
	OriginIATACode    *string
	MainPassengerName *string
	// PassengerLastName is the surname typed for the lookup; it wins over
	// MainPassengerName, as on a saved issuance (data.Issuance.BookingLastName).
	PassengerLastName   *string
	LatamPurchaseNumber *string
}

// LookupPreview is the part of a fetched booking the panel's form shows. It is a
// preview and nothing more: the itinerary itself (legs, passengers, seats) is only
// ever written by Sync, once the issuance has been saved.
type LookupPreview struct {
	FlightDate          *string
	OriginIATACode      *string
	DestinationIATACode *string
	MainPassengerName   *string
	LatamPurchaseNumber *string
	Source              string
}

// Lookup asks the airline's providers about a booking that has no issuance yet.
//
// It exists because the panel's form wants the itinerary *before* the operator types
// the values, and an issuance cannot be created without them - points, milheiro and
// account are NOT NULL. So the lookup runs on a transient issuance that is never
// saved, through exactly the same provider order and rules as Sync: the airline's own
// integration first, Traveos behind it, and never an import after the airline said
// the booking does not exist (see attemptProviders).
//
// The one thing it writes anywhere is the Traveos import, when that provider is the
// one that has to fetch the booking. That is deliberate, and safe for the Sync that
// follows the save: fetchOrImport searches before importing, so the second time
// around the booking is simply read.
//
// Nothing is recorded in booking_lookup_attempts: its issuance_id is NOT NULL, and a
// lookup has no issuance to point at. The Sync after the save records its own.
func Lookup(ctx context.Context, db *gorm.DB, registry *booking.Registry, input LookupInput) (*LookupPreview, error) {
	var airline data.Airline
	if err := db.WithContext(ctx).First(&airline, "id = ?", input.AirlineID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLookupAirlineNotFound
		}
		return nil, err
	}

	providers := resolveProviders(registry, airline)
	if len(providers) == 0 {
		return nil, ErrAirlineNotSupported
	}
	// Same gate as Sync, for the same reason: a vendor-only airline without its
	// catalog id can only answer 422.
	if len(providers) == 1 && providers[0] == registry.Fallback() && airline.TraveosAirlineID == nil {
		return nil, fmt.Errorf("%w: %s", ErrAirlineNotMapped, airline.Name)
	}

	transient := data.Issuance{
		ProgramLocator:      strings.TrimSpace(input.ProgramLocator),
		AirlineLocator:      input.AirlineLocator,
		OriginIATACode:      input.OriginIATACode,
		MainPassengerName:   input.MainPassengerName,
		PassengerLastName:   input.PassengerLastName,
		LatamPurchaseNumber: input.LatamPurchaseNumber,
	}

	fetched, source, err := attemptProviders(ctx, registry, providers, transient, airline, false, nil)
	if err != nil {
		return nil, err
	}

	preview := previewFields(fetched, func(code string) *string {
		return resolveAirportCode(db.WithContext(ctx), code)
	})
	preview.Source = source
	return preview, nil
}

// previewFields reads off a booking what applyIssuanceFields would write onto the
// issuance, without a database: origin of the first leg, destination where the
// outbound ends, the outbound's departure day, and the main passenger's full name.
//
// The airport codes go through resolveAirport for the same reason the legs do in
// applyBooking: a code missing from the airports table would make the create that
// follows fail on its foreign key, so an unknown one is left blank instead.
func previewFields(fetched *booking.Booking, resolveAirport func(string) *string) *LookupPreview {
	preview := &LookupPreview{}

	outbound := fetched.Outbound
	if len(outbound) == 0 {
		outbound = fetched.Inbound
	}
	if len(outbound) > 0 {
		preview.OriginIATACode = resolveAirport(outbound[0].OriginIATA)
		preview.DestinationIATACode = resolveAirport(outbound[len(outbound)-1].DestinationIATA)
		if !outbound[0].DepartureAt.IsZero() {
			day := outbound[0].DepartureAt.Format("2006-01-02")
			preview.FlightDate = &day
		}
	}

	if len(fetched.Passengers) > 0 {
		main := fetched.Passengers[0]
		if name := strings.TrimSpace(cleanGivenName(main.FirstName) + " " + strings.TrimSpace(main.LastName)); name != "" {
			preview.MainPassengerName = &name
		}
	}

	if number := strings.TrimSpace(fetched.PurchaseNumber); number != "" {
		preview.LatamPurchaseNumber = &number
	}

	return preview
}
