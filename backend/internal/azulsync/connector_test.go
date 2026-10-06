package azulsync

import (
	"context"
	"errors"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/promotion"
	"gorm.io/gorm"
)

// fakeLogin stands in for the login service: it hands back key, or fails with err.
type fakeLogin struct {
	key   string
	err   error
	calls int
}

func (f *fakeLogin) AzulLogin(_ context.Context, _ promotion.AzulLoginRequest) (string, error) {
	f.calls++
	return f.key, f.err
}

// withCredentials stores a login and password on the seeded account, which Start requires.
func withCredentials(t *testing.T, tx *gorm.DB, account data.Account) {
	t.Helper()
	login, password := "12345678900", "senha-nova"
	account.Login, account.Password = &login, &password
	// Save, not Update: BeforeSave is what encrypts the password at rest.
	if err := tx.Save(&account).Error; err != nil {
		t.Fatalf("storing credentials: %v", err)
	}
}

func TestConnectorStartRefusesWhatCannotLogIn(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		connector := NewConnector(tx, NewConnectTracker(), &fakeLogin{}, nil)

		noCredentials := seedAccount(t, tx, "")
		if err := connector.Start(context.Background(), noCredentials.ID); !errors.Is(err, ErrMissingCredentials) {
			t.Errorf("no credentials: err = %v, want ErrMissingCredentials", err)
		}

		notTracked := seedAccount(t, tx, "")
		withCredentials(t, tx, notTracked)
		if err := tx.Model(&data.Account{}).Where("id = ?", notTracked.ID).Update("tracks_statement", false).Error; err != nil {
			t.Fatalf("flipping tracking: %v", err)
		}
		if err := connector.Start(context.Background(), notTracked.ID); !errors.Is(err, ErrNotTracked) {
			t.Errorf("not tracked: err = %v, want ErrNotTracked", err)
		}

		if err := connector.Start(context.Background(), "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrAccountNotFound) {
			t.Errorf("unknown account: err = %v, want ErrAccountNotFound", err)
		}

		// A login already in flight for the account refuses a second browser.
		busy := seedAccount(t, tx, "")
		withCredentials(t, tx, busy)
		connector.Tracker.Begin(busy.ID)
		if err := connector.Start(context.Background(), busy.ID); !errors.Is(err, ErrAlreadyConnecting) {
			t.Errorf("in flight: err = %v, want ErrAlreadyConnecting", err)
		}
	})
}

// run is what Start launches in the background; it is called directly here so the test does not
// race a goroutine over the test transaction.
func TestConnectorRunStoresTheNewKey(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "old-key")
		withCredentials(t, tx, account)

		login := &fakeLogin{key: "new-key"}
		connector := NewConnector(tx, NewConnectTracker(), login, nil)
		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading: %v", err)
		}
		connector.Tracker.Begin(account.ID)
		connector.run(context.Background(), reloaded)

		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading: %v", err)
		}
		if reloaded.AzulCustomerKey == nil || *reloaded.AzulCustomerKey != "new-key" {
			t.Errorf("customerKey = %v, want new-key", reloaded.AzulCustomerKey)
		}
		// Back to pending: the account is eligible for the ticker again.
		if got := stateOf(t, tx, account.ID).State; got != data.AzulSyncPending {
			t.Errorf("state = %q, want pending", got)
		}
		if connector.Tracker.IsConnecting(account.ID) {
			t.Error("the tracker mark was not cleared")
		}
	})
}
