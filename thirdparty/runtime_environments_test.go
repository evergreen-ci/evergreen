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

func TestGetOSInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetOSInfo")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	result, err := c.getOSInfo(ctx, "ami-0e12ef25a5f7712a4", 0, 10)
	require.NoError(t, err)
	assert.Len(result, 10)
}

func TestGetToolchains(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetToolchains")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that there are no errors with ToolchainFilterOptions including the AMI and limit.
	ami := "ami-016662ab459a49e9d"
	opts := ToolchainFilterOptions{
		AMI:   ami,
		Limit: 10,
	}
	result, err := c.getToolchains(ctx, opts)
	require.NoError(t, err)
	assert.Len(result, 10)

	// Verify that we filter correctly by name and version.
	name := "nodejs"
	version := "toolchain_version_v16.17.0"
	opts = ToolchainFilterOptions{
		AMI:     ami,
		Page:    0,
		Limit:   5,
		Name:    name,
		Version: version,
	}
	result, err = c.getToolchains(ctx, opts)
	require.NoError(t, err)
	require.NotEmpty(t, result)
	assert.Equal(result[0].Name, name)
	assert.Equal(result[0].Version, version)
	assert.Len(result, 1)

	// Verify that we receive no results for a fake toolchain
	name = "blahblahblah"
	opts = ToolchainFilterOptions{
		AMI:   ami,
		Page:  0,
		Limit: 5,
		Name:  name,
	}
	result, err = c.getToolchains(ctx, opts)
	require.NoError(t, err)
	assert.Empty(result)

	// Verify that we receive an error when an AMI is not provided.
	_, err = c.getToolchains(ctx, ToolchainFilterOptions{})
	require.Error(t, err)
}
