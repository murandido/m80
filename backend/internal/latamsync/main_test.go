package latamsync

import (
	"os"
	"testing"
)

// Test key material, used only when the environment does not already carry the real thing.
// ENCRYPTION_KEY is 64 hex characters (32 bytes), the shape utils.Encrypt demands.
const testEncryptionKey = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

// TestMain supplies the ENCRYPTION_KEY the seeds here need: an account row carries the LATAM
// refresh token, which the model encrypts at rest in BeforeSave, so a seed cannot be written
// without the key. The disposable test container sets it in the environment, but `go test ./...`
// on a bare machine (and CI) does not, so it is set here as a default - an environment that
// already defines it is left alone, so this cannot mask a misconfigured one.
func TestMain(m *testing.M) {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		_ = os.Setenv("ENCRYPTION_KEY", testEncryptionKey)
	}
	os.Exit(m.Run())
}
