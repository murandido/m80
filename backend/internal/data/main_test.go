package data

import (
	"os"
	"testing"
)

// Test key material, used only when the environment does not already carry the real thing.
// ENCRYPTION_KEY is 64 hex characters (32 bytes), the shape utils.Encrypt demands.
const testEncryptionKey = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

// TestMain supplies the ENCRYPTION_KEY the account tests need: accounts.password and the two
// session/key columns are encrypted at rest in BeforeSave, so saving an account that carries one
// fails outright without it.
//
// The disposable test container sets it in the environment, which is why this was never needed
// before - and why its absence surfaced in CI rather than locally, on the first test here to save
// an encrypted field. An environment that already defines it is left alone, so this cannot mask a
// misconfigured one. The same guard exists in internal/azulsync and internal/smilessync.
func TestMain(m *testing.M) {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		_ = os.Setenv("ENCRYPTION_KEY", testEncryptionKey)
	}
	os.Exit(m.Run())
}
