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
