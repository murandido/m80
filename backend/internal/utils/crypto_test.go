package utils

import (
	"encoding/base64"
	"strings"
	"testing"
)

// validTestKey is a 32-byte AES-256 key expressed as a 64-char hex string.
const validTestKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", validTestKey)

	plaintext := "secret-password-123"
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if !strings.HasPrefix(encrypted, "enc:") {
		t.Errorf("expected enc: prefix, got %q", encrypted)
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("roundtrip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncrypt_ProducesUniqueNonces(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", validTestKey)

	plaintext := "same-input"
	first, _ := Encrypt(plaintext)
	second, _ := Encrypt(plaintext)
	if first == second {
		t.Error("two Encrypt calls on the same plaintext should produce different ciphertexts")
	}
}

func TestDecrypt_LegacyPlaintextPassthrough(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", validTestKey)

	legacy := "plain-text-no-prefix"
	result, err := Decrypt(legacy)
	if err != nil {
		t.Fatalf("unexpected error for legacy plaintext: %v", err)
	}
	if result != legacy {
		t.Errorf("got %q, want %q", result, legacy)
	}
}

func TestEncrypt_MissingKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "")

	_, err := Encrypt("anything")
	if err == nil {
		t.Error("expected error when ENCRYPTION_KEY is not set")
	}
}

func TestDecrypt_MissingKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "")

	_, err := Decrypt("enc:anythinghere")
	if err == nil {
		t.Error("expected error when ENCRYPTION_KEY is not set")
	}
}

func TestEncrypt_InvalidKeyHex(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "not-valid-hex!")

	_, err := Encrypt("anything")
	if err == nil {
		t.Error("expected error for non-hex ENCRYPTION_KEY")
	}
}

func TestEncrypt_WrongKeySize(t *testing.T) {
	// 16 bytes = AES-128 key expressed as 32 hex chars — not 32 bytes
	t.Setenv("ENCRYPTION_KEY", "0102030405060708090a0b0c0d0e0f10")

	_, err := Encrypt("anything")
	if err == nil {
		t.Error("expected error for key that is not exactly 32 bytes")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", validTestKey)

	encrypted, err := Encrypt("sensitive-data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Flip a byte in the middle of the base64 payload to simulate tampering.
	payload := strings.TrimPrefix(encrypted, "enc:")
	raw, _ := base64.StdEncoding.DecodeString(payload)
	raw[len(raw)/2] ^= 0xFF
	tampered := "enc:" + base64.StdEncoding.EncodeToString(raw)

	_, err = Decrypt(tampered)
	if err == nil {
		t.Error("expected authentication error for tampered ciphertext")
	}
}

func TestDecrypt_CiphertextTooShort(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", validTestKey)

	// Base64 of just 3 bytes — shorter than the 12-byte nonce.
	short := "enc:" + base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})
	_, err := Decrypt(short)
	if err == nil {
		t.Error("expected error for ciphertext shorter than nonce size")
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", validTestKey)

	_, err := Decrypt("enc:!!!not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64 in ciphertext")
	}
}

func TestEncryptDecrypt_EmptyString(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", validTestKey)

	encrypted, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt of empty string failed: %v", err)
	}
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt of empty string failed: %v", err)
	}
	if decrypted != "" {
		t.Errorf("got %q, want empty string", decrypted)
	}
}
