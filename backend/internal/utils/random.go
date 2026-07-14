package utils

import (
	"math/rand"
	"time"
)

// SleepRandom sleeps for a random amount of time to humanize
func SleepRandom(minMs, maxMs int) {
	if minMs >= maxMs {
		time.Sleep(time.Duration(minMs) * time.Millisecond)
		return
	}
	duration := rand.Intn(maxMs-minMs) + minMs // #nosec G404 -- jitter de delay do bot, não é sensível a segurança
	time.Sleep(time.Duration(duration) * time.Millisecond)
}
