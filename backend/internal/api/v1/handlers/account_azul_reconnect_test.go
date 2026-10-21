package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// fakeAzulStarter counts the Azul logins the account update asked for.
type fakeAzulStarter struct{ starts []string }

func (f *fakeAzulStarter) Start(_ context.Context, accountID string) error {
	f.starts = append(f.starts, accountID)
	return nil
}

// updateWithStarter PUTs body to the account with a counting connector behind the handler.
func updateWithStarter(t *testing.T, tx *gorm.DB, accountID, body string) *fakeAzulStarter {
	t.Helper()
	starter := &fakeAzulStarter{}
	handler := NewAccountHandler(tx)
	handler.AzulConnector = starter
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.UpdateAccount(rec, withIDs(req, map[string]string{"id": accountID}))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	return starter
}

func seedAzulAccountInState(t *testing.T, tx *gorm.DB, state string) data.Account {
	t.Helper()
	holder := seedPerson(t, tx, "Holder")
	password := "senha-antiga" //nolint:gosec // G101: fixed test fixture, not a real credential
	account := data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &holder.ID, Password: &password}
	create(t, tx, &account)
	create(t, tx, &data.AzulSyncState{AccountID: account.ID, State: state})
	return account
}

func TestUpdateAccount_NewPasswordReconnectsARefusedAzulAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAzulAccountInState(t, tx, data.AzulSyncNeedsReconnect)

		starter := updateWithStarter(t, tx, account.ID, `{"password":"senha-nova"}`)
		if len(starter.starts) != 1 || starter.starts[0] != account.ID {
			t.Errorf("starts = %v, want one for %s", starter.starts, account.ID)
		}
	})
}

func TestUpdateAccount_NoReconnectWithoutANewPasswordOrARefusedKey(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		// The same password resent (the edit form sends every field) is not a change.
		parked := seedAzulAccountInState(t, tx, data.AzulSyncNeedsReconnect)
		if starter := updateWithStarter(t, tx, parked.ID, `{"password":"senha-antiga"}`); len(starter.starts) != 0 {
			t.Errorf("same password: starts = %v, want none", starter.starts)
		}
		// Editing something else on a parked account does not log in either.
		if starter := updateWithStarter(t, tx, parked.ID, `{"waives_redemption_fee":true}`); len(starter.starts) != 0 {
			t.Errorf("no password: starts = %v, want none", starter.starts)
		}
		// A healthy account does not need a browser login because its password changed.
		healthy := seedAzulAccountInState(t, tx, data.AzulSyncOK)
		if starter := updateWithStarter(t, tx, healthy.ID, `{"password":"senha-nova"}`); len(starter.starts) != 0 {
			t.Errorf("healthy account: starts = %v, want none", starter.starts)
		}
	})
}

// The card reads the sync state off the account itself, so it can offer Reconectar without a
// request per card.
func TestFillLastSynced_CarriesTheAzulSyncState(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAzulAccountInState(t, tx, data.AzulSyncNeedsReconnect)

		responses := []schemas.AccountResponse{{ID: account.ID, AzulConnected: true}}
		fillLastSynced(tx, responses)
		if got := responses[0].AzulSyncState; got == nil || *got != data.AzulSyncNeedsReconnect {
			t.Errorf("azul_sync_state = %v, want needs_reconnect", got)
		}
		// Never synced: the state is there, the "atualizado há…" is not.
		if responses[0].AzulLastSyncedAt != nil {
			t.Errorf("azul_last_synced_at = %v, want nil", responses[0].AzulLastSyncedAt)
		}
	})
}
