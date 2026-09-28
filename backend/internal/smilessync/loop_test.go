package smilessync

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestGrowBackoff(t *testing.T) {
	t.Parallel()
	interval := 10 * time.Minute

	// The first push-back waits one interval, and each one after doubles - up to a ceiling
	// short enough that a transient limit clears within a working day.
	first := growBackoff(0, interval)
	if first != interval {
		t.Errorf("first backoff = %v, want %v", first, interval)
	}
	if second := growBackoff(first, interval); second != 2*interval {
		t.Errorf("second backoff = %v, want %v", second, 2*interval)
	}
	if capped := growBackoff(maxBackoff, interval); capped != maxBackoff {
		t.Errorf("capped backoff = %v, want %v", capped, maxBackoff)
	}
}

func TestNewLoopFallsBackToTheDefaultInterval(t *testing.T) {
	t.Parallel()
	for _, given := range []time.Duration{0, -time.Minute} {
		loop := NewLoop(nil, nil, nil, given)
		if loop.interval != DefaultInterval {
			t.Errorf("NewLoop(%v).interval = %v, want %v", given, loop.interval, DefaultInterval)
		}
	}
}

func TestEligibleAccountsNeedASession(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		connected := seedAccount(t, tx, "refresh-1")
		// No session means nothing to renew, so a round would be a call to a rate-limited
		// service for nothing. It stays pending until somebody connects it.
		notConnected := seedAccount(t, tx, "")

		accounts, err := eligibleAccounts(tx)
		if err != nil {
			t.Fatalf("eligibleAccounts: %v", err)
		}

		eligible := map[string]bool{}
		for _, account := range accounts {
			eligible[account.ID] = true
		}
		if !eligible[connected.ID] {
			t.Error("a tracked, connected account was left out")
		}
		if eligible[notConnected.ID] {
			t.Error("an account with no session was included")
		}
	})
}

// The two flags answer different questions, and the ticker asks only one of them. An account
// administered for someone else has a statement worth reading and no inventory to cost, and
// before migration 64 it could not be read at all.
func TestEligibilityFollowsTrackingNotOwnership(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		tracked := seedAccount(t, tx, "refresh-1")

		// Tracked, but the points are somebody else's: still read.
		notOurs := seedAccount(t, tx, "refresh-2")
		if err := tx.Model(&notOurs).Update("owns_point_inventory", false).Error; err != nil {
			t.Fatalf("updating the account: %v", err)
		}

		// Ours to cost, but nobody asked for its statement: not read. Turning tracking off is
		// how an operator stops a sync without disconnecting the account.
		untracked := seedAccount(t, tx, "refresh-3")
		if err := tx.Model(&untracked).Update("tracks_statement", false).Error; err != nil {
			t.Fatalf("updating the account: %v", err)
		}

		accounts, err := eligibleAccounts(tx)
		if err != nil {
			t.Fatalf("eligibleAccounts: %v", err)
		}
		eligible := map[string]bool{}
		for _, account := range accounts {
			eligible[account.ID] = true
		}

		if !eligible[tracked.ID] {
			t.Error("a tracked account with a session was left out")
		}
		if !eligible[notOurs.ID] {
			t.Error("a tracked account was left out because its points are not ours to cost")
		}
		if eligible[untracked.ID] {
			t.Error("an untracked account was read anyway")
		}
	})
}
