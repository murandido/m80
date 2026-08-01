package bot

import (
	"context"
	"strings"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/ticket"
	"github.com/murandido/m-80-backend/internal/utils"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// sendTicketPDF renders the ticket and posts it into the chat, when that chat
// asked for it with !pdf.
//
// # Why the bot generates it instead of calling the API
//
// The Notion doc has the bot fetching GET /api/issuances/{id}/pdf over HTTP.
// That route is JWT-gated and the bot has no user; adding an auth path for it,
// or an unauthenticated internal route, is more surface than the hop is worth.
// The bot imports the same two packages the route imports, so the property that
// decision wanted - one generator, never two ticket layouts - still holds. Only
// the HTTP hop is gone.
//
// # Why it runs after the reaction
//
// The group has already been told the emission worked. Rendering and uploading a
// file is not part of that answer, and a PDF service having a bad minute must
// never turn a successful EMISSAO into a ⚠️. So every failure here is a log line
// and nothing else.
func (b *Bot) sendTicketPDF(ctx context.Context, evt *events.Message, issuanceID string) {
	chatJID := evt.Info.Chat.String()
	if !b.pdfModeEnabled(ctx, chatJID) {
		return
	}

	issuance, seats, err := loadIssuanceForTicket(b.DB.WithContext(ctx), issuanceID)
	if err != nil {
		b.Logger.Errorf("pdf: failed to load issuance %s for the ticket: %v", issuanceID, err)
		return
	}
	if issuance.TraveosSyncedAt == nil {
		// Unreachable from the caller, which only runs after a successful sync.
		// Kept because a ticket without Traveos data is a blank document, and a
		// blank document sent to a client is worse than no document.
		b.Logger.Infof("pdf: issuance %s has no traveos data, not sending a ticket", issuanceID)
		return
	}

	document, err := ticket.Render(ticket.FromIssuance(issuance, seats))
	if err != nil {
		b.Logger.Errorf("pdf: failed to render the ticket for %s: %v", issuance.ProgramLocator, err)
		return
	}

	pdf, err := utils.GeneratePDF(document)
	if err != nil {
		b.Logger.Errorf("pdf: failed to generate the ticket pdf for %s: %v", issuance.ProgramLocator, err)
		return
	}

	locator := strings.TrimSpace(issuance.ProgramLocator)
	if issuance.AirlineLocator != nil && strings.TrimSpace(*issuance.AirlineLocator) != "" {
		locator = strings.TrimSpace(*issuance.AirlineLocator)
	}

	// Composing before a file, same as before text: the humanizer paces what the
	// group sees, and a document appearing with no warning reads as a bot, which
	// is the one thing this pipeline is built to avoid.
	b.SendPresenceTyping(ctx, evt)
	humanizer.DelayComposing()

	if err := b.SendDocument(ctx, evt, pdf, "bilhete-"+locator+".pdf", "application/pdf"); err != nil {
		b.Logger.Errorf("pdf: failed to send the ticket for %s: %v", locator, err)
		return
	}
	b.Logger.Infof("pdf: sent the ticket for %s to %s", locator, chatJID)
}

// loadIssuanceForTicket fetches everything the ticket draws, in two queries. It
// is the bot's own copy of the handler's loader rather than a shared helper:
// internal/api is the HTTP layer and internal/bot must not import it (see the
// layering rule in internal/bot/CLAUDE.md), and the alternative - a third package
// holding six lines of Preload - buys less than it costs.
func loadIssuanceForTicket(db *gorm.DB, id string) (data.Issuance, map[string][]data.IssuancePassengerFlight, error) {
	var issuance data.Issuance

	err := db.
		Preload("Airline").
		Preload("Flights", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("direction asc, leg_order asc")
		}).
		Preload("Flights.OriginAirport").
		Preload("Flights.DestinationAirport").
		Preload("Passengers", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("position asc")
		}).
		First(&issuance, "id = ?", id).Error
	if err != nil {
		return issuance, nil, err
	}

	passengerIDs := make([]string, 0, len(issuance.Passengers))
	for _, passenger := range issuance.Passengers {
		passengerIDs = append(passengerIDs, passenger.ID)
	}

	seats := make(map[string][]data.IssuancePassengerFlight, len(passengerIDs))
	if len(passengerIDs) == 0 {
		return issuance, seats, nil
	}

	var rows []data.IssuancePassengerFlight
	if err := db.Where("passenger_id IN ?", passengerIDs).Find(&rows).Error; err != nil {
		return issuance, nil, err
	}
	for _, row := range rows {
		seats[row.PassengerID] = append(seats[row.PassengerID], row)
	}
	return issuance, seats, nil
}
