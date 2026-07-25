package events

import (
	"sync"
	"testing"
	"time"
)

func TestBroker_BroadcastReachesEverySubscriber(t *testing.T) {
	broker := NewBroker()
	first := broker.Subscribe()
	second := broker.Subscribe()
	defer broker.Unsubscribe(first)
	defer broker.Unsubscribe(second)

	broker.Broadcast(Event{Type: "issuance.created", Payload: map[string]string{"id": "abc"}})

	for name, ch := range map[string]chan Event{"first": first, "second": second} {
		select {
		case got := <-ch:
			if got.Type != "issuance.created" {
				t.Errorf("%s: type: got %q, want %q", name, got.Type, "issuance.created")
			}
		case <-time.After(time.Second):
			t.Errorf("%s: no event received", name)
		}
	}
}

func TestBroker_UnsubscribeRemovesAndClosesClient(t *testing.T) {
	broker := NewBroker()
	ch := broker.Subscribe()

	if got := broker.ClientCount(); got != 1 {
		t.Fatalf("client count after subscribe: got %d, want 1", got)
	}

	broker.Unsubscribe(ch)

	if got := broker.ClientCount(); got != 0 {
		t.Errorf("client count after unsubscribe: got %d, want 0", got)
	}

	if _, open := <-ch; open {
		t.Error("channel: still open after unsubscribe, want closed")
	}
}

// A second Unsubscribe must be a no-op: the stream handler unsubscribes via
// defer, and a double call would otherwise panic on a closed channel.
func TestBroker_UnsubscribeTwiceIsSafe(t *testing.T) {
	broker := NewBroker()
	ch := broker.Subscribe()

	broker.Unsubscribe(ch)
	broker.Unsubscribe(ch)

	if got := broker.ClientCount(); got != 0 {
		t.Errorf("client count: got %d, want 0", got)
	}
}

func TestBroker_BroadcastDoesNotBlockOnFullBuffer(t *testing.T) {
	broker := NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	done := make(chan struct{})
	go func() {
		// Deliberately never drain ch: every send past clientBuffer must be dropped.
		for range clientBuffer * 3 {
			broker.Broadcast(Event{Type: "issuance.updated"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Broadcast blocked on a client that stopped reading")
	}

	if got := len(ch); got != clientBuffer {
		t.Errorf("buffered events: got %d, want %d", got, clientBuffer)
	}
}

// The race detector is the whole point of this test: subscribe, broadcast and
// unsubscribe all run concurrently, which is exactly what a busy panel does.
func TestBroker_ConcurrentSubscribeBroadcastUnsubscribe(t *testing.T) {
	broker := NewBroker()

	var wg sync.WaitGroup

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := broker.Subscribe()
			for range 5 {
				select {
				case <-ch:
				default:
				}
			}
			broker.Unsubscribe(ch)
		}()
	}

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				broker.Broadcast(Event{Type: "issuance.created", Payload: map[string]string{"id": "x"}})
			}
		}()
	}

	wg.Wait()

	if got := broker.ClientCount(); got != 0 {
		t.Errorf("client count after all clients left: got %d, want 0", got)
	}
}
