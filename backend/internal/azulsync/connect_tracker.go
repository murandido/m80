package azulsync

import "sync"

// ConnectTracker remembers which accounts have a one-time connect login running in the background.
// The connect is asynchronous - the handler kicks off the browser login and returns at once - so
// this is what lets the panel show "conectando" and what stops a second request from launching a
// duplicate browser while one is already in flight.
//
// It is process-local and ephemeral on purpose: the state is not worth a database column, and a
// login in flight does not survive an API restart anyway (the goroutine dies with the process, so
// a stale "connecting" would be a lie). It is an injected instance, not a package global, so tests
// stay isolated and race-free.
type ConnectTracker struct {
	mu       sync.Mutex
	inFlight map[string]bool
}

// NewConnectTracker returns an empty tracker. One is created per API process and shared by the
// connect handler (which marks accounts in and out) and the statement handler (which reads it into
// the sync-state badge).
func NewConnectTracker() *ConnectTracker {
	return &ConnectTracker{inFlight: make(map[string]bool)}
}

// Begin marks the account as connecting and returns true. It returns false when a connect is
// already running for that account, so the caller refuses the duplicate instead of starting a
// second browser (and, worse, hammering the same IP through the login service).
func (t *ConnectTracker) Begin(accountID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inFlight[accountID] {
		return false
	}
	t.inFlight[accountID] = true
	return true
}

// End clears the connecting mark, whether the login succeeded or failed. Always paired with a
// successful Begin, via defer.
func (t *ConnectTracker) End(accountID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.inFlight, accountID)
}

// IsConnecting reports whether a connect login is in flight for the account. Nil-safe so the
// statement handler can hold an optional tracker (the Smiles routes have none).
func (t *ConnectTracker) IsConnecting(accountID string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inFlight[accountID]
}
