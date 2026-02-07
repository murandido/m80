package bot

import (
	"go.mau.fi/whatsmeow/types/events"
)

// EventHandler handles the necessary events from whatsmeow
func (b *Bot) EventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe {
			return
		}

		rawText := GetText(v.Message)

		go func() {
			// delay to read the message
			SleepRandom(500, 1500)

			b.MarkMessageAsRead(v)
		}()
	}

}
