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

func TestOldestImageJob(t *testing.T) {
	assert := assert.New(t)
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

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
	assert.NoError(h1.Insert())
	assert.NoError(h2.Insert())
	assert.NoError(h3.Insert())

	j := NewOldestImageRemovalJob(h1, evergreen.ProviderNameDockerMock, "job-1")
	assert.False(j.Status().Completed)

	j.Run(context.Background())

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

}
