package util

import (
	"crypto/rand"
	"encoding/hex"
	"io"

	"github.com/pkg/errors"
)

func RandomString() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", errors.Wrap(err, "could not generate random string")
	}
	return hex.EncodeToString(b), nil
}
