package bot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/utils"
	"go.mau.fi/whatsmeow"
)

const (
	pairClientDisplayName = "Chrome (Linux)"
	brazilCountryCode     = "55"
)

var ErrAlreadyConnected = errors.New("bot: already connected, logout first")

// StartPairing begins a new pairing attempt. phone is the DDD+número the
// admin typed in (only used for PairMethodPhone; ignored for QR). Phone
// pairing completes synchronously and returns the code to show the admin. QR
// pairing returns immediately with an empty PairCode - the first QR frame
// arrives shortly after via the status store (and the SSE notification it
// triggers), since it streams multiple codes over time as they rotate.
//
// Deliberately does not take a context from the caller: whatsmeow's
// GetQRChannel ties its internal lifecycle to whatever context it's given,
// including disconnecting the client the moment that context is cancelled.
// Since the actual pairing completes asynchronously - the phone confirms
// over this same connection, well after the HTTP request that triggered
// pairing has already returned - accepting the request's context here would
// tear the connection down before the phone gets a chance to link. The
// context this creates lives until CancelPairing, a new StartPairing, or a
// terminal pairing event.
func (b *Bot) StartPairing(method PairMethod, phone string) (Status, error) {
	session := b.currentSession().Session
	if session.IsConnected() && session.IsLoggedIn() {
		return Status{}, ErrAlreadyConnected
	}

	b.cancelActivePairing()

	pairCtx, cancel := context.WithCancel(context.Background())
	b.pairMu.Lock()
	b.cancelPair = cancel
	b.pairMu.Unlock()

	if method == PairMethodQR {
		return b.startQRPairing(pairCtx)
	}
	return b.startPhonePairing(pairCtx, phone)
}

// startPhonePairing mirrors the original boot-time flow: prime the handshake
// by opening (and discarding the first item of) the QR channel, then request
// a numeric linking code for the given phone (DDD+número, prefixed with the
// country code whatsmeow expects).
func (b *Bot) startPhonePairing(ctx context.Context, phone string) (Status, error) {
	session := b.currentSession().Session

	qrChan, err := session.GetQRChannel(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("bot: get qr channel: %w", err)
	}

	if err := session.Connect(); err != nil {
		return Status{}, fmt.Errorf("bot: connect: %w", err)
	}

	<-qrChan

	fullPhone := brazilCountryCode + utils.ClearPhone(phone)
	code, err := session.PairPhone(ctx, fullPhone, true, whatsmeow.PairClientChrome, pairClientDisplayName)
	if err != nil {
		b.Status.Set(Status{State: StateDisconnected})
		return Status{}, fmt.Errorf("bot: pair phone: %w", err)
	}

	status := Status{State: StatePairing, PairMethod: PairMethodPhone, PairCode: code}
	b.Status.Set(status)
	return status, nil
}

// startQRPairing takes the long-lived context StartPairing already set up in
// b.cancelPair - not a request context, see StartPairing's docstring.
func (b *Bot) startQRPairing(ctx context.Context) (Status, error) {
	session := b.currentSession().Session

	qrChan, err := session.GetQRChannel(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("bot: get qr channel: %w", err)
	}

	if err := session.Connect(); err != nil {
		return Status{}, fmt.Errorf("bot: connect: %w", err)
	}

	go b.consumeQRChannel(ctx, qrChan)

	status := Status{State: StatePairing, PairMethod: PairMethodQR}
	b.Status.Set(status)
	return status, nil
}

// consumeQRChannel pushes every QR refresh into the status store until the
// channel closes (success, timeout, or an outer cancellation from
// CancelPairing). Terminal non-code events beyond a plain error just stop the
// loop - Connected/PairSuccess (handled in EventHandler) own the transition
// to StateConnected.
func (b *Bot) consumeQRChannel(ctx context.Context, qrChan <-chan whatsmeow.QRChannelItem) {
	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-qrChan:
			if !ok {
				return
			}

			switch item.Event {
			case whatsmeow.QRChannelEventCode:
				b.Status.Set(Status{State: StatePairing, PairMethod: PairMethodQR, PairCode: item.Code})
			case whatsmeow.QRChannelEventError:
				b.Logger.Errorf("qr pairing error: %v", item.Error)
				b.Status.Set(Status{State: StateDisconnected})
				return
			default:
				// "success" is followed by PairSuccess/Connected events, which
				// own the status transition from here. Anything else (e.g.
				// "timeout") just falls back to disconnected.
				if item.Event != "success" {
					b.Status.Set(Status{State: StateDisconnected})
				}
				return
			}
		}
	}
}

// CancelPairing stops an in-progress QR pairing loop (if any) and disconnects
// the socket. It is a no-op if the bot is already connected or idle.
func (b *Bot) CancelPairing(_ context.Context) Status {
	current := b.Status.Get()
	if current.State != StatePairing {
		return current
	}

	b.cancelActivePairing()
	b.currentSession().Session.Disconnect()

	status := Status{State: StateDisconnected}
	b.Status.Set(status)
	return status
}

func (b *Bot) cancelActivePairing() {
	b.pairMu.Lock()
	cancel := b.cancelPair
	b.cancelPair = nil
	b.pairMu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// Logout ends the active WhatsApp session, wiping the paired device. whatsmeow
// deletes the underlying device store as part of a successful logout, which
// permanently disables the client that ran it (see RecreateSession's
// docstring) - so a fresh, ready-to-pair client is swapped in right after.
func (b *Bot) Logout(ctx context.Context) (Status, error) {
	if err := b.currentSession().Session.Logout(ctx); err != nil {
		return Status{}, fmt.Errorf("bot: logout: %w", err)
	}

	if err := b.RecreateSession(ctx); err != nil {
		b.Logger.Errorf("failed to set up a fresh session after logout: %v", err)
	}

	status := Status{State: StateDisconnected}
	b.Status.Set(status)
	return status, nil
}

// RecreateSession swaps in a fresh, unpaired client bound to a brand new
// device. whatsmeow permanently disables a *whatsmeow.Client the instant its
// device is deleted (store.ErrDeviceDeleted, "invalid use of deleted
// device") - and it deletes the device automatically after *any* logout,
// remote (phone unlinked, banned, etc. - see the *events.LoggedOut case in
// EventHandler) or through our own Logout call above. Without this, the bot
// would need a full process restart to ever pair again.
func (b *Bot) RecreateSession(ctx context.Context) error {
	if b.newDeviceClient == nil {
		return errors.New("bot: no newDeviceClient factory configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, session, getJID, err := b.newDeviceClient(ctx)
	if err != nil {
		return fmt.Errorf("bot: create new device client: %w", err)
	}

	b.mu.Lock()
	b.session = deviceSession{Client: client, Session: session, GetJID: getJID}
	b.mu.Unlock()

	return nil
}
