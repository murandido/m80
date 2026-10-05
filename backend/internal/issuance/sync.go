package issuance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// Sync fills an issuance with the real itinerary: passengers, flights, seats and
// baggage, plus the route and trip type that the operator's own input never
// carries.
//
// It talks to a booking.Registry rather than to Traveos, because the itinerary is
// moving onto the airlines' own endpoints one carrier at a time and this function
// is the seam. What a provider is, and why the fetch can be followed by an import
// for exactly one of them, is in internal/booking.
//
// # Which provider answers
//
// The airline's own integration is asked first when it is both built and switched
// on, and Traveos catches it only when the airline could not be reached at all.
// A booking the airline says it does not have is an answer and ends there - see
// attemptProviders for why falling back on that one would spend money.
//
// # force
//
// force=false is the automatic path - the bot after an EMISSAO, and the panel's
// autocomplete. It does nothing when the issuance already has Traveos data,
// because there is nothing it could be trying to fix.
//
// force=true is the operator pressing "refetch" on purpose, and it means
// something different: not "this failed, try again" but "the flight changed,
// fetch the new data". A rebooked time, a swapped aircraft, a reaccommodation -
// the ticket is unchanged at the vendor, and it is our copy that is stale. So it
// proceeds even on an already-synced issuance, and asks Traveos to re-check the
// booking against the airline first rather than answering from its own snapshot.
func Sync(ctx context.Context, db *gorm.DB, registry *booking.Registry, issuanceID string, force bool) error {
	var issuance data.Issuance
	if err := db.WithContext(ctx).First(&issuance, "id = ?", issuanceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrIssuanceNotFound
		}
		return err
	}

	if !force && issuance.TraveosSyncedAt != nil {
		return nil
	}

	var airline data.Airline
	if err := db.WithContext(ctx).First(&airline, "id = ?", issuance.AirlineID).Error; err != nil {
		return err
	}

	providers := resolveProviders(registry, airline)
	if len(providers) == 0 {
		return ErrAirlineNotSupported
	}

	// Only asked of an airline whose sole provider is the vendor. Two refusals,
	// deliberately distinct: "not supported" is a choice someone made in the
	// airline CRUD, "not mapped" is a gap nobody filled in, and only the second one
	// is something an operator can go and fix. A carrier with its own integration
	// does not need the vendor's catalog id to be read, so demanding it here would
	// refuse a lookup that was going to work.
	if len(providers) == 1 && providers[0] == registry.Fallback() && airline.TraveosAirlineID == nil {
		return fmt.Errorf("%w: %s", ErrAirlineNotMapped, airline.Name)
	}

	// Past this line the vendor is involved, which can take minutes. Recording it on
	// the row is what lets every panel - not only the browser that asked for it, and
	// not only for as long as that browser stays on the screen - show the issuance as
	// busy. The clear runs on every exit, including a failure or a timeout, and
	// deliberately without ctx: an expired context must not skip the cleanup.
	if err := MarkTraveosSyncing(db.WithContext(ctx), issuanceID); err != nil {
		return err
	}
	var attempts []data.BookingLookupAttempt
	syncID := uuid.New().String()
	defer func() {
		recordLookupAttempts(db, attempts)
		if err := ClearTraveosSyncing(db, issuanceID); err != nil {
			slog.Error("failed to clear the traveos syncing marker",
				"issuance_id", issuanceID, "err", err)
		}
		// Recorded on every exit, success or not, and deliberately without ctx: the
		// point of the mark is that a lookup which found nothing is not repeated, and
		// a lookup that timed out is exactly the one worth not repeating on its own.
		if err := MarkTraveosAttempted(db, issuanceID); err != nil {
			slog.Error("failed to record the traveos attempt",
				"issuance_id", issuanceID, "err", err)
		}
	}()

	fetched, source, err := attemptProviders(ctx, registry, providers, issuance, airline, force,
		func(attempt data.BookingLookupAttempt) {
			attempt.SyncID = syncID
			attempts = append(attempts, attempt)
		})
	if err != nil {
		return err
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return applyBooking(tx, &issuance, &airline, fetched, source)
	})
}

// MarkTraveosSyncing records that a pull is starting. Exported because the panel's
// route marks the row itself before calling Sync: the sync is synchronous and can
// run for minutes, so the "started" event has to go out before it, not after.
// Marking twice is harmless - the second write just refreshes the timestamp.
//
// UpdateColumn rather than Update: this is bookkeeping about the row, not an edit
// of it, and bumping updated_at would make it look like somebody changed the data.
func MarkTraveosSyncing(db *gorm.DB, issuanceID string) error {
	return db.Model(&data.Issuance{}).
		Where("id = ?", issuanceID).
		UpdateColumn("traveos_syncing_at", time.Now()).Error
}

// MarkTraveosAttempted records that the vendor has been asked about this issuance.
// Distinct from traveos_synced_at, which only ever records a lookup that found
// something: this one also remembers the ones that came back empty, which is what
// stops the panel from asking again about a booking Traveos does not have.
//
// UpdateColumn for the same reason as its neighbours: bookkeeping about the row is
// not an edit of it, and bumping updated_at would read as somebody changing data.
func MarkTraveosAttempted(db *gorm.DB, issuanceID string) error {
	return db.Model(&data.Issuance{}).
		Where("id = ?", issuanceID).
		UpdateColumn("traveos_attempted_at", time.Now()).Error
}

// ClearTraveosSyncing takes the marker off. Exported for the same reason as its
// counterpart: a caller that marked the row has to be able to unmark it when the
// sync refuses before ever reaching the vendor.
func ClearTraveosSyncing(db *gorm.DB, issuanceID string) error {
	return db.Model(&data.Issuance{}).
		Where("id = ?", issuanceID).
		UpdateColumn("traveos_syncing_at", nil).Error
}

// resolveProviders lists who may be asked about this airline, in the order they
// are asked.
//
// The airline's own integration comes first when it is both registered and
// switched on; the vendor comes after, and only while the carrier is still marked
// as supported. Both at once is the normal state during the migration: the direct
// integration does the work and Traveos is the net underneath it. Neither is the
// state of a carrier nobody integrated, and the caller reports that as such.
func resolveProviders(registry *booking.Registry, airline data.Airline) []booking.Provider {
	var providers []booking.Provider

	// Registered and enabled are two different things on purpose: a new carrier
	// ships dark and is switched on from the airline screen, with no release in
	// between.
	if airline.DirectBookingEnabled {
		if direct, ok := registry.Direct(airline.Code); ok {
			providers = append(providers, direct)
		}
	}

	if airline.TraveosSupported {
		if fallback := registry.Fallback(); fallback != nil {
			providers = append(providers, fallback)
		}
	}

	return providers
}

// attemptProviders asks each provider in turn and returns the first booking, plus
// the name of whoever gave it up.
//
// ErrUnavailable moves on to the next provider with everything allowed. ErrNotFound
// moves on too, but from then on the later providers may only *search*: a direct
// integration identifies the booking by more than the locator - GOL wants the origin
// and a surname that match - so its "no such booking" is often a typo in one of those,
// and the vendor, which searches by locator alone, may well have it. What must never
// follow that answer is an *import*: a real, non-idempotent ticket created at the
// vendor for a locator the airline just said does not exist.
//
// Anything else is a refusal this layer already made on purpose, like an imported row
// or a missing required field, and repeating it against another provider would only
// produce the same refusal twice.
//
// When nobody finds the booking, the first "no such booking" is what is returned: it
// came from the airline and names what to check, which the vendor's does not.
func attemptProviders(
	ctx context.Context,
	registry *booking.Registry,
	providers []booking.Provider,
	issuance data.Issuance,
	airline data.Airline,
	force bool,
	record func(data.BookingLookupAttempt),
) (*booking.Booking, string, error) {
	var lastErr, absent error

	for _, provider := range providers {
		var (
			fetched *booking.Booking
			err     error
		)
		started := time.Now()
		if absent == nil {
			fetched, err = fetchOrImport(ctx, provider, issuance, airline, force)
		} else {
			fetched, err = searchOnly(ctx, provider, issuance, airline, force)
		}
		if record != nil {
			record(lookupAttempt(issuance, airline, provider.Name(), err, time.Since(started)))
		}

		// Reported before the error is even classified: this is the panel's freshest
		// evidence about whether a carrier is answering, and it is worth having from
		// the lookups that are happening anyway rather than only from a slow probe.
		registry.Observed(provider.Name(), err)

		switch {
		case err == nil:
			if absent != nil {
				slog.Warn("the airline did not find the booking but the vendor did - check the origin and surname on the issuance",
					"issuance_id", issuance.ID, "airline", airline.Code, "provider", provider.Name(), "airline_err", absent)
			}
			return fetched, provider.Name(), nil

		case errors.Is(err, booking.ErrUnavailable):
			lastErr = err
			slog.Warn("booking provider could not be reached, trying the next one",
				"issuance_id", issuance.ID, "airline", airline.Code, "provider", provider.Name(), "err", err)

		case errors.Is(err, booking.ErrNotFound):
			if absent == nil {
				absent = err
			}

		default:
			return nil, "", err
		}
	}

	if absent != nil {
		return nil, "", absent
	}
	return nil, "", lastErr
}

// searchOnly reads the booking from a provider without ever importing it. It is what
// a provider is allowed to do once an earlier one has answered that the booking does
// not exist - see attemptProviders.
func searchOnly(
	ctx context.Context,
	provider booking.Provider,
	issuance data.Issuance,
	airline data.Airline,
	force bool,
) (*booking.Booking, error) {
	request, err := bookingRequest(issuance, airline, force)
	if err != nil {
		return nil, err
	}
	return provider.Fetch(ctx, request)
}

// fetchOrImport returns the booking for this issuance, importing it into the
// provider first if that provider needs it and does not have it yet.
//
// # Why the search comes before the import
//
// Only Traveos has an import, and POST /tickets/emission is not idempotent:
// issuing the same locator twice returns success twice and creates two records on
// the vendor's side, with no idempotency key and no server-side duplicate check
// to fall back on. Its response carries no ticket code either, so reading the
// ticket back afterwards is mandatory regardless.
//
// Doing the lookup *first* makes every retry path safe for free, with no
// bookkeeping column needed to remember whether the import already happened: if
// the booking is already there, we skip straight to reading it. A provider that
// reads the airline directly implements no import at all, so for those this is
// simply one call.
func fetchOrImport(
	ctx context.Context,
	provider booking.Provider,
	issuance data.Issuance,
	airline data.Airline,
	force bool,
) (*booking.Booking, error) {
	request, err := bookingRequest(issuance, airline, force)
	if err != nil {
		return nil, err
	}

	fetched, err := provider.Fetch(ctx, request)
	if err == nil {
		return fetched, nil
	}
	if !errors.Is(err, booking.ErrNotFound) {
		return nil, err
	}

	// No import to attempt: the airline itself says there is no such booking. Whether
	// another provider may still search for it is attemptProviders' call, and it never
	// lets one import after this answer.
	importer, ok := provider.(booking.Importer)
	if !ok {
		return nil, err
	}

	// An imported issuance describes a booking made outside the system, usually months
	// earlier: the ticket exists at the airline whether or not the vendor knows about it.
	// Importing here would not be "fetching the itinerary", it would be buying a second
	// ticket for a flight that has already happened. Searching is the whole of what an
	// imported row is ever allowed to do.
	if issuance.Imported {
		return nil, ErrImportedNotAtVendor
	}

	// The import is not idempotent and answers nearly every failure with the same
	// opaque 500, so calling it with a field the airline itself requires still empty
	// burns a real ticket at the vendor to learn nothing - and the operator gets a
	// warning that names no cause, because the vendor named none. The flags are
	// already known here, so the refusal costs nothing.
	//
	// This sits after the search rather than before it: a booking that is already on
	// the vendor's side is read from their list, and none of these fields are used to
	// do it. Only the path that actually spends something is gated.
	if err := ValidateAirlineRequirements(airline, issuance.BookingLastName(), issuance.LatamPurchaseNumber); err != nil {
		return nil, err
	}

	if err := importer.EnsureAvailable(ctx, request); err != nil {
		return nil, err
	}

	fetched, err = provider.Fetch(ctx, request)
	if err != nil {
		if errors.Is(err, booking.ErrNotFound) {
			// The provider said the import succeeded but the booking is not there.
			// Worth its own error: unlike a failed import, a record may well exist on
			// the vendor's side, so a blind retry could duplicate it.
			return nil, fmt.Errorf("%w: %s", ErrTicketNotFoundAfterEmission, request.Locator)
		}
		return nil, err
	}
	return fetched, nil
}

// bookingRequest is what every provider is asked, built from the issuance.
func bookingRequest(issuance data.Issuance, airline data.Airline, force bool) (booking.Request, error) {
	locator := bookingLocator(issuance)
	if locator == "" {
		return booking.Request{}, ErrLocatorMissing
	}

	return booking.Request{
		AirlineCode: airline.Code,
		Locator:     locator,
		OriginIATA:  derefOrEmpty(issuance.OriginIATACode),
		// The surname only - a given name in this field makes the Traveos scrape
		// fail with the same opaque 500 as everything else, and several airline
		// endpoints reject it outright. The bot's EMISSAO line 2 already carries
		// just the surname, so it is passed through verbatim rather than guessed
		// at: there is no way to tell a compound surname from a given name plus
		// surname, and stripping the wrong token is worse than sending what the
		// operator typed.
		// The surname typed for the lookup wins over the passenger name, which the
		// first sync replaces with the full name - see data.Issuance.BookingLastName.
		LastName:        derefOrEmpty(issuance.BookingLastName()),
		PurchaseNumber:  derefOrEmpty(issuance.LatamPurchaseNumber),
		VendorAirlineID: airline.TraveosAirlineID,
		Force:           force,
	}, nil
}

// bookingLocator picks which of our two locators identifies the booking at the
// airline. The airline's own PNR wins when we have it: the program locator is
// the loyalty program's reference, and every provider here reaches the airline.
func bookingLocator(issuance data.Issuance) string {
	if issuance.AirlineLocator != nil && strings.TrimSpace(*issuance.AirlineLocator) != "" {
		return strings.TrimSpace(*issuance.AirlineLocator)
	}
	return strings.TrimSpace(issuance.ProgramLocator)
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// maxAttemptErrorLength keeps a vendor's HTML error page from landing whole in a
// diagnostic row; the start of the message is what names the cause.
const maxAttemptErrorLength = 500

// lookupAttempt describes one provider's answer for booking_lookup_attempts - temporary
// scaffolding for the move off Traveos, see migration 66.
func lookupAttempt(issuance data.Issuance, airline data.Airline, provider string, err error, took time.Duration) data.BookingLookupAttempt {
	attempt := data.BookingLookupAttempt{
		IssuanceID: issuance.ID,
		AirlineID:  airline.ID,
		Provider:   provider,
		Outcome:    data.LookupFound,
		DurationMS: int(took.Milliseconds()),
	}
	switch {
	case err == nil:
		return attempt
	case errors.Is(err, booking.ErrUnavailable):
		attempt.Outcome = data.LookupUnavailable
	case errors.Is(err, booking.ErrNotFound):
		attempt.Outcome = data.LookupNotFound
	default:
		attempt.Outcome = data.LookupRefused
	}
	message := err.Error()
	if len(message) > maxAttemptErrorLength {
		message = message[:maxAttemptErrorLength]
	}
	attempt.Error = &message
	return attempt
}

// recordLookupAttempts writes what the providers answered. Best-effort and without ctx,
// like the other bookkeeping on Sync's way out: it is evidence for deciding when an
// airline can drop Traveos, and losing a row of it must never fail the sync it describes.
func recordLookupAttempts(db *gorm.DB, attempts []data.BookingLookupAttempt) {
	if len(attempts) == 0 {
		return
	}
	if err := db.Create(&attempts).Error; err != nil {
		slog.Error("failed to record the booking lookup attempts",
			"issuance_id", attempts[0].IssuanceID, "err", err)
	}
}
