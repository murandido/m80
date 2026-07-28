package bot

import (
	"context"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
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

		// Read synchronously, right as the event arrives: whatsmeow
		// dispatches OfflineSyncPreview/Completed and the backlog of
		// *events.Message in between them in order, so this is the accurate
		// point to decide whether this particular message is part of that
		// backlog - not whenever the goroutine below happens to wake up.
		isBacklog := b.offlineSyncing.Load()

		// startedAt anchors the humanizer's response floor - captured here,
		// as close to the message's actual arrival as possible.
		startedAt := time.Now()

		go func() {
			humanizer.DelayMarkAsRead()

			b.processIncomingMessage(v, isBacklog, startedAt)
		}()

	case *events.OfflineSyncPreview:
		if v.Total > 0 {
			b.offlineSyncing.Store(true)
			b.Logger.Infof("catching up on %d message(s) missed while offline", v.Total)
		}

	case *events.OfflineSyncCompleted:
		b.offlineSyncing.Store(false)
		b.Logger.Infof("caught up on missed events (%d)", v.Count)

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

// processIncomingMessage marks the message read and, unless it arrived as
// part of catching up on a backlog after reconnecting, routes it: as a
// command if it looks like one, or as an EMISSAO if it looks like that.
// Extracted out of EventHandler's *events.Message case so tests can drive it
// directly without waiting on the randomized read delay above. startedAt is
// threaded through so the humanizer can enforce its response floor against the
// message's real arrival time, not whenever a given handler starts replying.
//
// An EMISSAO carries no "!" prefix - it is ordinary text the operators were
// already writing among themselves - so unlike a command it cannot be
// recognized by addressing alone. isEmissaoMessage is the cheap first filter;
// whether the bot may actually act on it is decided in handleEmissao, which
// checks that the chat is approved for monitoring before touching anything.
func (b *Bot) processIncomingMessage(v *events.Message, isBacklog bool, startedAt time.Time) {
	b.MarkMessageAsRead(v)

	if isBacklog {
		return
	}

	rawText := utils.GetText(v.Message)

	switch {
	case utils.IsCommand(rawText):
		b.RouteCommand(v, rawText, startedAt)
	case isEmissaoMessage(rawText):
		b.handleEmissao(v, rawText, startedAt)
	}
}
