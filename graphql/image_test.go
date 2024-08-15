package graphql

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
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
	ami := "ami-0f6b89500372d4a06"
	image := model.APIImage{
		AMI: &ami,
	}
	opts := thirdparty.PackageFilterOptions{
		AMI:  ami,
		Name: "python3-automat",
	}
	res, err := config.Resolvers.Image().Packages(ctx, &image, opts)
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	require.NotNil(t, res.Data[0])
	assert.Equal(t, utility.FromStringPtr(res.Data[0].Name), "python3-automat")
	assert.Equal(t, res.FilteredCount, 1)
	assert.Equal(t, res.TotalCount, 1618)
}

func TestToolchains(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestToolchains")
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	ami := "ami-0f6b89500372d4a06"
	image := model.APIImage{
		AMI: &ami,
	}
	opts := thirdparty.ToolchainFilterOptions{
		AMI:   ami,
		Name:  "golang",
		Limit: 1,
	}
	res, err := config.Resolvers.Image().Toolchains(ctx, &image, opts)
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	require.NotNil(t, res.Data[0])
	assert.Equal(t, utility.FromStringPtr(res.Data[0].Name), "golang")
	assert.Equal(t, res.FilteredCount, 33)
	assert.Equal(t, res.TotalCount, 49)
}

func TestEvents(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestEvents")
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))

	// Returns the correct number of events according to the limit.
	imageID := "ubuntu2204"
	image := model.APIImage{
		ID: &imageID,
	}
	res, err := config.Resolvers.Image().Events(ctx, &image, 5, 0)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Len(t, res.EventLogEntries, 5)
	assert.Equal(t, res.Count, 5)

	// Does not return the same events in different pages.
	firstPageAMIs := []string{}
	for _, event := range res.EventLogEntries {
		firstPageAMIs = append(firstPageAMIs, utility.FromStringPtr(event.AMIAfter))
	}
	res, err = config.Resolvers.Image().Events(ctx, &image, 5, 1)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Len(t, res.EventLogEntries, 5)
	assert.Equal(t, res.Count, 5)
	for _, event := range res.EventLogEntries {
		assert.False(t, utility.StringSliceContains(firstPageAMIs, utility.FromStringPtr(event.AMIAfter)))
	}
}

func TestDistros(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(distro.Collection), "unable to clear distro collection")
	config := New("/graphql")
	ctx := getContext(t)

	usr, err := user.GetOrCreateUser(apiUser, "User Name", "testuser@mongodb.com", "access_token", "refresh_token", []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)
	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

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
	d4 := &distro.Distro{
		Id:        "ubuntu1604-admin",
		ImageID:   "ubuntu1604",
		AdminOnly: true,
	}
	require.NoError(t, d4.Insert(ctx))
	imageID := "ubuntu1604"
	image := model.APIImage{
		ID: &imageID,
	}
	// Call distros resolver when user is not an admin.
	res, err := config.Resolvers.Image().Distros(ctx, &image)
	require.NoError(t, err)
	require.Len(t, res, 2)
	distroNames := []string{utility.FromStringPtr(res[0].Name), utility.FromStringPtr(res[1].Name)}
	assert.Contains(t, distroNames, "ubuntu1604-small")
	assert.Contains(t, distroNames, "ubuntu1604-large")

	// Call distros resolver when user is an admin.
	require.NoError(t, usr.AddRole("superuser"))
	res, err = config.Resolvers.Image().Distros(ctx, &image)
	require.NoError(t, err)
	require.Len(t, res, 3)
	distroNames = []string{utility.FromStringPtr(res[0].Name), utility.FromStringPtr(res[1].Name), utility.FromStringPtr(res[2].Name)}
	assert.Contains(t, distroNames, "ubuntu1604-small")
	assert.Contains(t, distroNames, "ubuntu1604-large")
	assert.Contains(t, distroNames, "ubuntu1604-admin")
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
