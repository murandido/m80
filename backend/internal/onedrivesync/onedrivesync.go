// Package onedrivesync keeps the Microsoft login the API reads the Tropas' workbooks with:
// starting and finishing the sign-in, renewing the token, and downloading a shared workbook.
// The HTTP side of Microsoft lives in carriers/onedrive; this package is the state around it.
package onedrivesync

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/murandido/m-80-backend/carriers/onedrive"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// Graph is the part of the Microsoft client this package uses, so tests can stand in for it.
type Graph interface {
	AuthorizeURL(state string) string
	ExchangeCode(ctx context.Context, code string) (onedrive.Tokens, error)
	Refresh(ctx context.Context, refreshToken string) (onedrive.Tokens, error)
	Me(ctx context.Context, accessToken string) (onedrive.Profile, error)
	DownloadShared(ctx context.Context, accessToken, shareURL string) ([]byte, error)
}

var (
	// ErrNotConfigured is the API running without the app registration (MICROSOFT_* unset).
	ErrNotConfigured = errors.New("conexão Microsoft não configurada neste servidor (MICROSOFT_CLIENT_ID/SECRET/REDIRECT_URI)")
	// ErrNotConnected is nobody having signed in yet, or the login having been removed.
	ErrNotConnected = errors.New("nenhuma conta Microsoft conectada: conecte em Dev Mode > Tropas")
	// ErrUnknownState is a callback whose state this process did not hand out, or handed out
	// to somebody else, or longer ago than pendingTTL.
	ErrUnknownState = errors.New("login Microsoft expirado ou de outra sessão: comece de novo")
)

// pendingTTL bounds how long a sign-in may take between Connect and the callback.
const pendingTTL = 15 * time.Minute

type pendingLogin struct {
	userID    string
	startedAt time.Time
}

// Service is one per API process: the token cache and the refresh lock only work if every
// caller shares them.
type Service struct {
	db    *gorm.DB
	graph Graph
	now   func() time.Time

	mu          sync.Mutex
	pending     map[string]pendingLogin
	accessToken string
	expiresAt   time.Time
}

// New builds the service. graph may be nil when the app registration is missing; every call
// then answers ErrNotConfigured instead of the API failing to boot.
func New(db *gorm.DB, graph Graph) *Service {
	return &Service{db: db, graph: graph, now: time.Now, pending: map[string]pendingLogin{}}
}

// Configured reports whether the app registration is present.
func (s *Service) Configured() bool { return s != nil && s.graph != nil }

// Begin starts a sign-in for userID and returns where to send their browser.
func (s *Service) Begin(userID string) (string, error) {
	if !s.Configured() {
		return "", ErrNotConfigured
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(raw)

	s.mu.Lock()
	defer s.mu.Unlock()
	for key, login := range s.pending {
		if login.userID == userID || s.now().Sub(login.startedAt) > pendingTTL {
			delete(s.pending, key)
		}
	}
	s.pending[state] = pendingLogin{userID: userID, startedAt: s.now()}
	return s.graph.AuthorizeURL(state), nil
}

// Finish completes the sign-in from the redirect: the state must be one this process handed
// to the same user, within pendingTTL, and is good once.
func (s *Service) Finish(ctx context.Context, userID, code, state string) (data.MicrosoftConnection, error) {
	if !s.Configured() {
		return data.MicrosoftConnection{}, ErrNotConfigured
	}
	s.mu.Lock()
	login, ok := s.pending[state]
	delete(s.pending, state)
	s.mu.Unlock()
	if !ok || login.userID != userID || s.now().Sub(login.startedAt) > pendingTTL {
		return data.MicrosoftConnection{}, ErrUnknownState
	}

	tokens, err := s.graph.ExchangeCode(ctx, code)
	if err != nil {
		return data.MicrosoftConnection{}, fmt.Errorf("trocar o código da Microsoft: %w", err)
	}
	profile, err := s.graph.Me(ctx, tokens.AccessToken)
	if err != nil {
		return data.MicrosoftConnection{}, fmt.Errorf("ler a conta Microsoft: %w", err)
	}

	now := s.now()
	connection := data.MicrosoftConnection{
		ConnectedByID: userID,
		AccountName:   nonEmpty(profile.Name),
		AccountEmail:  nonEmpty(profile.Email),
		RefreshToken:  tokens.RefreshToken,
		State:         data.MicrosoftConnectionOK,
		LastOKAt:      &now,
	}
	// Connecting again replaces the old login: the workbooks are read as one person.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&data.MicrosoftConnection{}).Error; err != nil {
			return err
		}
		return tx.Create(&connection).Error
	})
	if err != nil {
		return data.MicrosoftConnection{}, err
	}

	s.mu.Lock()
	s.accessToken, s.expiresAt = tokens.AccessToken, tokens.ExpiresAt
	s.mu.Unlock()
	return connection, nil
}

// Status is the current connection, or nil when there is none.
func (s *Service) Status() (*data.MicrosoftConnection, error) {
	var connection data.MicrosoftConnection
	err := s.db.Order("created_at desc").First(&connection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &connection, nil
}

// Disconnect forgets the login. The consent stays in the Microsoft account until removed
// there; without the refresh token the API can no longer use it.
func (s *Service) Disconnect() error {
	s.mu.Lock()
	s.accessToken, s.expiresAt = "", time.Time{}
	s.mu.Unlock()
	return s.db.Where("1 = 1").Delete(&data.MicrosoftConnection{}).Error
}

// Download fetches a shared workbook as the connected account.
func (s *Service) Download(ctx context.Context, shareURL string) ([]byte, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}
	token, err := s.token(ctx)
	if err != nil {
		return nil, err
	}
	content, err := s.graph.DownloadShared(ctx, token, shareURL)
	if errors.Is(err, onedrive.ErrNeedsLogin) {
		// The cached access token may have been revoked early; one fresh refresh decides.
		s.mu.Lock()
		s.accessToken = ""
		s.mu.Unlock()
		if token, err = s.token(ctx); err != nil {
			return nil, err
		}
		content, err = s.graph.DownloadShared(ctx, token, shareURL)
	}
	return content, err
}

// token returns a usable access token, refreshing it under the lock so two callers never
// spend the same rotating refresh token.
func (s *Service) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accessToken != "" && s.now().Before(s.expiresAt.Add(-time.Minute)) {
		return s.accessToken, nil
	}

	connection, err := s.Status()
	if err != nil {
		return "", err
	}
	if connection == nil {
		return "", ErrNotConnected
	}
	if connection.State == data.MicrosoftConnectionNeedsLogin {
		return "", fmt.Errorf("%w: a conta Microsoft precisa ser conectada de novo", onedrive.ErrNeedsLogin)
	}

	tokens, err := s.graph.Refresh(ctx, connection.RefreshToken)
	if err != nil {
		state := data.MicrosoftConnectionError
		if errors.Is(err, onedrive.ErrNeedsLogin) {
			state = data.MicrosoftConnectionNeedsLogin
		}
		detail := err.Error()
		if markErr := s.db.Model(&data.MicrosoftConnection{}).Where("id = ?", connection.ID).
			Updates(map[string]any{"state": state, "detail": detail}).Error; markErr != nil {
			slog.Error("onedrivesync: failed to record the connection state", "err", markErr)
		}
		return "", err
	}

	// The new refresh token replaces the old one before anything else: the old one is dead
	// the moment Microsoft answered, and losing the new one means signing in again.
	if tokens.RefreshToken != "" {
		connection.RefreshToken = tokens.RefreshToken
	}
	now := s.now()
	connection.State = data.MicrosoftConnectionOK
	connection.Detail = nil
	connection.LastOKAt = &now
	if err := s.db.Save(connection).Error; err != nil {
		return "", fmt.Errorf("guardar o token renovado da Microsoft: %w", err)
	}

	s.accessToken, s.expiresAt = tokens.AccessToken, tokens.ExpiresAt
	return s.accessToken, nil
}

func nonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
