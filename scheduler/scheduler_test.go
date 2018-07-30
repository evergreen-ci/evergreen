package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
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

			newHostsSpawned, err := spawnHosts(ctx, distro.Distro{}, 0, nil)
			So(err, ShouldBeNil)
			So(len(newHostsSpawned), ShouldEqual, 0)
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

				newHostsSpawned, err := spawnHosts(ctx, d, newHostsNeeded[id], nil)
				So(err, ShouldBeNil)

				So(newHostsNeeded[id], ShouldEqual, len(newHostsSpawned))
			}
		})
	})
}

func (s *SchedulerSuite) TestNumNewParentsNeeded() {
	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool"}
	pool := &evergreen.ContainerPool{Distro: "distro", Id: "test-pool", MaxContainers: 2}
	host1 := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &host.Host{
		Id:       "host3",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host4 := &host.Host{
		Id:            "host4",
		Distro:        d,
		Status:        evergreen.HostUninitialized,
		HasContainers: true,
	}

	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())
	s.NoError(host4.Insert())

	currentParents, err := host.FindAllRunningParentsByContainerPool(d.ContainerPool)
	s.NoError(err)
	numUninitializedParents, err := host.CountUninitializedParents()
	s.NoError(err)
	existingContainers, err := host.HostGroup(currentParents).FindRunningContainersOnParents()
	s.NoError(err)

	num := numNewParentsNeeded(len(currentParents), numUninitializedParents, 1, len(existingContainers), pool.MaxContainers)
	s.Equal(0, num)
}

func (s *SchedulerSuite) TestNumNewParentsNeeded2() {
	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool"}
	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 3}

	host1 := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &host.Host{
		Id:       "host3",
		Distro:   d,
		Status:   evergreen.HostTerminated,
		ParentID: "host1",
	}

	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	currentParents, err := host.FindAllRunningParentsByContainerPool(d.ContainerPool)
	s.NoError(err)
	numUninitializedParents, err := host.CountUninitializedParents()
	s.NoError(err)
	existingContainers, err := host.HostGroup(currentParents).FindRunningContainersOnParents()
	s.NoError(err)

	num := numNewParentsNeeded(len(currentParents), numUninitializedParents, 1, len(existingContainers), pool.MaxContainers)
	s.Equal(0, num)
}

func (s *SchedulerSuite) TestSpawnHostsParents() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool"}
	parent := distro.Distro{Id: "parent-distro", PoolSize: 3, Provider: evergreen.ProviderNameMock}
	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 2}
	host1 := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &host.Host{
		Id:       "host3",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	s.NoError(d.Insert())
	s.NoError(parent.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	currentParents, err := host.FindAllRunningParentsByContainerPool(pool.Id)
	s.NoError(err)
	numUninitializedParents, err := host.CountUninitializedParents()
	s.NoError(err)
	existingContainers, err := host.HostGroup(currentParents).FindRunningContainersOnParents()
	s.NoError(err)
	num := numNewParentsNeeded(len(currentParents), numUninitializedParents, 1, len(existingContainers), pool.MaxContainers)
	s.Equal(1, num)

	newHostsSpawned, err := spawnHosts(ctx, d, 1, pool)
	s.NoError(err)

	parents := 0
	children := 0
	for _, h := range newHostsSpawned {
		if s.True(h.HasContainers) {
			parents++
		} else if s.NotEmpty(h.ParentID) {
			children++
		}
	}

	s.Equal(1, parents)
	s.Equal(0, children)
}

func (s *SchedulerSuite) TestSpawnHostsContainers() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool"}
	parent := distro.Distro{Id: "parent-distro", PoolSize: 3, Provider: evergreen.ProviderNameMock}

	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 3}
	host1 := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &host.Host{
		Id:       "host3",
		Distro:   d,
		Status:   evergreen.HostTerminated,
		ParentID: "host1",
	}
	s.NoError(d.Insert())
	s.NoError(parent.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	newHostsSpawned, err := spawnHosts(ctx, d, 1, pool)
	s.NoError(err)

	currentParents, err := host.FindAllRunningParentsByContainerPool(pool.Id)
	s.NoError(err)
	numUninitializedParents, err := host.CountUninitializedParents()
	s.NoError(err)
	existingContainers, err := host.HostGroup(currentParents).FindRunningContainersOnParents()
	s.NoError(err)
	num := numNewParentsNeeded(len(currentParents), numUninitializedParents, 1, len(existingContainers), pool.MaxContainers)
	s.Equal(0, num)

	s.Equal(1, len(newHostsSpawned))
	s.NotEmpty(newHostsSpawned[0].ParentID)
}

func (s *SchedulerSuite) TestSpawnHostsParentsAndSomeContainers() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock, ContainerPool: "test-pool"}
	parent := distro.Distro{Id: "parent-distro", PoolSize: 3, Provider: evergreen.ProviderNameMock}

	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 3}
	host1 := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &host.Host{
		Id:       "host3",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	s.NoError(d.Insert())
	s.NoError(parent.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	currentParents, err := host.FindAllRunningParentsByContainerPool(pool.Id)
	s.NoError(err)
	numUninitializedParents, err := host.CountUninitializedParents()
	s.NoError(err)
	existingContainers, err := host.HostGroup(currentParents).FindRunningContainersOnParents()
	s.NoError(err)
	num := numNewParentsNeeded(len(currentParents), numUninitializedParents, 3, len(existingContainers), pool.MaxContainers)
	s.Equal(1, num)

	newHostsSpawned, err := spawnHosts(ctx, d, 3, pool)
	s.NoError(err)
	s.Equal(2, len(newHostsSpawned))

	parents := 0
	children := 0

	for _, h := range newHostsSpawned {
		if h.HasContainers {
			parents++
		} else if h.ParentID != "" {
			children++
		}
	}

	s.Equal(1, children)
	s.Equal(1, parents)
}

func (s *SchedulerSuite) TestSpawnHostsMaximumCapacity() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := distro.Distro{Id: "distro", PoolSize: 1, Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool"}
	pool := &evergreen.ContainerPool{Distro: "distro", Id: "test-pool", MaxContainers: 2}
	host1 := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id:       "host2",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	s.NoError(d.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())

	newHostsSpawned, err := spawnHosts(ctx, d, 2, pool)
	s.NoError(err)

	currentParents, err := host.FindAllRunningParentsByContainerPool(pool.Id)
	s.NoError(err)
	numUninitializedParents, err := host.CountUninitializedParents()
	s.NoError(err)
	existingContainers, err := host.HostGroup(currentParents).FindRunningContainersOnParents()
	s.NoError(err)
	num := numNewParentsNeeded(len(currentParents), numUninitializedParents, 2, len(existingContainers), pool.MaxContainers)
	s.Equal(1, num)

	s.Len(newHostsSpawned, 1)
	s.NotEmpty(newHostsSpawned[0].ParentID)
}

func (s *SchedulerSuite) TestFindAvailableParent() {
	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool"}
	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 2}
	durationOne := 20 * time.Minute
	durationTwo := 30 * time.Minute

	host1 := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id:                    "host2",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host3 := &host.Host{
		Id:          "host3",
		Distro:      d,
		Status:      evergreen.HostRunning,
		ParentID:    "host1",
		RunningTask: "task1",
	}
	host4 := &host.Host{
		Id:          "host4",
		Distro:      d,
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
	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock}
	pool := &evergreen.ContainerPool{Distro: "distro", Id: "test-pool", MaxContainers: 1}
	durationOne := 20 * time.Minute
	durationTwo := 30 * time.Minute

	host1 := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id:                    "host2",
		Distro:                distro.Distro{Id: "distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host3 := &host.Host{
		Id:          "host3",
		Distro:      distro.Distro{Id: "distro", ContainerPool: "test-pool"},
		Status:      evergreen.HostRunning,
		ParentID:    "host1",
		RunningTask: "task1",
	}
	host4 := &host.Host{
		Id:          "host4",
		Distro:      distro.Distro{Id: "distro", ContainerPool: "test-pool"},
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

func (s *SchedulerSuite) TestSpawnContainersStatic() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := []cloud.StaticHost{{Name: "host1"}, {Name: "host2"}, {Name: "host3"}}
	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameDocker,
		ContainerPool: "test-pool"}
	parent := distro.Distro{Id: "parent-distro", Provider: evergreen.ProviderNameStatic}
	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 1}

	host1 := &host.Host{
		Id:   "host1",
		Host: "host",
		User: "user",
		Distro: distro.Distro{
			Id:               "parent-distro",
			ProviderSettings: &map[string]interface{}{"hosts": hosts},
		},
		Provider:              evergreen.ProviderNameStatic,
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id: "host2",
		Distro: distro.Distro{
			Id:               "parent-distro",
			ProviderSettings: &map[string]interface{}{"hosts": hosts},
		},
		Provider:              evergreen.ProviderNameStatic,
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host3 := &host.Host{
		Id: "host3",
		Distro: distro.Distro{
			Id:               "parent-distro",
			ProviderSettings: &map[string]interface{}{"hosts": hosts},
		},
		Provider:              evergreen.ProviderNameStatic,
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}

	s.NoError(d.Insert())
	s.NoError(parent.Insert())
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	newHostsSpawned, err := spawnHosts(ctx, d, 4, pool)
	s.NoError(err)

	currentParents, err := host.FindAllRunningParentsByContainerPool(pool.Id)
	s.NoError(err)
	s.Equal(3, len(currentParents))
	numUninitializedParents, err := host.CountUninitializedParents()
	s.NoError(err)
	existingContainers, err := host.HostGroup(currentParents).FindRunningContainersOnParents()
	s.NoError(err)
	numNewParents := numNewParentsNeeded(len(currentParents), numUninitializedParents, 4, len(existingContainers), pool.MaxContainers)
	numNewParentsToSpawn, err := parentCapacity(parent, numNewParents, len(currentParents), pool)
	s.NoError(err)
	s.Equal(0, numNewParentsToSpawn)
	s.Len(newHostsSpawned, 3)

	for _, h := range newHostsSpawned {
		s.NotEmpty(h.ParentID)
	}
}
