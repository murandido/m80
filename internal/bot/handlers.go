package bot

import (
	"github.com/murandido/m-80-backend/internal/utils"
	"go.mau.fi/whatsmeow/types/events"
)

// EventHandler handles the necessary events from whatsmeow
func (b *Bot) EventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe {
			return
		}

		rawText := utils.GetText(v.Message)

		go func() {
			// delay to read the message
			utils.SleepRandom(500, 1500)

			b.MarkMessageAsRead(v)

			if utils.IsCommand(rawText) {
				b.RouteCommand(v, rawText)
				return
			}
		}()
	}

}
