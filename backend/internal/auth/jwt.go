package auth

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtSecret reads the secret on every call instead of caching it at package
// init: an init-time read is fixed before any test (or any caller) can set the
// variable, which made the signing path impossible to exercise outside a fully
// configured environment. The process environment doesn't change at runtime, so
// this costs a lookup and changes nothing in production.
func jwtSecret() []byte {
	return []byte(os.Getenv("JWT_SECRET"))
}

// TokenLifetime is how long a panel session lasts without being used. It is not a hard
// cap on a session: POST /auth/refresh trades a valid token for a fresh one, so someone who
// keeps using the panel is never logged out, and only a week of inactivity ends the session.
// The panel's session cookie is given the same max age.
const TokenLifetime = 7 * 24 * time.Hour

func GenerateToken(userID, phone, role string) (string, error) {
	secret := jwtSecret()
	if len(secret) == 0 {
		return "", errors.New("JWT secret not set in .env")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": userID,
		"phone":   phone,
		"role":    role,
		// iat is informational - nothing validates against it. Tokens signed before it was
		// added are still accepted, and the panel decides when to renew from exp alone.
		"iat": now.Unix(),
		"exp": now.Add(TokenLifetime).Unix(),
		"iss": "m-80-api",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func ValidateToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return jwtSecret(), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
