package smilessync

import (
	"sync"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/carriers/smiles"
)

func trackerAt(now *time.Time) *LoginTracker {
	tracker := NewLoginTracker()
	tracker.now = func() time.Time { return *now }
	return tracker
}

func TestPendingLoginIsRememberedUntilItAgesOut(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tracker := trackerAt(&now)

	tracker.Begin("conta-1", PendingLogin{
		MFAToken:       "mfa-secret",
		Authenticators: []smiles.Authenticator{{ID: "sms|1", Channel: "sms", Target: "***77"}},
	})

	login, ok := tracker.Pending("conta-1")
	if !ok || login.MFAToken != "mfa-secret" || len(login.Authenticators) != 1 {
		t.Fatalf("Pending = %+v, %v", login, ok)
	}

	// Past the window, the tenant has forgotten the token too. Reporting it as still pending
	// would turn "the code expired" into "your code is wrong".
	now = now.Add(pendingTTL + time.Second)
	if _, ok := tracker.Pending("conta-1"); ok {
		t.Fatal("an expired login is still reported as pending")
	}
}

func TestChallengedTiesTheTypedDigitsToAChallenge(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tracker := trackerAt(&now)
	tracker.Begin("conta-1", PendingLogin{MFAToken: "mfa-secret"})

	if !tracker.Challenged("conta-1", "oob-1") {
		t.Fatal("Challenged reported no pending login")
	}
	login, _ := tracker.Pending("conta-1")
	if login.OOBCode != "oob-1" {
		t.Errorf("OOBCode = %q", login.OOBCode)
	}

	// Asking for a second code does not renew the tenant's token, so it must not renew ours.
	now = now.Add(pendingTTL + time.Second)
	if tracker.Challenged("conta-1", "oob-2") {
		t.Error("an expired login accepted a new challenge")
	}
}

func TestBeginReplacesAnEarlierAttempt(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tracker := trackerAt(&now)

	tracker.Begin("conta-1", PendingLogin{MFAToken: "primeiro", OOBCode: "oob-1"})
	// Pressing connect again is how an operator recovers from a mistyped or lapsed code. The
	// earlier attempt is worthless, including its challenge.
	tracker.Begin("conta-1", PendingLogin{MFAToken: "segundo"})

	login, ok := tracker.Pending("conta-1")
	if !ok || login.MFAToken != "segundo" {
		t.Fatalf("Pending = %+v, %v", login, ok)
	}
	if login.OOBCode != "" {
		t.Errorf("OOBCode = %q, want the stale challenge dropped", login.OOBCode)
	}
}

func TestEndForgetsTheSpentToken(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tracker := trackerAt(&now)
	tracker.Begin("conta-1", PendingLogin{MFAToken: "mfa-secret"})

	tracker.End("conta-1")
	if _, ok := tracker.Pending("conta-1"); ok {
		t.Fatal("the login survived End")
	}
}

func TestATrackerIsSafeToShare(t *testing.T) {
	t.Parallel()
	tracker := NewLoginTracker()

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			account := "conta-1"
			if i%2 == 0 {
				account = "conta-2"
			}
			tracker.Begin(account, PendingLogin{MFAToken: "mfa"})
			tracker.Challenged(account, "oob")
			tracker.Pending(account)
			tracker.End(account)
		}(i)
	}
	wg.Wait()
}

func TestANilTrackerIsInert(t *testing.T) {
	t.Parallel()
	// Nil-safe so a caller without a tracker (a test exercising one step) still works.
	var tracker *LoginTracker
	tracker.Begin("conta-1", PendingLogin{})
	tracker.End("conta-1")
	if tracker.Challenged("conta-1", "oob") {
		t.Error("a nil tracker accepted a challenge")
	}
	if _, ok := tracker.Pending("conta-1"); ok {
		t.Error("a nil tracker reported a pending login")
	}
}

func TestSessionGuardSerialisesOneAccount(t *testing.T) {
	t.Parallel()
	guard := NewSessionGuard()

	// Two concurrent renewals of one account do not merely race: the tenant revokes the whole
	// session family, and the account goes back to a human for a new code.
	var mu sync.Mutex
	inside, maxInside := 0, 0
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := guard.Lock("conta-1")
			defer unlock()

			mu.Lock()
			inside++
			if inside > maxInside {
				maxInside = inside
			}
			mu.Unlock()

			time.Sleep(time.Millisecond)

			mu.Lock()
			inside--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if maxInside != 1 {
		t.Fatalf("%d renewals of one account overlapped, want 1 at a time", maxInside)
	}
}

func TestSessionGuardDoesNotSerialiseDifferentAccounts(t *testing.T) {
	t.Parallel()
	guard := NewSessionGuard()

	first := guard.Lock("conta-1")
	defer first()

	// A different account must not wait: a pass would serialise into one long queue.
	done := make(chan struct{})
	go func() {
		unlock := guard.Lock("conta-2")
		unlock()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a second account blocked on the first account's lock")
	}
}

func TestANilGuardIsInert(t *testing.T) {
	t.Parallel()
	var guard *SessionGuard
	unlock := guard.Lock("conta-1")
	unlock()
}
