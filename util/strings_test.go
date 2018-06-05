package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeReservedChars(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("asdf1234", EscapeJQLReservedChars("asdf1234"))
	assert.Equal(`q\+h\^`, EscapeJQLReservedChars("q+h^"))
	assert.Equal(`\{\}\[\]\(\)`, EscapeJQLReservedChars("{}[]()"))
	assert.Equal("", EscapeJQLReservedChars(""))
	assert.Equal(`\+\+\+\+`, EscapeJQLReservedChars("++++"))
}
