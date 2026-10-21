package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

type vaultWorld struct {
	user    data.User
	account data.Account
	program data.LoyaltyProgram
}

func seedVaultWorld(t *testing.T, tx *gorm.DB, password *string) vaultWorld {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Vault Air", Code: "V" + suffix}
	create(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Vault Program " + suffix, Code: "P" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)
	login := "login-" + suffix
	account := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: program.ID, Login: &login, Password: password}
	create(t, tx, &account)
	create(t, tx, &data.AccountNickname{AccountID: account.ID, LoyaltyProgramID: program.ID, Nickname: "nick-" + suffix})

	return vaultWorld{user: createUser(t, tx, "Vault user "+suffix), account: account, program: program}
}

func callCredentials(tx *gorm.DB, user data.User, accountID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/credentials", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.7:51234"
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", accountID)
	ctx := context.WithValue(asUser(req.Context(), user), chi.RouteCtxKey, rctx)

	rec := httptest.NewRecorder()
	NewAccountHandler(tx).PostAccountCredentials(rec, req.WithContext(ctx))
	return rec
}

func countAccesses(t *testing.T, tx *gorm.DB, accountID string) int64 {
	t.Helper()
	var count int64
	if err := tx.Model(&data.AccountCredentialAccess{}).Where("account_id = ?", accountID).Count(&count).Error; err != nil {
		t.Fatalf("failed to count credential accesses: %v", err)
	}
	return count
}

func TestGetVaultAccounts_ListsAccountsWithoutPasswords(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		secret := "s3cret-vault-password"
		world := seedVaultWorld(t, tx, &secret)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/accounts/vault", nil)
		NewAccountHandler(tx).GetVaultAccounts(rec, req.WithContext(asUser(req.Context(), world.user)))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), `"password"`) {
			t.Fatalf("the vault list must never carry a password: %s", rec.Body.String())
		}

		var envelope struct {
			Data []schemas.VaultAccountResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("failed to decode the response: %v", err)
		}
		var found *schemas.VaultAccountResponse
		for i := range envelope.Data {
			if envelope.Data[i].ID == world.account.ID {
				found = &envelope.Data[i]
			}
		}
		if found == nil {
			t.Fatalf("the seeded account is missing from the vault list")
		}
		if !found.HasPassword || found.Program.Code != world.program.Code || len(found.Nicknames) != 1 || found.HolderName == nil {
			t.Fatalf("unexpected vault row: %+v", *found)
		}
	})
}

func TestPostAccountCredentials_ReturnsThePasswordAndRecordsTheAccess(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		secret := "s3cret-vault-password"
		world := seedVaultWorld(t, tx, &secret)

		rec := callCredentials(tx, world.user, world.account.ID, `{"action":"fill"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var envelope struct {
			Data schemas.AccountCredentialsResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("failed to decode the response: %v", err)
		}
		if envelope.Data.Password != secret || envelope.Data.Login == nil || *envelope.Data.Login != *world.account.Login {
			t.Fatalf("unexpected credentials: %+v", envelope.Data)
		}

		var access data.AccountCredentialAccess
		if err := tx.Where("account_id = ?", world.account.ID).First(&access).Error; err != nil {
			t.Fatalf("the access was not recorded: %v", err)
		}
		if access.UserID != world.user.ID || access.Action != data.CredentialAccessFill ||
			access.ClientIP == nil || *access.ClientIP != "203.0.113.7" {
			t.Fatalf("unexpected access row: %+v", access)
		}
	})
}

func TestPostAccountCredentials_AccountWithoutPasswordRecordsNothing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedVaultWorld(t, tx, nil)

		rec := callCredentials(tx, world.user, world.account.ID, `{"action":"copy"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
		if countAccesses(t, tx, world.account.ID) != 0 {
			t.Fatalf("a refused read must not leave an access row")
		}
	})
}

func TestPostAccountCredentials_RejectsUnknownAccountsAndActions(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		secret := "s3cret-vault-password"
		world := seedVaultWorld(t, tx, &secret)

		if rec := callCredentials(tx, world.user, uuid.New().String(), `{"action":"copy"}`); rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for an unknown account, got %d", rec.Code)
		}
		if rec := callCredentials(tx, world.user, "not-a-uuid", `{"action":"copy"}`); rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for a malformed account id, got %d", rec.Code)
		}
		if rec := callCredentials(tx, world.user, world.account.ID, `{"action":"export"}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for an unknown action, got %d", rec.Code)
		}
		if countAccesses(t, tx, world.account.ID) != 0 {
			t.Fatalf("a rejected request must not leave an access row")
		}
	})
}

func TestPostAccountCredentials_ArchivedAccountIsNotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		secret := "s3cret-vault-password"
		world := seedVaultWorld(t, tx, &secret)
		if err := tx.Delete(&world.account).Error; err != nil {
			t.Fatalf("failed to archive the account: %v", err)
		}

		if rec := callCredentials(tx, world.user, world.account.ID, `{"action":"copy"}`); rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for an archived account, got %d", rec.Code)
		}
	})
}
