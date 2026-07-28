package bot

import (
	"context"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/utils"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// EventHandler handles the necessary events from whatsmeow
func (b *Bot) EventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe {
			return
		}

		// One line per stanza that claims to change an earlier message - an edit,
		// a pin, a delete. Ordinary traffic logs nothing, and when a correction by
		// edit does not land this is what says whether it ever reached the bot.
		if v.Info.Edit != "" {
			b.Logger.Infof("received a stanza with edit=%q for message %s from %s",
				v.Info.Edit, v.Info.ID, v.Info.Sender)
		}

		// Read synchronously, right as the event arrives: whatsmeow
		// dispatches OfflineSyncPreview/Completed and the backlog of
		// *events.Message in between them in order, so this is the accurate
		// point to decide whether this particular message is part of that
		// backlog - not whenever the goroutine below happens to wake up.
		isBacklog := b.offlineSyncing.Load()

		// startedAt anchors the humanizer's response floor - captured here,
		// as close to the message's actual arrival as possible.
		startedAt := time.Now()

		go func() {
			humanizer.DelayMarkAsRead()

			b.processIncomingMessage(v, isBacklog, startedAt)
		}()

	case *events.OfflineSyncPreview:
		if v.Total > 0 {
			b.offlineSyncing.Store(true)
			b.Logger.Infof("catching up on %d message(s) missed while offline", v.Total)
		}

	case *events.OfflineSyncCompleted:
		b.offlineSyncing.Store(false)
		b.Logger.Infof("caught up on missed events (%d)", v.Count)

	case *events.Connected:
		jid := ""
		if id := b.currentSession().GetJID(); id != nil {
			jid = id.String()
		}
		b.Status.Set(Status{State: StateConnected, JID: jid})

	case *events.Disconnected:
		b.Status.Set(Status{State: StateDisconnected})

	case *events.LoggedOut:
		b.Logger.Infof("session logged out by the phone, reason: %v", v.Reason)
		b.Status.Set(Status{State: StateDisconnected})

		// whatsmeow dispatches this event from its own goroutine and only
		// deletes the device store afterwards, in the goroutine that
		// triggered the logout - the two aren't synchronized. Racing
		// RecreateSession against that delete is unreliable: asking for a
		// new device before the old row is actually gone hands back a
		// client that still thinks it has a paired ID (GetQRChannel then
		// refuses with "no user ID in the client's Store"). Deleting it
		// ourselves first - idempotent even if whatsmeow's own delete also
		// runs - gives RecreateSession a guaranteed-clean slate. Backgrounded
		// so a slow DB round-trip never blocks the event dispatch goroutine.
		go func() {
			ctx := context.Background()
			if err := b.currentSession().Session.DeleteStore(ctx); err != nil {
				b.Logger.Warnf("failed to delete stale device store: %v", err)
			}
			if err := b.RecreateSession(ctx); err != nil {
				b.Logger.Errorf("failed to set up a fresh session after remote logout: %v", err)
			}
		}()

	case *events.PairSuccess:
		// The socket reconnects right after this; *events.Connected is what
		// actually flips the status to StateConnected.
		b.Logger.Infof("pair success for %s, waiting for reconnect", v.ID)
	}

}

// processIncomingMessage marks the message read and, unless it arrived as
// part of catching up on a backlog after reconnecting, routes it: an edit, a
// command, an EMISSAO, an ALTERACAO, a CANCELAMENTO or a DELETAR. Extracted
// out of EventHandler's *events.Message case so tests can drive it directly
// without waiting on the randomized read delay above. startedAt is threaded
// through so the humanizer can enforce its response floor against the
// message's real arrival time, not whenever a given handler starts replying.
//
// None of the four carries a "!" prefix - all are ordinary text the operators
// were already writing among themselves - so unlike a command they cannot be
// recognized by addressing alone. isEmissaoMessage, isAlteracaoMessage,
// isCancelamentoMessage and isDeletarMessage are the cheap first filter;
// whether the bot may actually act on the message is decided in the handler,
// which checks that the chat is approved for monitoring before touching
// anything.
//
// The edit case comes first because an edited EMISSAO still looks like an
// EMISSAO: what makes it a correction rather than a new issuance is only that
// it replaced a message already sent.
func (b *Bot) processIncomingMessage(v *events.Message, isBacklog bool, startedAt time.Time) {
	// Ahead of marking as read, which already needs the id of the message the
	// group can see rather than the id of the edit stanza.
	b.unwrapEdit(v)

	b.MarkMessageAsRead(v)

	if isBacklog {
		return
	}

	rawText := utils.GetText(v.Message)

	switch {
	case v.IsEdit:
		b.handleMessageEdit(v, rawText, startedAt)
	case utils.IsCommand(rawText):
		b.RouteCommand(v, rawText, startedAt)
	case isEmissaoMessage(rawText):
		b.handleEmissao(v, rawText, startedAt)
	case isAlteracaoMessage(rawText):
		b.handleAlteracao(v, rawText, startedAt)
	case isCancelamentoMessage(rawText):
		b.handleCancelamento(v, rawText, startedAt)
	case isDeletarMessage(rawText):
		b.handleDeletar(v, rawText, startedAt)
	}
}

// isMessageEdit reports whether this stanza is a live edit of an earlier message.
//
// Info.Edit is the authoritative signal: whatsmeow fills it straight from the
// stanza's "edit" attribute in parseMessageInfo, before decrypting anything, for
// every incoming message. It is the only marker that does not depend on how the
// payload happens to be wrapped - and it is what separates an edit ("1") from the
// other things that ride the same ProtocolMessage shape: pinning ("2") and the two
// flavours of delete ("7"/"8"), none of which are corrections.
//
// Neither of the signals tried before holds on its own. v.IsEdit is raised only
// for the EditedMessage wrapper (UnwrapRaw), and the ProtocolMessage type enum is
// worse than useless when absent: MESSAGE_EDIT is 14, so a missing type reads back
// as REVOKE - and neither form is even what a live edit arrives in today.
func isMessageEdit(v *events.Message) bool {
	if v.Info.Edit == types.EditAttributeMessageEdit {
		return true
	}
	if v.Info.Edit != types.EditAttributeEmpty {
		return false
	}

	// No edit attribute at all: history sync builds the event without one, so fall
	// back to the payload - but only when it actually carries replacement content,
	// which a revoke never does.
	return v.Message.GetProtocolMessage().GetEditedMessage() != nil ||
		v.Message.GetSecretEncryptedMessage().GetSecretEncType() == waE2E.SecretEncryptedMessage_MESSAGE_EDIT
}

// unwrapEdit rewrites an edited message into the message it became, in place.
//
// An edit arrives under the id of the edit stanza rather than of the message
// everyone can see in the chat, and its payload is wrapped in something that is
// not the new text. Left as is, utils.GetText finds nothing at all and a reaction
// would be attached to an id nobody's client displays.
//
// Today's WhatsApp nests two envelopes, and both have to come off:
//
//   - a SecretEncryptedMessage, sealed with the message secret of the message
//     being edited. Only that message's recipients hold the secret, so nothing
//     downstream can read a word of the edit until this is opened;
//   - inside it, a ProtocolMessage of type MESSAGE_EDIT, whose editedMessage is
//     the new content and whose key names the message being replaced. whatsmeow
//     peels this one itself on the history-sync path (ParseWebMessage) but never
//     on the live one - and an edit that arrives that way has no outer envelope,
//     which is why each layer is optional rather than assumed.
//
// Doing it here, at the single point where messages enter, means every handler
// downstream - marking as read, reacting, replying, reading the text - keeps
// working without knowing edits exist. It is also why this runs inside
// processIncomingMessage rather than in the event dispatch: opening the envelope
// reads the stored secret, and that must never block whatsmeow's dispatch loop.
func (b *Bot) unwrapEdit(v *events.Message) {
	if !isMessageEdit(v) {
		return
	}

	if sealed := v.Message.GetSecretEncryptedMessage(); sealed != nil {
		opened, err := b.currentSession().Client.DecryptSecretEncryptedMessage(context.Background(), v)
		if err != nil {
			// Nothing downstream can do with an edit that stays sealed, and
			// answering the wrong thing would be worse than answering nothing.
			b.Logger.Errorf("edicao: failed to open the edit of message %s: %v",
				sealed.GetTargetMessageKey().GetID(), err)
			return
		}
		if original := sealed.GetTargetMessageKey().GetID(); original != "" {
			v.Info.ID = original
		}
		v.Message = opened
	}

	// Unwrapped whenever the content is there, never gated on the type enum.
	if protocol := v.Message.GetProtocolMessage(); protocol != nil {
		if original := protocol.GetKey().GetID(); original != "" {
			v.Info.ID = original
		}
		if edited := protocol.GetEditedMessage(); edited != nil {
			v.Message = edited
		}
	}

	// Raised here rather than read here, so processIncomingMessage keeps routing
	// on one flag whichever form the edit came in.
	v.IsEdit = true
}
