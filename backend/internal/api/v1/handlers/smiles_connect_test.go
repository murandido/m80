package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/smiles"
	"github.com/murandido/m-80-backend/internal/smilessync"
	"gorm.io/gorm"
)

// tenantStub stands in for the Smiles Auth0 tenant. Each field is one step's answer, so a test
// says what the tenant does and nothing else.
type tenantStub struct {
	loginStatus   int
	loginBody     string
	authenticator string
	challengeBody string
	codeStatus    int
	codeBody      string
}

func newTenant(t *testing.T, stub tenantStub) *smiles.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			// The same URL serves the password login and the code exchange; the grant says which.
			if grant, _ := body["grant_type"].(string); strings.Contains(grant, "mfa-oob") {
				if stub.codeStatus != 0 && stub.codeStatus != http.StatusOK {
					w.WriteHeader(stub.codeStatus)
				}
				_, _ = w.Write([]byte(stub.codeBody))
				return
			}
			if stub.loginStatus != 0 && stub.loginStatus != http.StatusOK {
				w.WriteHeader(stub.loginStatus)
			}
			_, _ = w.Write([]byte(stub.loginBody))
		case "/mfa/authenticators":
			_, _ = w.Write([]byte(stub.authenticator))
		case "/mfa/challenge":
			_, _ = w.Write([]byte(stub.challengeBody))
		}
	}))
	t.Cleanup(server.Close)
	return smiles.New(smiles.WithAuthBaseURL(server.URL))
}

// mfaRequiredTenant is the ordinary path: the login asks for a code, one SMS channel is offered,
// and the code exchange succeeds.
func mfaRequiredTenant(t *testing.T, codeStatus int, codeBody string) *smiles.Client {
	t.Helper()
	return newTenant(t, tenantStub{
		loginStatus:   http.StatusForbidden,
		loginBody:     `{"error":"mfa_required","mfa_token":"mfa-secret"}`,
		authenticator: `[{"id":"sms|1","authenticator_type":"oob","oob_channel":"sms","name":"+55 62 99999-9977"}]`,
		challengeBody: `{"oob_code":"oob-1","binding_method":"prompt"}`,
		codeStatus:    codeStatus,
		codeBody:      codeBody,
	})
}

func sessionBody(t *testing.T) string {
	t.Helper()
	return `{"access_token":"` + smilesTestToken(t) + `","refresh_token":"refresh-1","expires_in":7200}`
}

// smilesTestToken is an unsigned JWT carrying the two namespaced claims a session needs. It is
// built here rather than imported because the claim names are part of the contract under test.
func smilesTestToken(t *testing.T) string {
	t.Helper()
	claims := `{"https://smiles.api/memberNumber":"112233",` +
		`"https://smiles.api/oag_access_token":"oag-token","exp":` +
		itoaTest(time.Now().Add(2*time.Hour).Unix()) + `}`
	return "header." + base64URL(claims) + ".signature"
}

func seedSmilesAccount(t *testing.T, tx *gorm.DB) (data.Account, data.User) {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Gol " + suffix, Code: "G" + suffix}
	create(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Smiles " + suffix, Code: "S" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)
	user := createUser(t, tx, "Operador "+suffix)

	//nolint:gosec // G101: fixed test fixtures, not real credentials
	login, password := "12345678900", "senha-da-conta"
	account := data.Account{
		LoyaltyProgramID:   program.ID,
		OwnsPointInventory: true,
		TracksStatement:    true,
		Login:              &login,
		Password:           &password,
	}
	create(t, tx, &account)
	return account, user
}

func smilesRequest(method, accountID, userID, body string) *http.Request {
	req := httptest.NewRequest(method, "/accounts/"+accountID+"/smiles-connect", strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", accountID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{ID: userID, Role: data.RoleAdmin})
	return req.WithContext(ctx)
}

func newSmilesHandler(tx *gorm.DB, client *smiles.Client) *SmilesConnectHandler {
	handler := NewSmilesConnectHandler(tx, smilessync.NewLoginTracker(), smilessync.NewSessionGuard())
	handler.Client = client
	// No first sync: it runs on its own goroutine, and a test that asserts on what was stored
	// must not have one reading the transaction it is about to roll back.
	handler.AfterConnect = nil
	return handler
}

func TestConnectOffersTheChannelsWithMaskedTargets(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusOK, sessionBody(t)))

		rec := httptest.NewRecorder()
		handler.ConnectSmilesAccount(rec, smilesRequest(http.MethodPost, account.ID, user.ID, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var response struct {
			Data struct {
				Connected      bool `json:"connected"`
				MFARequired    bool `json:"mfa_required"`
				Authenticators []struct {
					ID      string `json:"id"`
					Channel string `json:"channel"`
					Target  string `json:"target"`
				} `json:"authenticators"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decoding response: %v", err)
		}
		if !response.Data.MFARequired || response.Data.Connected {
			t.Fatalf("response = %+v", response.Data)
		}
		if len(response.Data.Authenticators) != 1 {
			t.Fatalf("got %d channels, want 1", len(response.Data.Authenticators))
		}
		// The holder's phone number must never reach the panel.
		if got := response.Data.Authenticators[0].Target; got != "***77" {
			t.Errorf("target = %q, want it masked", got)
		}
	})
}

func TestAChannelThatWasNeverOfferedIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusOK, sessionBody(t)))

		handler.ConnectSmilesAccount(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID, ""))

		// Taking the id on trust would let a caller aim a real SMS at an authenticator the
		// tenant never listed for this account.
		rec := httptest.NewRecorder()
		handler.ChallengeSmilesLogin(rec, smilesRequest(http.MethodPost, account.ID, user.ID,
			`{"authenticator_id":"sms|outro"}`))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestAChallengeWithoutALoginInFlightIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusOK, sessionBody(t)))

		rec := httptest.NewRecorder()
		handler.ChallengeSmilesLogin(rec, smilesRequest(http.MethodPost, account.ID, user.ID,
			`{"authenticator_id":"sms|1"}`))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestAWrongCodeIsRefusedWithoutStoringASession(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusForbidden,
			`{"error":"invalid_grant","error_description":"Invalid binding_code."}`))

		handler.ConnectSmilesAccount(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID, ""))
		handler.ChallengeSmilesLogin(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID,
			`{"authenticator_id":"sms|1"}`))

		rec := httptest.NewRecorder()
		handler.ConfirmSmilesLogin(rec, smilesRequest(http.MethodPost, account.ID, user.ID, `{"code":"000000"}`))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}

		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading account: %v", err)
		}
		if reloaded.SmilesRefreshToken != nil {
			t.Fatal("a refused code left a session behind")
		}

		// The login must still be in flight: the operator retypes the digits, and nothing
		// asks Smiles for another code - it counts attempts and throttles per channel.
		if _, ok := handler.Tracker.Pending(account.ID); !ok {
			t.Error("a mistyped code threw the login away; the operator would have to start over")
		}
	})
}

func TestAnExpiredChallengeAsksTheOperatorToStartOver(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusForbidden,
			`{"error":"invalid_grant","error_description":"MFA Authorization rejected: expired"}`))

		handler.ConnectSmilesAccount(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID, ""))
		handler.ChallengeSmilesLogin(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID,
			`{"authenticator_id":"sms|1"}`))

		rec := httptest.NewRecorder()
		handler.ConfirmSmilesLogin(rec, smilesRequest(http.MethodPost, account.ID, user.ID, `{"code":"123456"}`))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
		// Unlike a wrong code, an expired one cannot be retyped: the spent token is dropped.
		if _, ok := handler.Tracker.Pending(account.ID); ok {
			t.Error("an expired login is still in flight")
		}
	})
}

func TestAnAccountWithoutCredentialsCannotConnect(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		if err := tx.Model(&data.Account{}).Where("id = ?", account.ID).
			Updates(map[string]any{"login": nil, "password": nil}).Error; err != nil {
			t.Fatalf("clearing the credentials: %v", err)
		}
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusOK, sessionBody(t)))

		rec := httptest.NewRecorder()
		handler.ConnectSmilesAccount(rec, smilesRequest(http.MethodPost, account.ID, user.ID, ""))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestAnAccountThatDoesNotTrackItsStatementIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		if err := tx.Model(&data.Account{}).Where("id = ?", account.ID).
			Update("tracks_statement", false).Error; err != nil {
			t.Fatalf("updating the account: %v", err)
		}
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusOK, sessionBody(t)))

		rec := httptest.NewRecorder()
		handler.ConnectSmilesAccount(rec, smilesRequest(http.MethodPost, account.ID, user.ID, ""))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// Connecting asks whether the account is tracked, NOT whether its points are ours to price.
// Before migration 64 one flag answered both, and an account administered for someone else -
// whose redemptions are our own issuances - could not have its statement read at all.
func TestAnAccountWithoutItsOwnInventoryCanStillConnect(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		if err := tx.Model(&data.Account{}).Where("id = ?", account.ID).
			Update("owns_point_inventory", false).Error; err != nil {
			t.Fatalf("updating the account: %v", err)
		}
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusOK, sessionBody(t)))

		rec := httptest.NewRecorder()
		handler.ConnectSmilesAccount(rec, smilesRequest(http.MethodPost, account.ID, user.ID, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestTheRightCodeStoresAnEncryptedSessionAndConnectsTheAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusOK, sessionBody(t)))

		handler.ConnectSmilesAccount(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID, ""))
		handler.ChallengeSmilesLogin(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID,
			`{"authenticator_id":"sms|1"}`))

		rec := httptest.NewRecorder()
		handler.ConfirmSmilesLogin(rec, smilesRequest(http.MethodPost, account.ID, user.ID, `{"code":"123456"}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		// The session must be on disk as ciphertext: it is a live credential, and anyone
		// holding it can read the account.
		var stored string
		if err := tx.Raw("SELECT smiles_refresh_token FROM accounts WHERE id = ?", account.ID).
			Scan(&stored).Error; err != nil {
			t.Fatalf("reading the raw column: %v", err)
		}
		if stored == "" {
			t.Fatal("no session was stored")
		}
		if stored == "refresh-1" {
			t.Fatal("the session was stored in clear")
		}

		state, err := smilessync.LoadState(tx, account.ID)
		if err != nil {
			t.Fatalf("loading the sync state: %v", err)
		}
		if state.State != data.SmilesSyncPending {
			t.Errorf("state = %q, want pending until the first round lands", state.State)
		}

		// The spent token must not linger in memory once it has been exchanged.
		if _, ok := handler.Tracker.Pending(account.ID); ok {
			t.Error("the finished login is still in flight")
		}
	})
}

func TestDisconnectClearsTheSessionAndTheWindow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedSmilesAccount(t, tx)
		handler := newSmilesHandler(tx, mfaRequiredTenant(t, http.StatusOK, sessionBody(t)))

		handler.ConnectSmilesAccount(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID, ""))
		handler.ChallengeSmilesLogin(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID,
			`{"authenticator_id":"sms|1"}`))
		handler.ConfirmSmilesLogin(httptest.NewRecorder(), smilesRequest(http.MethodPost, account.ID, user.ID,
			`{"code":"123456"}`))

		rec := httptest.NewRecorder()
		handler.DisconnectSmilesAccount(rec, smilesRequest(http.MethodDelete, account.ID, user.ID, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading account: %v", err)
		}
		if reloaded.SmilesRefreshToken != nil {
			t.Fatal("the session survived a disconnect")
		}
	})
}
