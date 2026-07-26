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
	"time"

	"github.com/murandido/m-80-backend/internal/utils"
)

// DelayMarkAsRead delays marking an incoming message as read by 0.5-1.5s.
func DelayMarkAsRead() {
	utils.SleepRandom(500, 1500)
}

// DelayReaction delays sending an emoji reaction by 0.3-0.8s. Apply it after
// any preceding humanizer delay (e.g. DelayMarkAsRead), never in parallel.
func DelayReaction() {
	utils.SleepRandom(300, 800)
}

// DelayComposing delays sending a text or PDF reply by a fixed 0.8-2s
// "digitando..." pause. The range never varies with message or file size.
func DelayComposing() {
	utils.SleepRandom(800, 2000)
}

// EnforceResponseFloor blocks, if needed, until at least a randomized floor
// (1.5-3s, resolved once per call) has elapsed since startedAt. It's a floor,
// not a sum on top of real processing time: a response that already took
// longer than the floor (e.g. a real Traveos call) is sent immediately with
// no extra wait added.
func EnforceResponseFloor(startedAt time.Time) {
	floorMs := 1500 + rand.Intn(3000-1500) // #nosec G404 -- humanizer jitter, not security-sensitive
	floor := time.Duration(floorMs) * time.Millisecond

	if elapsed := time.Since(startedAt); elapsed < floor {
		time.Sleep(floor - elapsed)
	}
}
