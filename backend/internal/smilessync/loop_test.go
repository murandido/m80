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

func TestEligibleAccountsAreTheConnectedInventoryOnes(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		connected := seedAccount(t, tx, "refresh-1")
		notConnected := seedAccount(t, tx, "")

		// An account that does not own its inventory has no costing to feed, so a round on it
		// would be a call to a rate-limited service for nothing.
		notOurs := seedAccount(t, tx, "refresh-2")
		if err := tx.Model(&notOurs).Update("owns_point_inventory", false).Error; err != nil {
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
		if !eligible[connected.ID] {
			t.Error("a connected inventory account was left out")
		}
		if eligible[notConnected.ID] {
			t.Error("an account with no session was included")
		}
		if eligible[notOurs.ID] {
			t.Error("an account that does not own its inventory was included")
		}
	})
}
