package parsley

import (
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeExistingParsleySettings(t *testing.T) {
	oldSettings := Settings{}
	newSettings := Settings{
		JumpToFailingLineEnabled: utility.FalsePtr(),
	}
	changes := MergeExistingParsleySettings(oldSettings, newSettings)
	require.NotNil(t, changes)
	assert.Nil(t, changes.SectionsEnabled)
	require.NotNil(t, changes.JumpToFailingLineEnabled)
	assert.Equal(t, utility.FromBoolPtr(changes.JumpToFailingLineEnabled), false)

	oldSettings = Settings{
		SectionsEnabled:          utility.FalsePtr(),
		JumpToFailingLineEnabled: utility.TruePtr(),
	}
	newSettings = Settings{
		JumpToFailingLineEnabled: utility.FalsePtr(),
	}
	changes = MergeExistingParsleySettings(oldSettings, newSettings)
	require.NotNil(t, changes)
	require.NotNil(t, changes.SectionsEnabled)
	assert.Equal(t, utility.FromBoolPtr(changes.SectionsEnabled), false)
	require.NotNil(t, changes.JumpToFailingLineEnabled)
	assert.Equal(t, utility.FromBoolPtr(changes.JumpToFailingLineEnabled), false)

	oldSettings = Settings{
		SectionsEnabled:          utility.TruePtr(),
		JumpToFailingLineEnabled: utility.TruePtr(),
	}
	newSettings = Settings{
		JumpToFailingLineEnabled: utility.FalsePtr(),
	}
	changes = MergeExistingParsleySettings(oldSettings, newSettings)
	require.NotNil(t, changes)
	require.NotNil(t, changes.SectionsEnabled)
	assert.Equal(t, utility.FromBoolPtr(changes.SectionsEnabled), true)
	require.NotNil(t, changes.JumpToFailingLineEnabled)
	assert.Equal(t, utility.FromBoolPtr(changes.JumpToFailingLineEnabled), false)
}

func TestValidateFilters(t *testing.T) {
	filters := []Filter{
		{
			Expression:    "",
			CaseSensitive: false,
			ExactMatch:    true,
		},
	}
	err := ValidateFilters(filters)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-empty")

	filters = []Filter{
		{
			Expression:    "*.invalidregex",
			CaseSensitive: false,
			ExactMatch:    true,
		},
		{
			Expression:    "validregex",
			CaseSensitive: false,
			ExactMatch:    true,
		},
	}
	err = ValidateFilters(filters)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regexp")

	filters = []Filter{
		{
			Expression:    "duplicate",
			CaseSensitive: false,
			ExactMatch:    true,
		},
		{
			Expression:    "duplicate",
			CaseSensitive: false,
			ExactMatch:    true,
		},
	}
	err = ValidateFilters(filters)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate filter")

	filters = []Filter{
		{
			Expression:    "same_expression",
			CaseSensitive: false,
			ExactMatch:    true,
		},
		{
			Expression:    "same_expression",
			CaseSensitive: true,
			ExactMatch:    true,
		},
	}
	err = ValidateFilters(filters)
	require.NoError(t, err)

	filters = []Filter{
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
	err = ValidateFilters(filters)
	require.NoError(t, err)

	filters = []Filter{}
	err = ValidateFilters(filters)
	require.NoError(t, err)
}
