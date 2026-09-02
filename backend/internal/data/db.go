package data

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed migrations
var migrationsFS embed.FS

var DB *gorm.DB

func Connect() {
	migrateURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)

	if err := runMigrations(migrateURL); err != nil {
		log.Fatal("❌ Failed to run migrations: ", err)
	}
	log.Println("✅ Migrations applied!")

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=America/Sao_Paulo",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)

	config := &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel()),
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), config)
	if err != nil {
		log.Fatal("❌ Fail to connect to the database: ", err)
	}

	if err := configurePool(DB); err != nil {
		log.Fatal("❌ Fail to configure the connection pool: ", err)
	}

	log.Println("✅ Database connected!")
	// AutoMigrate removed — schema is managed exclusively by golang-migrate
}

// Pool sizing for the one box everything now shares.
//
// database/sql defaults to an unbounded number of open connections and only two
// idle ones, which is the worst pair for this workload: a panel page fans out
// several API calls at once, so most queries paid for a fresh TCP connection and
// a fresh Postgres backend process, and a burst could still open as many of them
// as it liked. Postgres forks per connection and runs on the same 12 GB machine
// as the API, the panel, the bot and a headless Chrome - "unbounded" is a way to
// lose the database to something that is not even the database's fault.
//
// Idle equals open on purpose: with a cap this small there is no reason to ever
// close a healthy connection and re-open it on the next request.
const (
	maxOpenConns    = 20
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute
)

func configurePool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("data.configurePool: getting the sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)
	// Recycled rather than kept forever, so a connection that outlived a database
	// restart or a network hiccup is retired on a schedule instead of on an error
	// somebody has to see first.
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	return nil
}

// gormLogLevel keeps the per-statement SQL log out of production.
//
// logger.Info prints every statement the API runs, which in development is the
// whole point and in production is a line of log per query, forever, on a box
// with one disk. Warn still reports slow queries (GORM's own 200ms threshold)
// and errors, which is the part worth having.
func gormLogLevel() logger.LogLevel {
	if os.Getenv("ENV") == "production" {
		return logger.Warn
	}
	return logger.Info
}

func runMigrations(databaseURL string) error {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("creating iofs source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, databaseURL)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			log.Println("⚠️ Error closing migrate instance:", srcErr, dbErr)
		}
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running up migrations: %w", err)
	}

	return nil
}
