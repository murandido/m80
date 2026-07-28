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
)
