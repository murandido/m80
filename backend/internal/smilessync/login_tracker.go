package smilessync

import (
	"sync"
	"time"

	"github.com/murandido/m-80-backend/carriers/smiles"
)

// pendingTTL is how long a half-finished login is kept. The tenant's mfa_token is short-lived
// and there is no way to ask how long is left, so this is a deliberate under-estimate: better
// to say "the code expired, start again" than to send digits the tenant has already forgotten
// about and report its refusal as a wrong code.
const pendingTTL = 5 * time.Minute

// PendingLogin is a login waiting on a verification code: the tenant's short-lived token, the
// channels the account holder registered, and - once a channel is chosen - the challenge the
// typed digits belong to.
type PendingLogin struct {
	MFAToken       string
	OOBCode        string
	Authenticators []smiles.Authenticator
	StartedAt      time.Time
}

// LoginTracker holds the logins that are mid-flight, one per account.
//
// In memory and never persisted, on purpose - and the reasoning is stronger than the one
// behind azulsync.ConnectTracker. That tracker keeps ephemeral state out of the database
// because a login in flight does not survive a restart, so a stored "connecting" would be a
// lie. This one keeps a live credential out of the database: the mfa_token can be exchanged
// for a session, it is useful for minutes, and there is no round it has to survive. An API
// restart mid-login costs the operator one more code, which is the correct price.
//
// An injected instance rather than a package global, so tests stay isolated and race-free.
type LoginTracker struct {
	mu      sync.Mutex
	pending map[string]*PendingLogin
	now     func() time.Time
}

// NewLoginTracker returns an empty tracker. One is created per API process and shared by the
// connect handlers.
func NewLoginTracker() *LoginTracker {
	return &LoginTracker{pending: make(map[string]*PendingLogin), now: time.Now}
}

// Begin records a login that is waiting on a verification code, replacing any earlier one for
// the same account. Replacing rather than refusing is what lets an operator who mistyped, or
// who let the window lapse, simply press connect again - the previous attempt is worthless the
// moment a new one starts.
func (t *LoginTracker) Begin(accountID string, login PendingLogin) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	login.StartedAt = t.now()
	t.pending[accountID] = &login
}

// Pending returns the login in flight for the account, or false when there is none or it has
// aged out. An expired entry is dropped on the way past, which is the only cleanup this map
// needs: entries are bounded by the number of accounts an operator can be connecting at once.
func (t *LoginTracker) Pending(accountID string) (PendingLogin, bool) {
	if t == nil {
		return PendingLogin{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	login, ok := t.pending[accountID]
	if !ok {
		return PendingLogin{}, false
	}
	if t.now().Sub(login.StartedAt) > pendingTTL {
		delete(t.pending, accountID)
		return PendingLogin{}, false
	}
	return *login, true
}

// Challenged records which challenge the account is now waiting on, so the typed digits can be
// tied back to it. It does not restart the clock: the tenant's token is what expires, and
// asking for a second code does not renew it.
func (t *LoginTracker) Challenged(accountID, oobCode string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	login, ok := t.pending[accountID]
	if !ok || t.now().Sub(login.StartedAt) > pendingTTL {
		return false
	}
	login.OOBCode = oobCode
	return true
}

// End forgets the login, whether it finished or was abandoned. Always called once a session is
// stored, so a spent mfa_token does not linger in memory.
func (t *LoginTracker) End(accountID string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, accountID)
}
