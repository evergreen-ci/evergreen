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
	result, err := c.GetImageNames(ctx)
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
	result, err := c.GetOSInfo(ctx, "ami-0e12ef25a5f7712a4", 0, 10)
	require.NoError(t, err)
	assert.Len(result, 10)
}

func TestGetPackages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetPackages")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that we filter correctly by limit and manager.
	manager := "pip"
	ami := "ami-0e12ef25a5f7712a4"
	limit := 10
	opts := PackageFilterOptions{
		AMI:     ami,
		Page:    0,
		Limit:   limit,
		Manager: manager,
	}
	result, err := c.getPackages(ctx, opts)
	require.NoError(t, err)
	require.Len(t, result, limit)
	for i := 0; i < limit; i++ {
		assert.Contains(result[i].Manager, manager)
	}

	// Verify that we filter correctly by both manager and name.
	name := "Automat"
	opts = PackageFilterOptions{
		AMI:     ami,
		Page:    0,
		Limit:   5,
		Name:    "Automat",
		Manager: manager,
	}
	result, err = c.getPackages(ctx, opts)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(result[0].Name, name)
	assert.Contains(result[0].Manager, manager)

	// Verify that there are no results for fake package name.
	opts = PackageFilterOptions{
		AMI:  ami,
		Name: "blahblahblah",
	}
	result, err = c.getPackages(ctx, opts)
	require.NoError(t, err)
	assert.Empty(result)

	// Verify that there are no errors with PackageFilterOptions only including the AMI.
	_, err = c.getPackages(ctx, PackageFilterOptions{AMI: ami})
	require.NoError(t, err)

	// Verify that there is an error with no AMI provided.
	_, err = c.getPackages(ctx, PackageFilterOptions{})
	require.Error(t, err)
}

func TestGetImageDiff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetImageDiff")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that getImageDiff correctly returns Toolchain/Package changes for a pair of sample AMIs.
	opts := ImageDiffOptions{
		BeforeAMI: "ami-029ab576546a58916",
		AfterAMI:  "ami-02b25f680ad574d33",
	}
	result, err := c.getImageDiff(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(result)
	for _, change := range result {
		assert.True(change.Type == "Toolchains" || change.Type == "Packages")
	}

	// Verify that getImageDiff finds no differences between the same AMI.
	opts = ImageDiffOptions{
		BeforeAMI: "ami-016662ab459a49e9d",
		AfterAMI:  "ami-016662ab459a49e9d",
	}
	result, err = c.getImageDiff(ctx, opts)
	require.NoError(t, err)
	assert.Empty(result)
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
	require.Len(t, result, 1)
	assert.Equal(result[0].Name, name)
	assert.Equal(result[0].Version, version)

	// Verify that we receive no results for a fake toolchain.
	opts = ToolchainFilterOptions{
		AMI:   ami,
		Page:  0,
		Limit: 5,
		Name:  "blahblahblah",
	}
	result, err = c.getToolchains(ctx, opts)
	require.NoError(t, err)
	assert.Empty(result)

	// Verify that we receive an error when an AMI is not provided.
	_, err = c.getToolchains(ctx, ToolchainFilterOptions{})
	require.Error(t, err)
}

func TestGetHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetHistory")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that getHistory errors when not provided the required distro field.
	_, err := c.GetHistory(ctx, DistroHistoryFilterOptions{})
	assert.Error(err)

	// Verify that getHistory provides images for a distribution.
	result, err := c.GetHistory(ctx, DistroHistoryFilterOptions{Distro: "ubuntu2204"})
	require.NoError(t, err)
	assert.NotEmpty(t, result)

	// Verify that getHistory functions correctly with page and limit.
	opts := DistroHistoryFilterOptions{
		Distro: "ubuntu2204",
		Page:   0,
		Limit:  15,
	}
	result, err = c.GetHistory(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Len(result, 15)
}
