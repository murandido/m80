package routes

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRegisterListenedChatRoutes_RequiresAdmin(t *testing.T) {
	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/bot/chats/pending"},
		{http.MethodGet, "/bot/chats/active"},
		{http.MethodPost, "/bot/chats/00000000-0000-0000-0000-000000000000/approve"},
		{http.MethodPost, "/bot/chats/00000000-0000-0000-0000-000000000000/deny"},
		{http.MethodPost, "/bot/chats/00000000-0000-0000-0000-000000000000/stop"},
		{http.MethodPost, "/bot/chats/00000000-0000-0000-0000-000000000000/deactivate"},
		{http.MethodPost, "/bot/chats/00000000-0000-0000-0000-000000000000/reactivate"},
	}

	for _, p := range paths {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			r := chi.NewRouter()
			r.Group(func(r chi.Router) {
				r.Use(withRole("emissor"))
				RegisterListenedChatRoutes(r, nil)
			})

			req := httptest.NewRequest(p.method, p.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}

// Proves /bot/chats/* is a static child that always wins over RegisterBotRoutes'
// "/bot/*" catch-all reverse proxy, regardless of registration order - the
// concrete risk called out in the implementation plan for this feature.
func TestRegisterListenedChatRoutes_DoesNotFallThroughToBotProxy(t *testing.T) {
	db := setupRoutesTestDB(t)

	botServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t.Errorf("bot proxy should never be reached for /bot/chats/*, got request to %s", req.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer botServer.Close()
	t.Setenv("BOT_INTERNAL_URL", botServer.URL)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(withRole("admin"))
		RegisterBotRoutes(r)
		RegisterListenedChatRoutes(r, db)
	})

	req := httptest.NewRequest(http.MethodGet, "/bot/chats/pending", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("expected the real handler's envelope, got: %s", rec.Body.String())
	}
}

var (
	routesTestDBOnce sync.Once
	routesTestDB     *gorm.DB
	routesTestDBErr  error
)

func setupRoutesTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	routesTestDBOnce.Do(func() {
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			os.Getenv("DB_PORT"),
		)

		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			routesTestDBErr = err
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			routesTestDBErr = err
			return
		}
		if err := sqlDB.Ping(); err != nil {
			routesTestDBErr = err
			return
		}

		data.Connect()
		routesTestDB = data.DB
	})

	if routesTestDBErr != nil {
		t.Skipf("skipping listened chat route tests: no test database available: %v", routesTestDBErr)
	}
	return routesTestDB
}
