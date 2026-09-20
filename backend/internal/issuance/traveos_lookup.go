package issuance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/traveos"
	"gorm.io/gorm"
)

// lookupTTL bounds how long a lookup waits for its issuance to be created. The pending
// row lives only in the operator's tab, so anything older than this belongs to a tab
// that was closed - and the ticket code fallback still covers a slow operator.
const lookupTTL = 2 * time.Hour

// LookupInput is what the panel knows about a booking before the issuance exists:
// exactly the fields Traveos is asked with, and nothing the operator types later.
type LookupInput struct {
	AirlineID           string
	ProgramLocator      string
	AirlineLocator      *string
	OriginIATACode      *string
	MainPassengerName   *string
	LatamPurchaseNumber *string
}

// LookupResult is the itinerary preview the grid fills its columns with while the
// operator is still typing points and prices.
//
// LookupID points at the full vendor payload held in LookupCache, which is what lets the
// create apply it without asking Traveos again. TicketCode is the fallback for when that
// entry is gone (a restart, or the TTL): reading a known ticket is one call, not a search.
type LookupResult struct {
	LookupID            string
	TicketCode          string
	FlightDate          *time.Time
	OriginIATACode      *string
	DestinationIATACode *string
	MainPassengerName   *string
	LatamPurchaseNumber *string
}

type lookupEntry struct {
	airlineID string
	ticket    *traveos.TicketItem
	details   *traveos.TicketData
	storedAt  time.Time
}

// LookupCache keeps what a lookup fetched until the issuance is created.
//
// In memory on purpose: the environment this serves runs a single API process, and
// losing an entry only costs the one TicketData call the ticket code fallback makes.
// Persisting a vendor payload for a row that may never be saved would be a table of
// abandoned drafts.
type LookupCache struct {
	mu      sync.Mutex
	entries map[string]lookupEntry
	now     func() time.Time
}

func NewLookupCache() *LookupCache {
	return &LookupCache{entries: map[string]lookupEntry{}, now: time.Now}
}

func (c *LookupCache) put(entry lookupEntry) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)

	c.mu.Lock()
	defer c.mu.Unlock()
	entry.storedAt = c.now()
	// Pruned on write rather than on a timer: abandoned drafts are the only thing that
	// accumulates here, and they only accumulate as fast as lookups are made.
	for key, existing := range c.entries {
		if c.now().Sub(existing.storedAt) > lookupTTL {
			delete(c.entries, key)
		}
	}
	c.entries[id] = entry
	return id, nil
}

// take hands the entry out once. A second create naming the same lookup would be a
// duplicated request, and it must not write the itinerary onto another issuance.
func (c *LookupCache) take(id string) (lookupEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[id]
	if !ok {
		return lookupEntry{}, false
	}
	delete(c.entries, id)
	if c.now().Sub(entry.storedAt) > lookupTTL {
		return lookupEntry{}, false
	}
	return entry, true
}

// Lookup fetches a booking from Traveos for an issuance that does not exist yet.
//
// It goes through the same findOrEmit as Sync, so the search-before-emission guarantee
// and the airline's required-field gate hold here too. Nothing is written to the
// database: the payload waits in the cache for ApplyLookup.
func Lookup(
	ctx context.Context,
	db *gorm.DB,
	client TraveosClient,
	cache *LookupCache,
	in LookupInput,
) (*LookupResult, error) {
	var airline data.Airline
	if err := db.WithContext(ctx).First(&airline, "id = ?", in.AirlineID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAirlineNotSupported
		}
		return nil, err
	}
	if !airline.TraveosSupported {
		return nil, ErrAirlineNotSupported
	}
	if airline.TraveosAirlineID == nil {
		return nil, fmt.Errorf("%w: %s", ErrAirlineNotMapped, airline.Name)
	}

	// Only ever held in memory - it is the shape findOrEmit reads its fields from.
	draft := data.Issuance{
		AirlineID:           airline.ID,
		ProgramLocator:      strings.TrimSpace(in.ProgramLocator),
		AirlineLocator:      in.AirlineLocator,
		OriginIATACode:      in.OriginIATACode,
		MainPassengerName:   in.MainPassengerName,
		LatamPurchaseNumber: in.LatamPurchaseNumber,
	}

	ticket, err := findOrEmit(ctx, client, draft, airline, false)
	if err != nil {
		return nil, err
	}

	details, err := client.TicketData(ctx, ticket.Code)
	if err != nil {
		return nil, fmt.Errorf("fetching traveos ticket data: %w", err)
	}

	lookupID, err := cache.put(lookupEntry{airlineID: airline.ID, ticket: ticket, details: details})
	if err != nil {
		return nil, err
	}

	result := previewTicketData(details)
	result.LookupID = lookupID
	result.TicketCode = ticket.Code
	return &result, nil
}

// previewTicketData reads the columns the grid shows off the vendor payload, following
// the same rules applyIssuanceFields uses when it writes them: origin from the first
// leg, destination from the end of the outbound, the first passenger as the main one.
// Airport codes are not checked against our table here - the preview is only shown,
// and the real write in ApplyLookup resolves them.
func previewTicketData(details *traveos.TicketData) LookupResult {
	var result LookupResult

	legs := append(append([]traveos.Flight{}, details.Flights.Outbound...), details.Flights.Inbound...)
	outbound := details.Flights.Outbound
	if len(outbound) == 0 {
		outbound = legs
	}

	if len(legs) > 0 {
		if !legs[0].DepartureAt.IsZero() {
			departure := legs[0].DepartureAt.Time
			result.FlightDate = &departure
		}
		result.OriginIATACode = previewAirportCode(legs[0].OriginDetails.IATA, legs[0].OriginIATA)
	}
	if len(outbound) > 0 {
		last := outbound[len(outbound)-1]
		result.DestinationIATACode = previewAirportCode(last.DestinationDetails.IATA, last.DestinationIATA)
	}

	if len(details.Emission.Passengers) > 0 {
		main := details.Emission.Passengers[0]
		if name := strings.TrimSpace(cleanGivenName(main.FirstName) + " " + strings.TrimSpace(main.LastName)); name != "" {
			result.MainPassengerName = &name
		}
	}
	if number := strings.TrimSpace(details.Emission.PurchaseNumber); number != "" {
		result.LatamPurchaseNumber = &number
	}

	return result
}

func previewAirportCode(values ...string) *string {
	code := strings.ToUpper(strings.TrimSpace(firstNonEmpty(values...)))
	if code == "" {
		return nil
	}
	return &code
}

// ApplyLookup writes a lookup's itinerary onto the issuance just created from it.
//
// With the entry still cached this makes no vendor call at all. Without it, the ticket
// code is enough to read the booking directly - never a search, never an emission. A
// lookup for another airline than the issuance ended up on is ignored: the operator
// changed the carrier after the lookup, and that itinerary is not this booking.
//
// Separate from Create and run after it commits: a failure here must not cost the
// operator the issuance, only the autofill, which the refetch button recovers.
func ApplyLookup(
	ctx context.Context,
	db *gorm.DB,
	client TraveosClient,
	cache *LookupCache,
	issuanceID, lookupID, ticketCode string,
) error {
	var issuance data.Issuance
	if err := db.WithContext(ctx).First(&issuance, "id = ?", issuanceID).Error; err != nil {
		return err
	}

	var ticket *traveos.TicketItem
	var details *traveos.TicketData

	entry, cached := cache.take(lookupID)
	switch {
	case cached && entry.airlineID != issuance.AirlineID:
		slog.Warn("traveos lookup was for another airline, not applying it",
			"issuance_id", issuanceID, "lookup_airline_id", entry.airlineID)
		return nil
	case cached:
		ticket, details = entry.ticket, entry.details
	case strings.TrimSpace(ticketCode) != "" && client != nil:
		fetched, err := client.TicketData(ctx, strings.TrimSpace(ticketCode))
		if err != nil {
			return fmt.Errorf("fetching traveos ticket data: %w", err)
		}
		ticket, details = &traveos.TicketItem{Code: strings.TrimSpace(ticketCode)}, fetched
	default:
		return nil
	}

	var airline data.Airline
	if err := db.WithContext(ctx).First(&airline, "id = ?", issuance.AirlineID).Error; err != nil {
		return err
	}

	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return applyTraveosData(tx, &issuance, &airline, ticket, details)
	}); err != nil {
		return err
	}
	return MarkTraveosAttempted(db, issuanceID)
}
