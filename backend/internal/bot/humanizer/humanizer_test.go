package humanizer

import (
	"testing"
	"time"
)

const tolerance = 50 * time.Millisecond

// Every test here asserts a real sleep, so the package's runtime is the sum of
// the delays it measures unless they overlap. They share nothing - the delays
// read a single atomic flag nobody flips in this binary - so running them at the
// same time costs the longest one instead of all of them added up.

func assertWithinRange(t *testing.T, elapsed, min, max time.Duration) {
	t.Helper()
	if elapsed < min-tolerance || elapsed > max+tolerance {
		t.Fatalf("elapsed %v outside [%v, %v]", elapsed, min, max)
	}
}

func TestDelayMarkAsRead_WithinRange(t *testing.T) {
	t.Parallel()

	start := time.Now()
	DelayMarkAsRead()
	assertWithinRange(t, time.Since(start), 500*time.Millisecond, 1500*time.Millisecond)
}

func TestDelayReaction_WithinRange(t *testing.T) {
	t.Parallel()

	start := time.Now()
	DelayReaction()
	assertWithinRange(t, time.Since(start), 300*time.Millisecond, 800*time.Millisecond)
}

func TestDelayComposing_WithinRange(t *testing.T) {
	t.Parallel()

	start := time.Now()
	DelayComposing()
	assertWithinRange(t, time.Since(start), 800*time.Millisecond, 2*time.Second)
}

func TestEnforceResponseFloor_SleepsUpToFloorWhenProcessingWasFast(t *testing.T) {
	t.Parallel()

	start := time.Now()
	EnforceResponseFloor(start)
	assertWithinRange(t, time.Since(start), 1500*time.Millisecond, 3*time.Second)
}

func TestEnforceResponseFloor_NoExtraDelayWhenProcessingAlreadyExceededFloor(t *testing.T) {
	t.Parallel()

	// Simulates a response that already took 5s to produce - past the 3s
	// ceiling of the floor - so no additional wait should be added.
	startedAt := time.Now().Add(-5 * time.Second)

	callStart := time.Now()
	EnforceResponseFloor(startedAt)
	if elapsed := time.Since(callStart); elapsed > tolerance {
		t.Fatalf("EnforceResponseFloor added %v of extra delay when processing already exceeded the floor", elapsed)
	}
}
