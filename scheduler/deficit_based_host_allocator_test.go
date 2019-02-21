package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDeficitBasedHostAllocator(t *testing.T) {
	var taskIds []string
	var runningTaskIds []string
	var hostIds []string
	var dist distro.Distro

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a deficit based host allocator,"+
		" determining the number of new hosts to spin up...", t, func() {

		taskIds = []string{"t1", "t2", "t3", "t4", "t5"}
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
			var duration time.Duration
			taskDurations := map[string]time.Duration{
				taskIds[0]: duration,
				taskIds[1]: duration,
				taskIds[2]: duration,
				taskIds[3]: duration,
			}
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

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
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
			var duration time.Duration
			taskDurations := map[string]time.Duration{
				taskIds[0]: duration,
				taskIds[1]: duration,
				taskIds[2]: duration,
				taskIds[3]: duration,
			}

			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1]},
			}
			dist.PoolSize = 1

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
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
			var duration time.Duration
			taskDurations := map[string]time.Duration{
				taskIds[0]: duration,
				taskIds[1]: duration,
			}

			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2]},
				{Id: hostIds[3]},
			}
			dist.PoolSize = len(hosts) + 5

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
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
			var duration time.Duration
			taskDurations := map[string]time.Duration{
				taskIds[0]: duration,
				taskIds[1]: duration,
			}

			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
			}
			dist.PoolSize = len(hosts) + 5

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
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
			var duration time.Duration
			taskDurations := map[string]time.Duration{
				taskIds[0]: duration,
				taskIds[1]: duration,
				taskIds[2]: duration,
				taskIds[3]: duration,
				taskIds[4]: duration,
			}

			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
				{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}
			dist.PoolSize = 9

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				Length:        5,
				TaskDurations: taskDurations,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			So(deficitNumNewHostsForDistro(ctx, hostAllocatorData, dist),
				ShouldEqual, 3)

			dist.PoolSize = 8

			distroQueueInfo = DistroQueueInfo{
				Distro:        dist,
				Length:        5,
				TaskDurations: taskDurations,
			}

			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
				// taskQueueItems: taskQueueItems,
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

			var duration time.Duration
			taskDurations := map[string]time.Duration{
				taskIds[0]: duration,
				taskIds[1]: duration,
				taskIds[2]: duration,
			}

			dist.PoolSize = 20
			dist.Provider = "static"

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
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
