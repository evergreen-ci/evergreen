package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecommissioningParentWithTerminatedContainers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)

	mockCloud := cloud.GetMockProvider()
	mockCloud.Reset()

	require.NoError(t, db.ClearCollections(host.Collection, distro.Collection))

	d2 := distro.Distro{Id: "d2", HostAllocatorSettings: distro.HostAllocatorSettings{MinimumHosts: 2}}
	assert.NoError(d2.Insert(ctx))

	now := time.Now()
	startTimeOld := now.Add(-1 * time.Hour)

	host1 := &host.Host{
		Id:                      "host1",
		Distro:                  d2,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: now,
		StartTime:               startTimeOld,
	}
	// host2 should be decommissioned as its containers have terminated
	host2 := &host.Host{
		Id:                      "host2",
		Distro:                  d2,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: now.Add(10 * time.Minute),
		StartTime:               startTimeOld,
	}
	host3 := &host.Host{
		Id:       "host3",
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host4 := &host.Host{
		Id:       "host4",
		Status:   evergreen.HostTerminated,
		ParentID: "host2",
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))

	// Running the job should not drop parents below min hosts
	j := NewParentDecommissionJob("two", d2.Id, 3)
	j.Run(ctx)

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)
	h2, err := host.FindOne(ctx, host.ById("host2"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h2.Status)

	// Setting min hosts lower should make host2 get decommissioned
	d2.HostAllocatorSettings.MinimumHosts = 1
	assert.NoError(d2.ReplaceOne(ctx))
	j.Run(ctx)

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	h1, err := host.FindOne(ctx, host.ById("host1"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h1.Status)

	h2, err = host.FindOne(ctx, host.ById("host2"))
	assert.NoError(err)
	assert.Equal(evergreen.HostDecommissioned, h2.Status)

	h3, err := host.FindOne(ctx, host.ById("host3"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h3.Status)

	h4, err := host.FindOne(ctx, host.ById("host4"))
	assert.NoError(err)
	assert.Equal(evergreen.HostTerminated, h4.Status)
}
