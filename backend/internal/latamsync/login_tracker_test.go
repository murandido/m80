package latamsync

import (
	"testing"
	"time"
)

func TestLoginTrackerTakesALoginOnce(t *testing.T) {
	t.Parallel()
	tracker := NewLoginTracker()
	tracker.Begin("state-1", PendingLogin{AccountID: "acc", Verifier: "v"})

	login, ok := tracker.Take("state-1")
	if !ok || login.AccountID != "acc" || login.Verifier != "v" {
		t.Fatalf("Take = %+v, %v", login, ok)
	}
	if _, ok := tracker.Take("state-1"); ok {
		t.Error("the same callback was accepted twice")
	}
}

func TestLoginTrackerKeepsOnlyTheLatestLoginPerAccount(t *testing.T) {
	t.Parallel()
	tracker := NewLoginTracker()
	tracker.Begin("old", PendingLogin{AccountID: "acc"})
	tracker.Begin("new", PendingLogin{AccountID: "acc"})
	tracker.Begin("other", PendingLogin{AccountID: "acc-2"})

	if _, ok := tracker.Take("old"); ok {
		t.Error("a superseded login was still accepted")
	}
	if _, ok := tracker.Take("new"); !ok {
		t.Error("the latest login was lost")
	}
	if _, ok := tracker.Take("other"); !ok {
		t.Error("another account's login was dropped")
	}
}

func TestLoginTrackerExpires(t *testing.T) {
	t.Parallel()
	tracker := NewLoginTracker()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	tracker.Begin("state", PendingLogin{AccountID: "acc"})

	now = now.Add(pendingTTL + time.Second)
	if _, ok := tracker.Take("state"); ok {
		t.Error("an expired login was accepted")
	}
}
