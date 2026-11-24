package onedrivesync

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/carriers/onedrive"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/testdb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These tests share microsoft_connections, which holds one row per installation, so they do
// not run in parallel.

func TestMain(m *testing.M) {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		// Not recoverable and not interesting: every test below would fail on it anyway.
		_ = os.Setenv("ENCRYPTION_KEY", strings.Repeat("ab", 32))
	}
	os.Exit(m.Run())
}

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testdb.RequireTestDatabase(t)
	testDBOnce.Do(func() {
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"), os.Getenv("DB_PORT"))
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			testDBErr = err
			return
		}
		if sqlDB, err := db.DB(); err != nil || sqlDB.Ping() != nil {
			testDBErr = fmt.Errorf("no database")
			return
		}
		data.Connect()
		testDB = data.DB
	})
	if testDBErr != nil {
		t.Skipf("skipping onedrivesync database tests: %v", testDBErr)
	}
	return testDB
}

func seedUser(t *testing.T, db *gorm.DB) data.User {
	t.Helper()
	n, err := rand.Int(rand.Reader, big.NewInt(10000000))
	if err != nil {
		t.Fatal(err)
	}
	user := data.User{Name: "onedrive test", Phone: fmt.Sprintf("1199%07d", n.Int64()), Password: "irrelevant"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Where("1 = 1").Delete(&data.MicrosoftConnection{})
		db.Unscoped().Delete(&user)
	})
	return user
}

// fakeGraph hands out a new refresh token on every refresh, like Microsoft does.
type fakeGraph struct {
	mu        sync.Mutex
	refreshes int
	refuse    bool
	lastState string
}

func (f *fakeGraph) AuthorizeURL(state string) string {
	f.lastState = state
	return "https://login.example/authorize?state=" + url.QueryEscape(state)
}

func (f *fakeGraph) ExchangeCode(_ context.Context, code string) (onedrive.Tokens, error) {
	if code != "good-code" {
		return onedrive.Tokens{}, onedrive.ErrNeedsLogin
	}
	return onedrive.Tokens{AccessToken: "access-0", RefreshToken: "refresh-0", ExpiresAt: time.Now().Add(-time.Hour)}, nil
}

func (f *fakeGraph) Refresh(_ context.Context, refreshToken string) (onedrive.Tokens, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse {
		return onedrive.Tokens{}, fmt.Errorf("%w: invalid_grant", onedrive.ErrNeedsLogin)
	}
	f.refreshes++
	if refreshToken != fmt.Sprintf("refresh-%d", f.refreshes-1) {
		return onedrive.Tokens{}, fmt.Errorf("%w: stale refresh token %s", onedrive.ErrNeedsLogin, refreshToken)
	}
	return onedrive.Tokens{
		AccessToken:  fmt.Sprintf("access-%d", f.refreshes),
		RefreshToken: fmt.Sprintf("refresh-%d", f.refreshes),
		ExpiresAt:    time.Now().Add(time.Hour),
	}, nil
}

func (f *fakeGraph) Me(context.Context, string) (onedrive.Profile, error) {
	return onedrive.Profile{Name: "Dono", Email: "dono@example.com"}, nil
}

func (f *fakeGraph) DownloadShared(_ context.Context, accessToken, shareURL string) ([]byte, error) {
	return []byte(accessToken + "|" + shareURL), nil
}

func TestConnectNeedsTheSameUsersStateOnce(t *testing.T) {
	db := setupTestDB(t)
	user := seedUser(t, db)
	other := seedUser(t, db)
	graph := &fakeGraph{}
	service := New(db, graph)

	if _, err := service.Begin(user.ID); err != nil {
		t.Fatal(err)
	}
	state := graph.lastState

	if _, err := service.Finish(context.Background(), other.ID, "good-code", state); !errors.Is(err, ErrUnknownState) {
		t.Errorf("another user's callback: got %v, want ErrUnknownState", err)
	}
	// The failed attempt above consumed the state: a state is good once, whoever presents it.
	if _, err := service.Finish(context.Background(), user.ID, "good-code", state); !errors.Is(err, ErrUnknownState) {
		t.Errorf("reused state: got %v, want ErrUnknownState", err)
	}

	if _, err := service.Begin(user.ID); err != nil {
		t.Fatal(err)
	}
	connection, err := service.Finish(context.Background(), user.ID, "good-code", graph.lastState)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if connection.AccountEmail == nil || *connection.AccountEmail != "dono@example.com" {
		t.Errorf("profile not stored: %+v", connection)
	}

	var raw string
	if err := db.Raw("SELECT refresh_token FROM microsoft_connections").Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "enc:") {
		t.Errorf("the refresh token is stored in clear")
	}
}

func TestDownloadRotatesTheRefreshTokenAndMarksARefusal(t *testing.T) {
	db := setupTestDB(t)
	user := seedUser(t, db)
	graph := &fakeGraph{}
	service := New(db, graph)
	if _, err := service.Begin(user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Finish(context.Background(), user.ID, "good-code", graph.lastState); err != nil {
		t.Fatal(err)
	}

	// The access token from the sign-in is already expired, so this refreshes once and
	// must store refresh-1; the second download reuses the cached access token.
	for range 2 {
		content, err := service.Download(context.Background(), "https://1drv.ms/x/abc")
		if err != nil || string(content) != "access-1|https://1drv.ms/x/abc" {
			t.Fatalf("download: %q %v", content, err)
		}
	}
	if graph.refreshes != 1 {
		t.Errorf("refreshed %d times, want 1", graph.refreshes)
	}
	stored, err := service.Status()
	if err != nil || stored == nil || stored.RefreshToken != "refresh-1" {
		t.Fatalf("the rotated refresh token was not stored: %+v %v", stored, err)
	}

	// A new process (empty cache) with a refusing Microsoft marks the login as needing a person.
	graph.refuse = true
	fresh := New(db, graph)
	if _, err := fresh.Download(context.Background(), "https://1drv.ms/x/abc"); !errors.Is(err, onedrive.ErrNeedsLogin) {
		t.Fatalf("refused refresh: got %v, want ErrNeedsLogin", err)
	}
	stored, err = fresh.Status()
	if err != nil || stored.State != data.MicrosoftConnectionNeedsLogin {
		t.Errorf("state after refusal: %+v %v", stored, err)
	}
}

func TestUnconfiguredServiceSaysSo(t *testing.T) {
	service := New(nil, nil)
	if _, err := service.Begin("x"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("begin: got %v", err)
	}
	if _, err := service.Download(context.Background(), "x"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("download: got %v", err)
	}
}
