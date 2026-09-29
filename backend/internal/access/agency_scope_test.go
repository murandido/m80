package access

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	testDBOnce   sync.Once
	testDB       *gorm.DB
	testDBErr    error
	phoneCounter int64
)

// setupTestDB mirrors the handlers' helper: probe first so a missing database skips
// instead of hitting data.Connect's log.Fatal, then connect through the production path so
// the schema comes from the real migrations.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDBOnce.Do(func() {
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"), os.Getenv("DB_PORT"))
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
		t.Skipf("skipping: no test database available: %v", testDBErr)
	}
	return testDB
}

func runInTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)
	t.Parallel()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	fn(tx)
}

func seedAgency(t *testing.T, tx *gorm.DB, name string) data.Agency {
	t.Helper()
	agency := data.Agency{Name: name + " " + uuid.New().String()[:8]}
	if err := tx.Create(&agency).Error; err != nil {
		t.Fatalf("seed agency: %v", err)
	}
	return agency
}

func seedUser(t *testing.T, tx *gorm.DB, role string) data.User {
	t.Helper()
	phone := fmt.Sprintf("1199%07d", atomic.AddInt64(&phoneCounter, 1)%10000000)
	user := data.User{Name: "Scoped", Phone: phone, Password: "irrelevant", Role: role}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return user
}

func grant(t *testing.T, tx *gorm.DB, user data.User, agencies ...data.Agency) {
	t.Helper()
	for _, agency := range agencies {
		if err := tx.Create(&data.UserAgency{UserID: user.ID, AgencyID: agency.ID}).Error; err != nil {
			t.Fatalf("grant: %v", err)
		}
	}
}

// defaultAgency reuses the database's default agency when one exists: the partial unique
// index allows only one, and a committed seed may already hold it.
func defaultAgency(t *testing.T, tx *gorm.DB) data.Agency {
	t.Helper()
	var agency data.Agency
	if err := tx.Where("is_default = ?", true).First(&agency).Error; err == nil {
		return agency
	}
	agency = data.Agency{Name: "Principal " + uuid.New().String()[:8], IsDefault: true}
	if err := tx.Create(&agency).Error; err != nil {
		t.Fatalf("seed default agency: %v", err)
	}
	return agency
}

func TestAgencyScope_Allows(t *testing.T) {
	t.Parallel()
	if !FullScope().Allows("anything") {
		t.Error("full scope must allow any agency")
	}
	if (AgencyScope{}).Allows("anything") {
		t.Error("empty scope must allow nothing")
	}
	scope := AgencyScope{IDs: []string{"a"}}
	if !scope.Allows("a") || scope.Allows("b") {
		t.Error("partial scope must allow exactly its ids")
	}
}

func TestAgencyScope_Intersect(t *testing.T) {
	t.Parallel()
	if got := FullScope().Intersect(nil); got != nil {
		t.Errorf("full scope, nil request: want nil (every agency), got %v", got)
	}
	if got := (AgencyScope{IDs: []string{"a", "b"}}).Intersect(nil); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("partial scope, nil request: want the scope, got %v", got)
	}
	if got := (AgencyScope{}).Intersect(nil); got == nil || len(got) != 0 {
		t.Errorf("empty scope, nil request: want an empty non-nil list, got %#v", got)
	}
	if got := (AgencyScope{IDs: []string{"a"}}).Intersect([]string{"a", "b"}); !slices.Equal(got, []string{"a"}) {
		t.Errorf("want only the allowed ids, got %v", got)
	}
}

func TestLoadAgencyScope(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		allowed := seedAgency(t, tx, "Allowed")
		seedAgency(t, tx, "Other")

		dev := seedUser(t, tx, data.RoleDev)
		scope, err := LoadAgencyScope(tx, dev)
		if err != nil || !scope.All {
			t.Fatalf("dev: want full scope, got %+v (err %v)", scope, err)
		}

		admin := seedUser(t, tx, data.RoleAdmin)
		grant(t, tx, admin, allowed)
		scope, err = LoadAgencyScope(tx, admin)
		if err != nil || scope.All || !slices.Equal(scope.IDs, []string{allowed.ID}) {
			t.Fatalf("admin: want only %s, got %+v (err %v)", allowed.ID, scope, err)
		}

		user := seedUser(t, tx, data.RoleUser)
		scope, err = LoadAgencyScope(tx, user)
		if err != nil || scope.All || len(scope.IDs) != 0 {
			t.Fatalf("user without grants: want empty scope, got %+v (err %v)", scope, err)
		}
	})
}

func TestRestrict(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		allowed := seedAgency(t, tx, "Allowed")
		other := seedAgency(t, tx, "Other")
		ids := []string{allowed.ID, other.ID}

		count := func(scope AgencyScope) int64 {
			var n int64
			scope.Restrict(tx.Model(&data.Agency{}).Where("id IN ?", ids), "agencies.id").Count(&n)
			return n
		}
		if n := count(FullScope()); n != 2 {
			t.Errorf("full scope: want 2, got %d", n)
		}
		if n := count(AgencyScope{IDs: []string{allowed.ID}}); n != 1 {
			t.Errorf("partial scope: want 1, got %d", n)
		}
		if n := count(AgencyScope{}); n != 0 {
			t.Errorf("empty scope: want 0, got %d", n)
		}
	})
}

func TestResolveDefaultAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		principal := defaultAgency(t, tx)
		first := seedAgency(t, tx, "AAA")
		second := seedAgency(t, tx, "ZZZ")

		if got, err := ResolveDefaultAgency(tx, FullScope()); err != nil || got != principal.ID {
			t.Errorf("full scope: want the main agency, got %s (err %v)", got, err)
		}
		if got, err := ResolveDefaultAgency(tx, AgencyScope{IDs: []string{second.ID, principal.ID}}); err != nil || got != principal.ID {
			t.Errorf("scope with the main agency: want it, got %s (err %v)", got, err)
		}
		if got, err := ResolveDefaultAgency(tx, AgencyScope{IDs: []string{second.ID, first.ID}}); err != nil || got != first.ID {
			t.Errorf("scope without the main agency: want the first by name, got %s (err %v)", got, err)
		}
		if _, err := ResolveDefaultAgency(tx, AgencyScope{}); !errors.Is(err, ErrNoAgencyAccess) {
			t.Errorf("empty scope: want ErrNoAgencyAccess, got %v", err)
		}
	})
}
