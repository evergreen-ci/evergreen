package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateHMACHash(t *testing.T) {
	assert := assert.New(t)
	body := []byte("Four score and seven bits ago")
	secret := []byte("i have the best beard")

	text, err := CalculateHMACHash(secret, body)
	assert.NoError(err)
	assert.Equal("sha256=d9d154a6958468d66ee12eec0fe7f9bc1c3dd6e1aa65851c66dd8be33f6ab1ae", text)
}
