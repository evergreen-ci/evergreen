package util

import (
	"crypto/rand"
	"encoding/hex"
)

func RandomString() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
