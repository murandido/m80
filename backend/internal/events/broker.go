// Package events provides a process-local publish/subscribe hub used to push
// server-sent events to connected browsers. It is deliberately domain-agnostic:
// it knows nothing about issuances, so other modules (the bot connection status,
// for one) can publish through the same broker without touching this package.
package events

import "sync"

// clientBuffer is how many events a single connection may fall behind before
// the broker starts dropping its events. A browser reading the stream drains
// this in microseconds; a stalled one is not allowed to slow anybody else down.
const clientBuffer = 16

// Event is one message on the wire. Type becomes the SSE `event:` field and
// Payload is marshalled into the `data:` field.
type Event struct {
	Type    string
	Payload any
}

// Broker fans one Event out to every currently connected client.
type Broker struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{clients: make(map[chan Event]struct{})}
}

// Subscribe registers a new client and returns the channel its events arrive
// on. The caller must call Unsubscribe when it is done, otherwise the client
// stays in the hub forever.
func (b *Broker) Subscribe() chan Event {
	ch := make(chan Event, clientBuffer)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[ch] = struct{}{}

	return ch
}

// Unsubscribe removes a client and closes its channel, which unblocks whatever
// is ranging over it. Closing happens under the write lock so it can never race
// with a Broadcast send, which only ever holds the read lock.
func (b *Broker) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.clients[ch]; !ok {
		return
	}

	delete(b.clients, ch)
	close(ch)
}

// Broadcast delivers an event to every connected client. The send is
// non-blocking: a client whose buffer is full loses this event rather than
// stalling the HTTP handler that published it.
func (b *Broker) Broadcast(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- event:
		default:
		}
	}
}

// ClientCount reports how many clients are currently connected. It exists so
// tests (and the connection logs) can assert that disconnecting a browser
// actually removes it from the hub.
func (b *Broker) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.clients)
}
