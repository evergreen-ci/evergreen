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

func TestGetImageDiff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetImageDiff")
	c := NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	opts1 := ImageDiffOptions{
		AMI:   "ami-029ab576546a58916",
		AMI2:  "ami-02b25f680ad574d33",
		Page:  0,
		Limit: 10,
	}
	result1, err := c.getImageDiff(ctx, opts1)
	require.NoError(t, err)
	assert.NotEmpty(result1)
	for _, change := range result1 {
		assert.Equal(change.Type, "Toolchains")
	}
	opts2 := ImageDiffOptions{
		AMI:   "ami-016662ab459a49e9d",
		AMI2:  "ami-0628416ca497d1b38",
		Page:  0,
		Limit: 10,
	}
	result2, err := c.getImageDiff(ctx, opts2)
	require.NoError(t, err)
	assert.NotEmpty(result2)
	for _, change := range result2 {
		assert.Equal(change.Type, "Packages")
	}
}
