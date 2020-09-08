package testutil

import (
	"crypto/rand"
	"encoding/hex"
)

// NewUUID generates a new 16-byte UUID.
func NewUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
