package latamsync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/latam"
	"gorm.io/gorm"
)

var roundTime = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func TestSyncAccountWritesTheBalance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		reader := &fakeReader{miles: 84321}

		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, roundTime); err != nil {
			t.Fatalf("SyncAccount: %v", err)
		}

		reloaded := reload(t, tx, account.ID)
		if reloaded.RealBalancePoints == nil || *reloaded.RealBalancePoints != 84321 {
			t.Errorf("real balance = %v, want 84321", reloaded.RealBalancePoints)
		}
		if reloaded.RealBalanceCheckedOn == nil {
			t.Error("the day of the reading was not recorded")
		}
		state := stateOf(t, tx, account.ID)
		if state.State != data.LatamSyncOK || state.LastSyncedAt == nil || state.ConsecutiveFailures != 0 {
			t.Errorf("state = %+v, want ok with a sync time", state)
		}
	})
}

func TestSyncAccountStoresTheRotatedSessionBeforeReading(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		// The balance read fails, so the round never writes a balance. The rotated token must
		// already be stored: if the tenant rotated it, the old one is dead.
		reader := &fakeReader{rotatedTo: "refresh-2", balanceErr: latam.ErrUnavailable}

		if err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, roundTime); err == nil {
			t.Fatal("expected the round to fail")
		}
		stored := reload(t, tx, account.ID).LatamRefreshToken
		if stored == nil || *stored != "refresh-2" {
			t.Fatalf("stored session = %v, want the rotated token", stored)
		}
		if state := stateOf(t, tx, account.ID); state.State != data.LatamSyncError || state.ConsecutiveFailures != 1 {
			t.Errorf("state = %q failures=%d, want error with one failure", state.State, state.ConsecutiveFailures)
		}
	})
}

func TestSyncAccountAsksForANewLoginWhenTheSessionIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		reader := &fakeReader{refreshErr: latam.ErrSessionExpired}

		err := SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, roundTime)
		if !errors.Is(err, latam.ErrSessionExpired) {
			t.Fatalf("err = %v, want ErrSessionExpired", err)
		}
		if state := stateOf(t, tx, account.ID); state.State != data.LatamSyncNeedsLogin {
			t.Errorf("state = %q, want needs_login", state.State)
		}
		if reader.balanceCalls != 0 {
			t.Error("the balance was read with a refused session")
		}

		// And the ticker stops renewing it: only a new login can help.
		accounts, err := eligibleAccounts(tx)
		if err != nil {
			t.Fatalf("eligibleAccounts: %v", err)
		}
		for _, a := range accounts {
			if a.ID == account.ID {
				t.Error("an account waiting on a new login is still eligible")
			}
		}
	})
}

func TestSyncAccountRaisesForAttentionAfterEnoughFailures(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		reader := &fakeReader{balanceErr: latam.ErrUnavailable}

		for range failureThreshold {
			_ = SyncAccount(context.Background(), tx, reader, NewSessionGuard(), account, roundTime)
		}
		if state := stateOf(t, tx, account.ID); state.State != data.LatamSyncNeedsAttention {
			t.Errorf("state = %q after %d failures, want needs_attention", state.State, failureThreshold)
		}
	})
}

func TestConnectStoresTheSessionAndReadsTheBalance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "")
		// A previous refused session must not survive a new login.
		if err := markNeedsLogin(tx, account.ID, "expirou"); err != nil {
			t.Fatal(err)
		}
		reader := &fakeReader{miles: 1000}
		session := latam.AccountSession{AccessToken: "a", IDToken: "i", RefreshToken: "fresh", UserID: "u"}

		if err := Connect(context.Background(), tx, reader, NewSessionGuard(), account.ID, session, roundTime); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		reloaded := reload(t, tx, account.ID)
		if reloaded.LatamRefreshToken == nil || *reloaded.LatamRefreshToken != "fresh" {
			t.Errorf("stored session = %v, want the new one", reloaded.LatamRefreshToken)
		}
		if reloaded.RealBalancePoints == nil || *reloaded.RealBalancePoints != 1000 {
			t.Errorf("real balance = %v, want 1000", reloaded.RealBalancePoints)
		}
		if state := stateOf(t, tx, account.ID); state.State != data.LatamSyncOK {
			t.Errorf("state = %q, want ok", state.State)
		}
	})
}

func TestDisconnectForgetsTheSessionButKeepsTheBalance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "refresh-1")
		if err := SyncAccount(context.Background(), tx, &fakeReader{miles: 500}, NewSessionGuard(), account, roundTime); err != nil {
			t.Fatal(err)
		}

		if err := Disconnect(tx, NewSessionGuard(), account.ID); err != nil {
			t.Fatalf("Disconnect: %v", err)
		}
		reloaded := reload(t, tx, account.ID)
		if reloaded.LatamRefreshToken != nil {
			t.Error("the session survived a disconnect")
		}
		if reloaded.RealBalancePoints == nil || *reloaded.RealBalancePoints != 500 {
			t.Errorf("real balance = %v, want the last reading kept", reloaded.RealBalancePoints)
		}
		var count int64
		tx.Model(&data.LatamSyncState{}).Where("account_id = ?", account.ID).Count(&count)
		if count != 0 {
			t.Error("the state row survived a disconnect")
		}
	})
}

func TestEligibleAccountsAreTheLinkedOnes(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		linked := seedAccount(t, tx, "refresh-1")
		unlinked := seedAccount(t, tx, "")

		accounts, err := eligibleAccounts(tx)
		if err != nil {
			t.Fatalf("eligibleAccounts: %v", err)
		}
		got := map[string]data.Account{}
		for _, a := range accounts {
			got[a.ID] = a
		}
		if a, ok := got[linked.ID]; !ok {
			t.Error("the linked account was not eligible")
		} else if a.LatamRefreshToken == nil || *a.LatamRefreshToken != "refresh-1" {
			t.Errorf("token = %v, want it decrypted for the reader", a.LatamRefreshToken)
		}
		if _, ok := got[unlinked.ID]; ok {
			t.Error("an account with no session was eligible")
		}
	})
}

func TestGrowBackoff(t *testing.T) {
	t.Parallel()
	if got := growBackoff(0, DefaultInterval); got != DefaultInterval {
		t.Errorf("first backoff = %v, want %v", got, DefaultInterval)
	}
	if got := growBackoff(DefaultInterval, DefaultInterval); got != maxBackoff {
		t.Errorf("second backoff = %v, want it capped at %v", got, maxBackoff)
	}
}
