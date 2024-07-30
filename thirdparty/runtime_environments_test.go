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

	// Verify that providing no AMI produces an error.
	_, err := c.GetOSInfo(ctx, OSInfoFilterOptions{})
	assert.Error(err)

	// Verify that we correctly filter only providing the AMI.
	opts := OSInfoFilterOptions{
		AMI: "ami-0e12ef25a5f7712a4",
	}
	result, err := c.GetOSInfo(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(result)

	// Verify that we correctly filter by AMI and limit.
	opts = OSInfoFilterOptions{
		AMI:   "ami-0e12ef25a5f7712a4",
		Page:  0,
		Limit: 10,
	}
	result, err = c.GetOSInfo(ctx, opts)
	require.NoError(t, err)
	assert.Len(result, 10)

	// Verify that we correctly filter by AMI and name.
	opts = OSInfoFilterOptions{
		AMI:  "ami-0f6b89500372d4a06",
		Name: "Kernel",
	}
	result, err = c.GetOSInfo(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(result)
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
	result, err := c.GetPackages(ctx, opts)
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
	result, err = c.GetPackages(ctx, opts)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(result[0].Name, name)
	assert.Contains(result[0].Manager, manager)

	// Verify that there are no results for fake package name.
	opts = PackageFilterOptions{
		AMI:  ami,
		Name: "blahblahblah",
	}
	result, err = c.GetPackages(ctx, opts)
	require.NoError(t, err)
	assert.Empty(result)

	// Verify that there are no errors with PackageFilterOptions only including the AMI.
	_, err = c.GetPackages(ctx, PackageFilterOptions{AMI: ami})
	require.NoError(t, err)

	// Verify that there is an error with no AMI provided.
	_, err = c.GetPackages(ctx, PackageFilterOptions{})
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
		AMIBefore: "ami-029ab576546a58916",
		AMIAfter:  "ami-02b25f680ad574d33",
	}
	result, err := c.getImageDiff(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(result)
	for _, change := range result {
		assert.True(change.Type == "Toolchains" || change.Type == "Packages")
	}

	// Verify that getImageDiff finds no differences between the same AMI.
	opts = ImageDiffOptions{
		AMIBefore: "ami-016662ab459a49e9d",
		AMIAfter:  "ami-016662ab459a49e9d",
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
	result, err := c.GetToolchains(ctx, opts)
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
	result, err = c.GetToolchains(ctx, opts)
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
	result, err = c.GetToolchains(ctx, opts)
	require.NoError(t, err)
	assert.Empty(result)

	// Verify that we receive an error when an AMI is not provided.
	_, err = c.GetToolchains(ctx, ToolchainFilterOptions{})
	require.Error(t, err)
}

func TestGetHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetHistory")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that getHistory errors when not provided the required imageid field.
	_, err := c.getHistory(ctx, ImageHistoryFilterOptions{})
	assert.Error(err)

	// Verify that getHistory provides images for a distribution.
	result, err := c.getHistory(ctx, ImageHistoryFilterOptions{ImageID: "ubuntu2204"})
	require.NoError(t, err)
	assert.NotEmpty(t, result)

	// Verify that getHistory functions correctly with page and limit.
	opts := ImageHistoryFilterOptions{
		ImageID: "ubuntu2204",
		Page:    0,
		Limit:   15,
	}
	result, err = c.getHistory(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Len(result, 15)
}

func TestGetImageInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetImageInfo")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	result, err := c.GetImageInfo(ctx, "ubuntu2204")
	require.NoError(t, err)
	require.NotEmpty(t, result)
	assert.NotEmpty(result.Name)
	assert.NotEmpty(result.VersionID)
	assert.NotEmpty(result.Kernel)
	assert.NotEmpty(result.LastDeployed)
	assert.NotEmpty(result.AMI)
}

func TestGetEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()

	testutil.ConfigureIntegrationTest(t, config, "TestGetEvents")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that GetEvents errors when not provided the required distro field.
	_, err := c.GetEvents(ctx, EventHistoryOptions{})
	assert.Error(err)

	// Verify that GetEvents errors with missing limit.
	_, err = c.GetEvents(ctx, EventHistoryOptions{Image: "ubuntu2204"})
	assert.Error(err)

	// Verify that GetEvents functions correctly with page and limit and returns in chronological order.
	opts := EventHistoryOptions{
		Image: "ubuntu2204",
		Page:  0,
		Limit: 5,
	}
	result, err := c.GetEvents(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Len(result, 5)
	for i := 0; i < len(result)-1; i++ {
		assert.Greater(result[i].Timestamp, result[i+1].Timestamp)
	}
}
