package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeReservedChars(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("asdf1234", EscapeJiraReservedChars("asdf1234"))
	assert.Equal(`q\+h\^`, EscapeJiraReservedChars("q+h^"))
	assert.Equal(`\{\}\[\]\(\)`, EscapeJiraReservedChars("{}[]()"))
	assert.Equal("", EscapeJiraReservedChars(""))
	assert.Equal(`\+\+\+\+`, EscapeJiraReservedChars("++++"))
}
