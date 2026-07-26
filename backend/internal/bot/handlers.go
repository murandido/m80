package bot

import (
	"context"

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

	case *events.Connected:
		jid := ""
		if id := b.currentSession().GetJID(); id != nil {
			jid = id.String()
		}
		b.Status.Set(Status{State: StateConnected, JID: jid})

	case *events.Disconnected:
		b.Status.Set(Status{State: StateDisconnected})

	case *events.LoggedOut:
		b.Logger.Infof("session logged out by the phone, reason: %v", v.Reason)
		b.Status.Set(Status{State: StateDisconnected})

		// whatsmeow dispatches this event from its own goroutine and only
		// deletes the device store afterwards, in the goroutine that
		// triggered the logout - the two aren't synchronized. Racing
		// RecreateSession against that delete is unreliable: asking for a
		// new device before the old row is actually gone hands back a
		// client that still thinks it has a paired ID (GetQRChannel then
		// refuses with "no user ID in the client's Store"). Deleting it
		// ourselves first - idempotent even if whatsmeow's own delete also
		// runs - gives RecreateSession a guaranteed-clean slate. Backgrounded
		// so a slow DB round-trip never blocks the event dispatch goroutine.
		go func() {
			ctx := context.Background()
			if err := b.currentSession().Session.DeleteStore(ctx); err != nil {
				b.Logger.Warnf("failed to delete stale device store: %v", err)
			}
			if err := b.RecreateSession(ctx); err != nil {
				b.Logger.Errorf("failed to set up a fresh session after remote logout: %v", err)
			}
		}()

	case *events.PairSuccess:
		// The socket reconnects right after this; *events.Connected is what
		// actually flips the status to StateConnected.
		b.Logger.Infof("pair success for %s, waiting for reconnect", v.ID)
	}

}
