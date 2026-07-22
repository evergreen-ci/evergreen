package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellQuote(t *testing.T) {
	for name, tc := range map[string]struct {
		input    string
		expected string
	}{
		"PlainString":         {input: "my-task", expected: `'my-task'`},
		"StringWithSpaces":    {input: "my task", expected: `'my task'`},
		"EmptyString":         {input: "", expected: `''`},
		"DoubleQuote":         {input: `a"b`, expected: `'a"b'`},
		"SingleQuote":         {input: "a'b", expected: `'a'\''b'`},
		"CommandSubstitution": {input: "$(cmd)", expected: `'$(cmd)'`},
		"Backtick":            {input: "`cmd`", expected: "'`cmd`'"},
		"Newline":             {input: "a\nb", expected: "'a\nb'"},
		"Backslash":           {input: `a\b`, expected: `'a\b'`},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, ShellQuote(tc.input))
		})
	}
}
