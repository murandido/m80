package issuance

import "errors"

// The sentinel errors below are the service's contract with its callers. They
// exist because this package has two consumers that report failures in
// completely different media - the REST handler turns them into HTTP statuses,
// the WhatsApp bot into a reply the operator reads on their phone - and neither
// can classify a failure from a formatted string alone.
//
// Their text is a developer-facing description, not the wording either caller
// shows: the API's own capitalized messages are restated in the handler's error
// mapping, since Go error strings are conventionally uncapitalized.
var (
	ErrLoyaltyProgramNotFound  = errors.New("loyalty program not found")
	ErrNoDefaultAgency         = errors.New("no default agency configured")
	ErrIssuerNotFound          = errors.New("issuer not found")
	ErrAccountNicknameNotFound = errors.New("account nickname not found")
	ErrAccountNicknameMismatch = errors.New("account nickname does not belong to the given account")
	ErrProgramLocatorTaken     = errors.New("program locator is already used by an active issuance")

	// ErrAirportNotFound is always wrapped with the offending IATA code, so the
	// bot can tell the operator which of the two codes they mistyped.
	ErrAirportNotFound = errors.New("airport not found")

	// The two below are only raised at the completion boundary - see
	// ValidateAirlineRequirements for why an incomplete issuance is allowed to
	// miss them.
	ErrLastNameRequired       = errors.New("this airline requires the main passenger's last name")
	ErrPurchaseNumberRequired = errors.New("this airline requires the purchase number")

	// ErrIssuanceAlreadyCancelled is raised by Cancel when the issuance handed
	// to it is already inactive - cancelling it twice would overwrite whatever
	// refund amounts the first cancellation recorded.
	ErrIssuanceAlreadyCancelled = errors.New("issuance already cancelled")

	// The rest are raised by Sync. None of them describe a Traveos failure: the
	// vendor's own errors keep their sentinels from internal/traveos, and the bot
	// in particular must not turn one into a claim about the cause, because every
	// emission failure but one arrives as the same opaque 500.
	ErrIssuanceNotFound = errors.New("issuance not found")

	// ErrAirlineNotSupported means someone turned Traveos off for this carrier -
	// a choice, not a gap. Distinct from ErrAirlineNotMapped, which is a gap
	// someone can go and fill.
	ErrAirlineNotSupported = errors.New("this airline is not integrated with traveos")

	// ErrAirlineNotMapped is always wrapped with the airline's name, since the fix
	// is to go and set its traveos_airline_id.
	ErrAirlineNotMapped = errors.New("this airline has no traveos id configured")

	// ErrLocatorMissing should be unreachable - program_locator is NOT NULL - but
	// a blank string would make us search Traveos for nothing and then issue a
	// ticket with an empty locator.
	ErrLocatorMissing = errors.New("issuance has no locator to look up")

	// ErrTicketNotFoundAfterEmission is the awkward case: Traveos reported the
	// emission as successful but the ticket is not in its own list. A record may
	// exist on their side, so this is deliberately not treated as a plain failure
	// that invites an immediate retry - that would risk a duplicate.
	ErrTicketNotFoundAfterEmission = errors.New("traveos issued the ticket but it is not in the ticket list")

	// ErrImportedNotAtVendor ends the sync of an imported issuance whose booking
	// Traveos does not know about. It is a dead end on purpose and not a failure to
	// retry: the alternative branch - issuing the ticket - would spend money on a
	// flight that already departed. See findOrEmit.
	ErrImportedNotAtVendor = errors.New("this issuance was imported and its booking is not at traveos")

	// ErrFlightMissingDeparture aborts the sync rather than writing a zero date
	// into issuance_flights.departure_at, which is NOT NULL. A leg with no
	// departure time is not a flight anyone can travel on.
	ErrFlightMissingDeparture = errors.New("traveos returned a flight with no departure time")
)
