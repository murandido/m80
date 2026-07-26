package bot

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// SendMessage sends a message to a specific chat JID
func (b *Bot) SendMessage(chatID types.JID, text string) {
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}

	_, err := b.currentSession().Client.SendMessage(context.Background(), chatID, msg)
	if err != nil {
		// TODO: audit this error
		fmt.Println("Error sending:", err)
	}
}

// ReplyMessage replies quoting the original message
func (b *Bot) ReplyMessage(originalMsg *events.Message, text string) {
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

	_, err := b.currentSession().Client.SendMessage(context.Background(), originalMsg.Info.Chat, msg)
	if err != nil {
		// TODO: audit this error
		fmt.Println("Error sending the reply:", err)
	}
}

func (b *Bot) MarkMessageAsRead(evt *events.Message) {
	err := b.currentSession().Client.MarkRead(
		context.Background(),
		[]types.MessageID{evt.Info.ID},
		evt.Info.Timestamp,
		evt.Info.Chat,
		evt.Info.Sender,
	)
	if err != nil {
		// TODO: audit this error
		fmt.Println("Error marking message as read:", err)
	}
}

// SendPresenceTyping send to the chat that the bot is typing
func (b *Bot) SendPresenceTyping(evt *events.Message) {
	err := b.currentSession().Client.SendChatPresence(
		context.Background(),
		evt.Info.Chat,
		types.ChatPresenceComposing,
		types.ChatPresenceMediaText,
	)
	// TODO: audit this error
	if err != nil {
		fmt.Println("Error sendind typing presence:", err)
	}
}
