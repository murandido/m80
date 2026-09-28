package handlers

import (
	"encoding/base64"
	"strconv"
)

// base64URL and itoaTest build the unsigned test JWT in smiles_connect_test.go. They live here
// so that file reads as the contract it pins rather than as string plumbing.
func base64URL(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func itoaTest(value int64) string {
	return strconv.FormatInt(value, 10)
}
