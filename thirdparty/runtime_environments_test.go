package thirdparty

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetImageNames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetImageNames")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	result, err := c.getImageNames(ctx)
	assert.NoError(err)
	assert.NotEmpty(result)
	assert.NotContains(result, "")
}

func TestGetPackages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetPackages")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	manager := "pip"
	ami := "ami-0e12ef25a5f7712a4"
	opts1 := PackageFilterOptions{
		Page:    0,
		Limit:   10,
		Manager: manager,
	}
	result1, err := c.getPackages(ctx, ami, opts1)
	require.NoError(t, err)
	assert.NotEmpty(result1)
	assert.Len(result1, 10)
	require.NotEqual(t, result1[0].Name, "")
	assert.NotEqual(result1[0].Version, "")
	assert.Contains(result1[0].Manager, manager)
	opts2 := PackageFilterOptions{
		Page:    0,
		Limit:   5,
		Name:    result1[0].Name,
		Manager: manager,
	}
	result2, err := c.getPackages(ctx, ami, opts2)
	require.NoError(t, err)
	assert.NotEmpty(result2)
	assert.Equal(result2[0].Name, result2[0].Name)
	assert.Contains(result2[0].Manager, manager)
}
