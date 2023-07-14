package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
)

func TestOldestImageJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)

	assert.NoError(db.Clear(host.Collection))

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

	j := NewOldestImageRemovalJob(h1, evergreen.ProviderNameDockerMock, "job-1")
	assert.False(j.Status().Completed)

	j.Run(context.Background())

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

}
