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

func TestGetHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetHistory")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)

	// Verify that getHistory errors when not provided the required distro field.
	_, err := c.getHistory(ctx, DistroHistoryFilterOptions{})
	require.Error(t, err)

	// Verify that getHistory provides images for a distribution.
	imageNames, err := c.getImageNames(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, imageNames)
	result, err := c.getHistory(ctx, DistroHistoryFilterOptions{Distro: imageNames[0]})
	require.NoError(t, err)
	assert.NotEmpty(t, result)

	// Verify that getHistory functions correctly with page and limit
	opts := DistroHistoryFilterOptions{
		Distro: imageNames[0],
		Page:   0,
		Limit:  20,
	}
	result, err = c.getHistory(ctx, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}
