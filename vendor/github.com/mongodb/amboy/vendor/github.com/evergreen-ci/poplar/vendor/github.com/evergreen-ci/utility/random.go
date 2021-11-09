package utility

import (
	"crypto/rand"
	"encoding/hex"
)

// MakeRandomString constructs a hex-encoded random string of a
// specific length. The size reflects the number of random bytes, not
// the length of the string.
func MakeRandomString(size int) string {
	b := make([]byte, size)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)

}

// RandomString returns a hex-encoded cryptographically random string.
// This function always returns 16bytes of randomness encoded in a 32
// character string.
func RandomString() string { return MakeRandomString(16) }
