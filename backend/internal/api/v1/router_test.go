package v1

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/murandido/m-80-backend/internal/auth"
	"github.com/murandido/m-80-backend/internal/azulsync"
	"github.com/murandido/m-80-backend/internal/booking"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/health"
	"github.com/murandido/m-80-backend/internal/latamsync"
	"github.com/murandido/m-80-backend/internal/smilessync"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error
	// Each package that seeds users owns a range of phone numbers, so packages running
	// against the same Postgres at once never collide on idx_users_phone.
	phoneCounter int64 = 6_000_000
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	testDBOnce.Do(func() {
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			os.Getenv("DB_PORT"),
		)
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			testDBErr = err
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			testDBErr = err
			return
		}
		if err := sqlDB.Ping(); err != nil {
			testDBErr = err
			return
		}
		data.Connect()
		testDB = data.DB
	})

	if testDBErr != nil {
		t.Skipf("skipping router tests: no test database available: %v", testDBErr)
	}
	return testDB
}

func TestMain(m *testing.M) {
	// Set once for the whole binary: t.Setenv is not allowed in parallel tests.
	if os.Getenv("JWT_SECRET") == "" {
		_ = os.Setenv("JWT_SECRET", "test-secret-for-router-tests")
	}
	os.Exit(m.Run())
}

// tokenForRole commits nothing: the user lives in a transaction the test rolls back, and the
// router is built over that same transaction so RequireAuth can see the row.
func tokenForRole(t *testing.T, tx *gorm.DB, role string) string {
	t.Helper()
	phone := fmt.Sprintf("1199%07d", atomic.AddInt64(&phoneCounter, 1)%10000000)
	user := data.User{Name: "Router " + role, Phone: phone, Password: "irrelevant", Role: role}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatalf("failed to seed %s user: %v", role, err)
	}
	token, err := auth.GenerateToken(user.ID, user.Phone, user.Role)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	return token
}

// newTestRouter wires the real router with inert dependencies: nothing here logs in to an
// airline, the routes only need the objects to exist at registration time.
func newTestRouter(tx *gorm.DB) http.Handler {
	connector := azulsync.NewConnector(tx, azulsync.NewConnectTracker(), nil, nil)
	return NewRouter(tx, events.NewBroker(), booking.NewRegistry(nil), health.NewMonitor(func() {}),
		smilessync.NewSessionGuard(), latamsync.NewSessionGuard(), connector)
}

func get(router http.Handler, path, token string) int {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

// staffRoutes is a sample of the private group: the ones that hand out the most (people,
// accounts with their decrypted password, the vault, users, the realtime stream).
var staffRoutes = []string{
	"/people",
	"/accounts",
	"/accounts/vault",
	"/clients",
	"/issuances",
	"/users",
	"/status",
	"/events",
}

// The regression this guards: every authenticated user used to reach the whole private
// group, so a role made for outsiders would have inherited people, accounts and passwords.
func TestRouter_StatementClientIsKeptOutOfStaffRoutes(t *testing.T) {
	db := setupTestDB(t)
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })

	router := newTestRouter(tx)
	token := tokenForRole(t, tx, data.RoleStatementClient)

	for _, path := range staffRoutes {
		if code := get(router, path, token); code != http.StatusForbidden {
			t.Errorf("GET %s as statement_client = %d, want 403", path, code)
		}
	}

	// /auth/me stays open: the panel needs it to know where to send this user.
	if code := get(router, "/auth/me", token); code != http.StatusOK {
		t.Errorf("GET /auth/me as statement_client = %d, want 200", code)
	}
}

// The other side of the same gate: a wrong role list would lock the team out of the panel.
func TestRouter_StaffStillReachTheirRoutes(t *testing.T) {
	db := setupTestDB(t)
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })

	router := newTestRouter(tx)

	for _, role := range []string{data.RoleUser, data.RoleAdmin, data.RoleDev} {
		token := tokenForRole(t, tx, role)
		// /events streams forever and /status needs a live monitor; both are covered by the
		// 403 test above, and the role gate is the same middleware for every route here.
		for _, path := range []string{"/people", "/accounts", "/clients", "/issuances", "/users"} {
			if code := get(router, path, token); code == http.StatusForbidden || code == http.StatusUnauthorized {
				t.Errorf("GET %s as %s = %d, want access", path, role, code)
			}
		}
	}
}
