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

func TestOperatingSystem(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	ami := "ami-0f6b89500372d4a06"
	image := model.APIImage{
		AMI: &ami,
	}
	opts := thirdparty.OSInfoFilterOptions{
		AMI:  ami,
		Name: "Kernel",
	}
	res, err := config.Resolvers.Image().OperatingSystem(ctx, &image, opts)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Data, 1)
	require.NotNil(t, res.Data[0])
	assert.Equal(t, "Kernel", utility.FromStringPtr(res.Data[0].Name))
	assert.Equal(t, 1, res.FilteredCount)
	assert.Equal(t, 13, res.TotalCount)
}

func TestPackages(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
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
	assert.Equal(t, "python3-automat", utility.FromStringPtr(res.Data[0].Name))
	assert.Equal(t, 1, res.FilteredCount)
	assert.Equal(t, 1618, res.TotalCount)
}

func TestToolchains(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
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
	assert.Equal(t, "golang", utility.FromStringPtr(res.Data[0].Name))
	assert.Equal(t, 33, res.FilteredCount)
	assert.Equal(t, 49, res.TotalCount)
}

func TestFiles(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	ami := "ami-00db9124d254b2a91"
	image := model.APIImage{
		AMI: &ami,
	}
	opts := thirdparty.FileFilterOptions{
		AMI:   ami,
		Limit: 1,
	}
	res, err := config.Resolvers.Image().Files(ctx, &image, opts)
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	require.NotNil(t, res.Data[0])
	assert.Equal(t, 295, res.FilteredCount)
	assert.Equal(t, 295, res.TotalCount)
}

func TestEvents(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
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
	assert.Equal(t, 5, res.Count)

	// Does not return the same events in different pages.
	firstPageAMIs := []string{}
	for _, event := range res.EventLogEntries {
		firstPageAMIs = append(firstPageAMIs, utility.FromStringPtr(event.AMIAfter))
	}
	res, err = config.Resolvers.Image().Events(ctx, &image, 5, 1)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Len(t, res.EventLogEntries, 5)
	assert.Equal(t, 5, res.Count)
	for _, event := range res.EventLogEntries {
		assert.False(t, utility.StringSliceContains(firstPageAMIs, utility.FromStringPtr(event.AMIAfter)))
	}
}

func TestDistros(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(distro.Collection), "unable to clear distro collection")
	config := New("/graphql")
	ctx := getContext(t)

	usr, err := user.GetOrCreateUser(testUser, "User Name", "testuser@mongodb.com", "access_token", "refresh_token", []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)
	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
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
	require.NoError(t, usr.AddRole(t.Context(), "superuser"))
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
	testutil.ConfigureIntegrationTest(t, testConfig)
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
		Id:      "ubuntu1804-small",
		ImageID: "ubuntu1804",
	}
	require.NoError(t, d3.Insert(ctx))
	taskA := &task.Task{
		Id:         "task_a",
		DistroId:   "ubuntu1604-small",
		FinishTime: time.Date(2023, time.February, 1, 10, 30, 15, 0, time.UTC),
	}
	require.NoError(t, taskA.Insert(t.Context()))
	taskB := &task.Task{
		Id:         "task_b",
		DistroId:   "ubuntu1604-large",
		FinishTime: time.Date(2023, time.March, 1, 10, 30, 15, 0, time.UTC),
	}
	require.NoError(t, taskB.Insert(t.Context()))
	taskC := &task.Task{
		Id:         "task_c",
		DistroId:   "ubuntu2204-large",
		FinishTime: time.Date(2023, time.April, 1, 10, 30, 15, 0, time.UTC),
	}
	require.NoError(t, taskC.Insert(t.Context()))
	image := model.APIImage{
		ID: utility.ToStringPtr("ubuntu1604"),
	}
	// Returns latest task that ran on the image.
	res, err := config.Resolvers.Image().LatestTask(ctx, &image)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "task_b", utility.FromStringPtr(res.Id))

	// Returns nil if no task has ever ran on the image.
	image = model.APIImage{
		ID: utility.ToStringPtr("ubuntu1804"),
	}
	res, err = config.Resolvers.Image().LatestTask(ctx, &image)
	require.NoError(t, err)
	assert.Nil(t, res)
}
