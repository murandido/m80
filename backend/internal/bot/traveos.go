package bot

import (
	"context"
	"errors"

	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"go.mau.fi/whatsmeow/types/events"
)

// reactionPartial is the bot's answer when the issuance was registered but
// Traveos did not give up its data. It is a fourth reaction on top of the
// ✅/🕐/🚀/❌ vocabulary, and the only one that means "half of it worked".
//
// It carries no text on purpose. Traveos answers nearly every failure with the
// same opaque 500 - a wrong locator, a wrong surname, the airline's own site
// being down and a bug inside Traveos are indistinguishable from the outside -
// so any reason the bot could put in the group would be invented. The real error
// goes to the log, which is where someone can act on it, and the operator gets
// an honest "saved, but incomplete": the row is on the grid, and the refetch
// button on the panel is one click away.
const reactionPartial = "⚠️"

// reactionFetching is shown while the bot is actually talking to Traveos.
//
// It exists because 🕐 means "queued, waiting its turn", and leaving that up for
// the whole fetch reads as a bot that hung with the message stuck in the queue
// forever. The vendor call routinely takes dozens of seconds and is allowed 120,
// which is far longer than anything else the group ever waits on - long enough
// that silence is indistinguishable from a crash.
//
// ⏳ instead of 🕐 on purpose: the hourglass is work in progress, the clock is a
// wait. They are different facts and the group can act on the difference.
const reactionFetching = "⏳"

// traveosOutcome separates "nothing happened, and that is fine" from "nothing
// happened, and the operator needs to know". Both leave the issuance incomplete,
// but only one of them is worth a warning in the group.
type traveosOutcome int

const (
	// traveosFetched: the itinerary landed and the issuance is now complete.
	traveosFetched traveosOutcome = iota
	// traveosSkipped: there was nothing to fetch. Not a failure, and nothing
	// changed in the database either.
	traveosSkipped
	// traveosFailed: Traveos was asked and did not deliver.
	traveosFailed
)

// ok reports whether the group should be told the whole thing worked.
func (o traveosOutcome) ok() bool { return o != traveosFailed }

// syncTraveos pulls the itinerary for an issuance that was just written.
//
// It runs outside the creating transaction, and must stay that way: the emission
// call alone is allowed 120 seconds, and holding a Postgres transaction open for
// that is not acceptable. It does run inside the per-PNR lock, which from here on
// guards money and not just database consistency - a second message about the
// same locator emitting a second real ticket at the vendor is the failure this
// prevents.
func (b *Bot) syncTraveos(
	ctx context.Context,
	evt *events.Message,
	flow, issuanceID, locator string,
	force bool,
) traveosOutcome {
	// No credentials configured. That is a deployment choice, not something wrong
	// with this issuance, so it must not put a warning on every message the group
	// sends - it goes to the log and the issuance stays as the operator typed it.
	if b.Traveos == nil {
		b.Logger.Infof("%s: traveos is not configured, leaving %s incomplete", flow, locator)
		return traveosSkipped
	}

	// Announced here rather than at the call sites so both triggers - the EMISSAO
	// and the correction that retries it - say the same thing, and so nothing can
	// claim a vendor call that is not about to happen. No humanizer delay: this is
	// a progress indicator, and pacing it would defeat the point.
	//
	// The caller replaces it with 🚀 or ⚠️ when this returns, so no path leaves the
	// hourglass up. A cancelled ctx skips it, like every other visible reaction.
	if evt != nil {
		b.ReactMessage(ctx, evt, reactionFetching)
	}

	err := issuancesvc.Sync(ctx, b.DB, b.Traveos, issuanceID, force)
	switch {
	case err == nil:
		b.Logger.Infof("%s: pulled the traveos itinerary for %s", flow, locator)
		return traveosFetched

	// Not a failure either: somebody decided in the airline CRUD that this
	// airline is not fetched from Traveos, so an incomplete issuance is the
	// expected outcome and 🚀 is the honest answer.
	case errors.Is(err, issuancesvc.ErrAirlineNotSupported):
		b.Logger.Infof("%s: airline of %s is not fetched from traveos, skipping", flow, locator)
		return traveosSkipped

	case ctx.Err() != nil:
		// The job was cancelled - by a DELETAR, or by the operator deleting their
		// own message. Whoever cancelled it owes the group an answer; this one is
		// not a Traveos problem and must not be reported as one.
		b.Logger.Infof("%s: traveos fetch for %s was cancelled", flow, locator)
		return traveosFailed

	default:
		b.Logger.Errorf("%s: failed to pull the traveos itinerary for %s: %v", flow, locator, err)
		return traveosFailed
	}
}
