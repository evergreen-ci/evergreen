package graphql

import (
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestPackages(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestPackages")
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	manager := "pip"
	testPackage := "Automat"
	image := thirdparty.Image{
		AMI: "ami-0f6b89500372d4a06",
	}
	opts := PackageOpts{
		Manager: &manager,
		Name:    &testPackage,
	}
	res, err := config.Resolvers.Image().Packages(ctx, &image, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, res)
	assert.Equal(t, res[0].Name, testPackage)
}
