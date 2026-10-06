package azulsync

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestGrowBackoff(t *testing.T) {
	t.Parallel()
	interval := 10 * time.Minute

	// Starts at the interval, doubles, and caps at maxBackoff.
	first := growBackoff(0, interval)
	if first != interval {
		t.Errorf("first backoff = %v, want %v", first, interval)
	}
	if got := growBackoff(first, interval); got != 20*time.Minute {
		t.Errorf("second backoff = %v, want 20m", got)
	}
	if got := growBackoff(45*time.Minute, interval); got != maxBackoff {
		t.Errorf("backoff = %v, want it capped at %v", got, maxBackoff)
	}
}

func TestEligibleAccountsFiltersByKeyAlone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		connected := seedAccount(t, tx, "cust-key-1") // tracked + has a key
		noKey := seedAccount(t, tx, "")               // tracked, no key yet

		// An account that does not track its statement is still read: its balance is read for
		// every connected account, and SyncAccount stops there for it.
		notTracked := seedAccount(t, tx, "cust-key-2")
		if err := tx.Model(&notTracked).Update("tracks_statement", false).Error; err != nil {
			t.Fatalf("flipping tracking: %v", err)
		}

		// Whereas an account whose points are not ours to price IS read: its statement is worth
		// watching even though it has no inventory to cost. See migration 64.
		notOwned := seedAccount(t, tx, "cust-key-3")
		if err := tx.Model(&notOwned).Update("owns_point_inventory", false).Error; err != nil {
			t.Fatalf("flipping ownership: %v", err)
		}

		accounts, err := eligibleAccounts(tx)
		if err != nil {
			t.Fatalf("eligibleAccounts: %v", err)
		}

		got := map[string]bool{}
		for _, a := range accounts {
			got[a.ID] = true
		}
		if !got[connected.ID] {
			t.Error("the connected account was not eligible")
		}
		if got[noKey.ID] {
			t.Error("an account with no customerKey was eligible")
		}
		if !got[notTracked.ID] {
			t.Error("a connected account was skipped because it does not track its statement")
		}
		if !got[notOwned.ID] {
			t.Error("a tracked account was skipped because its points are not ours to cost")
		}

		// The eligible account's key comes back decrypted, ready for the reader.
		for _, a := range accounts {
			if a.ID == connected.ID && (a.AzulCustomerKey == nil || *a.AzulCustomerKey != "cust-key-1") {
				t.Errorf("customerKey = %v, want the decrypted cust-key-1", a.AzulCustomerKey)
			}
		}
	})
}
