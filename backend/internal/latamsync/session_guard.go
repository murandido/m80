package latamsync

import "sync"

// SessionGuard serialises everything that may renew one account's session: the ticker and the
// connect handler's first read.
//
// Auth0 can rotate refresh tokens with reuse detection, where two renewals from the same stored
// token revoke the whole session - which is what happened on Smiles (see
// smilessync.SessionGuard). Whether LATAM's tenant does the same was not measured; a lock per
// account costs nothing, and finding out the other way costs every holder a new login.
//
// One instance per API process, shared by the ticker and the handlers. Nil-safe, so a caller
// without one (a test exercising one round) still works.
type SessionGuard struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewSessionGuard returns an empty guard.
func NewSessionGuard() *SessionGuard {
	return &SessionGuard{locks: make(map[string]*sync.Mutex)}
}

// Lock takes the account's lock and returns the function that releases it.
func (g *SessionGuard) Lock(accountID string) func() {
	if g == nil {
		return func() {}
	}
	g.mu.Lock()
	lock, ok := g.locks[accountID]
	if !ok {
		lock = &sync.Mutex{}
		g.locks[accountID] = lock
	}
	g.mu.Unlock()

	lock.Lock()
	return lock.Unlock
}
