package bot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// handleEmissao registers the issuance an EMISSAO message describes. Unlike a
// "!" command it is not addressed at the bot at all - it is the note the
// operator would have written anyway - so the bot has to be conservative about
// which of them it acts on:
//
//   - only in a chat an admin approved (listened_chats), so the bot never writes
//     to the database because of a message in some unrelated group;
//   - only from a phone number that belongs to a user.
//
// Neither of those produces a visible response. Someone typing in a chat the
// bot merely happens to be in should not get a reaction telling them the bot is
// reading - the same reasoning as cmdRegistrarChat's silent branches.
func (b *Bot) handleEmissao(ctx context.Context, evt *events.Message, text string, startedAt time.Time) {
	user, scope, allowed := b.authorizeSender(evt, "emissao")
	if !allowed {
		return
	}

	msg, err := parseEmissao(text)
	if err != nil {
		b.Logger.Infof("emissao: could not parse a message from %s: %v", user.Name, err)
		b.refuse(ctx, evt, startedAt, refusalText("", err))
		return
	}

	// Resolution and creation share one transaction: an account auto-created
	// from a nickname must not survive a message that turns out to be
	// unpersistable, or the panel fills up with orphans nobody asked for.
	var created data.Issuance
	err = b.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		input, err := resolveEmissao(tx, msg, scope)
		if err != nil {
			return err
		}
		if input.AgencyID == nil {
			if input.AgencyID, err = defaultAgencyFor(tx, scope); err != nil {
				return err
			}
		}

		created, err = issuancesvc.Create(tx, input, user.ID)
		return err
	})

	if err != nil {
		b.Logger.Errorf("emissao: failed to register %s from %s: %v", msg.ProgramLocator, user.Name, err)
		b.refuse(ctx, evt, startedAt, refusalText(msg.ProgramLocator, err))
		return
	}

	// Remember which issuance this message produced, so editing it later is
	// recognised as a correction of that issuance rather than a new EMISSAO.
	b.issued.remember(evt.Info.ID, issuedMessage{IssuanceID: created.ID, ProgramLocator: msg.ProgramLocator})

	b.Logger.Infof("emissao: registered issuance %s (%s) from %s", created.ID, msg.ProgramLocator, user.Name)

	// The panel hears about the partial row before Traveos is even asked. The
	// fetch can take a minute or more, and a grid that shows nothing for that long
	// looks like a bot that swallowed the message.
	b.notifyIssuanceCreated()

	// force=false: this is the first fetch for a brand new issuance, so the mode
	// that refuses to touch an already-synced one is exactly right. The deliberate
	// "the flight changed" refetch is the panel's button, never a message.
	outcome := b.syncTraveos(ctx, evt, "emissao", created.ID, msg.ProgramLocator, false)
	if outcome == traveosFetched {
		b.notifyIssuanceUpdated()
	}

	// Only now, after the transaction committed and with the work done, does the
	// humanizer come in: it exists to pace what the group can see, never the
	// database work above.
	//
	// A reaction and nothing else. The group is where the team works, not where
	// the bot reports for duty, so a confirmation line saying what everyone can
	// already see is just noise in the scrollback. 🚀 means understood and done;
	// ⚠️ means saved but incomplete (see reactionPartial); only a refusal earns a
	// message, because that carries something a reaction cannot - see refuse.
	humanizer.DelayReaction()
	humanizer.EnforceResponseFloor(startedAt)
	b.ReactMessage(ctx, evt, successReaction(outcome))

	// The file comes after the answer, never as part of it: rendering and
	// uploading a PDF must not be able to turn a successful EMISSAO into a ⚠️.
	// Only a fetched itinerary produces one - there is no ticket to draw from an
	// issuance the vendor never confirmed.
	if outcome == traveosFetched {
		b.sendTicketPDF(ctx, evt, created.ID)
	}
}

// successReaction picks between "done" and "saved, but incomplete".
func successReaction(outcome traveosOutcome) string {
	if outcome.ok() {
		return "🚀"
	}
	return reactionPartial
}

// authorizeSender applies the two gates every unaddressed message has to pass
// before the bot acts on it, and returns the user who sent it. Shared by EMISSAO
// and ALTERACAO because the reasoning is identical for both, and so is the
// silence: neither refusal produces a reaction or a reply, so nobody typing in a
// group the bot merely happens to be in learns that it reads.
//
// flow only labels the log lines.
func (b *Bot) authorizeSender(evt *events.Message, flow string) (*data.User, access.AgencyScope, bool) {
	chatJID := evt.Info.Chat.String()

	listening, err := b.isListeningTo(chatJID)
	if err != nil {
		b.Logger.Errorf("%s: failed to check listened_chats for %s: %v", flow, chatJID, err)
		return nil, access.AgencyScope{}, false
	}
	if !listening {
		b.Logger.Infof("%s: chat %s is not approved for monitoring, ignoring", flow, chatJID)
		return nil, access.AgencyScope{}, false
	}

	user, err := b.resolveSenderUser(evt)
	if err != nil {
		b.Logger.Errorf("%s: failed to resolve sender %s: %v", flow, evt.Info.Sender, err)
		return nil, access.AgencyScope{}, false
	}
	if user == nil {
		b.Logger.Infof("%s: sender %s is not a known user, ignoring", flow, evt.Info.Sender)
		return nil, access.AgencyScope{}, false
	}

	// The bot acts on behalf of a panel user, so it is bound by the same agencies the panel
	// binds them to - otherwise WhatsApp would be the way around every grant. A failure to
	// read them is an outage, not a verdict, and stays silent like the gates above.
	scope, err := access.LoadAgencyScope(b.DB, *user)
	if err != nil {
		b.Logger.Errorf("%s: failed to load the agencies of %s: %v", flow, user.Name, err)
		return nil, access.AgencyScope{}, false
	}
	return user, scope, true
}

// defaultAgencyFor is the agency a new EMISSAO lands on when line 4 names none. It is settled
// here rather than left to issuancesvc.Create's fallback, which is the main agency - one the
// sender may not be allowed to see, so the issuance would vanish from their own panel.
func defaultAgencyFor(tx *gorm.DB, scope access.AgencyScope) (*string, error) {
	agencyID, err := access.ResolveDefaultAgency(tx, scope)
	if errors.Is(err, access.ErrNoAgencyAccess) {
		return nil, operatorErrorf(err, "Você não tem acesso a nenhuma agência")
	}
	if err != nil {
		return nil, err
	}
	return &agencyID, nil
}

// errIssuanceOutOfScope builds the refusal for an issuance the sender may not touch. It reads
// exactly like the one for a locator that matches nothing, so the group learns nothing about
// another agency's bookings.
func errIssuanceOutOfScope(locator string) error {
	return operatorErrorf(nil, "Não encontrei nenhuma emissão com o localizador %q", locator)
}

// isListeningTo reports whether the chat was approved by an admin and is
// currently being monitored. Both conditions matter: StopListenedChat leaves an
// approved row in place and only flips active off.
func (b *Bot) isListeningTo(chatJID string) (bool, error) {
	var count int64
	err := b.DB.Model(&data.ListenedChat{}).
		Where("chat_jid = ? AND status = ? AND active = ?", chatJID, "approved", true).
		Count(&count).Error

	return count > 0, err
}

// refusalText picks the text to send back for a rejected message - an EMISSAO,
// an ALTERACAO, a CANCELAMENTO or a DELETAR, whether it failed at the parser
// or inside the transaction.
//
// Only an operatorError is quoted as it stands - that type exists precisely to
// mark a message as written for the group. The typed failures the issuance
// service can raise get their own wording, and anything else is answered
// generically: a group chat is no place for a database error, and the real one
// is already in the log.
//
// locator is empty when the message never parsed far enough to have one.
func refusalText(locator string, err error) string {
	var operator operatorError
	if errors.As(err, &operator) {
		return operator.Error()
	}

	switch {
	case errors.Is(err, issuancesvc.ErrProgramLocatorTaken):
		return fmt.Sprintf("Já existe uma emissão ativa com o localizador %s", locator)
	case errors.Is(err, issuancesvc.ErrNoDefaultAgency):
		return "Nenhuma agência padrão está configurada no painel"
	case errors.Is(err, issuancesvc.ErrIssuanceAlreadyCancelled):
		return fmt.Sprintf("A emissão %s já está cancelada", locator)
	default:
		return "Não consegui registrar essa emissão, verifique com o suporte"
	}
}

// refuse answers a rejected message. It reacts with ❌ as well as replying so
// the failure is visible at a glance when scrolling back through a busy group,
// where the reply itself may be far from the message it refers to.
//
// A cancelled job is the one failure that says nothing. Its transaction was
// rolled back by whoever cancelled it - a DELETAR, or the operator deleting the
// message - and that person's own message is what answers the group. Telling
// them separately that the work they cancelled did not go through would be
// noise, and "Não consegui registrar essa emissão" would be an outright lie.
func (b *Bot) refuse(ctx context.Context, evt *events.Message, startedAt time.Time, message string) {
	if ctx.Err() != nil {
		b.Logger.Infof("message %s was cancelled, not answering: %s", evt.Info.ID, message)
		return
	}

	humanizer.DelayReaction()
	humanizer.EnforceResponseFloor(startedAt)
	b.ReactMessage(ctx, evt, "❌")
	b.ReplyMessage(ctx, evt, message)
}
