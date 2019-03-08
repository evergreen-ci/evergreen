package scheduler

import (
	"context"
	"testing"

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
		dist = distro.Distro{Provider: "ec2"}

		Convey("if there are no tasks to run, no new hosts should be needed",
			func() {
				hosts := []host.Host{
					{Id: hostIds[0]},
					{Id: hostIds[1]},
					{Id: hostIds[2]},
				}
				dist.PoolSize = len(hosts) + 5

				hostAllocatorData := &HostAllocatorData{
					Distro:        dist,
					ExistingHosts: hosts,
				}

				So(deficitNumNewHostsForDistro(ctx, hostAllocatorData,
					dist), ShouldEqual, 0)
			})

		Convey("if the number of existing hosts equals the max hosts, no new"+
			" hosts can be spawned", func() {
			dist.PoolSize = 0

			hostAllocatorData := &HostAllocatorData{
				ExistingHosts: []host.Host{},
				Distro:        dist,
			}

			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 0)
			hosts := []host.Host{
				{Id: hostIds[0]},
			}
			dist.PoolSize = len(hosts)

			distroQueueInfo := model.DistroQueueInfo{
				Length: 4,
			}

			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 0)
		})

		Convey("if the number of existing hosts exceeds the max hosts, no new"+
			" hosts can be spawned", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1]},
			}
			dist.PoolSize = 1

			distroQueueInfo := model.DistroQueueInfo{
				Length: 4,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 0)
		})

		Convey("if the number of tasks to run is less than the number of free"+
			" hosts, no new hosts are needed", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2]},
				{Id: hostIds[3]},
			}
			dist.PoolSize = len(hosts) + 5

			distroQueueInfo := model.DistroQueueInfo{
				Length: 2,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 0)

		})

		Convey("if the number of tasks to run is equal to the number of free"+
			" hosts, no new hosts are needed", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
			}
			dist.PoolSize = len(hosts) + 5

			distroQueueInfo := model.DistroQueueInfo{
				Length: 2,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 0)
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
			dist.PoolSize = 9

			distroQueueInfo := model.DistroQueueInfo{
				Length: 5,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 3)

			dist.PoolSize = 8

			distroQueueInfo = model.DistroQueueInfo{
				Length: 5,
			}

			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}
			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 3)
			dist.PoolSize = 7
			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}
			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 2)
			dist.PoolSize = 6
			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}
			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 1)
		})

		Convey("if the distro cannot be used to spawn hosts, then no new hosts"+
			" can be spawned", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
			}
			dist.PoolSize = 20
			dist.Provider = "static"

			distroQueueInfo := model.DistroQueueInfo{
				Length: 3,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 0)
		})

	})

}
