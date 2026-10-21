package latamsync

import (
	"sync"
	"time"
)

// pendingTTL is how long a login handed out to the operator is kept. It covers typing a password
// and answering LATAM's verification with room to spare; past it the operator presses Conectar
// again, which costs nothing.
const pendingTTL = 15 * time.Minute

// PendingLogin is a LATAM login waiting for its callback: which account it is for, and the
// verifier the answer must be exchanged with.
type PendingLogin struct {
	AccountID string
	Verifier  string
	StartedAt time.Time
}

// LoginTracker holds the logins handed out and not yet finished, keyed by their OAuth state.
//
// Keyed by state rather than by account because the callback arrives with nothing but LATAM's
// answer, often in a new tab, so the state inside that answer is the only thing that can say
// which account it was for. In memory and never persisted, like smilessync.LoginTracker: the verifier turns a code
// into a session, and an API restart mid-login costs the operator one more click.
type LoginTracker struct {
	mu      sync.Mutex
	pending map[string]PendingLogin
	now     func() time.Time
}

// NewLoginTracker returns an empty tracker. One is created per API process.
func NewLoginTracker() *LoginTracker {
	return &LoginTracker{pending: make(map[string]PendingLogin), now: time.Now}
}

// Begin records a login, dropping any earlier one for the same account: the previous attempt is
// worthless the moment a new one starts.
func (t *LoginTracker) Begin(state string, login PendingLogin) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, existing := range t.pending {
		if existing.AccountID == login.AccountID || t.now().Sub(existing.StartedAt) > pendingTTL {
			delete(t.pending, key)
		}
	}
	login.StartedAt = t.now()
	t.pending[state] = login
}

// Take returns the login the state belongs to and forgets it - a callback is good for one
// exchange, so a second paste of the same address must not find it. False when there is none or
// it has aged out.
func (t *LoginTracker) Take(state string) (PendingLogin, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	login, ok := t.pending[state]
	if !ok {
		return PendingLogin{}, false
	}
	delete(t.pending, state)
	if t.now().Sub(login.StartedAt) > pendingTTL {
		return PendingLogin{}, false
	}
	return login, true
}
