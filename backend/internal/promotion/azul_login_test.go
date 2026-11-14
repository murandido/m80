package promotion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func azulTestClient(url string) *Client {
	return &Client{azulLoginURL: url, http: http.DefaultClient}
}

func TestAzulLogin_ReturnsCustomerKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			t.Errorf("path = %q, want /login", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"customer_key":"abc123hash"}`))
	}))
	defer srv.Close()

	key, err := azulTestClient(srv.URL).AzulLogin(context.Background(),
		AzulLoginRequest{DocumentNumber: "00011122233", Password: "secret"})
	if err != nil {
		t.Fatalf("AzulLogin: %v", err)
	}
	if key != "abc123hash" {
		t.Errorf("key = %q, want abc123hash", key)
	}
}

func TestAzulLogin_FailedLoginIsTyped(t *testing.T) {
	// A 502 carrying an error message is a login that failed, not a transport fault: the caller
	// must be able to tell it apart to show the operator why.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"login did not surface a customerKey"}`))
	}))
	defer srv.Close()

	_, err := azulTestClient(srv.URL).AzulLogin(context.Background(),
		AzulLoginRequest{DocumentNumber: "x", Password: "y"})
	if !errors.Is(err, ErrAzulLoginFailed) {
		t.Fatalf("err = %v, want ErrAzulLoginFailed", err)
	}
}

func TestAzulLogin_EmptyKeyIsFailure(t *testing.T) {
	// 200 but no key is still a failure - the SPA never surfaced a customerKey.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"customer_key":""}`))
	}))
	defer srv.Close()

	_, err := azulTestClient(srv.URL).AzulLogin(context.Background(),
		AzulLoginRequest{DocumentNumber: "x", Password: "y"})
	if !errors.Is(err, ErrAzulLoginFailed) {
		t.Fatalf("err = %v, want ErrAzulLoginFailed", err)
	}
}

func TestAzulLogin_ServerErrorIsTransport(t *testing.T) {
	// A 500/409 is the login service failing, not a credential outcome: the
	// connector must not count it toward needs_attention.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"login transport failed; try again shortly"}`))
	}))
	defer srv.Close()

	_, err := azulTestClient(srv.URL).AzulLogin(context.Background(),
		AzulLoginRequest{DocumentNumber: "x", Password: "y"})
	if err == nil {
		t.Fatal("expected an error for a server failure")
	}
	if errors.Is(err, ErrAzulLoginFailed) {
		t.Error("a service failure must not read as a login failure")
	}
}

func TestAzulLogin_EdgeBlockIsNotALoginFailure(t *testing.T) {
	// A 503 is the Azul edge refusing the shared device or the rate. Counting it
	// would push every account that tries to connect into needs_attention.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"edge refused the login (HTTP 403)"}`))
	}))
	defer srv.Close()

	_, err := azulTestClient(srv.URL).AzulLogin(context.Background(),
		AzulLoginRequest{DocumentNumber: "x", Password: "y"})
	if err == nil {
		t.Fatal("expected an error for an edge block")
	}
	if errors.Is(err, ErrAzulLoginFailed) {
		t.Error("an edge block must not read as a login failure")
	}
}

func TestAzulLogin_UnreachableIsTransportError(t *testing.T) {
	// A dead service is not a login failure: it must NOT read as ErrAzulLoginFailed, or the
	// panel would tell the operator their password is wrong when the service is simply down.
	_, err := azulTestClient("http://127.0.0.1:0").AzulLogin(context.Background(),
		AzulLoginRequest{DocumentNumber: "x", Password: "y"})
	if err == nil {
		t.Fatal("expected an error for an unreachable service")
	}
	if errors.Is(err, ErrAzulLoginFailed) {
		t.Error("an unreachable service must not read as a login failure")
	}
}

func TestAzulLogin_CarriesTheServiceReason(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantReason  string
		wantAccount bool
	}{
		{"wrong password", http.StatusBadGateway, `{"error":"credentials refused (HTTP 401)","reason":"credentials"}`, AzulLoginReasonCredentials, true},
		{"locked", http.StatusBadGateway, `{"error":"account locked (HTTP 423)","reason":"locked"}`, AzulLoginReasonLocked, true},
		{"502 without reason", http.StatusBadGateway, `{"error":"login failed"}`, AzulLoginReasonRejected, true},
		{"502 with an infra reason", http.StatusBadGateway, `{"error":"x","reason":"blocked"}`, AzulLoginReasonRejected, true},
		{"edge block", http.StatusServiceUnavailable, `{"error":"edge refused","reason":"blocked"}`, AzulLoginReasonBlocked, false},
		{"disabled", http.StatusServiceUnavailable, `{"error":"disabled","reason":"disabled"}`, AzulLoginReasonDisabled, false},
		// The status bounds the class: an account reason on a 503 must never mark the account.
		{"503 with an account reason", http.StatusServiceUnavailable, `{"error":"x","reason":"credentials"}`, AzulLoginReasonBlocked, false},
		{"queue full", http.StatusConflict, `{"error":"queued","reason":"busy"}`, AzulLoginReasonBusy, false},
		{"service failure", http.StatusInternalServerError, `{"error":"transport","reason":"unavailable"}`, AzulLoginReasonUnavailable, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, err := azulTestClient(srv.URL).AzulLogin(context.Background(),
				AzulLoginRequest{DocumentNumber: "x", Password: "y"})
			var loginErr *AzulLoginError
			if !errors.As(err, &loginErr) {
				t.Fatalf("err = %v, want an *AzulLoginError", err)
			}
			if loginErr.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", loginErr.Reason, tc.wantReason)
			}
			if got := errors.Is(err, ErrAzulLoginFailed); got != tc.wantAccount {
				t.Errorf("errors.Is(ErrAzulLoginFailed) = %v, want %v", got, tc.wantAccount)
			}
		})
	}
}
