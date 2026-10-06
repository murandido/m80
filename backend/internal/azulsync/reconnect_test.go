package azulsync

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/murandido/m-80-backend/internal/azul"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// What happens when Azul stops recognising a stored customerKey (points/resume answers 404): the
// account is parked in needs_reconnect, the ticker stops reading it, and it gets one automatic
// login. Measured on 05/10/2026: a password change left an account failing 717 rounds in a row.

// countingStarter records which accounts the loop asked to reconnect.
type countingStarter struct {
	mu     sync.Mutex
	starts []string
}

func (c *countingStarter) Start(_ context.Context, accountID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.starts = append(c.starts, accountID)
	return nil
}

func TestSyncAccount_RefusedKeyParksTheAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "dead-key")
		// Failures from before the key died should not linger once it is parked.
		mustCreate(t, tx, &data.AzulSyncState{AccountID: account.ID, State: data.AzulSyncError, ConsecutiveFailures: 2})

		err := SyncAccount(context.Background(), tx, &fakeReader{balanceErr: azul.ErrNotFound}, account, day("2026-10-05"))
		if !errors.Is(err, ErrCustomerKeyRefused) {
			t.Fatalf("err = %v, want ErrCustomerKeyRefused", err)
		}
		state := stateOf(t, tx, account.ID)
		if state.State != data.AzulSyncNeedsReconnect {
			t.Errorf("state = %q, want needs_reconnect", state.State)
		}
		if state.ConsecutiveFailures != 0 {
			t.Errorf("failures = %d, want 0", state.ConsecutiveFailures)
		}
	})
}

func TestEligibleAccountsSkipsAParkedAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		parked := seedAccount(t, tx, "dead-key")
		mustCreate(t, tx, &data.AzulSyncState{AccountID: parked.ID, State: data.AzulSyncNeedsReconnect})
		// needs_attention is still read: its cause may be transient (the edge), unlike a dead key.
		attention := seedAccount(t, tx, "live-key")
		mustCreate(t, tx, &data.AzulSyncState{AccountID: attention.ID, State: data.AzulSyncNeedsAttention})

		accounts, err := eligibleAccounts(tx)
		if err != nil {
			t.Fatalf("eligibleAccounts: %v", err)
		}
		got := map[string]bool{}
		for _, a := range accounts {
			got[a.ID] = true
		}
		if got[parked.ID] {
			t.Error("an account parked in needs_reconnect was eligible")
		}
		if !got[attention.ID] {
			t.Error("an account in needs_attention was skipped")
		}
	})
}

func TestRunPassReconnectsARefusedKeyOnlyOnce(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "dead-key")
		starter := &countingStarter{}
		loop := NewLoop(tx, &fakeReader{balanceErr: azul.ErrNotFound}, starter, 0)

		loop.runPass(context.Background())
		loop.runPass(context.Background())

		if len(starter.starts) != 1 || starter.starts[0] != account.ID {
			t.Errorf("reconnects = %v, want exactly one for %s", starter.starts, account.ID)
		}
	})
}

func TestMarkLoginFailureKeepsAParkedAccountParked(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "dead-key")
		mustCreate(t, tx, &data.AzulSyncState{AccountID: account.ID, State: data.AzulSyncNeedsReconnect})

		if err := MarkLoginFailure(tx, account.ID, errors.New("wrong password")); err != nil {
			t.Fatalf("MarkLoginFailure: %v", err)
		}
		state := stateOf(t, tx, account.ID)
		if state.State != data.AzulSyncNeedsReconnect {
			t.Errorf("state = %q, want needs_reconnect (a failed login must not put it back in the ticker)", state.State)
		}
		if state.ConsecutiveFailures != 0 {
			t.Errorf("failures = %d, want 0", state.ConsecutiveFailures)
		}
	})
}

func TestConnectorRunUnparksTheAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "dead-key")
		withCredentials(t, tx, account)
		mustCreate(t, tx, &data.AzulSyncState{AccountID: account.ID, State: data.AzulSyncNeedsReconnect})

		connector := NewConnector(tx, NewConnectTracker(), &fakeLogin{key: "new-key"}, nil)
		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading: %v", err)
		}
		connector.Tracker.Begin(account.ID)
		connector.run(context.Background(), reloaded)

		if got := stateOf(t, tx, account.ID).State; got != data.AzulSyncPending {
			t.Errorf("state = %q, want pending (eligible again)", got)
		}
	})
}
