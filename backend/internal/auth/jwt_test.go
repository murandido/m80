package auth

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestMain(m *testing.M) {
	// jwtSecret is set at package init from os.Getenv — override it directly
	// for a deterministic test secret before any test runs.
	jwtSecret = []byte("test-secret-for-jwt-unit-tests!!")
	os.Exit(m.Run())
}

func TestGenerateAndValidateToken_Roundtrip(t *testing.T) {
	token, err := GenerateToken("user-123", "11999998888", "admin")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token string")
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims["user_id"] != "user-123" {
		t.Errorf("user_id: got %v, want user-123", claims["user_id"])
	}
	if claims["phone"] != "11999998888" {
		t.Errorf("phone: got %v, want 11999998888", claims["phone"])
	}
	if claims["role"] != "admin" {
		t.Errorf("role: got %v, want admin", claims["role"])
	}
	if claims["iss"] != "m-80-api" {
		t.Errorf("iss: got %v, want m-80-api", claims["iss"])
	}
}

func TestGenerateToken_ExpiresInSevenDays(t *testing.T) {
	token, err := GenerateToken("u1", "11911112222", "user")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims, _ := ValidateToken(token)
	expUnix := int64(claims["exp"].(float64))
	exp := time.Unix(expUnix, 0)

	sevenDays := time.Now().Add(7 * 24 * time.Hour)
	diff := sevenDays.Sub(exp)
	if diff < 0 {
		diff = -diff
	}
	if diff > 5*time.Second {
		t.Errorf("expiry is not ~7 days from now: %v", exp)
	}
}

func TestGenerateToken_EmptySecret(t *testing.T) {
	saved := jwtSecret
	jwtSecret = []byte{}
	defer func() { jwtSecret = saved }()

	_, err := GenerateToken("u1", "11911112222", "user")
	if err == nil {
		t.Error("expected error when JWT secret is empty")
	}
}

func TestValidateToken_TamperedToken(t *testing.T) {
	token, _ := GenerateToken("u1", "11911112222", "user")

	// Corrupt the signature (last segment).
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("unexpected token format")
	}
	parts[2] = parts[2] + "tampered"
	tampered := strings.Join(parts, ".")

	_, err := ValidateToken(tampered)
	if err == nil {
		t.Error("expected error for tampered token signature")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	claims := jwt.MapClaims{
		"user_id": "u1",
		"phone":   "11911112222",
		"role":    "user",
		"exp":     time.Now().Add(-1 * time.Hour).Unix(),
		"iss":     "m-80-api",
	}
	expired, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)

	_, err := ValidateToken(expired)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestValidateToken_WrongSigningMethod(t *testing.T) {
	// Build a token header that claims RS256 but is actually unsigned — just to
	// trigger the method-mismatch guard in ValidateToken.
	fakeToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1MSJ9.invalidsig"

	_, err := ValidateToken(fakeToken)
	if err == nil {
		t.Error("expected error for unexpected signing method")
	}
}

func TestValidateToken_MalformedToken(t *testing.T) {
	_, err := ValidateToken("not.a.jwt")
	if err == nil {
		t.Error("expected error for malformed token")
	}
}
