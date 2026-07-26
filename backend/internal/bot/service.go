package bot

import (
	"context"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type ClientBase interface {
	SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (resp whatsmeow.SendResponse, err error)
	SendChatPresence(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error
	MarkRead(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID, receiptTypeExtra ...types.ReceiptType) error
}

// SessionClient is the subset of *whatsmeow.Client needed to drive the
// connection lifecycle (pairing, logout) - kept separate from ClientBase so
// tests can fake connection behavior without also faking message sending.
type SessionClient interface {
	Connect() error
	Disconnect()
	IsConnected() bool
	IsLoggedIn() bool
	PairPhone(ctx context.Context, phone string, showPushNotification bool, clientType whatsmeow.PairClientType, clientDisplayName string) (string, error)
	GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error)
	Logout(ctx context.Context) error
	// DeleteStore deletes the underlying device. whatsmeow already does this
	// itself on every logout, but asynchronously and unsynchronized with the
	// *events.LoggedOut dispatch - see the case in EventHandler. Calling it
	// ourselves, and awaiting it, is how we get a deterministic point after
	// which asking for a new device is guaranteed to see it gone.
	DeleteStore(ctx context.Context) error
}

// sessionAdapter adapts a concrete *whatsmeow.Client to ClientBase and
// SessionClient. Needed only because DeleteStore isn't a method on
// *whatsmeow.Client itself - it's reached through the Store field
// (client.Store.Delete) - so plain embedding wouldn't satisfy SessionClient
// on its own.
type sessionAdapter struct {
	*whatsmeow.Client
}

func (s *sessionAdapter) DeleteStore(ctx context.Context) error {
	return s.Store.Delete(ctx)
}

// WrapClient adapts a concrete *whatsmeow.Client so it satisfies ClientBase
// and SessionClient. Exported so cmd/bot can use the exact same adapter for
// both the client built at boot and every client newDeviceClient builds
// afterwards.
func WrapClient(client *whatsmeow.Client) (ClientBase, SessionClient, func() *types.JID) {
	adapter := &sessionAdapter{Client: client}
	return adapter, adapter, func() *types.JID { return client.Store.ID }
}

// deviceSession bundles the whatsmeow client with everything derived from it.
// Bundled together because they always change as one unit: whatsmeow
// permanently disables a *whatsmeow.Client the moment its underlying device
// is deleted (see store.ErrDeviceDeleted), which happens automatically after
// any logout - remote or from RecreateSession's own Logout call - so
// recovering means swapping in a brand new client, not just reconnecting.
type deviceSession struct {
	Client  ClientBase
	Session SessionClient
	// GetJID returns the currently paired JID, or nil if there isn't one.
	// A closure rather than an interface method because *whatsmeow.Client
	// exposes it as a field (Store.ID), not a method.
	GetJID func() *types.JID
}

type Bot struct {
	// newDeviceClient builds a client bound to a brand new, unpaired device -
	// used by RecreateSession to recover after the current device is
	// deleted. Returns the pieces rather than a concrete *whatsmeow.Client
	// so it stays fakeable in tests; the caller (cmd/bot) is responsible for
	// registering EventHandler on whatever concrete client it builds. nil is
	// tolerated (RecreateSession just logs and gives up) so tests that never
	// exercise that path don't need to provide one.
	newDeviceClient func(ctx context.Context) (ClientBase, SessionClient, func() *types.JID, error)

	Logger waLog.Logger
	Status *StatusStore

	mu      sync.RWMutex
	session deviceSession

	pairMu     sync.Mutex
	cancelPair context.CancelFunc
}

func New(client *whatsmeow.Client, logger waLog.Logger, newDeviceClient func(ctx context.Context) (ClientBase, SessionClient, func() *types.JID, error), statusNotify func()) *Bot {
	return &Bot{
		newDeviceClient: newDeviceClient,
		Logger:          logger,
		Status:          NewStatusStore(statusNotify),
		session:         sessionFor(client),
	}
}

func sessionFor(client *whatsmeow.Client) deviceSession {
	c, s, j := WrapClient(client)
	return deviceSession{Client: c, Session: s, GetJID: j}
}

// currentSession returns a snapshot of the client currently in use. Always
// read through this rather than caching a session across an await point,
// since RecreateSession can swap it out concurrently.
func (b *Bot) currentSession() deviceSession {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.session
}

// Disconnect closes the currently active session. Exposed for cmd/bot's
// shutdown path, which can't reach into the unexported session field
// directly and may be holding a *whatsmeow.Client that RecreateSession has
// since replaced.
func (b *Bot) Disconnect() {
	b.currentSession().Session.Disconnect()
}

// NewTestBot builds a Bot around a fake SessionClient, for cmd/bot's HTTP
// handler tests, which live in a different package and so can't set the
// unexported session field directly the way internal/bot's own tests do.
// Not for production use - internal/ already keeps this out of reach outside
// this module.
func NewTestBot(session SessionClient, getJID func() *types.JID, logger waLog.Logger, status *StatusStore) *Bot {
	return &Bot{
		Logger: logger,
		Status: status,
		session: deviceSession{
			Session: session,
			GetJID:  getJID,
		},
	}
}
