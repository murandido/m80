package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
)

// A blind index: a deterministic, keyed digest of a sensitive value, stored in place of the
// value itself. It answers "is this the same document as that one?" - which is all the
// beneficiary matching needs - and nothing else.
//
// Two decisions worth spelling out, because both look like the obvious alternative:
//
//   - Not Encrypt(). AES-GCM here uses a random nonce on purpose, so the same CPF produces a
//     different ciphertext every time. That is right for a value you want to read back and
//     useless for one you want to look up or index.
//   - Not a plain SHA-256. The check digits leave on the order of 10^9 valid CPFs, so an
//     unkeyed hash of one is recovered by exhaustive search in minutes. The key is what makes
//     a leaked dump useless on its own, since it never lives in the database.
//
// The flip side: losing CPF_INDEX_KEY does not lose data, but it does orphan every
// fingerprint already stored - matching silently falls back to the name and the same person
// starts a second beneficiary row. Treat it like ENCRYPTION_KEY: it belongs in the backup.

func getCPFIndexKey() ([]byte, error) {
	keyHex := os.Getenv("CPF_INDEX_KEY")
	if keyHex == "" {
		return nil, errors.New("CPF_INDEX_KEY environment variable is not set")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, errors.New("CPF_INDEX_KEY must be a valid hex string")
	}
	if len(key) != 32 {
		return nil, errors.New("CPF_INDEX_KEY must decode to exactly 32 bytes")
	}
	return key, nil
}

// Fingerprint returns the blind index of value: hex-encoded HMAC-SHA256 under CPF_INDEX_KEY.
// Callers pass an already-normalized value (see NormalizeCPF) - "123.456.789-09" and
// "12345678909" are different inputs and would not match each other.
func Fingerprint(value string) (string, error) {
	key, err := getCPFIndexKey()
	if err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil)), nil
}
