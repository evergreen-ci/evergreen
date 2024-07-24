package graphql

import (
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPackages")
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	manager := "pip"
	image := thirdparty.Image{
		ID: "ubuntu2204",
	}
	opts := PackageOpts{
		Manager: &manager,
	}
	res, err := config.Resolvers.Image().Packages(ctx, &image, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, res)
}
