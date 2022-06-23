package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeReservedChars(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("asdf1234", EscapeJQLReservedChars("asdf1234"))
	assert.Equal(`q\\+h\\^`, EscapeJQLReservedChars("q+h^"))
	assert.Equal(`\\{\\}\\[\\]\\(\\)`, EscapeJQLReservedChars("{}[]()"))
	assert.Equal("", EscapeJQLReservedChars(""))
	assert.Equal(`\\+\\+\\+\\+`, EscapeJQLReservedChars("++++"))
}

func TestCoalesce(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("one", CoalesceString("", "one", "two"))
	assert.Equal("one", CoalesceStrings([]string{"", ""}, "", "one", "two"))
	assert.Equal("a", CoalesceStrings([]string{"", "a"}, "", "one", "two"))
	assert.Equal("one", CoalesceStrings(nil, "", "one", "two"))
	assert.Equal("", CoalesceStrings(nil, "", ""))
}
