package bot

import (
	"math/rand"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
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
