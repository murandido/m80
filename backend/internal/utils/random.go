package utils

import (
	"math/rand"
	"time"
)

// SleepRandom sleeps for a random amount of time to humanize
func SleepRandom(minMs, maxMs int) {
	duration := rand.Intn(maxMs-minMs) + minMs
	time.Sleep(time.Duration(duration) * time.Millisecond)
}
