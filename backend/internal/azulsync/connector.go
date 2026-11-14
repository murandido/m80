package azulsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/carriers/azul"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/promotion"
	"gorm.io/gorm"
)

// The reasons Start refuses to launch a login. The connect endpoint turns them into 422/409;
// the automatic triggers (the ticker, a password change) just log them.
var (
	ErrAccountNotFound    = errors.New("azulsync: account not found")
	ErrMissingCredentials = errors.New("azulsync: account has no stored login and password")
	ErrAlreadyConnecting  = errors.New("azulsync: a login is already running for this account")
)

// The reasons StoreBrowserKey refuses a customerKey captured in the operator's browser. Nothing is
// stored in any of them.
var (
	// ErrDocumentMismatch is a key from a login made with a CPF other than the account's: storing
	// it would point this account at another holder's points.
	ErrDocumentMismatch = errors.New("azulsync: the key came from a login with another document")
	// ErrKeyRejected is Azul answering that it does not know the key.
	ErrKeyRejected = errors.New("azulsync: Azul did not recognise the captured customerKey")
	// ErrKeyUnverified is the check itself failing (edge block, rate limit, outage): the key may be
	// fine, but an unchecked key is not stored.
	ErrKeyUnverified = errors.New("azulsync: could not check the captured customerKey with Azul")
	// ErrVerifierUnavailable is a process without the read client (no Firebase config).
	ErrVerifierUnavailable = errors.New("azulsync: no read client to check a captured customerKey")
)

// Login is the slice of the promotion client the connector needs: the browser login that turns
// stored credentials into a customerKey. An interface so the flow can be tested without the
// login service; *promotion.Client satisfies it.
type Login interface {
	AzulLogin(ctx context.Context, req promotion.AzulLoginRequest) (string, error)
}

// Connector runs the "connect this Azul account" flow: log in through the login service to
// discover the customerKey, store it, and pull the first statement window.
//
// It lives here rather than in the HTTP handler because three things start it: the operator's
// Conectar/Reconectar button, the ticker when Azul stops recognising a stored key, and saving a
// new password on an account in that state. One instance per process, built in cmd/api, so all
// three share the same tracker - two of them racing on one account must not launch two browsers.
type Connector struct {
	DB      *gorm.DB
	Tracker *ConnectTracker
	Login   Login
	// Reader is the read client for the first sync. Optional: without it (no Firebase config)
	// the account is left connected and the ticker pulls the window later.
	Reader Reader
}

// NewConnector builds the connector. reader may be nil.
func NewConnector(db *gorm.DB, tracker *ConnectTracker, login Login, reader Reader) *Connector {
	return &Connector{DB: db, Tracker: tracker, Login: login, Reader: reader}
}

// Start validates the account and launches its login in the background, returning at once. The
// login takes minutes and can retry across several browser sessions, far longer than a request
// should stay open, so the caller never waits for it; progress shows on the panel through the
// tracker ("conectando") and the sync state.
//
// The login runs on context.WithoutCancel(ctx): it keeps the caller's values but not its
// cancellation, so an operator closing the tab does not abort a browser mid-login.
func (c *Connector) Start(ctx context.Context, accountID string) error {
	var account data.Account
	if err := c.DB.First(&account, "id = ?", accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
		}
		return err
	}
	// Any account may connect: the balance is read for every connected account, and
	// "Acompanhar extrato" only decides whether the statement is pulled on top of it.
	if account.Login == nil || *account.Login == "" || account.Password == nil || *account.Password == "" {
		return ErrMissingCredentials
	}
	// Refuse a duplicate rather than launch a second browser for the same account.
	if !c.Tracker.Begin(account.ID) {
		return ErrAlreadyConnecting
	}

	go c.run(context.WithoutCancel(ctx), account)
	return nil
}

// run performs the background login and records the outcome. The tracker mark is always cleared
// at the end.
func (c *Connector) run(ctx context.Context, account data.Account) {
	defer c.Tracker.End(account.ID)

	// account.Login/Password come back decrypted from the AfterFind hook, ready to send.
	key, err := c.Login.AzulLogin(ctx, promotion.AzulLoginRequest{
		DocumentNumber: *account.Login,
		Password:       *account.Password,
	})
	if err != nil {
		// A login outcome about the account (bad password) counts toward its
		// failure streak. An edge block on the shared device, a rate limit or a
		// transport failure - the login service down or its queue full - is an
		// infrastructure problem, not a credential one, and must not raise
		// needs_attention: the caller can simply try again.
		//
		// Either way the reason is written for the panel: an operator who clicked Conectar must
		// see why nothing happened, not a card that silently went back to "não conectada".
		mark := MarkConnectInterrupted
		if errors.Is(err, promotion.ErrAzulLoginFailed) {
			mark = MarkLoginFailure
		}
		if markErr := mark(c.DB, account.ID, err); markErr != nil {
			slog.Error("azul connect: failed to record the login outcome", "account_id", account.ID, "err", markErr)
		}
		// The credentials are never logged; err from the login service is already credential-free.
		slog.Warn("azul connect: login did not complete", "account_id", account.ID, "err", err)
		return
	}

	if err := c.storeKey(account, key); err != nil {
		slog.Error("azul connect: connected but failed to store the key", "account_id", account.ID, "err", err)
		return
	}
	slog.Info("azul connect: account connected", "account_id", account.ID)

	// Pull the first statement window (up to a month) right away, so the panel shows the
	// reconciliation as soon as the connect finishes instead of waiting for the next ticker round.
	// This runs whether or not the periodic ticker (AZUL_SYNC_ENABLED) is on - connecting is an
	// explicit action, and the operator expects the extract to be there.
	c.firstSync(ctx, account.ID)
}

// storeKey keeps a discovered customerKey and puts the account back to pending, so the ticker
// (and the first sync) pick it up. BeforeSave encrypts the key the same way it does the password.
func (c *Connector) storeKey(account data.Account, key string) error {
	account.AzulCustomerKey = &key
	if err := c.DB.Save(&account).Error; err != nil {
		return fmt.Errorf("storing the customerKey: %w", err)
	}
	if err := MarkConnected(c.DB, account.ID); err != nil {
		return fmt.Errorf("recording the state: %w", err)
	}
	return nil
}

// StoreBrowserKey connects an account with a customerKey captured from a login the operator made
// in their own browser - the fallback for when the login service cannot get through. The key
// arrives from the panel, so it is checked before it is kept:
//
//   - the login it came from was made with this account's CPF (the extension reports the CPF the
//     login was typed with; another holder's key would silently read someone else's points);
//   - Azul answers points/resume for it, the same read every sync round starts with.
//
// Only then is it stored, exactly like a key from the login service. The caller runs FirstSync
// afterwards (in the background: the operator should see "conectada" without waiting for the
// statement). It refuses while a server-side login for the account is in flight, so the two
// paths never race on the same row.
func (c *Connector) StoreBrowserKey(ctx context.Context, accountID, customerKey, loginDocument string) error {
	var account data.Account
	if err := c.DB.First(&account, "id = ?", accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
		}
		return err
	}
	if account.Login == nil || digitsOnly(*account.Login) == "" {
		return ErrMissingCredentials
	}
	if digitsOnly(loginDocument) != digitsOnly(*account.Login) {
		return ErrDocumentMismatch
	}
	if c.Reader == nil {
		return ErrVerifierUnavailable
	}
	if !c.Tracker.Begin(account.ID) {
		return ErrAlreadyConnecting
	}
	defer c.Tracker.End(account.ID)

	if _, err := c.Reader.Balance(ctx, customerKey); err != nil {
		if errors.Is(err, azul.ErrNotFound) {
			return fmt.Errorf("%w: %w", ErrKeyRejected, err)
		}
		return fmt.Errorf("%w: %w", ErrKeyUnverified, err)
	}
	if err := c.storeKey(account, customerKey); err != nil {
		return err
	}
	slog.Info("azul connect: account connected through the operator's browser", "account_id", account.ID)
	return nil
}

// FirstSync pulls the first statement window for a freshly connected account; see firstSync.
func (c *Connector) FirstSync(ctx context.Context, accountID string) {
	c.firstSync(ctx, accountID)
}

// digitsOnly keeps the digits of a document, so a CPF stored as 123.456.789-00 compares equal
// to the 12345678900 a login was typed with.
func digitsOnly(value string) string {
	var digits strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	return digits.String()
}

// firstSync runs one read round for a freshly connected account. The account is re-read so the
// AfterFind hook hands SyncAccount the decrypted customerKey.
func (c *Connector) firstSync(ctx context.Context, accountID string) {
	if c.Reader == nil {
		slog.Warn("azul connect: read client unavailable; first sync deferred to the ticker", "account_id", accountID)
		return
	}
	var account data.Account
	if err := c.DB.First(&account, "id = ?", accountID).Error; err != nil {
		slog.Error("azul connect: reloading account for the first sync", "account_id", accountID, "err", err)
		return
	}
	if err := SyncAccount(ctx, c.DB, c.Reader, account, time.Now()); err != nil {
		// SyncAccount already records the failure in the sync state; log and move on.
		slog.Warn("azul connect: first sync did not complete", "account_id", accountID, "err", err)
		return
	}
	slog.Info("azul connect: first sync done", "account_id", accountID)
}
