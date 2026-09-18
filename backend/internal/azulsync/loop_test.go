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

func TestEligibleAccountsFiltersByOwnershipAndKey(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		connected := seedAccount(t, tx, "cust-key-1") // owns inventory + has a key
		noKey := seedAccount(t, tx, "")               // owns inventory, no key yet

		// An account that does not own its inventory, even with a key, is not eligible.
		notOwned := seedAccount(t, tx, "cust-key-2")
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
		if got[notOwned.ID] {
			t.Error("an account that does not own its inventory was eligible")
		}

		// The eligible account's key comes back decrypted, ready for the reader.
		for _, a := range accounts {
			if a.ID == connected.ID && (a.AzulCustomerKey == nil || *a.AzulCustomerKey != "cust-key-1") {
				t.Errorf("customerKey = %v, want the decrypted cust-key-1", a.AzulCustomerKey)
			}
		}
	})
}
