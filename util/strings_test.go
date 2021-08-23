package util

import (
	"sort"
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
	sort.Strings(difference)

	// GetSetDifference returns the elements in A that are not in B
	assert.Equal(3, len(difference))
	assert.Equal("four", difference[0])
	assert.Equal("one", difference[1])
	assert.Equal("three", difference[2])
}

func TestCoalesce(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("one", CoalesceString("", "one", "two"))
	assert.Equal("one", CoalesceStrings([]string{"", ""}, "", "one", "two"))
	assert.Equal("a", CoalesceStrings([]string{"", "a"}, "", "one", "two"))
	assert.Equal("one", CoalesceStrings(nil, "", "one", "two"))
	assert.Equal("", CoalesceStrings(nil, "", ""))
}

func TestStringContainsSliceRegex(t *testing.T) {
	domains := []string{".*.corp.mongodb.com", "https://something.mongodb.com"}
	assert.Equal(t, true, StringContainsSliceRegex(domains, "https://patch-analysis-ui.server-tig.staging.corp.mongodb.com"))
	assert.Equal(t, true, StringContainsSliceRegex(domains, "https://something.mongodb.com"))
	assert.Equal(t, false, StringContainsSliceRegex(domains, "corp.mongodb.com"))
	assert.Equal(t, false, StringContainsSliceRegex(domains, "https://something-else.mongodb.com"))
}
