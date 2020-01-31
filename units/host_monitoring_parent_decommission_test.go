package units

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

func TestDecommissioningParentWithTerminatedContainers(t *testing.T) {
	assert := assert.New(t)

	mockCloud := cloud.GetMockProvider()
	mockCloud.Reset()

	require.NoError(t, db.Clear(host.Collection), "error clearing %v collections", host.Collection)
	require.NoError(t, db.Clear(distro.Collection), "Error clearing '%v' collection", distro.Collection)

	d2 := &distro.Distro{Id: "d2"}
	assert.NoError(d2.Insert())

	now := time.Now()
	startTimeOld := now.Add(-1 * time.Hour)

	host1 := &host.Host{
		Id:                      "host1",
		Distro:                  *d2,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: now,
		StartTime:               startTimeOld,
	}
	// host2 should be decommissioned as its containers have terminated
	host2 := &host.Host{
		Id:                      "host2",
		Distro:                  *d2,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: now.Add(10 * time.Minute),
		StartTime:               startTimeOld,
	}
	host3 := &host.Host{
		Id:       "host3",
		Status:   evergreen.HostDecommissioned,
		ParentID: "host1",
	}
	host4 := &host.Host{
		Id:       "host4",
		Status:   evergreen.HostTerminated,
		ParentID: "host2",
	}
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())

	// MaxContainers: 3
	j := NewParentDecommissionJob("two", (*d2).Id, 3)
	assert.False(j.Status().Completed)

	j.Run(context.Background())

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	h1, err := host.FindOne(host.ById("host1"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h1.Status)

	h2, err := host.FindOne(host.ById("host2"))
	assert.NoError(err)
	assert.Equal(evergreen.HostDecommissioned, h2.Status)

	h3, err := host.FindOne(host.ById("host3"))
	assert.NoError(err)
	assert.Equal(evergreen.HostDecommissioned, h3.Status)

	h4, err := host.FindOne(host.ById("host4"))
	assert.NoError(err)
	assert.Equal(evergreen.HostTerminated, h4.Status)
}
