package bot

import (
	"go.mau.fi/whatsmeow/types/events"
)

// cmdPing just reply every "!ping" message
func (b *Bot) cmdPing(evt *events.Message) {
	SleepRandom(300, 500)

	b.SendPresenceTyping(evt)

	SleepRandom(1000, 1500)

	b.ReplyMessage(evt, "Pong! 🏓")
}
