package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestHostMonitoringContainerStateJob(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.Clear(host.Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	h4 := &host.Host{
		Id:       "container-3",
		Status:   evergreen.HostUninitialized,
		ParentID: "parent-1",
	}
	assert.NoError(h1.Insert(ctx))
	assert.NoError(h2.Insert(ctx))
	assert.NoError(h3.Insert(ctx))
	assert.NoError(h4.Insert(ctx))

	j := NewHostMonitorContainerStateJob(env, h1, evergreen.ProviderNameDockerMock, "job-1")
	assert.False(j.Status().Completed)

	j.Run(context.Background())

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	container1, err := host.FindOne(ctx, host.ById("container-1"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, container1.Status)

	container2, err := host.FindOne(ctx, host.ById("container-2"))
	assert.NoError(err)
	assert.Equal(evergreen.HostTerminated, container2.Status)

	container3, err := host.FindOne(ctx, host.ById("container-3"))
	assert.NoError(err)
	assert.Equal(evergreen.HostUninitialized, container3.Status)
}
