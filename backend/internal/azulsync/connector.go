package azulsync

import (
	"context"
	"errors"
	"log/slog"
	"time"

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
		if markErr := MarkLoginFailure(c.DB, account.ID, err); markErr != nil {
			slog.Error("azul connect: failed to record login failure", "account_id", account.ID, "err", markErr)
		}
		// The credentials are never logged; err from the login service is already credential-free.
		slog.Warn("azul connect: login did not complete", "account_id", account.ID, "err", err)
		return
	}

	// BeforeSave encrypts the customerKey the same way it does the password.
	account.AzulCustomerKey = &key
	if err := c.DB.Save(&account).Error; err != nil {
		slog.Error("azul connect: connected but failed to store the customerKey", "account_id", account.ID, "err", err)
		return
	}
	if err := MarkConnected(c.DB, account.ID); err != nil {
		slog.Error("azul connect: connected but failed to record the state", "account_id", account.ID, "err", err)
		return
	}
	slog.Info("azul connect: account connected", "account_id", account.ID)

	// Pull the first statement window (up to a month) right away, so the panel shows the
	// reconciliation as soon as the connect finishes instead of waiting for the next ticker round.
	// This runs whether or not the periodic ticker (AZUL_SYNC_ENABLED) is on - connecting is an
	// explicit action, and the operator expects the extract to be there.
	c.firstSync(ctx, account.ID)
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
