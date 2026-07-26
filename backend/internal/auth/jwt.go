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

func GenerateToken(userID, phone, role string) (string, error) {
	secret := jwtSecret()
	if len(secret) == 0 {
		return "", errors.New("JWT secret not set in .env")
	}

	// the token expires in 7 days
	expirationTime := time.Now().Add(7 * 24 * time.Hour)

	claims := jwt.MapClaims{
		"user_id": userID,
		"phone":   phone,
		"role":    role,
		"exp":     expirationTime.Unix(),
		"iss":     "m-80-api",
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
