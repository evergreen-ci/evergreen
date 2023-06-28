package scheduler

import (
	"context"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/suite"
)

type SchedulerSuite struct {
	suite.Suite
}

func TestSchedulerSpawnSuite(t *testing.T) {
	suite.Run(t, new(SchedulerSuite))
}

func (s *SchedulerSuite) SetupTest() {
	s.NoError(db.ClearCollections("hosts"))
	s.NoError(db.ClearCollections("distro"))
	s.NoError(db.ClearCollections("tasks"))
}

func TestSpawnHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When spawning hosts", t, func() {

		distroIds := []string{"d1", "d2", "d3"}
		Convey("if there are no hosts to be spawned, the Scheduler should not"+
			" make any calls to the Manager", func() {

			newHostsSpawned, err := SpawnHosts(ctx, distro.Distro{}, 0, nil)
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
				d := distro.Distro{
					Id:       id,
					Provider: evergreen.ProviderNameMock,
					HostAllocatorSettings: distro.HostAllocatorSettings{
						MaximumHosts: 3,
					},
				}

				newHostsSpawned, err := SpawnHosts(ctx, d, newHostsNeeded[id], nil)
				So(err, ShouldBeNil)

				So(newHostsNeeded[id], ShouldEqual, len(newHostsSpawned))
			}
		})
	})
}

func (s *SchedulerSuite) TestSpawnHostsParents() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	providerSettings := birch.NewDocument(birch.EC.String("image_url", "my-image"))
	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool", ProviderSettingsList: []*birch.Document{providerSettings}}
	parent := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
	}
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
	s.NoError(d.Insert(ctx))
	s.NoError(parent.Insert(ctx))
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	newHostsSpawned, err := SpawnHosts(ctx, d, 1, pool)
	s.NoError(err)

	parents := 0
	children := 0
	for _, h := range newHostsSpawned {
		if h.HasContainers {
			parents++
		} else if s.NotEmpty(h.ParentID) {
			children++
		}
	}

	s.Equal(1, parents)
	s.Equal(1, children)
}

func (s *SchedulerSuite) TestSpawnHostsContainers() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	providerSettings := birch.NewDocument(birch.EC.String("image_url", "my-image"))
	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool", ProviderSettingsList: []*birch.Document{providerSettings}}
	parent := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
	}

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
	s.NoError(d.Insert(ctx))
	s.NoError(parent.Insert(ctx))
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	newHostsSpawned, err := SpawnHosts(ctx, d, 1, pool)
	s.NoError(err)

	s.Require().Equal(1, len(newHostsSpawned))
	s.NotEmpty(newHostsSpawned[0].ParentID)
	s.False(newHostsSpawned[0].HasContainers)
}

func (s *SchedulerSuite) TestSpawnHostsParentsAndSomeContainers() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	providerSettings := birch.NewDocument(birch.EC.String("image_url", "my-image"))
	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock, ContainerPool: "test-pool",
		ProviderSettingsList: []*birch.Document{providerSettings}}
	parent := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 2,
		},
	}

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
	s.NoError(d.Insert(ctx))
	s.NoError(parent.Insert(ctx))
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	newHostsSpawned, err := SpawnHosts(ctx, d, 5, pool)
	s.NoError(err)
	// 1 parent, 3 children on new parent, 1 child on old parent
	s.Equal(5, len(newHostsSpawned))

	parents := 0
	children := 0

	for _, h := range newHostsSpawned {
		if h.HasContainers {
			parents++
		} else if h.ParentID != "" {
			children++
		}
	}

	s.Equal(4, children)
	s.Equal(1, parents)
}

func (s *SchedulerSuite) TestSpawnHostsOneNewParent() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	providerSettings := birch.NewDocument(birch.EC.String("image_url", "my-image"))
	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock, ContainerPool: "test-pool",
		ProviderSettingsList: []*birch.Document{providerSettings}}
	parent := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 2,
		},
	}

	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 3}

	s.NoError(d.Insert(ctx))
	s.NoError(parent.Insert(ctx))

	newHostsSpawned, err := SpawnHosts(ctx, d, 1, pool)
	s.NoError(err)
	// 1 parent, 1 child
	s.Equal(2, len(newHostsSpawned))

	parentHost := host.Host{}
	childHost := host.Host{}

	for _, h := range newHostsSpawned {
		if h.HasContainers {
			parentHost = h
		} else if h.ParentID != "" {
			childHost = h
		}
	}

	s.Require().NotEmpty(childHost)
	s.Require().NotEmpty(parentHost)

	s.Equal(childHost.ParentID, parentHost.Id)
	parentDoc, err := childHost.GetParent()
	s.NoError(err)
	s.Require().NotNil(parentDoc)
	s.Equal(parentHost.Id, parentDoc.Id)
}

func (s *SchedulerSuite) TestSpawnHostsMaximumCapacity() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	providerSettings := birch.NewDocument(birch.EC.String("image_url", "my-image"))
	d := distro.Distro{
		Id:                   "distro",
		Provider:             evergreen.ProviderNameMock,
		ContainerPool:        "test-pool",
		ProviderSettingsList: []*birch.Document{providerSettings},
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 1,
		},
	}
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
	s.NoError(d.Insert(ctx))
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())

	newHostsSpawned, err := SpawnHosts(ctx, d, 2, pool)
	s.NoError(err)

	s.Require().Len(newHostsSpawned, 1)
	s.NotEmpty(newHostsSpawned[0].ParentID)
	s.False(newHostsSpawned[0].HasContainers)
}

func (s *SchedulerSuite) TestSpawnContainersStatic() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	providerSettings := birch.NewDocument(birch.EC.String("image_url", "my-image"))
	hosts := []cloud.StaticHost{{Name: "host1"}, {Name: "host2"}, {Name: "host3"}}
	hostSettings := birch.NewDocument(birch.EC.Interface("hosts", hosts))
	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameDocker,
		ContainerPool: "test-pool", ProviderSettingsList: []*birch.Document{providerSettings}}

	parent := distro.Distro{Id: "parent-distro", Provider: evergreen.ProviderNameStatic,
		ProviderSettingsList: []*birch.Document{providerSettings}}
	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 1}

	host1 := &host.Host{
		Id:   "host1",
		Host: "host",
		User: "user",
		Distro: distro.Distro{
			Id:                   "parent-distro",
			ProviderSettingsList: []*birch.Document{hostSettings},
		},
		Provider:              evergreen.ProviderNameStatic,
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &host.Host{
		Id: "host2",
		Distro: distro.Distro{
			Id:                   "parent-distro",
			ProviderSettingsList: []*birch.Document{hostSettings},
		},
		Provider:              evergreen.ProviderNameStatic,
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host3 := &host.Host{
		Id: "host3",
		Distro: distro.Distro{
			Id:                   "parent-distro",
			ProviderSettingsList: []*birch.Document{hostSettings},
		},
		Provider:              evergreen.ProviderNameStatic,
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}

	s.NoError(d.Insert(ctx))
	s.NoError(parent.Insert(ctx))
	s.NoError(host1.Insert())
	s.NoError(host2.Insert())
	s.NoError(host3.Insert())

	newHostsSpawned, err := SpawnHosts(ctx, d, 4, pool)
	s.NoError(err)
	s.Len(newHostsSpawned, 3)

	for _, h := range newHostsSpawned {
		s.NotEmpty(h.ParentID)
	}
}
