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

func TestGetSetDifference(t *testing.T) {
	assert := assert.New(t)
	setA := []string{"one", "four", "five", "three", "two"}
	setB := []string{"five", "two"}
	difference := GetSetDifference(setA, setB)

	// GetSetDifference returns the elements in A that are not in B
	assert.Equal(3, len(difference))
	assert.Equal("one", difference[0])
	assert.Equal("four", difference[1])
	assert.Equal("three", difference[2])
}
