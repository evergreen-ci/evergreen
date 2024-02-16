package parsley

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateParsleyFilters(t *testing.T) {
	parsleyFilters := []ParsleyFilter{
		{
			Expression:    "",
			CaseSensitive: false,
			ExactMatch:    true,
		},
	}
	err := ValidateParsleyFilters(parsleyFilters)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-empty")

	parsleyFilters = []ParsleyFilter{
		{
			Expression:    "*.badregex",
			CaseSensitive: false,
			ExactMatch:    true,
		},
	}
	err = ValidateParsleyFilters(parsleyFilters)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regexp")

	parsleyFilters = []ParsleyFilter{
		{
			Expression:    "duplicate",
			CaseSensitive: false,
			ExactMatch:    true,
		},
		{
			Expression:    "duplicate",
			CaseSensitive: true,
			ExactMatch:    true,
		},
	}
	err = ValidateParsleyFilters(parsleyFilters)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate filter expression")

	parsleyFilters = []ParsleyFilter{
		{
			Expression:    "^abc",
			CaseSensitive: false,
			ExactMatch:    true,
		},
		{
			Expression:    "def",
			CaseSensitive: false,
			ExactMatch:    false,
		},
	}
	err = ValidateParsleyFilters(parsleyFilters)
	require.NoError(t, err)
}
