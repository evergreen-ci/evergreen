package units

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
)

func TestDecommissioningContainersOnParent(t *testing.T) {
	assert := assert.New(t)

	mockCloud := cloud.GetMockProvider()
	mockCloud.Reset()

	testutil.HandleTestingErr(db.Clear(host.Collection), t, "error clearing %v collections", host.Collection)
	testutil.HandleTestingErr(db.Clear(distro.Collection), t, "Error clearing '%v' collection", distro.Collection)

	d1 := &distro.Distro{Id: "d1"}
	assert.NoError(d1.Insert())

	startTime := time.Now().Add(-1 * time.Hour)

	host1 := &host.Host{
		Id:                      "host1",
		Distro:                  *d1,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(15 * time.Minute),
		StartTime:               startTime,
	}
	host2 := &host.Host{
		Id:                      "host2",
		Distro:                  *d1,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(30 * time.Minute),
		StartTime:               startTime,
	}
	// host3 should be deommissioned as soonest-finishing d1 parent
	host3 := &host.Host{
		Id:                      "host3",
		Distro:                  *d1,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(10 * time.Minute),
		StartTime:               startTime,
	}
	host4 := &host.Host{
		Id:       "host4",
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host5 := &host.Host{
		Id:       "host5",
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host6 := &host.Host{
		Id:       "host6",
		Status:   evergreen.HostRunning,
		ParentID: "host2",
	}
	// host 7 should be decommissioned
	host7 := &host.Host{
		Id:       "host7",
		Status:   evergreen.HostRunning,
		ParentID: "host3",
	}
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())

	// MaxContainers: 2
	j := NewParentDecommissionJob("one", (*d1).Id, 2)
	assert.False(j.Status().Completed)

	j.Run(context.Background())

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	h1, err := host.FindOne(host.ById("host1"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h1.Status)

	h2, err := host.FindOne(host.ById("host2"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h2.Status)

	h3, err := host.FindOne(host.ById("host3"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h3.Status)

	h4, err := host.FindOne(host.ById("host4"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h4.Status)

	h5, err := host.FindOne(host.ById("host5"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h5.Status)

	h6, err := host.FindOne(host.ById("host6"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h6.Status)

	h7, err := host.FindOne(host.ById("host7"))
	assert.NoError(err)
	assert.Equal(evergreen.HostDecommissioned, h7.Status)
}

func TestDecommissioningParentWithTerminatedContainers(t *testing.T) {
	assert := assert.New(t)

	mockCloud := cloud.GetMockProvider()
	mockCloud.Reset()

	testutil.HandleTestingErr(db.Clear(host.Collection), t, "error clearing %v collections", host.Collection)
	testutil.HandleTestingErr(db.Clear(distro.Collection), t, "Error clearing '%v' collection", distro.Collection)

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
