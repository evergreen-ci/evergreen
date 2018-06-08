package scheduler

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

var schedulerTestConf = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(schedulerTestConf.SessionFactory())
}

func TestSpawnHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When spawning hosts", t, func() {

		distroIds := []string{"d1", "d2", "d3"}
		Convey("if there are no hosts to be spawned, the Scheduler should not"+
			" make any calls to the Manager", func() {
			newHostsNeeded := map[string]int{
				distroIds[0]: 0,
				distroIds[1]: 0,
				distroIds[2]: 0,
			}

			newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
			So(err, ShouldBeNil)
			So(len(newHostsSpawned[distroIds[0]]), ShouldEqual, 0)
			So(len(newHostsSpawned[distroIds[1]]), ShouldEqual, 0)
			So(len(newHostsSpawned[distroIds[2]]), ShouldEqual, 0)
		})

		Convey("if there are hosts to be spawned, the Scheduler should make"+
			" one call to the Manager for each host, and return the"+
			" results bucketed by distro", func() {

			newHostsNeeded := map[string]int{
				distroIds[0]: 3,
				distroIds[1]: 0,
				distroIds[2]: 1,
			}

			for _, id := range distroIds {
				d := distro.Distro{Id: id, PoolSize: 3, Provider: evergreen.ProviderNameMock}
				So(d.Insert(), ShouldBeNil)
			}

			newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
			So(err, ShouldBeNil)
			distroZeroHosts := newHostsSpawned[distroIds[0]]
			distroOneHosts := newHostsSpawned[distroIds[1]]
			distroTwoHosts := newHostsSpawned[distroIds[2]]
			So(len(distroZeroHosts), ShouldEqual, 3)
			So(distroZeroHosts[0].Distro.Id, ShouldEqual, distroIds[0])
			So(distroZeroHosts[1].Distro.Id, ShouldEqual, distroIds[0])
			So(distroZeroHosts[2].Distro.Id, ShouldEqual, distroIds[0])
			So(len(distroOneHosts), ShouldEqual, 0)
			So(len(distroTwoHosts), ShouldEqual, 1)
			So(distroTwoHosts[0].Distro.Id, ShouldEqual, distroIds[2])
		})

		Reset(func() {
			So(db.Clear(distro.Collection), ShouldBeNil)
			So(db.Clear(host.Collection), ShouldBeNil)
		})
	})
}

func TestCalcNewParentsNeeded(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts"))

	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		MaxContainers: 2}
	host1 := &host.Host{
		Id:            "host1",
		Host:          "host",
		User:          "user",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &host.Host{
		Id:       "host3",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostTerminated,
		ParentID: "host1",
	}

	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())

	CurrentParents, err := host.FindAllRunningParents()
	assert.NoError(err)
	ExistingContainers, err := host.FindAllRunningContainers()
	assert.NoError(err)

	num := CalcNewParentsNeeded(len(CurrentParents), len(ExistingContainers), 1, d)
	assert.Equal(1, num)
}

func TestCalcNewParentsNeeded2(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts"))
	assert.NoError(db.ClearCollections("distro"))

	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		MaxContainers: 3}
	host1 := &host.Host{
		Id:            "host1",
		Host:          "host",
		User:          "user",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &host.Host{
		Id:       "host3",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostTerminated,
		ParentID: "host1",
	}

	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())

	CurrentParents, err := host.FindAllRunningParents()
	assert.NoError(err)
	ExistingContainers, err := host.FindAllRunningContainers()
	assert.NoError(err)

	num := CalcNewParentsNeeded(len(CurrentParents), len(ExistingContainers), 1, d)
	assert.Equal(0, num)
}

func TestSpawnHostsParents(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts"))
	assert.NoError(db.ClearCollections("distro"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		MaxContainers: 2}
	host1 := &host.Host{
		Id:            "host1",
		Host:          "host",
		User:          "user",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &host.Host{
		Id:       "host3",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostTerminated,
		ParentID: "host1",
	}
	assert.NoError(d.Insert())
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())

	newHostsNeeded := map[string]int{
		"distro": 1,
	}
	newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
	assert.NoError(err)

	CurrentParents, err := host.FindAllRunningParents()
	assert.NoError(err)
	ExistingContainers, err := host.FindAllRunningContainers()
	assert.NoError(err)
	num := CalcNewParentsNeeded(len(CurrentParents), len(ExistingContainers), 1, d)
	assert.Equal(1, num)

	assert.Equal(1, len(newHostsSpawned["distro"]))
	assert.True(newHostsSpawned["distro"][0].HasContainers)

}

func TestSpawnHostsContainers(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts"))
	assert.NoError(db.ClearCollections("distro"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		MaxContainers: 3}
	host1 := &host.Host{
		Id:            "host1",
		Host:          "host",
		User:          "user",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &host.Host{
		Id:       "host3",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostTerminated,
		ParentID: "host1",
	}
	assert.NoError(d.Insert())
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())

	newHostsNeeded := map[string]int{
		"distro": 1,
	}
	newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
	assert.NoError(err)

	CurrentParents, err := host.FindAllRunningParents()
	assert.NoError(err)
	ExistingContainers, err := host.FindAllRunningContainers()
	assert.NoError(err)
	num := CalcNewParentsNeeded(len(CurrentParents), len(ExistingContainers), 1, d)
	assert.Equal(0, num)

	assert.Equal(1, len(newHostsSpawned["distro"]))
	assert.NotEmpty(newHostsSpawned["distro"][0].ParentID)
}

func TestSpawnHostsMaximumCapacity(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts"))
	assert.NoError(db.ClearCollections("distro"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := distro.Distro{Id: "distro", PoolSize: 1, Provider: evergreen.ProviderNameMock,
		MaxContainers: 2}
	host1 := &host.Host{
		Id:            "host1",
		Host:          "host",
		User:          "user",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	assert.NoError(d.Insert())
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())

	newHostsNeeded := map[string]int{
		"distro": 2,
	}
	newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
	assert.NoError(err)

	CurrentParents, err := host.FindAllRunningParents()
	assert.NoError(err)
	ExistingContainers, err := host.FindAllRunningContainers()
	assert.NoError(err)
	num := CalcNewParentsNeeded(len(CurrentParents), len(ExistingContainers), 2, d)
	assert.Equal(1, num)

	assert.Equal(1, len(newHostsSpawned["distro"]))
	assert.NotEmpty(newHostsSpawned["distro"][0].ParentID)

}
