package bot

import (
	"strings"

	"github.com/murandido/m-80-backend/internal/utils"
	"go.mau.fi/whatsmeow/types/events"
)

// RouteCommand routes the commands to their respective functions
func (b *Bot) RouteCommand(evt *events.Message, text string) {
	cmd := strings.Split(strings.ToLower(strings.TrimSpace(text)), " ")

	switch cmd[0] {
	case "!ping":
		b.cmdPing(evt)

	default:
		return
	}
}

// cmdPing just reply every "!ping" message
func (b *Bot) cmdPing(evt *events.Message) {
	utils.SleepRandom(300, 500)

	b.SendPresenceTyping(evt)

	utils.SleepRandom(1000, 1500)

	b.ReplyMessage(evt, "Pong! 🏓")
}
