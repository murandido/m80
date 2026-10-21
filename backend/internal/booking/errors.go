package booking

import (
	"errors"
	"fmt"
)

// The two sentinels below are the axis of the whole migration off Traveos, so
// they are worth stating plainly: every failure a provider can have is either
// "the airline answered, and the answer is no" or "we never reached the
// airline", and the two mean opposite things everywhere downstream.
//
//   - ErrNotFound is an answer. The provider is healthy; the locator is wrong,
//     the booking is gone, or - for an airline that also checks the origin and
//     the surname - one of those does not match. The next provider may still
//     search for it, but never import it (see issuance.attemptProviders), and
//     marking the carrier down over it would paint the panel red every time
//     somebody typos a PNR.
//
//   - ErrUnavailable is the absence of an answer: an edge block, a refused
//     token, a timeout, a body that did not parse. Here a fallback is exactly
//     right, and the carrier's light should go out.
//
// The same test tells the two apart on the wire: a JSON body means the request
// reached the airline's application, an HTML "Access Denied" from the edge
// means it did not.
var (
	ErrNotFound    = errors.New("booking: the provider has no such booking")
	ErrUnavailable = errors.New("booking: the provider could not be reached")
)

// Unavailable marks a transport-level failure as ErrUnavailable while keeping
// the original cause in the chain.
//
// The cause is joined rather than formatted in, and that matters beyond tidiness:
// the issuance handler answers several of internal/traveos' own sentinels with
// their own HTTP status, and flattening the cause into a string would silently
// demote every one of them to the generic 502. cause may be nil.
//
// It exists so a provider never has to remember the wrapping verb: the mistake
// that matters here is returning a bare error, which the sync would then have no
// way to classify and would have to treat as fatal.
func Unavailable(cause error, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if cause == nil {
		return fmt.Errorf("%w: %s", ErrUnavailable, message)
	}
	return fmt.Errorf("%w: %s: %w", ErrUnavailable, message, cause)
}

// NotFound wraps a business-level "no such booking" the same way.
func NotFound(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrNotFound, fmt.Sprintf(format, args...))
}

// Reached reports whether an error means the provider's application answered.
//
// Success and ErrNotFound both count: the point of this question is health, and
// an airline that correctly rejects a made-up locator is an airline that is up.
// It is what both the synthetic probe and the observed-health recorder ask.
func Reached(err error) bool {
	return err == nil || errors.Is(err, ErrNotFound)
}
