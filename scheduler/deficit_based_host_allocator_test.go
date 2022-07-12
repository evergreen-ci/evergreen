package scheduler

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDeficitBasedHostAllocator(t *testing.T) {
	var runningTaskIds []string
	var hostIds []string
	var dist distro.Distro

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a deficit based host allocator,"+
		" determining the number of new hosts to spin up...", t, func() {
		runningTaskIds = []string{"t1", "t2", "t3", "t4", "t5"}
		hostIds = []string{"h1", "h2", "h3", "h4", "h5"}
		// kim: TODO: check this test
		dist = distro.Distro{Provider: evergreen.ProviderNameEc2Fleet}

		Convey("if there are no tasks to run, no new hosts should be needed",
			func() {
				hosts := []host.Host{
					{Id: hostIds[0]},
					{Id: hostIds[1]},
					{Id: hostIds[2]},
				}
				dist.HostAllocatorSettings.MaximumHosts = len(hosts) + 5

				hostAllocatorData := &HostAllocatorData{
					Distro:        dist,
					ExistingHosts: hosts,
				}

				numNewHosts, numFreeHosts := deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
				So(numNewHosts, ShouldEqual, 0)
				So(numFreeHosts, ShouldEqual, 3)
			})

		Convey("if the number of existing hosts equals the max hosts, no new"+
			" hosts can be spawned", func() {
			dist.HostAllocatorSettings.MaximumHosts = 0

			hostAllocatorData := &HostAllocatorData{
				ExistingHosts: []host.Host{},
				Distro:        dist,
			}

			numNewHosts, numFreeHosts := deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 0)
			So(numFreeHosts, ShouldEqual, 0)

			hosts := []host.Host{
				{Id: hostIds[0]},
			}
			dist.HostAllocatorSettings.MaximumHosts = len(hosts)

			distroQueueInfo := model.DistroQueueInfo{
				Length: 4,
			}

			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			numNewHosts, numFreeHosts = deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 0)
			So(numFreeHosts, ShouldEqual, 1)
		})

		Convey("if the number of existing hosts exceeds the max hosts, no new"+
			" hosts can be spawned", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1]},
			}
			dist.HostAllocatorSettings.MaximumHosts = 1

			distroQueueInfo := model.DistroQueueInfo{
				Length: 4,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			numNewHosts, numFreeHosts := deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 0)
			So(numFreeHosts, ShouldEqual, 2)
		})

		Convey("if the number of tasks to run is less than the number of free"+
			" hosts, no new hosts are needed", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2]},
				{Id: hostIds[3]},
			}
			dist.HostAllocatorSettings.MaximumHosts = len(hosts) + 5

			distroQueueInfo := model.DistroQueueInfo{
				Length: 2,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			numNewHosts, numFreeHosts := deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 0)
			So(numFreeHosts, ShouldEqual, 3)

		})

		Convey("if the number of tasks to run is equal to the number of free"+
			" hosts, no new hosts are needed", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
			}
			dist.HostAllocatorSettings.MaximumHosts = len(hosts) + 5

			distroQueueInfo := model.DistroQueueInfo{
				Length: 2,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			numNewHosts, numFreeHosts := deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 0)
			So(numFreeHosts, ShouldEqual, 2)
		})

		Convey("if the number of tasks to run exceeds the number of free"+
			" hosts, new hosts are needed up to the maximum allowed for the"+
			" distro", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
				{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}
			dist.HostAllocatorSettings.MaximumHosts = 9

			distroQueueInfo := model.DistroQueueInfo{
				Length: 5,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			numNewHosts, numFreeHosts := deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 3)
			So(numFreeHosts, ShouldEqual, 2)

			dist.HostAllocatorSettings.MaximumHosts = 8

			distroQueueInfo = model.DistroQueueInfo{
				Length: 5,
			}

			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			numNewHosts, numFreeHosts = deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 3)
			So(numFreeHosts, ShouldEqual, 2)

			dist.HostAllocatorSettings.MaximumHosts = 7
			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			numNewHosts, numFreeHosts = deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 2)
			So(numFreeHosts, ShouldEqual, 2)
			dist.HostAllocatorSettings.MaximumHosts = 6
			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			numNewHosts, numFreeHosts = deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 1)
			So(numFreeHosts, ShouldEqual, 2)
		})

		Convey("if the distro cannot be used to spawn hosts, then no new hosts"+
			" can be spawned", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
			}
			dist.HostAllocatorSettings.MaximumHosts = 20
			dist.Provider = evergreen.ProviderNameStatic

			distroQueueInfo := model.DistroQueueInfo{
				Length: 3,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			numNewHosts, numFreeHosts := deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist)
			So(numNewHosts, ShouldEqual, 0)
			So(numFreeHosts, ShouldEqual, 1)
		})

	})

}
