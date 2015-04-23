package scheduler

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func init() {
	db.SetGlobalSessionProvider(
		db.SessionFactoryFromConfig(hostAllocatorTestConf))
	if hostAllocatorTestConf.Scheduler.LogFile != "" {
		mci.SetLogger(hostAllocatorTestConf.Scheduler.LogFile)
	}
}

func TestDeficitBasedHostAllocator(t *testing.T) {
	var taskIds []string
	var runningTaskIds []string
	var hostIds []string
	var distro model.Distro
	var hostAllocator *DeficitBasedHostAllocator

	Convey("With a deficit based host allocator,"+
		" determining the number of new hosts to spin up...", t, func() {

		hostAllocator = &DeficitBasedHostAllocator{}
		taskIds = []string{"t1", "t2", "t3", "t4", "t5"}
		runningTaskIds = []string{"t1", "t2", "t3", "t4", "t5"}
		hostIds = []string{"h1", "h2", "h3", "h4", "h5"}
		distro = model.Distro{Provider: "ec2"}

		Convey("if there are no tasks to run, no new hosts should be needed",
			func() {
				hosts := []model.Host{
					model.Host{Id: hostIds[0]},
					model.Host{Id: hostIds[1]},
					model.Host{Id: hostIds[2]},
				}
				distro.MaxHosts = len(hosts) + 5

				hostAllocatorData := &HostAllocatorData{
					existingDistroHosts: map[string][]model.Host{
						"": hosts,
					},
					distros: map[string]model.Distro{
						"": distro,
					},
				}

				So(hostAllocator.numNewHostsForDistro(hostAllocatorData,
					distro, hostAllocatorTestConf), ShouldEqual, 0)
			})

		Convey("if the number of existing hosts equals the max hosts, no new"+
			" hosts can be spawned", func() {
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
				model.TaskQueueItem{Id: taskIds[2]},
				model.TaskQueueItem{Id: taskIds[3]},
			}
			distro.MaxHosts = 0

			hostAllocatorData := &HostAllocatorData{
				existingDistroHosts: map[string][]model.Host{},
				distros: map[string]model.Distro{
					"": distro,
				},
			}

			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 0)
			hosts := []model.Host{
				model.Host{Id: hostIds[0]},
			}
			distro.MaxHosts = len(hosts)

			hostAllocatorData = &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]model.Host{
					"": hosts,
				},
				distros: map[string]model.Distro{
					"": distro,
				},
			}

			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 0)
		})

		Convey("if the number of existing hosts exceeds the max hosts, no new"+
			" hosts can be spawned", func() {

			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
				model.TaskQueueItem{Id: taskIds[2]},
				model.TaskQueueItem{Id: taskIds[3]},
			}
			hosts := []model.Host{
				model.Host{Id: hostIds[0]},
				model.Host{Id: hostIds[1]},
			}
			distro.MaxHosts = 1

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]model.Host{
					"": hosts,
				},
				distros: map[string]model.Distro{
					"": distro,
				},
			}

			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 0)
		})

		Convey("if the number of tasks to run is less than the number of free"+
			" hosts, no new hosts are needed", func() {
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
			}
			hosts := []model.Host{
				model.Host{Id: hostIds[0]},
				model.Host{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				model.Host{Id: hostIds[2]},
				model.Host{Id: hostIds[3]},
			}
			distro.MaxHosts = len(hosts) + 5

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]model.Host{
					"": hosts,
				},
				distros: map[string]model.Distro{
					"": distro,
				},
			}

			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 0)

		})

		Convey("if the number of tasks to run is equal to the number of free"+
			" hosts, no new hosts are needed", func() {
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
			}
			hosts := []model.Host{
				model.Host{Id: hostIds[0]},
				model.Host{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				model.Host{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				model.Host{Id: hostIds[3]},
			}
			distro.MaxHosts = len(hosts) + 5

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]model.Host{
					"": hosts,
				},
				distros: map[string]model.Distro{
					"": distro,
				},
			}

			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 0)
		})

		Convey("if the number of tasks to run exceeds the number of free"+
			" hosts, new hosts are needed up to the maximum allowed for the"+
			" distro", func() {
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
				model.TaskQueueItem{Id: taskIds[2]},
				model.TaskQueueItem{Id: taskIds[3]},
				model.TaskQueueItem{Id: taskIds[4]},
			}
			hosts := []model.Host{
				model.Host{Id: hostIds[0]},
				model.Host{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				model.Host{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				model.Host{Id: hostIds[3]},
				model.Host{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}
			distro.MaxHosts = 9

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]model.Host{
					"": hosts,
				},
				distros: map[string]model.Distro{
					"": distro,
				},
			}

			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 3)

			distro.MaxHosts = 8
			hostAllocatorData = &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]model.Host{
					"": hosts,
				},
				distros: map[string]model.Distro{
					"": distro,
				},
			}
			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 3)
			distro.MaxHosts = 7
			hostAllocatorData = &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]model.Host{
					"": hosts,
				},
				distros: map[string]model.Distro{
					"": distro,
				},
			}
			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 2)
			distro.MaxHosts = 6
			hostAllocatorData = &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]model.Host{
					"": hosts,
				},
				distros: map[string]model.Distro{
					"": distro,
				},
			}
			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 1)
		})

		Convey("if the distro cannot be used to spawn hosts, then no new hosts"+
			" can be spawned", func() {
			hosts := []model.Host{
				model.Host{Id: hostIds[0]},
			}
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
				model.TaskQueueItem{Id: taskIds[2]},
			}
			distro.MaxHosts = 20
			distro.Provider = "static"
			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]model.Host{
					"": hosts,
				},
				distros: map[string]model.Distro{
					"": distro,
				},
			}
			So(hostAllocator.numNewHostsForDistro(hostAllocatorData, distro, hostAllocatorTestConf),
				ShouldEqual, 0)
		})

	})

}
