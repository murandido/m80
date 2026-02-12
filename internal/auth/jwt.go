package auth

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte(os.Getenv("JWT_SECRET"))

func GenerateToken(userID, phone, role string) (string, error) {
	if len(jwtSecret) == 0 {
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
	return token.SignedString(jwtSecret)
}
