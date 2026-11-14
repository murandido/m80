package azulsync

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/carriers/azul"
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

// Connecting is open to every account: the balance is read for all of them, and "Acompanhar
// extrato" only decides whether the statement is pulled on top of it.
func TestConnectorStartsAnAccountThatDoesNotTrackItsStatement(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		connector := NewConnector(tx, NewConnectTracker(), &fakeLogin{}, nil)
		notTracked := seedAccount(t, tx, "")
		withCredentials(t, tx, notTracked)
		if err := tx.Model(&data.Account{}).Where("id = ?", notTracked.ID).Update("tracks_statement", false).Error; err != nil {
			t.Fatalf("flipping tracking: %v", err)
		}
		// A login already marked in flight keeps Start from launching the background goroutine,
		// which would race the test transaction. Reaching that refusal is the proof: it is the
		// last check, after every validation of the account itself.
		connector.Tracker.Begin(notTracked.ID)
		if err := connector.Start(context.Background(), notTracked.ID); !errors.Is(err, ErrAlreadyConnecting) {
			t.Errorf("not tracked: err = %v, want it to pass validation (ErrAlreadyConnecting)", err)
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

// A connect the Azul edge turned away says nothing about the password: the reason is written for
// the operator, and neither the state nor the failure streak moves.
func TestConnectorRunRecordsAnEdgeBlockWithoutCountingIt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "")
		withCredentials(t, tx, account)
		login := &fakeLogin{err: &promotion.AzulLoginError{Reason: promotion.AzulLoginReasonBlocked}}
		connector := NewConnector(tx, NewConnectTracker(), login, nil)
		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading: %v", err)
		}
		connector.Tracker.Begin(account.ID)
		connector.run(context.Background(), reloaded)

		state := stateOf(t, tx, account.ID)
		if state.State != data.AzulSyncPending || state.ConsecutiveFailures != 0 {
			t.Errorf("state = %q failures = %d, want pending with 0", state.State, state.ConsecutiveFailures)
		}
		if state.Detail == nil || *state.Detail != ConnectFailureMessage(login.err) {
			t.Errorf("detail = %v, want the edge-block message", state.Detail)
		}
	})
}

// A refused password does count, and the operator reads why in Portuguese.
func TestConnectorRunCountsARefusedPassword(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAccount(t, tx, "")
		withCredentials(t, tx, account)
		login := &fakeLogin{err: &promotion.AzulLoginError{Reason: promotion.AzulLoginReasonCredentials}}
		connector := NewConnector(tx, NewConnectTracker(), login, nil)
		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading: %v", err)
		}
		connector.Tracker.Begin(account.ID)
		connector.run(context.Background(), reloaded)

		state := stateOf(t, tx, account.ID)
		if state.State != data.AzulSyncError || state.ConsecutiveFailures != 1 {
			t.Errorf("state = %q failures = %d, want error with 1", state.State, state.ConsecutiveFailures)
		}
		if state.Detail == nil || *state.Detail != ConnectFailureMessage(login.err) {
			t.Errorf("detail = %v, want the refused-password message", state.Detail)
		}
	})
}

func TestConnectFailureMessageNeverShowsAStatusCode(t *testing.T) {
	reasons := []string{
		promotion.AzulLoginReasonCredentials, promotion.AzulLoginReasonLocked,
		promotion.AzulLoginReasonRejected, promotion.AzulLoginReasonNoKey,
		promotion.AzulLoginReasonBlocked, promotion.AzulLoginReasonBusy,
		promotion.AzulLoginReasonDisabled, promotion.AzulLoginReasonUnavailable, "unknown",
	}
	seen := map[string]bool{}
	for _, reason := range reasons {
		message := ConnectFailureMessage(&promotion.AzulLoginError{Reason: reason, Message: "HTTP 401"})
		if strings.Contains(message, "HTTP") {
			t.Errorf("%s: message %q leaks the technical detail", reason, message)
		}
		seen[message] = true
	}
	if len(seen) < 8 {
		t.Errorf("only %d distinct messages for %d reasons", len(seen), len(reasons))
	}
	if got := ConnectFailureMessage(errors.New("dial tcp: refused")); strings.Contains(got, "dial") {
		t.Errorf("a plain error leaked into the message: %q", got)
	}
}

// The browser fallback: a key captured in the operator's browser is checked before it is kept.
func TestStoreBrowserKey(t *testing.T) {
	keyOf := func(t *testing.T, tx *gorm.DB, id string) string {
		t.Helper()
		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", id).Error; err != nil {
			t.Fatalf("reloading: %v", err)
		}
		if reloaded.AzulCustomerKey == nil {
			return ""
		}
		return *reloaded.AzulCustomerKey
	}

	t.Run("stores a key that Azul answers for, from a login with the account's CPF", func(t *testing.T) {
		runInTx(t, func(tx *gorm.DB) {
			account := seedAccount(t, tx, "")
			withCredentials(t, tx, account) // login 12345678900
			reader := &fakeReader{balances: []azul.Balance{{Points: 1000}}}
			connector := NewConnector(tx, NewConnectTracker(), &fakeLogin{}, reader)

			// The extension reports the CPF as typed, which may carry the mask.
			maskedCPF := "123.456.789-00"
			if err := connector.StoreBrowserKey(context.Background(), account.ID, "browser-key", maskedCPF); err != nil {
				t.Fatalf("StoreBrowserKey: %v", err)
			}
			if got := keyOf(t, tx, account.ID); got != "browser-key" {
				t.Errorf("customerKey = %q, want browser-key", got)
			}
			if got := stateOf(t, tx, account.ID).State; got != data.AzulSyncPending {
				t.Errorf("state = %q, want pending", got)
			}
			if connector.Tracker.IsConnecting(account.ID) {
				t.Error("the tracker mark was not cleared")
			}
		})
	})

	refusals := []struct {
		name     string
		reader   *fakeReader
		document string
		want     error
	}{
		{"another holder's login", &fakeReader{balances: []azul.Balance{{}}}, "99988877766", ErrDocumentMismatch},
		{"a key Azul does not know", &fakeReader{balanceErr: azul.ErrNotFound}, "12345678900", ErrKeyRejected},
		{"a check the edge blocked", &fakeReader{balanceErr: azul.ErrEdgeBlocked}, "12345678900", ErrKeyUnverified},
		{"no read client", nil, "12345678900", ErrVerifierUnavailable},
	}
	for _, tc := range refusals {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				account := seedAccount(t, tx, "")
				withCredentials(t, tx, account)
				var reader Reader
				if tc.reader != nil {
					reader = tc.reader
				}
				connector := NewConnector(tx, NewConnectTracker(), &fakeLogin{}, reader)

				err := connector.StoreBrowserKey(context.Background(), account.ID, "browser-key", tc.document)
				if !errors.Is(err, tc.want) {
					t.Fatalf("err = %v, want %v", err, tc.want)
				}
				if got := keyOf(t, tx, account.ID); got != "" {
					t.Errorf("customerKey = %q, want nothing stored", got)
				}
			})
		})
	}

	t.Run("refuses while a server-side login runs", func(t *testing.T) {
		runInTx(t, func(tx *gorm.DB) {
			account := seedAccount(t, tx, "")
			withCredentials(t, tx, account)
			reader := &fakeReader{balances: []azul.Balance{{}}}
			connector := NewConnector(tx, NewConnectTracker(), &fakeLogin{}, reader)
			connector.Tracker.Begin(account.ID)

			err := connector.StoreBrowserKey(context.Background(), account.ID, "browser-key", "12345678900")
			if !errors.Is(err, ErrAlreadyConnecting) {
				t.Fatalf("err = %v, want ErrAlreadyConnecting", err)
			}
			if reader.balanceAt != 0 {
				t.Error("the key was checked with Azul while another login was running")
			}
		})
	})
}
