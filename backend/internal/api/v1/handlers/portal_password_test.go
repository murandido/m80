package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// portalPasswordWithStarter PUTs a new airline password from the portal, as RequireLinkedAccount
// would hand it over (the program code already in the context), with a counting connector behind.
func portalPasswordWithStarter(t *testing.T, tx *gorm.DB, accountID, program, body string) *fakeAzulStarter {
	t.Helper()
	starter := &fakeAzulStarter{}
	handler := NewPortalHandler(tx, NewStatementHandler(tx), nil)
	handler.AzulConnector = starter
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.LinkedProgramContextKey, program))
	rec := httptest.NewRecorder()
	handler.UpdatePortalPassword(rec, withIDs(req, map[string]string{"id": accountID}))
	if rec.Code != http.StatusOK {
		t.Fatalf("portal password: %d %s", rec.Code, rec.Body.String())
	}
	return starter
}

// A client who changed the password at Azul is exactly who lands in needs_reconnect, so saving
// the new one in the portal must start the same single retry the staff screen starts.
func TestUpdatePortalPassword_NewPasswordReconnectsARefusedAzulAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account := seedAzulAccountInState(t, tx, data.AzulSyncNeedsReconnect)

		starter := portalPasswordWithStarter(t, tx, account.ID, "AZUL", `{"password":"senha-nova"}`)
		if len(starter.starts) != 1 || starter.starts[0] != account.ID {
			t.Errorf("starts = %v, want one for %s", starter.starts, account.ID)
		}
	})
}

func TestUpdatePortalPassword_NoReconnectWithoutANewPasswordOrARefusedKey(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		// Resending the password Azul already refused would only spend a login on it.
		parked := seedAzulAccountInState(t, tx, data.AzulSyncNeedsReconnect)
		if starter := portalPasswordWithStarter(t, tx, parked.ID, "AZUL", `{"password":"senha-antiga"}`); len(starter.starts) != 0 {
			t.Errorf("same password: starts = %v, want none", starter.starts)
		}
		healthy := seedAzulAccountInState(t, tx, data.AzulSyncOK)
		if starter := portalPasswordWithStarter(t, tx, healthy.ID, "AZUL", `{"password":"senha-nova"}`); len(starter.starts) != 0 {
			t.Errorf("healthy account: starts = %v, want none", starter.starts)
		}
		// A Smiles account has no Azul login to start, whatever its rows say.
		if starter := portalPasswordWithStarter(t, tx, parked.ID, "SMILES", `{"password":"outra-senha"}`); len(starter.starts) != 0 {
			t.Errorf("smiles account: starts = %v, want none", starter.starts)
		}
	})
}
