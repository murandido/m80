package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/bot"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// fakeSession is a minimal bot.SessionClient double for exercising the HTTP
// handlers without a real WhatsApp connection.
type fakeSession struct {
	connected bool
	loggedIn  bool
	pairCode  string
	qrChan    chan whatsmeow.QRChannelItem
}

func (f *fakeSession) Connect() error    { f.connected = true; return nil }
func (f *fakeSession) Disconnect()       { f.connected = false }
func (f *fakeSession) IsConnected() bool { return f.connected }
func (f *fakeSession) IsLoggedIn() bool  { return f.loggedIn }
func (f *fakeSession) Logout(_ context.Context) error {
	f.loggedIn = false
	f.connected = false
	return nil
}

func (f *fakeSession) PairPhone(_ context.Context, _ string, _ bool, _ whatsmeow.PairClientType, _ string) (string, error) {
	return f.pairCode, nil
}

func (f *fakeSession) GetQRChannel(_ context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return f.qrChan, nil
}

func (f *fakeSession) DeleteStore(_ context.Context) error { return nil }

func newTestBot() *bot.Bot {
	session := &fakeSession{
		pairCode: "111-222",
		qrChan:   make(chan whatsmeow.QRChannelItem, 1),
	}
	session.qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode}

	return bot.NewTestBot(session, func() *types.JID { return nil }, waLog.Noop, bot.NewStatusStore(func() {}), nil, func() {})
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (body: %s)", err, rec.Body.String())
	}
	return body
}

func TestGetStatus(t *testing.T) {
	r := newInternalRouter(newTestBot())

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, _ := body["data"].(map[string]any)
	if data["state"] != string(bot.StateDisconnected) {
		t.Fatalf("state: got %+v", data)
	}
}

func TestPostPair_DefaultsToPhone(t *testing.T) {
	r := newInternalRouter(newTestBot())

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"phone":"11999999999"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, _ := body["data"].(map[string]any)
	if data["pair_method"] != string(bot.PairMethodPhone) || data["pair_code"] != "111-222" {
		t.Fatalf("data: got %+v", data)
	}
}

func TestPostPair_InvalidPhone_Returns400(t *testing.T) {
	r := newInternalRouter(newTestBot())

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"phone":"123"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPostPair_QR_DoesNotRequirePhone(t *testing.T) {
	r := newInternalRouter(newTestBot())

	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"method":"qr"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestPostPairCancel_Noop(t *testing.T) {
	r := newInternalRouter(newTestBot())

	req := httptest.NewRequest(http.MethodPost, "/pair/cancel", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, _ := body["data"].(map[string]any)
	if data["state"] != string(bot.StateDisconnected) {
		t.Fatalf("data: got %+v", data)
	}
}

func TestPostLogout(t *testing.T) {
	r := newInternalRouter(newTestBot())

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}
