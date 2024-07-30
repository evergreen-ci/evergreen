package graphql

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
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
	ami := "ami-0f6b89500372d4a06"
	image := model.APIImage{
		AMI: &ami,
	}
	opts := thirdparty.PackageFilterOptions{
		AMI:     ami,
		Manager: manager,
		Name:    testPackage,
	}
	res, err := config.Resolvers.Image().Packages(ctx, &image, opts)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.NotNil(t, res[0])
	assert.Equal(t, testPackage, utility.FromStringPtr(res[0].Name))
}

func TestToolchains(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestToolchains")
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	testToolchain := "golang"
	ami := "ami-0f6b89500372d4a06"
	image := model.APIImage{
		AMI: &ami,
	}
	opts := thirdparty.ToolchainFilterOptions{
		AMI:   ami,
		Limit: 5,
	}
	res, err := config.Resolvers.Image().Toolchains(ctx, &image, opts)
	require.NoError(t, err)
	require.Len(t, res, 5)
	require.NotNil(t, res[0])
	assert.Equal(t, testToolchain, utility.FromStringPtr(res[0].Name))
}

func TestEvents(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestEvents")
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))

	// Returns the correct number of events according to the limit.
	imageID := "amazon2"
	image1 := model.APIImage{
		ID: &imageID,
	}
	res, err := config.Resolvers.Image().Events(ctx, &image1, 5, 0)
	require.NoError(t, err)
	assert.Len(t, res, 5)

	// Errors when nil image provided.
	image2 := model.APIImage{}
	_, err = config.Resolvers.Image().Events(ctx, &image2, 5, 0)
	grip.Debug(err)
	require.Error(t, err)
}

func TestDistros(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(distro.Collection), "unable to clear distro collection")
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestDistros")
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	d1 := &distro.Distro{
		Id:      "ubuntu1604-large",
		ImageID: "ubuntu1604",
	}
	require.NoError(t, d1.Insert(ctx))
	d2 := &distro.Distro{
		Id:      "ubuntu1604-small",
		ImageID: "ubuntu1604",
	}
	require.NoError(t, d2.Insert(ctx))
	d3 := &distro.Distro{
		Id:      "rhel82-small",
		ImageID: "rhel82",
	}
	require.NoError(t, d3.Insert(ctx))
	imageID := "ubuntu1604"
	image := model.APIImage{
		ID: &imageID,
	}
	res, err := config.Resolvers.Image().Distros(ctx, &image)
	require.NoError(t, err)
	require.Len(t, res, 2)
	distroNames := []string{*res[0].Name, utility.FromStringPtr(res[1].Name)}
	assert.Contains(t, distroNames, "ubuntu1604-small")
	assert.Contains(t, distroNames, "ubuntu1604-large")
}

func TestLatestTask(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(distro.Collection, task.Collection),
		"unable to clear distro and task collections")
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestLatestTask")
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	d1 := &distro.Distro{
		Id:      "ubuntu1604-large",
		ImageID: "ubuntu1604",
	}
	require.NoError(t, d1.Insert(ctx))
	d2 := &distro.Distro{
		Id:      "ubuntu1604-small",
		ImageID: "ubuntu1604",
	}
	require.NoError(t, d2.Insert(ctx))
	taskA := &task.Task{
		Id:         "task_a",
		DistroId:   "ubuntu1604-small",
		FinishTime: time.Date(2023, time.February, 1, 10, 30, 15, 0, time.UTC),
	}
	require.NoError(t, taskA.Insert())
	taskB := &task.Task{
		Id:         "task_b",
		DistroId:   "ubuntu1604-large",
		FinishTime: time.Date(2023, time.March, 1, 10, 30, 15, 0, time.UTC),
	}
	require.NoError(t, taskB.Insert())
	taskC := &task.Task{
		Id:         "task_c",
		DistroId:   "ubuntu2204-large",
		FinishTime: time.Date(2023, time.April, 1, 10, 30, 15, 0, time.UTC),
	}
	require.NoError(t, taskC.Insert())
	imageID := "ubuntu1604"
	image := model.APIImage{
		ID: &imageID,
	}
	res, err := config.Resolvers.Image().LatestTask(ctx, &image)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, utility.FromStringPtr(res.Id), "task_b")
}
