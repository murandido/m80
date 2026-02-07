package bot

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// SleepRandom sleeps for a random amount of time to humanize
func SleepRandom(minMs, maxMs int) {
	duration := rand.Intn(maxMs-minMs) + minMs
	time.Sleep(time.Duration(duration) * time.Millisecond)
}

// GetText extracts the text from any type of message.
func GetText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}

	if txt := msg.GetConversation(); txt != "" {
		return txt
	}

	if txt := msg.GetExtendedTextMessage().GetText(); txt != "" {
		return txt
	}

	if txt := msg.GetImageMessage().GetCaption(); txt != "" {
		return txt
	}

	return ""
}

// IsCommand returns if the string has the conditions to be a valid command
func IsCommand(msg string) bool {
	if strings.HasPrefix(msg, "!") {
		return true
	}

	return false
}

// SendMessage sends a message to a specific chat JID
func (b *Bot) SendMessage(chatID types.JID, text string) {
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}

	_, err := b.Client.SendMessage(context.Background(), chatID, msg)
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

	_, err := b.Client.SendMessage(context.Background(), originalMsg.Info.Chat, msg)
	if err != nil {
		// TODO: audit this error
		fmt.Println("Error sending the reply:", err)
	}
}
