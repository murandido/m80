package utils

import (
	"strings"
	"testing"
)

const (
	testIndexKey      = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	otherTestIndexKey = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
)

func TestFingerprintIsDeterministic(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testIndexKey)

	first, err := Fingerprint("52998224725")
	if err != nil {
		t.Fatalf("Fingerprint returned an error: %v", err)
	}
	second, err := Fingerprint("52998224725")
	if err != nil {
		t.Fatalf("Fingerprint returned an error: %v", err)
	}

	// Unlike Encrypt, which is deliberately non-deterministic. This is the property the
	// unique index and the beneficiary match both depend on.
	if first != second {
		t.Fatalf("same input produced different fingerprints: %q and %q", first, second)
	}
	if len(first) != 64 {
		t.Errorf("fingerprint = %q, want 64 hex characters", first)
	}
	if strings.Contains(first, "52998224725") {
		t.Error("fingerprint leaked the value it was derived from")
	}
}

func TestFingerprintDiffersPerValueAndPerKey(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testIndexKey)
	one, _ := Fingerprint("52998224725")
	two, _ := Fingerprint("11144477735")
	if one == two {
		t.Fatal("different CPFs produced the same fingerprint")
	}

	// Rotating the key orphans the stored fingerprints - matching silently falls back to
	// the name. Asserted so the consequence is documented in a test, not only in a comment.
	t.Setenv("CPF_INDEX_KEY", otherTestIndexKey)
	underOtherKey, _ := Fingerprint("52998224725")
	if underOtherKey == one {
		t.Fatal("the key does not affect the fingerprint")
	}
}

func TestFingerprintRefusesABadKey(t *testing.T) {
	for _, tc := range []struct{ name, key string }{
		{"missing", ""},
		{"not hex", "not-a-hex-string"},
		{"wrong length", "0123456789abcdef"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CPF_INDEX_KEY", tc.key)
			if _, err := Fingerprint("52998224725"); err == nil {
				t.Error("expected an error, got none")
			}
		})
	}
}
