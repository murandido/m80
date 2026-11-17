package issuance

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
)

// previewBookingTTL is how long a booking found by the panel's preview stays
// available to the Sync that follows the save.
//
// The window only has to cover the operator typing the values a draft needs before
// it can be saved (points, milheiro, account) - usually seconds, sometimes a few
// minutes. Fifteen minutes is generous for that and still short enough that the
// itinerary being written is one the airline returned moments ago.
const previewBookingTTL = 15 * time.Minute

// previewBookings is the process-wide cache Lookup fills and Sync drains. Only the
// API calls Lookup, so in the bot process it simply stays empty and every Sync
// misses, which is exactly the old behaviour.
//
// In-process on purpose: the API runs as a single container, and losing the cache
// on a restart only costs the provider call this exists to save.
var previewBookings = newPreviewBookingCache(previewBookingTTL, time.Now)

// previewBookingCache remembers the bookings the preview found, so the Sync right
// after the save does not ask the airline (or import at the vendor) a second time.
//
// Every entry is taken at most once: the first Sync that matches consumes it, and
// any later one - a "refetch" press, say - goes to the provider as it always did.
type previewBookingCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	entries map[string]*previewBookingEntry
}

type previewBookingEntry struct {
	booking    *booking.Booking
	source     string
	durationMS int
	expiresAt  time.Time
	// keys lists every key this entry is stored under, so taking it through one of
	// them removes the others too and the booking can never be applied twice.
	keys []string
}

func newPreviewBookingCache(ttl time.Duration, now func() time.Time) *previewBookingCache {
	return &previewBookingCache{ttl: ttl, now: now, entries: map[string]*previewBookingEntry{}}
}

// put stores a found booking under each of keys. The booking is detached from the
// caller's copy first (see detachBooking).
func (c *previewBookingCache) put(keys []string, fetched *booking.Booking, source string, durationMS int) {
	keys = slices.Compact(slices.Sorted(slices.Values(keys)))
	entry := &previewBookingEntry{
		booking:    detachBooking(fetched),
		source:     source,
		durationMS: durationMS,
		expiresAt:  c.now().Add(c.ttl),
		keys:       keys,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Swept here rather than by a goroutine: the map holds a handful of drafts at a
	// time, and a preview is the only thing that ever adds to it.
	now := c.now()
	for key, existing := range c.entries {
		if !now.Before(existing.expiresAt) {
			delete(c.entries, key)
		}
	}

	for _, key := range keys {
		// A newer preview of the same booking replaces the older one entirely, aliases
		// included, so a stale alias cannot outlive it.
		if previous, ok := c.entries[key]; ok {
			c.removeLocked(previous)
		}
		c.entries[key] = entry
	}
}

// take returns and removes the entry stored under key, if it has not expired.
func (c *previewBookingCache) take(key string) (*previewBookingEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.removeLocked(entry)
	if !c.now().Before(entry.expiresAt) {
		return nil, false
	}
	return entry, true
}

func (c *previewBookingCache) removeLocked(entry *previewBookingEntry) {
	for _, key := range entry.keys {
		if c.entries[key] == entry {
			delete(c.entries, key)
		}
	}
}

// detachBooking copies the parts of a booking that applyBooking rewrites in place.
//
// normaliseForStorage upper-cases the cabin of every leg and the names of every
// passenger by writing into the slice elements. A provider is free to hand back a
// booking it also keeps (a per-process cache of its own, say), so without the copy
// the Sync that consumes this entry would be editing memory the provider may hand
// to someone else. The nested slices (seats, baggage, raw services) are only ever
// read downstream, so sharing them is safe.
func detachBooking(fetched *booking.Booking) *booking.Booking {
	detached := *fetched
	detached.Outbound = slices.Clone(fetched.Outbound)
	detached.Inbound = slices.Clone(fetched.Inbound)
	detached.Passengers = slices.Clone(fetched.Passengers)
	return &detached
}

// previewKey identifies a booking by everything a provider is asked with: the same
// fields as the panel's attemptKey, run through the backend's own reading of them
// (BookingLastName decides which surname is sent) so a transient preview issuance
// and the saved one produce the same key.
func previewKey(airlineID string, issuance data.Issuance) string {
	return strings.Join([]string{
		airlineID,
		normaliseKeyPart(issuance.ProgramLocator),
		normaliseKeyPart(derefOrEmpty(issuance.AirlineLocator)),
		normaliseKeyPart(derefOrEmpty(issuance.LatamPurchaseNumber)),
		normaliseKeyPart(derefOrEmpty(issuance.OriginIATACode)),
		normaliseKeyPart(derefOrEmpty(issuance.BookingLastName())),
	}, "\x1f")
}

// previewKeys lists the keys a preview is stored under: the issuance as it was
// asked about, and the issuance as the panel saves it once the preview's answer is
// on the form.
//
// The second one is what makes the cache hit at all in the common case. The panel
// writes the preview's origin, full passenger name and purchase number back into
// the draft (use-issuance-board.ts), so the row that gets saved usually differs
// from the one that was looked up - most often because the surname became the full
// name, which BookingLastName then sends.
func previewKeys(airlineID string, asked data.Issuance, preview *LookupPreview) []string {
	saved := asked
	if preview.OriginIATACode != nil {
		saved.OriginIATACode = preview.OriginIATACode
	}
	if preview.MainPassengerName != nil {
		saved.MainPassengerName = preview.MainPassengerName
	}
	if preview.LatamPurchaseNumber != nil {
		saved.LatamPurchaseNumber = preview.LatamPurchaseNumber
	}
	return []string{previewKey(airlineID, asked), previewKey(airlineID, saved)}
}

func normaliseKeyPart(value string) string {
	return strings.ToUpper(strings.Join(strings.Fields(value), " "))
}
