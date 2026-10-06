package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/latam"
	"github.com/murandido/m-80-backend/internal/latamsync"
	"gorm.io/gorm"
)

// latamStub stands in for both LATAM hosts: the Auth0 token endpoint and the balance BFF.
func latamStub(t *testing.T, tokenStatus int) *latam.AccountClient {
	t.Helper()
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"https://xp.latamairlines.com_userId":"user-42"}`))
	idToken := "header." + claims + ".signature"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oauth/token":
			if tokenStatus != http.StatusOK {
				w.WriteHeader(tokenStatus)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "access", "id_token": idToken, "refresh_token": "refresh-1",
			})
		case strings.HasSuffix(r.URL.Path, "/user-42/loyalty"):
			_, _ = w.Write([]byte(`{"loyalty":{"miles":84321,"totalPoints":10}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return latam.NewAccountClient(latam.WithAccountsBaseURL(server.URL), latam.WithAccountBaseURL(server.URL),
		latam.WithAccountHTTPClient(server.Client()))
}

func seedLatamAccount(t *testing.T, tx *gorm.DB) (data.Account, data.User) {
	t.Helper()
	suffix := uuid.New().String()[:8]
	airline := data.Airline{Name: "Latam " + suffix, Code: "L" + suffix}
	create(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "LATAM Pass " + suffix, Code: "L" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)
	user := createUser(t, tx, "Operador "+suffix)
	// Neither flag: the balance is for every account.
	account := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: program.ID}
	create(t, tx, &account)
	return account, user
}

func latamRequest(method, path, accountID, userID, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	if accountID != "" {
		rctx.URLParams.Add("id", accountID)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{ID: userID, Role: data.RoleAdmin})
	return req.WithContext(ctx)
}

// startLogin runs the first step and returns the state the login page was given.
func startLogin(t *testing.T, handler *LatamConnectHandler, accountID, userID string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.StartLatamLogin(rec, latamRequest(http.MethodPost, "/accounts/"+accountID+"/latam-connect", accountID, userID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("start: status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data schemas.LatamConnectResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding the start response: %v", err)
	}
	page, err := url.Parse(envelope.Data.AuthorizeURL)
	if err != nil {
		t.Fatalf("parsing the login page: %v", err)
	}
	return page.Query().Get("state")
}

func callbackBody(state string) string {
	callback := latam.AppRedirectURI + "?code=abc123&state=" + url.QueryEscape(state)
	raw, _ := json.Marshal(schemas.LatamCallbackRequest{CallbackURL: callback})
	return string(raw)
}

func TestLatamConnectLinksTheAccountAndReadsTheBalance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedLatamAccount(t, tx)
		handler := NewLatamConnectHandler(tx, latamStub(t, http.StatusOK), latamsync.NewLoginTracker(), latamsync.NewSessionGuard())

		state := startLogin(t, handler, account.ID, user.ID)

		rec := httptest.NewRecorder()
		handler.FinishLatamLogin(rec, latamRequest(http.MethodPost, "/latam-connect/callback", "", user.ID, callbackBody(state)))
		if rec.Code != http.StatusOK {
			t.Fatalf("callback: status = %d (body: %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), account.ID) {
			t.Errorf("the callback did not name the account: %s", rec.Body.String())
		}

		var stored string
		if err := tx.Raw("SELECT latam_refresh_token FROM accounts WHERE id = ?", account.ID).Scan(&stored).Error; err != nil {
			t.Fatalf("reading the raw column: %v", err)
		}
		if stored == "" || stored == "refresh-1" {
			t.Errorf("stored session = %q, want it encrypted", stored)
		}
		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatal(err)
		}
		if reloaded.RealBalancePoints == nil || *reloaded.RealBalancePoints != 84321 {
			t.Errorf("real balance = %v, want 84321", reloaded.RealBalancePoints)
		}

		// The same answer is good once.
		again := httptest.NewRecorder()
		handler.FinishLatamLogin(again, latamRequest(http.MethodPost, "/latam-connect/callback", "", user.ID, callbackBody(state)))
		if again.Code != http.StatusConflict {
			t.Errorf("replayed callback: status = %d, want 409", again.Code)
		}

		// And the badge reports it.
		status := httptest.NewRecorder()
		handler.LatamConnectStatus(status, latamRequest(http.MethodGet, "/accounts/"+account.ID+"/latam-status", account.ID, user.ID, ""))
		if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"connected":true`) ||
			!strings.Contains(status.Body.String(), `"last_balance_points":84321`) {
			t.Errorf("status = %d %s", status.Code, status.Body.String())
		}
	})
}

func TestLatamCallbackRefusesWhatItCannotUse(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedLatamAccount(t, tx)
		handler := NewLatamConnectHandler(tx, latamStub(t, http.StatusForbidden), latamsync.NewLoginTracker(), latamsync.NewSessionGuard())

		cases := []struct {
			name string
			body string
			want int
		}{
			{"no body", ``, http.StatusBadRequest},
			{"not an answer", `{"callback_url":"olá"}`, http.StatusUnprocessableEntity},
			{"a login nobody started", callbackBody("never-issued"), http.StatusConflict},
		}
		for _, tc := range cases {
			rec := httptest.NewRecorder()
			handler.FinishLatamLogin(rec, latamRequest(http.MethodPost, "/latam-connect/callback", "", user.ID, tc.body))
			if rec.Code != tc.want {
				t.Errorf("%s: status = %d, want %d (body: %s)", tc.name, rec.Code, tc.want, rec.Body.String())
			}
		}

		// A real login whose code the tenant refuses (too old, already used).
		state := startLogin(t, handler, account.ID, user.ID)
		rec := httptest.NewRecorder()
		handler.FinishLatamLogin(rec, latamRequest(http.MethodPost, "/latam-connect/callback", "", user.ID, callbackBody(state)))
		if rec.Code != http.StatusConflict {
			t.Errorf("refused code: status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestLatamDisconnectForgetsTheSession(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedLatamAccount(t, tx)
		handler := NewLatamConnectHandler(tx, latamStub(t, http.StatusOK), latamsync.NewLoginTracker(), latamsync.NewSessionGuard())
		state := startLogin(t, handler, account.ID, user.ID)
		handler.FinishLatamLogin(httptest.NewRecorder(), latamRequest(http.MethodPost, "/latam-connect/callback", "", user.ID, callbackBody(state)))

		rec := httptest.NewRecorder()
		handler.DisconnectLatamAccount(rec, latamRequest(http.MethodDelete, "/accounts/"+account.ID+"/latam-connect", account.ID, user.ID, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("disconnect: status = %d (body: %s)", rec.Code, rec.Body.String())
		}
		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatal(err)
		}
		if reloaded.LatamRefreshToken != nil {
			t.Error("the session survived a disconnect")
		}
	})
}
