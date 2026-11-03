package smilessync

import "testing"

// A session without a refresh token renews nothing: the tenant refuses it on the next round,
// and storing it is how a blocked account ended up with a column that decrypted to "" while
// the panel kept offering Connect (measured 2026-10-30).
func TestStoreSessionRefusesAnEmptyRefreshToken(t *testing.T) {
	guard := NewSessionGuard()
	for _, token := range []string{"", "   ", "\t"} {
		if err := StoreSession(nil, guard, "any-account", token); err == nil {
			t.Errorf("token %q was accepted", token)
		}
	}
}
