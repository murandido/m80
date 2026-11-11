package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func mustCreate(t *testing.T, tx *gorm.DB, value any) {
	t.Helper()
	if err := tx.Create(value).Error; err != nil {
		t.Fatalf("seed %T: %v", value, err)
	}
}

// portalAccount seeds an account in the program with the real code the portal dispatches on.
// The code is unique, so a parallel test holding it makes this one wait for its rollback.
func portalAccount(t *testing.T, tx *gorm.DB, code string) data.Account {
	t.Helper()
	var program data.LoyaltyProgram
	if err := tx.Where("code = ?", code).First(&program).Error; err != nil {
		suffix := uuid.New().String()[:8]
		airline := data.Airline{Name: "Air " + suffix, Code: "T" + suffix}
		mustCreate(t, tx, &airline)
		program = data.LoyaltyProgram{Name: code, Code: code, DefaultAirlineID: airline.ID}
		mustCreate(t, tx, &program)
	}
	holder := data.Person{Name: "Titular " + uuid.New().String()[:8]}
	mustCreate(t, tx, &holder)
	//nolint:gosec // G101: fixed test fixture, not a real credential
	login, password := "12345678900", "senha-antiga"
	account := data.Account{
		HolderID: &holder.ID, LoyaltyProgramID: program.ID, TracksStatement: true,
		Login: &login, Password: &password,
	}
	mustCreate(t, tx, &account)
	return account
}

// statementClient seeds a statement_client granted the given accounts and returns its token.
func statementClient(t *testing.T, tx *gorm.DB, accounts ...data.Account) string {
	t.Helper()
	token := tokenForRole(t, tx, data.RoleStatementClient)
	var user data.User
	if err := tx.Where("role = ?", data.RoleStatementClient).Order("created_at DESC").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		mustCreate(t, tx, &data.UserAccount{UserID: user.ID, AccountID: account.ID})
	}
	return token
}

func call(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPortal_ClientSeesOnlyTheirAccounts(t *testing.T) {
	db := setupTestDB(t)
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })
	router := newTestRouter(tx)

	granted := portalAccount(t, tx, "AZUL")
	other := portalAccount(t, tx, "AZUL")
	token := statementClient(t, tx, granted)

	rec := call(router, http.MethodGet, "/portal/accounts", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "senha-antiga") || strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("the list leaks the password: %s", rec.Body.String())
	}
	var list struct {
		Data []struct {
			ID          string `json:"id"`
			ProgramCode string `json:"program_code"`
			HasPassword bool   `json:"has_password"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0].ID != granted.ID || list.Data[0].ProgramCode != "AZUL" || !list.Data[0].HasPassword {
		t.Fatalf("list = %+v, want only the granted Azul account", list.Data)
	}

	if rec := call(router, http.MethodGet, "/portal/accounts/"+granted.ID+"/entries", token, ""); rec.Code != http.StatusOK {
		t.Errorf("granted entries: %d %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/entries", "/status", "/export"} {
		if rec := call(router, http.MethodGet, "/portal/accounts/"+other.ID+path, token, ""); rec.Code != http.StatusNotFound {
			t.Errorf("someone else's account %s: %d, want 404", path, rec.Code)
		}
	}
	if rec := call(router, http.MethodGet, "/portal/accounts/not-a-uuid/entries", token, ""); rec.Code != http.StatusNotFound {
		t.Errorf("malformed id: %d, want 404", rec.Code)
	}

	// A granted account that was archived stops being reachable.
	if err := tx.Delete(&granted).Error; err != nil {
		t.Fatal(err)
	}
	if rec := call(router, http.MethodGet, "/portal/accounts/"+granted.ID+"/entries", token, ""); rec.Code != http.StatusNotFound {
		t.Errorf("deleted account: %d, want 404", rec.Code)
	}
}

func TestPortal_PasswordIsWriteOnlyAndConnectFollowsTheProgram(t *testing.T) {
	db := setupTestDB(t)
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })
	router := newTestRouter(tx)

	azul := portalAccount(t, tx, "AZUL")
	smiles := portalAccount(t, tx, "SMILES")
	token := statementClient(t, tx, azul, smiles)

	rec := call(router, http.MethodPut, "/portal/accounts/"+azul.ID+"/password", token, `{"password": "senha-nova"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("password: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "senha-nova") {
		t.Fatal("the password route echoed the password")
	}
	var stored string
	if err := tx.Raw("SELECT password FROM accounts WHERE id = ?", azul.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "enc:") {
		t.Fatalf("password stored as %q, want encrypted", stored)
	}
	var reloaded data.Account
	if err := tx.First(&reloaded, "id = ?", azul.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.Password == nil || *reloaded.Password != "senha-nova" {
		t.Fatalf("decrypted password = %v, want senha-nova", reloaded.Password)
	}

	if rec := call(router, http.MethodPut, "/portal/accounts/"+azul.ID+"/password", token, `{"password": "  "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("blank password: %d, want 400", rec.Code)
	}

	// The Azul connect only exists for an Azul account, and there is no Smiles connect at all.
	if rec := call(router, http.MethodPost, "/portal/accounts/"+smiles.ID+"/azul-connect", token, ""); rec.Code != http.StatusNotFound {
		t.Errorf("azul-connect on a Smiles account: %d, want 404", rec.Code)
	}
	if rec := call(router, http.MethodPost, "/portal/accounts/"+smiles.ID+"/smiles-connect", token, ""); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("smiles-connect: %d, want no such route", rec.Code)
	}
}

// The last sync error is raw (internal hostnames, proxy errors) and stays with the team.
func TestPortal_SyncDetailIsNotShown(t *testing.T) {
	db := setupTestDB(t)
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })
	router := newTestRouter(tx)

	azul := portalAccount(t, tx, "AZUL")
	token := statementClient(t, tx, azul)
	detail := "promo service unreachable: azul-login-service-dev:8083"
	if err := tx.Create(&data.AzulSyncState{AccountID: azul.ID, State: "error", Detail: &detail}).Error; err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/status", "/entries"} {
		rec := call(router, http.MethodGet, "/portal/accounts/"+azul.ID+path, token, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "azul-login-service") {
			t.Errorf("%s leaks the sync detail: %s", path, rec.Body.String())
		}
	}
}

func TestPortal_StaffAreKeptOut(t *testing.T) {
	db := setupTestDB(t)
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })
	router := newTestRouter(tx)

	for _, role := range []string{data.RoleUser, data.RoleAdmin} {
		if rec := call(router, http.MethodGet, "/portal/accounts", tokenForRole(t, tx, role), ""); rec.Code != http.StatusForbidden {
			t.Errorf("GET /portal/accounts as %s: %d, want 403", role, rec.Code)
		}
	}
}
