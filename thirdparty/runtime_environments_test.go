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
	opts := PackageFilterOptions{
		Page:    0,
		Limit:   10,
		Manager: manager,
	}
	result, err := c.getPackages(ctx, ami, opts)
	require.NoError(t, err)
	assert.NotEmpty(result)
	assert.Len(result, 10)
	assert.Contains(result[0].Manager, manager)
	name := "Automat"
	opts = PackageFilterOptions{
		Page:    0,
		Limit:   5,
		Name:    name,
		Manager: manager,
	}
	result, err = c.getPackages(ctx, ami, opts)
	require.NoError(t, err)
	require.NotEmpty(t, result)
	assert.Equal(result[0].Name, name)
	assert.Contains(result[0].Manager, manager)
	assert.Len(result, 1)
}
