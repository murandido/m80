// Package humanizer applies WhatsApp-visible delays to the bot's actions so
// its behavior doesn't look scripted to automated detection. It only ever
// delays things a human would visibly see on WhatsApp - reading a message,
// reacting, showing "digitando...", and sending the final reply. It must
// never be used to delay internal processing (queue dispatch, per-PNR
// locking, external calls like Traveos, or commands that intentionally skip
// the pipeline) - keep it that way even as those pieces get built.
package humanizer

import (
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/murandido/m-80-backend/internal/utils"
)

// delaysEnabled gates every sleep in this package.
//
// It exists for one reason: internal/bot's suite spent roughly 90% of its wall
// clock asleep in here. That is the whole point in production and pure cost in
// a test, and there was enough of it to trip go test's 10 minute default once
// the packages ran under load together.
//
// Two things about the shape are deliberate. It is not automatic -
// testing.Testing() is true inside *this* package's tests too, and those have
// to keep timing the real thing. And it is atomic rather than a plain bool
// because the bot reads these functions from per-message goroutines, so a test
// flipping the flag must not race a delay already in flight.
var delaysEnabled atomic.Bool

func init() {
	// atomic.Bool zeroes to false, and the delays are on unless a test says
	// otherwise.
	delaysEnabled.Store(true)
}

// SetDelaysEnabled turns this package's sleeps on or off and returns a function
// that restores the previous setting.
//
// It is for tests. Production must never call it: switching the delays off is
// exactly the scripted behavior this package exists to avoid.
func SetDelaysEnabled(enabled bool) (restore func()) {
	previous := delaysEnabled.Swap(enabled)
	return func() { delaysEnabled.Store(previous) }
}

// DelayMarkAsRead delays marking an incoming message as read by 0.5-1.5s.
func DelayMarkAsRead() {
	if !delaysEnabled.Load() {
		return
	}
	utils.SleepRandom(500, 1500)
}

// DelayReaction delays sending an emoji reaction by 0.3-0.8s. Apply it after
// any preceding humanizer delay (e.g. DelayMarkAsRead), never in parallel.
func DelayReaction() {
	if !delaysEnabled.Load() {
		return
	}
	utils.SleepRandom(300, 800)
}

// DelayComposing delays sending a text or PDF reply by a fixed 0.8-2s
// "digitando..." pause. The range never varies with message or file size.
func DelayComposing() {
	if !delaysEnabled.Load() {
		return
	}
	utils.SleepRandom(800, 2000)
}

// EnforceResponseFloor blocks, if needed, until at least a randomized floor
// (1.5-3s, resolved once per call) has elapsed since startedAt. It's a floor,
// not a sum on top of real processing time: a response that already took
// longer than the floor (e.g. a real Traveos call) is sent immediately with
// no extra wait added.
func EnforceResponseFloor(startedAt time.Time) {
	if !delaysEnabled.Load() {
		return
	}

	floorMs := 1500 + rand.Intn(3000-1500) // #nosec G404 -- humanizer jitter, not security-sensitive
	floor := time.Duration(floorMs) * time.Millisecond

	if elapsed := time.Since(startedAt); elapsed < floor {
		time.Sleep(floor - elapsed)
	}
}
