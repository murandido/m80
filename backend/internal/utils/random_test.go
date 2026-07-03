package utils

import (
	"testing"
)

func TestSleepRandom_DoesNotPanicWhenMinEqualsMax(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SleepRandom panicked with min == max: %v", r)
		}
	}()
	SleepRandom(0, 0)
}

func TestSleepRandom_DoesNotPanicWhenMinGreaterThanMax(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SleepRandom panicked with min > max: %v", r)
		}
	}()
	SleepRandom(100, 50)
}
