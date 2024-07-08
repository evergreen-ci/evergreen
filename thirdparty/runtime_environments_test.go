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

	// Verify that we filter correctly by limit and manager.
	manager := "pip"
	ami := "ami-0e12ef25a5f7712a4"
	limit := 10
	opts := PackageFilterOptions{
		AMIID:   ami,
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
		AMIID:   ami,
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
		AMIID: ami,
		Name:  "blahblahblah",
	}
	result, err = c.getPackages(ctx, opts)
	require.NoError(t, err)
	assert.Empty(result)

	// Verify that there are no errors with PackageFilterOptions only including the AMI.
	_, err = c.getPackages(ctx, PackageFilterOptions{AMIID: ami})
	require.NoError(t, err)

	// Verify that there is an error with no AMI provided.
	_, err = c.getPackages(ctx, PackageFilterOptions{})
	require.Error(t, err)
}
