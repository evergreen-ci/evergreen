package evergreen

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAdminDuration(t *testing.T) {
	for name, test := range map[string]struct {
		input    string
		expected time.Duration
	}{
		"Seconds":  {input: "30s", expected: 30 * time.Second},
		"Minutes":  {input: "5m", expected: 5 * time.Minute},
		"Days":     {input: "5d", expected: 5 * 24 * time.Hour},
		"Weeks":    {input: "1w", expected: 7 * 24 * time.Hour},
		"Compound": {input: "1w2d3h4m5s", expected: 9*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second},
		"Negative": {input: "-1d2h", expected: -(26 * time.Hour)},
		"Zero":     {input: "0s", expected: 0},
	} {
		t.Run(name, func(t *testing.T) {
			actual, err := ParseAdminDuration(test.input)
			require.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}

	t.Run("InvalidInputShouldError", func(t *testing.T) {
		_, err := ParseAdminDuration("one day")
		require.Error(t, err)
	})
}

func TestFormatAdminDuration(t *testing.T) {
	for name, test := range map[string]struct {
		input    time.Duration
		expected string
	}{
		"Zero":     {input: 0, expected: "0s"},
		"Subday":   {input: 5 * time.Minute, expected: "5m"},
		"Days":     {input: 5 * 24 * time.Hour, expected: "5d"},
		"Compound": {input: 5*24*time.Hour + 3*time.Hour + 2*time.Minute, expected: "5d3h2m"},
		"Negative": {input: -(2 * 24 * time.Hour), expected: "-2d"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, FormatAdminDuration(test.input))
		})
	}
}
