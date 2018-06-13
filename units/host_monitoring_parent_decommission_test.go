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

func TestParentDecommissionJob(t *testing.T) {

	assert := assert.New(t)
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	mockCloud := cloud.GetMockProvider()
	mockCloud.Reset()

	testutil.HandleTestingErr(db.Clear(host.Collection), t, "error clearing %v collections", host.Collection)
	testutil.HandleTestingErr(db.Clear(distro.Collection), t, "Error clearing '%v' collection", distro.Collection)

	d1 := &distro.Distro{Id: "d1", MaxContainers: 2}
	d2 := &distro.Distro{Id: "d2", MaxContainers: 3}
	assert.NoError(d1.Insert())
	assert.NoError(d2.Insert())

	host1 := &host.Host{
		Id:                      "host1",
		Distro:                  *d1,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(15 * time.Minute),
	}
	host2 := &host.Host{
		Id:                      "host2",
		Distro:                  *d1,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(30 * time.Minute),
	}
	// host3 should be deommissioned as soonest-finishing d1 parent
	host3 := &host.Host{
		Id:                      "host3",
		Distro:                  *d1,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(10 * time.Minute),
	}
	host4 := &host.Host{
		Id:                      "host4",
		Distro:                  *d2,
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(5 * time.Minute),
	}
	host5 := &host.Host{
		Id:       "host5",
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host6 := &host.Host{
		Id:       "host6",
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host7 := &host.Host{
		Id:       "host7",
		Status:   evergreen.HostRunning,
		ParentID: "host2",
	}
	host8 := &host.Host{
		Id:       "host8",
		Status:   evergreen.HostRunning,
		ParentID: "host3",
	}
	host9 := &host.Host{
		Id:       "host9",
		Status:   evergreen.HostRunning,
		ParentID: "host4",
	}
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())
	assert.NoError(host8.Insert())
	assert.NoError(host9.Insert())

	j := NewParentDecommissionJob("one", "d1")
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
	assert.Equal(evergreen.HostDecommissioned, h3.Status)

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
	assert.Equal(evergreen.HostRunning, h7.Status)

	h8, err := host.FindOne(host.ById("host8"))
	assert.NoError(err)
	assert.Equal(evergreen.HostDecommissioned, h8.Status)

	h9, err := host.FindOne(host.ById("host9"))
	assert.NoError(err)
	assert.Equal(evergreen.HostRunning, h9.Status)

}
