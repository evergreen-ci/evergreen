package units

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestBuildingContainerImageJob(t *testing.T) {
	assert := assert.New(t)
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	assert.NoError(db.Clear(host.Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	assert.NoError(env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))

	h1 := &host.Host{
		Id:            "parent-1",
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	h2 := &host.Host{
		Id:       "container-1",
		Status:   evergreen.HostRunning,
		ParentID: "parent-1",
	}
	h3 := &host.Host{
		Id:       "container-2",
		Status:   evergreen.HostRunning,
		ParentID: "parent-1",
	}
	assert.NoError(h1.Insert())
	assert.NoError(h2.Insert())
	assert.NoError(h3.Insert())

	j := NewBuildingContainerImageJob(env, h1, cloud.ContainerImageSettings{URL: "image-url", Method: distro.DockerImageBuildTypeImport}, evergreen.ProviderNameDockerMock)
	assert.False(j.Status().Completed)

	j.Run(context.Background())

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

}
