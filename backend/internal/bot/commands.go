package bot

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/data"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// Every command the bot answers. Listed here rather than only in RouteCommand's
// switch because the intake stage needs to know, before reacting to anything,
// whether a "!" message is one the bot will actually answer - an unknown command
// is ignored, and acknowledging it with ✅ would promise a reply that never comes.
const (
	cmdNamePing          = "!ping"
	cmdNameRegistrarChat = "!registrar-chat"
)

// commandName is the leading word of a command, lowercased. "" for a message
// that has no first word at all.
func commandName(text string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// isKnownCommand reports whether this is a command the bot answers.
func isKnownCommand(text string) bool {
	switch commandName(text) {
	case cmdNamePing, cmdNameRegistrarChat:
		return true
	default:
		return false
	}
}

// RouteCommand routes the commands to their respective functions. startedAt
// is the message's arrival time, threaded through so each command can
// enforce the humanizer's response floor right before its final reply.
func (b *Bot) RouteCommand(ctx context.Context, evt *events.Message, text string, startedAt time.Time) {
	switch commandName(text) {
	case cmdNamePing:
		b.cmdPing(ctx, evt, startedAt)

	case cmdNameRegistrarChat:
		b.cmdRegistrarChat(ctx, evt, startedAt)

	default:
		// Unreachable: intake only admits known commands. Kept so adding a
		// command to the list above without a case here fails loudly in the log
		// rather than leaving the message hanging on its 🕐.
		b.Logger.Warnf("comando: %q was admitted but has no route", commandName(text))
	}
}

// cmdPing just reply every "!ping" message.
//
// It reacts 🚀 as well as replying, which no other text-answering path does.
// Every admitted message is acknowledged with ✅ and then 🕐 before any work,
// and a reaction is only ever replaced, never cleared - so a command that
// answers with text alone would leave its clock on screen forever. The reply is
// still the point; the 🚀 is what closes the indicator the pipeline opened.
func (b *Bot) cmdPing(ctx context.Context, evt *events.Message, startedAt time.Time) {
	b.SendPresenceTyping(ctx, evt)
	humanizer.DelayComposing()

	humanizer.EnforceResponseFloor(startedAt)
	b.ReplyMessage(ctx, evt, "Pong! 🏓")
	b.ReactMessage(ctx, evt, "🚀")
}

// cmdRegistrarChat handles "!registrar-chat": resolves the sender to a known
// user, and if found, creates a pending listened_chats entry for an admin to
// approve or deny later. Reacts with an emoji instead of replying with text -
// no response text is needed for this command.
//
// 🚀 is the bot's one success reaction, shared with EMISSAO: understood and
// done. It is also what a duplicate request gets, which used to be answered
// with silence. Silence stopped being available once intake started
// acknowledging the message with ✅ and 🕐: leaving those in place would show a
// clock that never resolves. "What you asked for is already true" is a success
// anyway - the same reasoning a no-op correction already follows.
func (b *Bot) cmdRegistrarChat(ctx context.Context, evt *events.Message, startedAt time.Time) {
	user, err := b.resolveSenderUser(evt)
	if err != nil {
		b.Logger.Errorf("registrar-chat: failed to resolve sender %s: %v", evt.Info.Sender, err)
		return
	}
	if user == nil {
		// Silently ignored on purpose: an unknown sender gets no visible
		// reaction at all, not even an error emoji.
		b.Logger.Infof("registrar-chat: sender %s is not a known user, ignoring", evt.Info.Sender)
		return
	}

	chatJID := evt.Info.Chat.String()
	friendlyName := b.friendlyNamePtr(ctx, evt)

	var existing data.ListenedChat
	err = b.DB.First(&existing, "chat_jid = ?", chatJID).Error
	switch {
	case err == nil:
		// Still waiting on a decision, or already approved and being
		// monitored right now - either way that's a real duplicate request,
		// and the state it asks for already holds.
		if existing.Status == "pending" || existing.Active {
			b.Logger.Infof("registrar-chat: chat %s already pending/active, nothing to do", chatJID)
			humanizer.DelayReaction()
			humanizer.EnforceResponseFloor(startedAt)
			b.ReactMessage(ctx, evt, "🚀")
			return
		}

		// Deactivated permanently by an admin (see DeactivateListenedChat) -
		// stays deactivated until an admin explicitly reactivates it from the
		// panel; the bot never reopens it on its own. Unlike the branch above
		// this is a refusal, so it says why: 🚀 would claim the chat is now
		// pending approval when nothing was written at all.
		if existing.Status == "deactivated" {
			b.Logger.Infof("registrar-chat: chat %s is deactivated, refusing to reopen", chatJID)
			b.refuse(ctx, evt, startedAt, "Esse chat foi desativado por um admin e só pode ser reativado pelo painel")
			return
		}

		// Denied, or approved-but-stopped (soft-stop via StopListenedChat
		// only flips active off, it never deletes the row or changes
		// status). Neither is "in flight" anymore, so a fresh request
		// reopens the same row instead of being permanently blocked by a
		// decision that no longer applies.
		existing.RequestedByID = user.ID
		existing.Status = "pending"
		existing.Active = false
		existing.ApprovedByID = nil
		existing.FriendlyName = friendlyName
		if err := b.DB.Save(&existing).Error; err != nil {
			b.Logger.Errorf("registrar-chat: failed to reopen listened_chats row for %s: %v", chatJID, err)
			b.refuse(ctx, evt, startedAt, registrarChatFailure)
			return
		}
		humanizer.DelayReaction()
		humanizer.EnforceResponseFloor(startedAt)
		b.ReactMessage(ctx, evt, "🚀")
		b.notifyChatRequested()

	case errors.Is(err, gorm.ErrRecordNotFound):
		chat := data.ListenedChat{
			ChatJID:       chatJID,
			FriendlyName:  friendlyName,
			RequestedByID: user.ID,
			Status:        "pending",
			Active:        false,
		}
		if err := b.DB.Create(&chat).Error; err != nil {
			b.Logger.Errorf("registrar-chat: failed to create listened_chats row for %s: %v", chatJID, err)
			b.refuse(ctx, evt, startedAt, registrarChatFailure)
			return
		}
		humanizer.DelayReaction()
		humanizer.EnforceResponseFloor(startedAt)
		b.ReactMessage(ctx, evt, "🚀")
		b.notifyChatRequested()

	default:
		b.Logger.Errorf("registrar-chat: failed to check existing listened_chats for %s: %v", chatJID, err)
		b.refuse(ctx, evt, startedAt, registrarChatFailure)
	}
}

// registrarChatFailure is what a database failure says to the group. Same
// reasoning as refusalText's default branch: the real error is already in the
// log, and a Postgres message has no business being echoed into a WhatsApp
// group.
const registrarChatFailure = "Não consegui registrar esse chat, verifique com o suporte"

// friendlyNamePtr resolves a human-readable name for the chat and wraps it
// as *string (nil when empty), ready to assign directly to
// data.ListenedChat.FriendlyName.
func (b *Bot) friendlyNamePtr(ctx context.Context, evt *events.Message) *string {
	name := b.resolveChatDisplayName(ctx, evt)
	if name == "" {
		return nil
	}
	return &name
}

// resolveChatDisplayName resolves a human-readable name for the chat a
// !registrar-chat came from - the group's name for group chats, or the
// sender's own WhatsApp display name for DMs. Returns "" (never an error) on
// any failure, so a slow/unavailable group-info lookup never blocks creating
// the listened_chats row - it only means friendly_name stays nil.
func (b *Bot) resolveChatDisplayName(ctx context.Context, evt *events.Message) string {
	if evt.Info.Chat.Server == types.GroupServer {
		info, err := b.currentSession().Client.GetGroupInfo(ctx, evt.Info.Chat)
		if err != nil || info == nil {
			b.Logger.Warnf("registrar-chat: failed to resolve group name for %s: %v", evt.Info.Chat, err)
			return ""
		}
		return info.Name
	}
	return evt.Info.PushName
}
