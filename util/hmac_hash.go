package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/pkg/errors"
)

// CalculateHMACHash calculates a sha256 HMAC has of the body with the given
// secret. The body must NOT be modified after calculating this hash.
// The string result will be prefixed with "sha256=", followed by the HMAC hash.
// When validating this hash, use a constant-time compare to avoid vulnerability
// to timing attacks.
func CalculateHMACHash(secret []byte, body []byte) (string, error) {
	// from genMAC in github.com/google/go-github/github/messages.go
	mac := hmac.New(sha256.New, secret)
	n, err := mac.Write(body)
	if n != len(body) {
		return "", errors.Errorf("Body length expected to be %d, but was %d", len(body), n)
	}
	if err != nil {
		return "", err
	}

	return "sha256=" + hex.EncodeToString(mac.Sum(nil)), nil
}
