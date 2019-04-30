package queue

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// RandomString returns a cryptographically random string.
func randomString(x int) string {
	b := make([]byte, x)
	_, _ = rand.Read(b) // nolint
	return hex.EncodeToString(b)
}

func addJobsSuffix(s string) string {
	return s + ".jobs"
}

func trimJobsSuffix(s string) string {
	return strings.TrimSuffix(s, ".jobs")
}

func addGroupSufix(s string) string {
	return s + ".group"
}
