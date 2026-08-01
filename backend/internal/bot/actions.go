package bot

import (
	"context"
	"fmt"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// SendMessage sends a message to a specific chat JID, unattached to any message
// in it - which is what a message about something that no longer exists needs,
// since there is nothing left to quote or react to.
func (b *Bot) SendMessage(ctx context.Context, chatID types.JID, text string) {
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}

	if _, err := b.currentSession().Client.SendMessage(ctx, chatID, msg); err != nil {
		b.Logger.Errorf("failed to send a message to %s: %v", chatID, err)
	}
}

// ReplyMessage replies quoting the original message.
//
// Like every other visible action, it runs on the job's own context: a job that
// was cancelled halfway must not still be typing into the group afterwards.
func (b *Bot) ReplyMessage(ctx context.Context, originalMsg *events.Message, text string) {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:      proto.String(originalMsg.Info.ID),
				Participant:   proto.String(originalMsg.Info.Sender.String()),
				QuotedMessage: originalMsg.Message,
			},
		},
	}

	_, err := b.currentSession().Client.SendMessage(ctx, originalMsg.Info.Chat, msg)
	if err != nil {
		b.Logger.Errorf("failed to reply to message %s in %s: %v", originalMsg.Info.ID, originalMsg.Info.Chat, err)
	}
}

// ReactMessage reacts to the original message with the given emoji.
//
// A reaction replaces whatever the bot reacted with before - WhatsApp keeps one
// per sender per message, it does not stack them - which is what makes ✅ → 🕐 →
// 🚀 read as a single progress indicator rather than three marks.
//
// Failures are logged rather than swallowed: now that a reaction is the first
// thing the bot does, one that never lands is the difference between "the bot
// ignored me" and "the bot is down", and the group cannot tell those apart
// without this line in the log.
func (b *Bot) ReactMessage(ctx context.Context, evt *events.Message, emoji string) {
	msg := b.currentSession().Client.BuildReaction(evt.Info.Chat, evt.Info.Sender, evt.Info.ID, emoji)

	_, err := b.currentSession().Client.SendMessage(ctx, evt.Info.Chat, msg)
	if err != nil {
		b.Logger.Errorf("failed to react %s to message %s in %s: %v", emoji, evt.Info.ID, evt.Info.Chat, err)
	}
}

// acknowledge is the bot's first answer to a message it is allowed to act on:
// ✅, "I saw this". It comes before any work, and before knowing whether the work
// will succeed, so it is the one reaction that never says anything about the
// outcome.
//
// It is only ever sent after the gate has passed. Someone typing in a group the
// bot merely happens to be in must not learn that it reads - which is why a
// rejected gate produces no reaction at all, not even this one.
func (b *Bot) acknowledge(ctx context.Context, evt *events.Message) {
	humanizer.DelayReaction()
	b.ReactMessage(ctx, evt, "✅")
}

// markQueued replaces the ✅ with 🕐: accepted, waiting its turn behind the other
// work in this chat. It is skipped by anything that deliberately bypasses the
// queue, because a clock there would claim a wait that never happens.
func (b *Bot) markQueued(ctx context.Context, evt *events.Message) {
	humanizer.DelayReaction()
	b.ReactMessage(ctx, evt, "🕐")
}

// closeInterrupted takes the 🕐 off a message whose job was stopped, and does
// nothing for any other ending.
//
// ❌ with no text, which is the one place that combination appears. The refusal
// text always explains something the operator could not have known; here they
// already know, because stopping the job is what they just asked for. What they
// cannot see without this is *which* pending message the DELETAR caught, since
// the 🚀 lands on the DELETAR itself and the clock would otherwise stay on the
// message it interrupted forever.
//
// Deliberately on a fresh context: the job's own was cancelled, and sending the
// reaction on it would cancel the reaction too.
func (b *Bot) closeInterrupted(j *job) {
	if !j.wasInterrupted() {
		return
	}

	humanizer.DelayReaction()
	b.ReactMessage(context.Background(), j.evt, "❌")
}

func (b *Bot) MarkMessageAsRead(ctx context.Context, evt *events.Message) {
	err := b.currentSession().Client.MarkRead(
		ctx,
		[]types.MessageID{evt.Info.ID},
		evt.Info.Timestamp,
		evt.Info.Chat,
		evt.Info.Sender,
	)
	if err != nil {
		b.Logger.Errorf("failed to mark message %s in %s as read: %v", evt.Info.ID, evt.Info.Chat, err)
	}
}

// SendPresenceTyping send to the chat that the bot is typing
func (b *Bot) SendPresenceTyping(ctx context.Context, evt *events.Message) {
	err := b.currentSession().Client.SendChatPresence(
		ctx,
		evt.Info.Chat,
		types.ChatPresenceComposing,
		types.ChatPresenceMediaText,
	)
	if err != nil {
		b.Logger.Errorf("failed to send a typing presence to %s: %v", evt.Info.Chat, err)
	}
}

// SendDocument sends a file as a reply to the message that produced it.
//
// Two steps, and the first one is easy to miss: WhatsApp does not accept file
// bytes inside a message. They are uploaded to its media servers first, and the
// message carries only the resulting keys - drop any one of them and the
// recipient sees a document that will not open.
func (b *Bot) SendDocument(ctx context.Context, originalMsg *events.Message, content []byte, fileName, mimeType string) error {
	uploaded, err := b.currentSession().Client.Upload(ctx, content, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("uploading %s: %w", fileName, err)
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String(mimeType),
			FileName:      proto.String(fileName),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:      proto.String(originalMsg.Info.ID),
				Participant:   proto.String(originalMsg.Info.Sender.String()),
				QuotedMessage: originalMsg.Message,
			},
		},
	}

	if _, err := b.currentSession().Client.SendMessage(ctx, originalMsg.Info.Chat, msg); err != nil {
		return fmt.Errorf("sending %s to %s: %w", fileName, originalMsg.Info.Chat, err)
	}
	return nil
}
