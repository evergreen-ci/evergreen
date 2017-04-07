package util

import (
	"crypto/rand"
	"encoding/hex"
)

// RandomString returns a cryptographically random string.
func RandomString() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
