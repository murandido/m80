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

// RouteCommand routes the commands to their respective functions. startedAt
// is the message's arrival time, threaded through so each command can
// enforce the humanizer's response floor right before its final reply.
func (b *Bot) RouteCommand(evt *events.Message, text string, startedAt time.Time) {
	cmd := strings.Split(strings.ToLower(strings.TrimSpace(text)), " ")

	switch cmd[0] {
	case "!ping":
		b.cmdPing(evt, startedAt)

	case "!registrar-chat":
		b.cmdRegistrarChat(evt, startedAt)

	default:
		return
	}
}

// cmdPing just reply every "!ping" message
func (b *Bot) cmdPing(evt *events.Message, startedAt time.Time) {
	b.SendPresenceTyping(evt)
	humanizer.DelayComposing()

	humanizer.EnforceResponseFloor(startedAt)
	b.ReplyMessage(evt, "Pong! 🏓")
}

// cmdRegistrarChat handles "!registrar-chat": resolves the sender to a known
// user, and if found, creates a pending listened_chats entry for an admin to
// approve or deny later. Reacts with an emoji instead of replying with text -
// no response text is needed for this command.
//
// 🚀 is the bot's one success reaction, shared with EMISSAO: understood and
// done. (✅ is reserved for "read it", which nothing needs to say yet.)
func (b *Bot) cmdRegistrarChat(evt *events.Message, startedAt time.Time) {
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
	friendlyName := b.friendlyNamePtr(evt)

	var existing data.ListenedChat
	err = b.DB.First(&existing, "chat_jid = ?", chatJID).Error
	switch {
	case err == nil:
		// Still waiting on a decision, or already approved and being
		// monitored right now - either way that's a real duplicate request.
		// Silently ignored, same as an unknown sender - no reaction needed
		// to tell someone "yeah, we know" about something already in flight.
		if existing.Status == "pending" || existing.Active {
			b.Logger.Infof("registrar-chat: chat %s already pending/active, ignoring", chatJID)
			return
		}

		// Deactivated permanently by an admin (see DeactivateListenedChat) -
		// stays deactivated until an admin explicitly reactivates it from the
		// panel; the bot never reopens it on its own.
		if existing.Status == "deactivated" {
			b.Logger.Infof("registrar-chat: chat %s is deactivated, ignoring", chatJID)
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
			return
		}
		humanizer.DelayReaction()
		humanizer.EnforceResponseFloor(startedAt)
		b.ReactMessage(evt, "🚀")
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
			return
		}
		humanizer.DelayReaction()
		humanizer.EnforceResponseFloor(startedAt)
		b.ReactMessage(evt, "🚀")
		b.notifyChatRequested()

	default:
		b.Logger.Errorf("registrar-chat: failed to check existing listened_chats for %s: %v", chatJID, err)
	}
}

// friendlyNamePtr resolves a human-readable name for the chat and wraps it
// as *string (nil when empty), ready to assign directly to
// data.ListenedChat.FriendlyName.
func (b *Bot) friendlyNamePtr(evt *events.Message) *string {
	name := b.resolveChatDisplayName(evt)
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
func (b *Bot) resolveChatDisplayName(evt *events.Message) string {
	if evt.Info.Chat.Server == types.GroupServer {
		info, err := b.currentSession().Client.GetGroupInfo(context.Background(), evt.Info.Chat)
		if err != nil || info == nil {
			b.Logger.Warnf("registrar-chat: failed to resolve group name for %s: %v", evt.Info.Chat, err)
			return ""
		}
		return info.Name
	}
	return evt.Info.PushName
}
