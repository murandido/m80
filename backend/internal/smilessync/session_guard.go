package smilessync

import "sync"

// SessionGuard serialises everything that may renew one account's session.
//
// It is not an optimisation. The tenant rotates the refresh token on every renewal and treats
// a reuse as theft: two rounds starting from the same stored token do not merely race, they
// get the whole session revoked and send the account back to a human for a new verification
// code. That was measured, the hard way, while the contract was being read - two concurrent
// renewals, and the family was gone.
//
// One instance per API process, injected and shared by the ticker and the connect handlers -
// the same shape as azulsync.ConnectTracker, and for the same reason: the thing being
// protected is process-wide, so a package global would work but would be a mutable global,
// and tests would share it.
//
// Nil-safe, so a caller that has no guard (a test exercising one round on its own) still
// works.
type SessionGuard struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewSessionGuard returns an empty guard.
func NewSessionGuard() *SessionGuard {
	return &SessionGuard{locks: make(map[string]*sync.Mutex)}
}

// Lock takes the account's lock and returns the function that releases it. Always used with
// defer, and always around the whole round rather than the renewal alone: the balance and the
// statement are read with the session that renewal produced.
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
