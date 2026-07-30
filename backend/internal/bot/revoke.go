package bot

import (
	"context"
	"errors"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// isMessageRevoke reports whether this stanza deletes an earlier message for
// everyone.
//
// Same authoritative signal as an edit, a different value: whatsmeow fills
// Info.Edit from the stanza attribute before decrypting anything, and "7" is the
// sender deleting their own message while "8" is a group admin deleting someone
// else's. Both mean the message is gone from every phone in the chat.
//
// The fallback covers history sync, which builds the event without the
// attribute. It insists on a target key, because that is the whole content of a
// revoke - unlike an edit, it carries no replacement text, and a ProtocolMessage
// with no type at all reads back as REVOKE (the enum's zero value), so trusting
// the type alone would swallow anything malformed.
func isMessageRevoke(v *events.Message) bool {
	switch v.Info.Edit {
	case types.EditAttributeSenderRevoke, types.EditAttributeAdminRevoke:
		return true
	case types.EditAttributeEmpty:
		protocol := v.Message.GetProtocolMessage()
		return protocol.GetType() == waE2E.ProtocolMessage_REVOKE && protocol.GetKey().GetID() != ""
	default:
		return false
	}
}

// revokedMessageID is the id of the message that was deleted.
//
// The documented place is the ProtocolMessage key, which is what whatsmeow's own
// BuildRevoke fills in. Real deletions sent from WhatsApp Web do not always
// arrive that way: one observed live carried edit="7" and no readable
// ProtocolMessage at all, addressing the target through the stanza id instead -
// the same trick an edit stanza uses.
//
// Falling back to Info.ID is safe rather than a guess. The stanza id is either
// the deleted message's - in which case it is exactly right - or a fresh id of
// its own, in which case nothing in either map is filed under it and the lookup
// simply finds nothing. A wrong id cannot delete the wrong issuance; it can only
// find none.
func revokedMessageID(v *events.Message) string {
	if id := v.Message.GetProtocolMessage().GetKey().GetID(); id != "" {
		return id
	}
	return string(v.Info.ID)
}

// handleMessageRevoke undoes whatever the deleted message had set in motion.
//
// A revoke carries no text at all, only the id of the message it removed, so the
// only way to connect it to an issuance is through the two maps the bot keeps:
// the job registry while the work is in flight, and issued once it committed.
// That is a hard limit, not a shortcut - there is no PNR in the stanza to look
// up. A message older than the issued map, or from before the last restart, is
// therefore left alone.
//
// The same three states as DELETAR, and for the same reason cancelled first and
// checked afterwards: a job cancelled a moment too late leaves a committed row,
// and the second half of this finds it.
//
//  1. queued or running: the job is cancelled. Nothing was written.
//  2. already saved: soft delete, exactly what DELETAR does to the same row.
//  3. anything else - small talk, a message the bot never acted on, a message it
//     no longer remembers: nothing happens, and the reason goes to the log.
//
// None of it reacts. The message the bot would react to is the one that was just
// deleted; there is nothing left in the chat to attach an emoji to. Only a
// failure speaks, as a plain line in the chat, because an issuance that silently
// survived the deletion of the message that created it is a phantom nobody would
// think to look for.
func (b *Bot) handleMessageRevoke(ctx context.Context, evt *events.Message, _ time.Time) {
	user, allowed := b.authorizeSender(evt, "delecao")
	if !allowed {
		return
	}

	target := revokedMessageID(evt)
	if target == "" {
		b.Logger.Warnf("delecao: revoke stanza %s names no message, ignoring", evt.Info.ID)
		return
	}
	b.Logger.Infof("delecao: %s deleted message %s", user.Name, target)

	if live, found := b.jobs().lookupByMessage(target); found && live.cancel(false) {
		b.Logger.Infof("delecao: %s deleted message %s while its job was still in flight, cancelling it",
			user.Name, target)
		live.awaitFinish(interruptedJobGrace)
	}

	remembered, known := b.issued.lookup(target)
	if !known {
		// Either the job was stopped before it ever wrote, or the message never
		// created an issuance in the first place - a deleted ALTERACAO, a deleted
		// piece of conversation, or one older than the map remembers. Undoing a
		// correction that already landed is not something the bot can do, and
		// guessing is worse than leaving it be.
		b.Logger.Infof("delecao: nothing saved behind deleted message %s, nothing to undo", target)
		return
	}

	deleted, err := b.deleteIssuanceBehind(ctx, remembered, user.ID)
	if err != nil {
		b.Logger.Errorf("delecao: failed to delete the issuance behind message %s for %s: %v",
			target, user.Name, err)
		// Unattached to any message: the one it would quote was just deleted.
		// Skipped when the failure was the cancellation itself, since the send
		// would be cancelled too and whoever cancelled it already knows.
		if ctx.Err() == nil {
			b.SendMessage(ctx, evt.Info.Chat, "Não consegui apagar a emissão "+remembered.ProgramLocator+
				" da mensagem que foi excluída, verifique no painel")
		}
		return
	}
	if !deleted {
		b.Logger.Infof("delecao: issuance %s was already gone, nothing to undo", remembered.ProgramLocator)
		return
	}

	b.Logger.Infof("delecao: deleted issuance %s because %s deleted the message that created it",
		remembered.ProgramLocator, user.Name)
	b.notifyIssuanceDeleted()
}

// deleteIssuanceBehind soft-deletes the issuance a remembered message created,
// through the very same service the panel and DELETAR use. Reports false when
// the row is not there to delete - cancelled, already deleted, or rolled back by
// the cancellation just above.
//
// It looks the issuance up by id rather than by locator: the locator is freed
// for reuse once an issuance is cancelled, so a lookup by locator could land on
// a different issuance that happens to carry the same PNR today.
func (b *Bot) deleteIssuanceBehind(ctx context.Context, remembered issuedMessage, actorID string) (bool, error) {
	deleted := false

	err := b.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var issuance data.Issuance
		err := tx.Where("id = ? AND active = ?", remembered.IssuanceID, true).First(&issuance).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		deleted = true
		return issuancesvc.Delete(tx, issuance, actorID)
	})

	return deleted, err
}
