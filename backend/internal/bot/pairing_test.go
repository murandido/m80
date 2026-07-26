package bot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// fakeSession is a minimal SessionClient double so pairing logic can be
// tested without a real WhatsApp connection.
type fakeSession struct {
	connected bool
	loggedIn  bool

	connectErr   error
	pairPhoneErr error
	pairCode     string
	qrChan       chan whatsmeow.QRChannelItem
	qrErr        error
	logoutErr    error

	connectCalls     int
	disconnectCalls  int
	logoutCalls      int
	deleteStoreCalls int
	pairPhoneArg     string
	qrChannelCtx     context.Context

	// onDeleteStore, if set, is called synchronously from DeleteStore - lets
	// a test observe exactly when the deletion happened relative to other
	// calls (e.g. RecreateSession's factory).
	onDeleteStore func()
}

func (f *fakeSession) Connect() error {
	f.connectCalls++
	if f.connectErr != nil {
		return f.connectErr
	}
	f.connected = true
	return nil
}

func (f *fakeSession) Disconnect() {
	f.disconnectCalls++
	f.connected = false
}

func (f *fakeSession) IsConnected() bool { return f.connected }
func (f *fakeSession) IsLoggedIn() bool  { return f.loggedIn }

func (f *fakeSession) PairPhone(_ context.Context, phone string, _ bool, _ whatsmeow.PairClientType, _ string) (string, error) {
	f.pairPhoneArg = phone
	if f.pairPhoneErr != nil {
		return "", f.pairPhoneErr
	}
	return f.pairCode, nil
}

func (f *fakeSession) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	f.qrChannelCtx = ctx
	if f.qrErr != nil {
		return nil, f.qrErr
	}
	return f.qrChan, nil
}

func (f *fakeSession) Logout(_ context.Context) error {
	f.logoutCalls++
	if f.logoutErr != nil {
		return f.logoutErr
	}
	f.loggedIn = false
	f.connected = false
	return nil
}

func (f *fakeSession) DeleteStore(_ context.Context) error {
	f.deleteStoreCalls++
	if f.onDeleteStore != nil {
		f.onDeleteStore()
	}
	return nil
}

// newTestBot builds a Bot around the given fake session. notify defaults to a
// no-op; pass one that signals a channel when a test needs to synchronize
// with an async status update (e.g. the QR consumer goroutine).
func newTestBot(session *fakeSession, notify func()) *Bot {
	if notify == nil {
		notify = func() {}
	}
	return &Bot{
		Logger: waLog.Noop,
		Status: NewStatusStore(notify),
		session: deviceSession{
			Session: session,
			GetJID:  func() *types.JID { return nil },
		},
	}
}

func TestStartPairing_Phone_Success(t *testing.T) {
	session := &fakeSession{
		pairCode: "123-456",
		qrChan:   make(chan whatsmeow.QRChannelItem, 1),
	}
	session.qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "discarded"}
	b := newTestBot(session, nil)

	status, err := b.StartPairing(PairMethodPhone, "11999999999")
	if err != nil {
		t.Fatalf("StartPairing: %v", err)
	}
	if status.State != StatePairing || status.PairMethod != PairMethodPhone || status.PairCode != "123-456" {
		t.Fatalf("status: got %+v", status)
	}
	if session.connectCalls != 1 {
		t.Fatalf("connect calls: got %d, want 1", session.connectCalls)
	}
}

func TestStartPairing_Phone_PrefixesCountryCode(t *testing.T) {
	session := &fakeSession{
		pairCode: "123-456",
		qrChan:   make(chan whatsmeow.QRChannelItem, 1),
	}
	session.qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode}
	b := newTestBot(session, nil)

	if _, err := b.StartPairing(PairMethodPhone, "(11) 99999-9999"); err != nil {
		t.Fatalf("StartPairing: %v", err)
	}
	if session.pairPhoneArg != "5511999999999" {
		t.Fatalf("phone passed to whatsmeow: got %q, want %q", session.pairPhoneArg, "5511999999999")
	}
}

func TestStartPairing_Phone_PairPhoneError(t *testing.T) {
	session := &fakeSession{
		pairPhoneErr: errors.New("boom"),
		qrChan:       make(chan whatsmeow.QRChannelItem, 1),
	}
	session.qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode}
	b := newTestBot(session, nil)

	_, err := b.StartPairing(PairMethodPhone, "11999999999")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := b.Status.Get().State; got != StateDisconnected {
		t.Fatalf("status after failed pairing: got %q, want %q", got, StateDisconnected)
	}
}

// Regression test for the bug where GetQRChannel was given the HTTP
// request's context - whatsmeow disconnects the client the moment that
// context is cancelled, which happened almost immediately after the request
// handler returned, well before the phone had a chance to complete pairing.
// The context StartPairing hands to GetQRChannel must stay alive on its own
// until CancelPairing (or a new StartPairing) explicitly ends it.
func TestStartPairing_QRChannelContextOutlivesTheCall(t *testing.T) {
	session := &fakeSession{
		pairCode: "123-456",
		qrChan:   make(chan whatsmeow.QRChannelItem, 1),
	}
	session.qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode}
	b := newTestBot(session, nil)

	if _, err := b.StartPairing(PairMethodPhone, "11999999999"); err != nil {
		t.Fatalf("StartPairing: %v", err)
	}

	if err := session.qrChannelCtx.Err(); err != nil {
		t.Fatalf("context passed to GetQRChannel was already done right after StartPairing returned: %v", err)
	}

	b.CancelPairing(context.Background())

	if err := session.qrChannelCtx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("context passed to GetQRChannel: got %v, want context.Canceled after CancelPairing", err)
	}
}

func TestStartPairing_AlreadyConnected(t *testing.T) {
	session := &fakeSession{connected: true, loggedIn: true}
	b := newTestBot(session, nil)

	_, err := b.StartPairing(PairMethodPhone, "11999999999")
	if !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("err: got %v, want ErrAlreadyConnected", err)
	}
}

func TestStartPairing_QR_StreamsCodes(t *testing.T) {
	session := &fakeSession{qrChan: make(chan whatsmeow.QRChannelItem)}
	notifyCh := make(chan struct{}, 4)
	b := newTestBot(session, func() { notifyCh <- struct{}{} })

	status, err := b.StartPairing(PairMethodQR, "")
	if err != nil {
		t.Fatalf("StartPairing: %v", err)
	}
	if status.State != StatePairing || status.PairMethod != PairMethodQR || status.PairCode != "" {
		t.Fatalf("initial status: got %+v", status)
	}
	// StartPairing's own Set (the initial "pairing, no code yet" status) fires
	// a notify synchronously before the QR consumer goroutine has read
	// anything - drain it before waiting for code-specific notifications.
	waitForNotify(t, notifyCh)

	session.qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "first-code"}
	waitForNotify(t, notifyCh)

	if got := b.Status.Get(); got.PairCode != "first-code" {
		t.Fatalf("status after first code: got %+v", got)
	}

	session.qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "second-code"}
	waitForNotify(t, notifyCh)

	if got := b.Status.Get(); got.PairCode != "second-code" {
		t.Fatalf("status after refreshed code: got %+v", got)
	}
}

func TestCancelPairing_StopsQRLoopAndDisconnects(t *testing.T) {
	session := &fakeSession{qrChan: make(chan whatsmeow.QRChannelItem)}
	notifyCh := make(chan struct{}, 4)
	b := newTestBot(session, func() { notifyCh <- struct{}{} })

	if _, err := b.StartPairing(PairMethodQR, ""); err != nil {
		t.Fatalf("StartPairing: %v", err)
	}

	status := b.CancelPairing(context.Background())
	if status.State != StateDisconnected {
		t.Fatalf("status after cancel: got %+v", status)
	}
	if session.disconnectCalls != 1 {
		t.Fatalf("disconnect calls: got %d, want 1", session.disconnectCalls)
	}
}

func TestCancelPairing_NoopWhenNotPairing(t *testing.T) {
	session := &fakeSession{}
	b := newTestBot(session, nil)

	status := b.CancelPairing(context.Background())
	if status.State != StateDisconnected {
		t.Fatalf("status: got %+v", status)
	}
	if session.disconnectCalls != 0 {
		t.Fatalf("disconnect calls: got %d, want 0", session.disconnectCalls)
	}
}

func TestLogout_Success(t *testing.T) {
	session := &fakeSession{connected: true, loggedIn: true}
	b := newTestBot(session, nil)

	status, err := b.Logout(context.Background())
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if status.State != StateDisconnected {
		t.Fatalf("status: got %+v", status)
	}
	if session.logoutCalls != 1 {
		t.Fatalf("logout calls: got %d, want 1", session.logoutCalls)
	}
}

func TestLogout_Error(t *testing.T) {
	session := &fakeSession{logoutErr: errors.New("boom")}
	b := newTestBot(session, nil)

	if _, err := b.Logout(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

// whatsmeow permanently disables a client the moment its device is deleted,
// which it does automatically after every logout (see RecreateSession's
// docstring). These two tests are the regression coverage for "invalid use
// of deleted device": after RecreateSession runs, further pairing attempts
// must go through the fresh session, never the old (now-broken) one.

func TestRecreateSession_SwapsInFreshSession(t *testing.T) {
	oldSession := &fakeSession{}
	newSession := &fakeSession{
		pairCode: "999-999",
		qrChan:   make(chan whatsmeow.QRChannelItem, 1),
	}
	newSession.qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode}

	b := &Bot{
		Logger:  waLog.Noop,
		Status:  NewStatusStore(func() {}),
		session: deviceSession{Session: oldSession, GetJID: func() *types.JID { return nil }},
		newDeviceClient: func(_ context.Context) (ClientBase, SessionClient, func() *types.JID, error) {
			return nil, newSession, func() *types.JID { return nil }, nil
		},
	}

	if err := b.RecreateSession(context.Background()); err != nil {
		t.Fatalf("RecreateSession: %v", err)
	}

	if _, err := b.StartPairing(PairMethodPhone, "11999999999"); err != nil {
		t.Fatalf("StartPairing after RecreateSession: %v", err)
	}
	if newSession.connectCalls != 1 {
		t.Fatalf("new session connect calls: got %d, want 1", newSession.connectCalls)
	}
	if oldSession.connectCalls != 0 {
		t.Fatalf("old (deleted) session connect calls: got %d, want 0", oldSession.connectCalls)
	}
}

func TestLogout_RecreatesSessionForNextPairing(t *testing.T) {
	oldSession := &fakeSession{connected: true, loggedIn: true}
	newSession := &fakeSession{
		pairCode: "999-999",
		qrChan:   make(chan whatsmeow.QRChannelItem, 1),
	}
	newSession.qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode}

	b := &Bot{
		Logger:  waLog.Noop,
		Status:  NewStatusStore(func() {}),
		session: deviceSession{Session: oldSession, GetJID: func() *types.JID { return nil }},
		newDeviceClient: func(_ context.Context) (ClientBase, SessionClient, func() *types.JID, error) {
			return nil, newSession, func() *types.JID { return nil }, nil
		},
	}

	if _, err := b.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if oldSession.logoutCalls != 1 {
		t.Fatalf("old session logout calls: got %d, want 1", oldSession.logoutCalls)
	}

	if _, err := b.StartPairing(PairMethodPhone, "11999999999"); err != nil {
		t.Fatalf("StartPairing after Logout: %v", err)
	}
	if newSession.connectCalls != 1 {
		t.Fatalf("new session connect calls: got %d, want 1", newSession.connectCalls)
	}
	if oldSession.connectCalls != 0 {
		t.Fatalf("old (deleted) session connect calls: got %d, want 0", oldSession.connectCalls)
	}
}

// Regression test for a race between whatsmeow's own (unsynchronized, own
// goroutine) device deletion after a remote logout and RecreateSession
// asking for a new device too early - which handed back a device that still
// had the old JID, and GetQRChannel then refused with "no user ID in the
// client's Store". EventHandler must delete the store itself, synchronously,
// before ever asking for a new device.
func TestEventHandler_LoggedOut_DeletesStoreBeforeRecreating(t *testing.T) {
	var mu sync.Mutex
	var order []string

	oldSession := &fakeSession{
		onDeleteStore: func() {
			mu.Lock()
			order = append(order, "delete")
			mu.Unlock()
		},
	}
	newSession := &fakeSession{}
	done := make(chan struct{})

	b := &Bot{
		Logger:  waLog.Noop,
		Status:  NewStatusStore(func() {}),
		session: deviceSession{Session: oldSession, GetJID: func() *types.JID { return nil }},
		newDeviceClient: func(_ context.Context) (ClientBase, SessionClient, func() *types.JID, error) {
			mu.Lock()
			order = append(order, "recreate")
			mu.Unlock()
			close(done)
			return nil, newSession, func() *types.JID { return nil }, nil
		},
	}

	b.EventHandler(&events.LoggedOut{Reason: events.ConnectFailureLoggedOut})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RecreateSession's factory to be called")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "delete" || order[1] != "recreate" {
		t.Fatalf("call order: got %v, want [delete recreate]", order)
	}
	if oldSession.deleteStoreCalls != 1 {
		t.Fatalf("delete store calls: got %d, want 1", oldSession.deleteStoreCalls)
	}
}

func waitForNotify(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for status notification")
	}
}
