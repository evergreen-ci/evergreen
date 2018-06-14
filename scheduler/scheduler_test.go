package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/suite"
)

type SchedulerSuite struct {
	suite.Suite
}

func TestSchedulerSpawnSuite(t *testing.T) {
	suite.Run(t, new(SchedulerSuite))
}

func (s *SchedulerSuite) TearDownTest() {
	s.NoError(db.ClearCollections("hosts"))
	s.NoError(db.ClearCollections("distro"))
	s.NoError(db.ClearCollections("tasks"))
}

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

func (s *SchedulerSuite) TestNumNewParentsNeeded() {
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
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}

	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	currentParents, err := host.FindAllRunningParentsByDistro(d.Id)
	s.NoError(err)
	existingContainers, err := host.FindAllRunningContainers()
	s.NoError(err)

	num := numNewParentsNeeded(len(currentParents), 1, len(existingContainers), d)
	s.Equal(1, num)
}

func (s *SchedulerSuite) TestNumNewParentsNeeded2() {
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

	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	currentParents, err := host.FindAllRunningParentsByDistro(d.Id)
	s.NoError(err)
	existingContainers, err := host.FindAllRunningContainers()
	s.NoError(err)

	num := numNewParentsNeeded(len(currentParents), 1, len(existingContainers), d)
	s.Equal(0, num)
}

func (s *SchedulerSuite) TestSpawnHostsParents() {
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
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	s.NoError(d.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	newHostsNeeded := map[string]int{
		"distro": 1,
	}
	newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
	s.NoError(err)

	currentParents, err := host.FindAllRunningParentsByDistro(d.Id)
	s.NoError(err)
	existingContainers, err := host.FindAllRunningContainers()
	s.NoError(err)
	num := numNewParentsNeeded(len(currentParents), 1, len(existingContainers), d)
	s.Equal(1, num)

	s.Equal(1, len(newHostsSpawned["distro"]))
	s.True(newHostsSpawned["distro"][0].HasContainers)

}

func (s *SchedulerSuite) TestSpawnHostsContainers() {
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
	s.NoError(d.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	newHostsNeeded := map[string]int{
		"distro": 1,
	}
	newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
	s.NoError(err)

	currentParents, err := host.FindAllRunningParentsByDistro(d.Id)
	s.NoError(err)
	existingContainers, err := host.FindAllRunningContainers()
	s.NoError(err)
	num := numNewParentsNeeded(len(currentParents), 1, len(existingContainers), d)
	s.Equal(0, num)

	s.Equal(1, len(newHostsSpawned["distro"]))
	s.NotEmpty(newHostsSpawned["distro"][0].ParentID)
}

func (s *SchedulerSuite) TestSpawnHostsParentsAndSomeContainers() {
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
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	s.NoError(d.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	newHostsNeeded := map[string]int{
		"distro": 3,
	}
	newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
	s.NoError(err)

	currentParents, err := host.FindAllRunningParentsByDistro(d.Id)
	s.NoError(err)
	existingContainers, err := host.FindAllRunningContainers()
	s.NoError(err)
	num := numNewParentsNeeded(len(currentParents), 3, len(existingContainers), d)
	s.Equal(1, num)

	s.Equal(2, len(newHostsSpawned["distro"]))
	s.True(newHostsSpawned["distro"][0].HasContainers)
	s.NotEmpty(newHostsSpawned["distro"][1].ParentID)
}

func (s *SchedulerSuite) TestSpawnHostsMaximumCapacity() {
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
	s.NoError(d.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())

	newHostsNeeded := map[string]int{
		"distro": 2,
	}
	newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
	s.NoError(err)

	currentParents, err := host.FindAllRunningParentsByDistro(d.Id)
	s.NoError(err)
	existingContainers, err := host.FindAllRunningContainers()
	s.NoError(err)
	num := numNewParentsNeeded(len(currentParents), 2, len(existingContainers), d)
	s.Equal(1, num)

	s.Equal(1, len(newHostsSpawned["distro"]))
	s.NotEmpty(newHostsSpawned["distro"][0].ParentID)

}

func (s *SchedulerSuite) TestFindAvailableParent() {
	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		MaxContainers: 2}
	durationOne := 20 * time.Minute
	durationTwo := 30 * time.Minute

	host1 := &host.Host{
		Id:            "host1",
		Host:          "host",
		User:          "user",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host2 := &host.Host{
		Id:            "host2",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host3 := &host.Host{
		Id:          "host3",
		Distro:      distro.Distro{Id: "distro"},
		Status:      evergreen.HostRunning,
		ParentID:    "host1",
		RunningTask: "task1",
	}
	host4 := &host.Host{
		Id:          "host4",
		Distro:      distro.Distro{Id: "distro"},
		Status:      evergreen.HostRunning,
		ParentID:    "host2",
		RunningTask: "task2",
	}
	task1 := task.Task{
		Id: "task1",
		DurationPrediction: util.CachedDurationValue{
			Value: durationOne,
		},
		BuildVariant: "bv1",
		StartTime:    time.Now(),
	}
	task2 := task.Task{
		Id: "task2",
		DurationPrediction: util.CachedDurationValue{
			Value: durationTwo,
		},
		BuildVariant: "bv1",
		StartTime:    time.Now(),
	}
	s.NoError(d.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())
	s.NoError(host4.Insert())
	s.NoError(task1.Insert())
	s.NoError(task2.Insert())

	availableParent, err := findAvailableParent(d)
	s.NoError(err)

	s.Equal("host2", availableParent.Id)
}

func (s *SchedulerSuite) TestFindNoAvailableParent() {
	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		MaxContainers: 1}
	durationOne := 20 * time.Minute
	durationTwo := 30 * time.Minute

	host1 := &host.Host{
		Id:            "host1",
		Host:          "host",
		User:          "user",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host2 := &host.Host{
		Id:            "host2",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host3 := &host.Host{
		Id:          "host3",
		Distro:      distro.Distro{Id: "distro"},
		Status:      evergreen.HostRunning,
		ParentID:    "host1",
		RunningTask: "task1",
	}
	host4 := &host.Host{
		Id:          "host4",
		Distro:      distro.Distro{Id: "distro"},
		Status:      evergreen.HostRunning,
		ParentID:    "host2",
		RunningTask: "task2",
	}
	task1 := task.Task{
		Id: "task1",
		DurationPrediction: util.CachedDurationValue{
			Value: durationOne,
		}, BuildVariant: "bv1",
		StartTime: time.Now(),
	}
	task2 := task.Task{
		Id: "task2",
		DurationPrediction: util.CachedDurationValue{
			Value: durationTwo,
		}, BuildVariant: "bv1",
		StartTime: time.Now(),
	}
	s.NoError(d.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())
	s.NoError(host4.Insert())
	s.NoError(task1.Insert())
	s.NoError(task2.Insert())

	availableParent, err := findAvailableParent(d)
	s.EqualError(err, "No available parent found for container")
	s.Empty(availableParent.Id)
}
