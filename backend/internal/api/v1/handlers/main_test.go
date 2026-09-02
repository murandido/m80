package handlers

import (
	"os"
	"testing"
)

// Test key material, used only when the environment does not already carry the real thing.
// Both are the shape their loaders demand: 64 hex characters, 32 bytes.
const (
	testEncryptionKey = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	testIndexKey      = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testJWTSecret     = "test-secret"
)

// TestMain supplies the process-wide environment this package needs, starting with the two
// keys its models need to save a row at all.
//
// Since migration 38 a Person carrying a CPF cannot be written without them: the number is
// encrypted under ENCRYPTION_KEY and its blind index derived under CPF_INDEX_KEY, and both
// failures are deliberately loud rather than silent - a person saved with no fingerprint would
// be a document nothing keeps unique. That is right in production and inconvenient here, where
// `go test ./...` runs against a bare Postgres with only the DB variables set.
//
// JWT_SECRET joins them for a smaller reason: auth.GenerateToken refuses to sign without
// one, and the login tests used to set it per call with t.Setenv - which a parallel test
// is not allowed to do. Set once here, it is the same secret and one fewer reason for
// these tests to have to run one at a time.
//
// Defaults, not overrides: an environment that already defines them (docker compose does) is
// left alone, so this cannot mask a misconfigured one.
func TestMain(m *testing.M) {
	setIfAbsent("ENCRYPTION_KEY", testEncryptionKey)
	setIfAbsent("CPF_INDEX_KEY", testIndexKey)
	setIfAbsent("JWT_SECRET", testJWTSecret)
	os.Exit(m.Run())
}

func setIfAbsent(name, value string) {
	if os.Getenv(name) == "" {
		// Errors here are not recoverable and not interesting: a failing Setenv means the
		// process cannot hold environment at all, and every test below would say so anyway.
		_ = os.Setenv(name, value)
	}
}
