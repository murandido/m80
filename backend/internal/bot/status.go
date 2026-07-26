package bot

import (
	"sync"
	"time"
)

// ConnectionState is the coarse state of the whatsmeow session as seen by the
// admin panel.
type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StatePairing      ConnectionState = "pairing"
	StateConnected    ConnectionState = "connected"
)

// PairMethod is which whatsmeow pairing flow produced the current PairCode.
type PairMethod string

const (
	PairMethodPhone PairMethod = "phone"
	PairMethodQR    PairMethod = "qr"
)

// Status is the full snapshot returned by GET /status.
type Status struct {
	State      ConnectionState `json:"state"`
	JID        string          `json:"jid,omitempty"`
	PairMethod PairMethod      `json:"pair_method,omitempty"`
	PairCode   string          `json:"pair_code,omitempty"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// StatusStore holds the current connection Status behind a mutex, since it is
// read by HTTP handlers and written by the whatsmeow event handler goroutine
// concurrently. Every Set calls the notify callback so callers (cmd/bot) can
// push the change out over HTTP without this package knowing anything about
// HTTP - see internal/bot/CLAUDE.md.
type StatusStore struct {
	mu     sync.RWMutex
	status Status
	notify func()
}

// NewStatusStore creates a store starting in the disconnected state. notify is
// called (synchronously) after every Set; pass a no-op if nothing needs to
// react to changes.
func NewStatusStore(notify func()) *StatusStore {
	return &StatusStore{
		status: Status{State: StateDisconnected, UpdatedAt: time.Now()},
		notify: notify,
	}
}

func (s *StatusStore) Get() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

func (s *StatusStore) Set(status Status) {
	status.UpdatedAt = time.Now()

	s.mu.Lock()
	s.status = status
	s.mu.Unlock()

	s.notify()
}
