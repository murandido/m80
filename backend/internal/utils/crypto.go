package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
)

const encryptedPrefix = "enc:"

// getEncryptionKey reads ENCRYPTION_KEY from the environment and returns the raw 32-byte key.
// The env var must be a 64-character hex string (generated with: openssl rand -hex 32).
func getEncryptionKey() ([]byte, error) {
	keyHex := os.Getenv("ENCRYPTION_KEY")
	if keyHex == "" {
		return nil, errors.New("ENCRYPTION_KEY environment variable is not set")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, errors.New("ENCRYPTION_KEY must be a valid hex string")
	}
	if len(key) != 32 {
		return nil, errors.New("ENCRYPTION_KEY must decode to exactly 32 bytes (AES-256)")
	}
	return key, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a prefixed, base64-encoded ciphertext.
// Format: "enc:<base64(nonce[12] + ciphertext + tag[16])>"
func Encrypt(plaintext string) (string, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Seal appends ciphertext + tag to nonce
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encryptedPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts a value produced by Encrypt.
// Values without the "enc:" prefix are returned as-is (legacy plaintext passthrough).
func Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, encryptedPrefix) {
		return ciphertext, nil
	}

	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, encryptedPrefix))
	if err != nil {
		return "", errors.New("crypto: failed to base64-decode ciphertext")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("crypto: ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", errors.New("crypto: decryption failed — wrong key or tampered data")
	}

	return string(plaintext), nil
}
