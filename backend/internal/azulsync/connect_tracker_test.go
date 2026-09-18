package azulsync

import (
	"sync"
	"testing"
)

func TestConnectTracker_BeginRefusesDuplicateUntilEnd(t *testing.T) {
	tracker := NewConnectTracker()

	if !tracker.Begin("acc-1") {
		t.Fatal("first Begin should succeed")
	}
	if !tracker.IsConnecting("acc-1") {
		t.Fatal("account should read as connecting after Begin")
	}
	if tracker.Begin("acc-1") {
		t.Fatal("a second Begin for the same account must be refused while in flight")
	}
	// A different account is independent.
	if !tracker.Begin("acc-2") {
		t.Fatal("Begin for a different account should succeed")
	}

	tracker.End("acc-1")
	if tracker.IsConnecting("acc-1") {
		t.Fatal("account should not read as connecting after End")
	}
	if !tracker.Begin("acc-1") {
		t.Fatal("Begin should succeed again once the previous one ended")
	}
}

func TestConnectTracker_NilIsNotConnecting(t *testing.T) {
	var tracker *ConnectTracker
	if tracker.IsConnecting("acc-1") {
		t.Fatal("a nil tracker must report not connecting, not panic")
	}
}

func TestConnectTracker_ConcurrentBeginElectsOneWinner(t *testing.T) {
	tracker := NewConnectTracker()
	const goroutines = 50
	var wins int32
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range [goroutines]struct{}{} {
		go func() {
			defer wg.Done()
			if tracker.Begin("acc") {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("exactly one concurrent Begin should win, got %d", wins)
	}
}
