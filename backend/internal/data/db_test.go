package data

import (
	"testing"

	"gorm.io/gorm/logger"
)

// The per-statement SQL log is a development tool. Leaving it on in production
// writes a log line per query on a box with one disk, which is why this is
// decided by ENV rather than left at GORM's default.
func TestGormLogLevel(t *testing.T) {
	cases := map[string]logger.LogLevel{
		"production":  logger.Warn,
		"development": logger.Info,
		"test":        logger.Info,
		"":            logger.Info,
	}

	for env, want := range cases {
		t.Run(env, func(t *testing.T) {
			t.Setenv("ENV", env)
			if got := gormLogLevel(); got != want {
				t.Fatalf("gormLogLevel() with ENV=%q = %v, want %v", env, got, want)
			}
		})
	}
}
