package cloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanLaunchTemplateName(t *testing.T) {
	for name, params := range map[string]struct {
		input    string
		expected string
	}{
		"AlreadyClean": {
			input:    "abcdefghijklmnopqrstuvwxyz0123456789_()/-",
			expected: "abcdefghijklmnopqrstuvwxyz0123456789_()/-",
		},
		"IncludesInvalid": {
			input:    "abcdef*123456",
			expected: "abcdef123456",
		},
		"AllInvalid": {
			input:    "!@#$%^&*",
			expected: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, params.expected, cleanLaunchTemplateName(params.input))
		})
	}
}
