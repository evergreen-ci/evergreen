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
	testutil.ConfigureIntegrationTest(t, config, "TestGetToolchains")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	ami := "ami-016662ab459a49e9d"
	opts := ToolchainFilterOptions{
		Page:  0,
		Limit: 10,
	}
	result, err := c.getToolchains(ctx, ami, opts)
	require.NoError(t, err)
	assert.Len(result, 10)
	name := "nodejs"
	version := "toolchain_version_v16.17.0"
	opts = ToolchainFilterOptions{
		Page:    0,
		Limit:   5,
		Name:    name,
		Version: version,
	}
	result, err = c.getToolchains(ctx, ami, opts)
	require.NoError(t, err)
	require.NotEmpty(t, result)
	assert.Equal(result[0].Name, name)
	assert.Equal(result[0].Version, version)
	assert.Len(result, 1)
	name = "blahblahblah"
	opts = ToolchainFilterOptions{
		Page:  0,
		Limit: 5,
		Name:  name,
	}
	result, err = c.getToolchains(ctx, ami, opts)
	require.NoError(t, err)
	assert.Empty(result)
}
