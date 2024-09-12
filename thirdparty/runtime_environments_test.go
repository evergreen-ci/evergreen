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
	testutil.ConfigureIntegrationTest(t, config, t.Name())
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
	testutil.ConfigureIntegrationTest(t, config, t.Name())
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that providing no AMI produces an error.
	_, err := c.GetOSInfo(ctx, OSInfoFilterOptions{})
	assert.Error(err)

	// Verify that we can get OS data for a given AMI.
	ami := "ami-0e12ef25a5f7712a4"
	opts := OSInfoFilterOptions{
		AMI: ami,
	}
	result, err := c.GetOSInfo(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(result.Data)
	assert.Equal(18, result.FilteredCount)
	assert.Equal(18, result.TotalCount)

	// Verify that we can get OS data with limit and page.
	opts = OSInfoFilterOptions{
		AMI:   ami,
		Page:  0,
		Limit: 10,
	}
	result, err = c.GetOSInfo(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(result.Data, 10)
	assert.Equal(18, result.FilteredCount)
	assert.Equal(18, result.TotalCount)

	// Verify that we filter correctly by name.
	opts = OSInfoFilterOptions{
		AMI:  ami,
		Name: "Kernel",
	}
	result, err = c.GetOSInfo(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(result.Data, 10)
	assert.Equal(1, result.FilteredCount)
	assert.Equal(18, result.TotalCount)

	// Verify that there are no results for nonexistent OS field.
	opts = OSInfoFilterOptions{
		AMI:  ami,
		Name: "blahblahblah",
	}
	result, err = c.GetOSInfo(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(result.Data)
	assert.Equal(0, result.FilteredCount)
	assert.Equal(18, result.TotalCount)
}

func TestGetPackages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, t.Name())
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that we can get package data with limit and page.
	ami := "ami-0e12ef25a5f7712a4"
	opts := PackageFilterOptions{
		AMI:   ami,
		Page:  0,
		Limit: 10,
	}
	result, err := c.GetPackages(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Data, 10)
	assert.Equal(1538, result.FilteredCount)
	assert.Equal(1538, result.TotalCount)

	// Verify that we filter correctly by name.
	opts = PackageFilterOptions{
		AMI:   ami,
		Page:  0,
		Limit: 5,
		Name:  "Automat",
	}
	result, err = c.GetPackages(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Data, 1)
	assert.Equal("Automat", result.Data[0].Name)
	assert.Equal(1, result.FilteredCount)
	assert.Equal(1538, result.TotalCount)

	// Verify that there are no results for a nonexistent package.
	opts = PackageFilterOptions{
		AMI:   ami,
		Page:  0,
		Limit: 5,
		Name:  "xyz",
	}
	result, err = c.GetPackages(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(result.Data)
	assert.Equal(0, result.FilteredCount)
	assert.Equal(1538, result.TotalCount)

	// Verify that there are no errors with PackageFilterOptions only including the AMI.
	_, err = c.GetPackages(ctx, PackageFilterOptions{AMI: ami})
	require.NoError(t, err)

	// Verify that there is an error with no AMI provided.
	_, err = c.GetPackages(ctx, PackageFilterOptions{})
	require.Error(t, err)
}

func TestGetToolchains(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, t.Name())
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that we can get toolchain data with limit and page.
	ami := "ami-016662ab459a49e9d"
	opts := ToolchainFilterOptions{
		AMI:   ami,
		Limit: 10,
	}
	result, err := c.GetToolchains(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(result.Data, 10)
	assert.Equal(41, result.FilteredCount)
	assert.Equal(41, result.TotalCount)

	// Verify that we filter correctly by name.
	opts = ToolchainFilterOptions{
		AMI:   ami,
		Page:  0,
		Limit: 5,
		Name:  "nodejs",
	}
	result, err = c.GetToolchains(ctx, opts)
	require.NoError(t, err)
	require.NotEmpty(t, result)
	require.NotNil(t, result)
	require.Len(t, result.Data, 3)
	assert.Equal("nodejs", result.Data[0].Name)
	assert.Equal("nodejs", result.Data[1].Name)
	assert.Equal("nodejs", result.Data[2].Name)
	assert.Equal(3, result.FilteredCount)
	assert.Equal(41, result.TotalCount)

	// Verify that we receive no results for a nonexistent toolchain.
	opts = ToolchainFilterOptions{
		AMI:   ami,
		Page:  0,
		Limit: 5,
		Name:  "xyz",
	}
	result, err = c.GetToolchains(ctx, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(result.Data)
	assert.Equal(0, result.FilteredCount)
	assert.Equal(41, result.TotalCount)

	// Verify that we receive an error when an AMI is not provided.
	_, err = c.GetToolchains(ctx, ToolchainFilterOptions{})
	require.Error(t, err)
}

func TestGetImageDiff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, t.Name())
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that getImageDiff correctly returns Toolchain/Package changes for a pair of sample AMIs.
	opts := diffFilterOptions{
		AMIBefore: "ami-029ab576546a58916",
		AMIAfter:  "ami-02b25f680ad574d33",
	}
	result, err := c.getImageDiff(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(result)
	for _, change := range result {
		assert.True(change.Type == APITypeOS || change.Type == APITypePackages || change.Type == APITypeToolchains)
	}

	// Verify that getImageDiff finds no differences between the same AMI.
	opts = diffFilterOptions{
		AMIBefore: "ami-016662ab459a49e9d",
		AMIAfter:  "ami-016662ab459a49e9d",
	}
	result, err = c.getImageDiff(ctx, opts)
	require.NoError(t, err)
	assert.Empty(result)
}

func TestGetHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, t.Name())
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that getHistory errors when not provided the required imageid field.
	opts := historyFilterOptions{}
	_, err := c.getHistory(ctx, opts)
	assert.Error(err)

	// Verify that getHistory provides images for a distribution.
	opts = historyFilterOptions{ImageID: "ubuntu2204"}
	result, err := c.getHistory(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, result)

	// Verify that getHistory functions correctly with page and limit.
	opts = historyFilterOptions{
		ImageID: "ubuntu2204",
		Page:    0,
		Limit:   15,
	}
	result, err = c.getHistory(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(result)
	assert.Len(result, 15)
}

func TestGetEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()

	testutil.ConfigureIntegrationTest(t, config, t.Name())
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that GetEvents errors when not provided the required distro field.
	opts := EventHistoryOptions{}
	_, err := c.GetEvents(ctx, opts)
	assert.Error(err)

	// Verify that GetEvents errors with missing limit.
	opts = EventHistoryOptions{Image: "ubuntu2204"}
	_, err = c.GetEvents(ctx, opts)
	assert.Error(err)

	// Verify that GetEvents functions correctly with page and limit and returns in chronological order.
	opts = EventHistoryOptions{
		Image: "ubuntu2204",
		Page:  0,
		Limit: 5,
	}
	result, err := c.GetEvents(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(result)
	assert.Len(result, 5)

	// Verify that timestamps are in chronological order.
	for i := 0; i < len(result)-1; i++ {
		assert.Greater(result[i].Timestamp, result[i+1].Timestamp)
	}

	// Verify that entries have been correctly transformed to use our custom enums.
	for _, r := range result {
		for _, entry := range r.Entries {
			assert.True(entry.Action == ImageEventEntryActionAdded || entry.Action == ImageEventEntryActionDeleted || entry.Action == ImageEventEntryActionUpdated)
			assert.True(entry.Type == ImageEventTypeOperatingSystem || entry.Type == ImageEventTypePackage || entry.Type == ImageEventTypeToolchain)
		}
	}
}

func TestBuildImageEventEntry(t *testing.T) {
	diff := ImageDiffChange{
		Type:          APITypeOS,
		BeforeVersion: "",
		AfterVersion:  "1.0",
	}
	result, err := buildImageEventEntry(diff)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ImageEventEntryActionAdded, result.Action)
	assert.Equal(t, ImageEventTypeOperatingSystem, result.Type)

	diff = ImageDiffChange{
		Type:          APITypePackages,
		BeforeVersion: "1.0",
		AfterVersion:  "",
	}
	result, err = buildImageEventEntry(diff)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ImageEventEntryActionDeleted, result.Action)
	assert.Equal(t, ImageEventTypePackage, result.Type)

	diff = ImageDiffChange{
		Type:          APITypeToolchains,
		BeforeVersion: "1.0",
		AfterVersion:  "2.0",
	}
	result, err = buildImageEventEntry(diff)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ImageEventEntryActionUpdated, result.Action)
	assert.Equal(t, ImageEventTypeToolchain, result.Type)
}

func TestGetImageInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, t.Name())
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	result, err := c.GetImageInfo(ctx, "ubuntu2204")
	require.NoError(t, err)
	require.NotEmpty(t, result)
	assert.NotEmpty(result.LastDeployed)
	assert.NotEmpty(result.AMI)
}
