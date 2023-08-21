package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func init() {
	testutil.Setup()
}

func TestBuildingContainerImageJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	assert := assert.New(t)

	assert.NoError(db.Clear(host.Collection))
	env := testutil.NewEnvironment(ctx, t)

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
	assert.NoError(h1.Insert(ctx))
	assert.NoError(h2.Insert(ctx))
	assert.NoError(h3.Insert(ctx))

	j := NewBuildingContainerImageJob(env, h1, host.DockerOptions{Image: "image-url", Method: distro.DockerImageBuildTypeImport}, evergreen.ProviderNameDockerMock)
	assert.False(j.Status().Completed)

	j.Run(ctx)

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

}
