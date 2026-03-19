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

func TestAZToRegion(t *testing.T) {
	assert.Equal(t, "us-east-1", AZToRegion("us-east-1a"))
	assert.Equal(t, "us-west-2", AZToRegion("us-west-2b"))
	assert.Equal(t, "eu-west-1", AZToRegion("eu-west-1c"))
	assert.Equal(t, "", AZToRegion(""))
	assert.Equal(t, "", AZToRegion("a"))
}
