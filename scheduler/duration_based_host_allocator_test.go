package scheduler

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
	"strconv"
	"testing"
	"time"
)

var (
	hostAllocatorTestConf = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(
		db.SessionFactoryFromConfig(hostAllocatorTestConf))
	if hostAllocatorTestConf.Scheduler.LogFile != "" {
		evergreen.SetLogger(hostAllocatorTestConf.Scheduler.LogFile)
	}
}

func TestDurationBasedNewHostsNeeded(t *testing.T) {
	/*
		Note that this is a functional test and its validity relies on the
		values of:
		1. MaxDurationPerDistroHost
		2. SharedTasksAllocationProportion
	*/
	Convey("When calling the duration based NewHostsNeeded...", t, func() {
		taskIds := []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7"}
		distroIds := []string{"d0", "d1", "d2", "d3"}
		hostIds := []string{"h1", "h2", "h3", "h4", "h5", "h6", "h7"}

		expDurs := []time.Duration{
			18 * time.Hour,
			17 * time.Hour,
			16 * time.Hour,
			15 * time.Hour,
			14 * time.Hour,
			13 * time.Hour,
			12 * time.Hour,
		}

		// create a task run distro map such that we have a mix of distros a
		// given set of tasks can run on
		taskRunDistros := map[string][]string{
			taskIds[0]: []string{distroIds[0], distroIds[1]},
			taskIds[1]: []string{distroIds[1], distroIds[2]},
			taskIds[2]: []string{distroIds[0], distroIds[2]},
			taskIds[3]: []string{distroIds[1], distroIds[2]},
			taskIds[4]: []string{distroIds[0], distroIds[2]},
		}

		taskDurations := model.ProjectTaskDurations{}

		distroSlice := []distro.Distro{
			distro.Distro{
				Id:       distroIds[0],
				Provider: "static",
				PoolSize: 5,
			},
			distro.Distro{
				Id:       distroIds[1],
				Provider: "ec2",
				PoolSize: 10,
			},
			distro.Distro{
				Id:       distroIds[2],
				Provider: "ec2",
				PoolSize: 12,
			},
		}

		taskQueueItems := []model.TaskQueueItem{
			model.TaskQueueItem{Id: taskIds[0], ExpectedDuration: expDurs[0]},
			model.TaskQueueItem{Id: taskIds[1], ExpectedDuration: expDurs[1]},
			model.TaskQueueItem{Id: taskIds[2], ExpectedDuration: expDurs[2]},
			model.TaskQueueItem{Id: taskIds[3], ExpectedDuration: expDurs[3]},
			model.TaskQueueItem{Id: taskIds[4], ExpectedDuration: expDurs[4]},
			model.TaskQueueItem{Id: taskIds[5], ExpectedDuration: expDurs[5]},
			model.TaskQueueItem{Id: taskIds[6], ExpectedDuration: expDurs[6]},
		}

		hosts := [][]host.Host{
			[]host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1]},
				host.Host{Id: hostIds[2]},
			},
			[]host.Host{
				host.Host{Id: hostIds[3]},
				host.Host{Id: hostIds[4]},
				host.Host{Id: hostIds[5]},
			},
			[]host.Host{},
		}

		durationBasedHostAllocator := &DurationBasedHostAllocator{}

		Convey("ensure that the distro schedule data is used to spin "+
			"up new hosts if needed",
			func() {
				hostAllocatorData := HostAllocatorData{
					taskQueueItems: map[string][]model.TaskQueueItem{
						distroIds[0]: taskQueueItems,
						distroIds[1]: taskQueueItems,
						distroIds[2]: taskQueueItems[:4],
					},
					existingDistroHosts: map[string][]host.Host{
						distroIds[0]: hosts[0],
						distroIds[1]: hosts[1],
						distroIds[2]: hosts[2],
					},
					distros: map[string]distro.Distro{
						distroIds[0]: distroSlice[0],
						distroIds[1]: distroSlice[1],
						distroIds[2]: distroSlice[2],
					},
					projectTaskDurations: taskDurations,
					taskRunDistros:       taskRunDistros,
				}

				// integration test of duration based host allocator
				newHostsNeeded, err := durationBasedHostAllocator.
					NewHostsNeeded(hostAllocatorData, hostAllocatorTestConf)

				So(err, ShouldBeNil)

				// only distros with only static hosts should be zero
				So(newHostsNeeded[distroIds[0]], ShouldEqual, 0)
				So(newHostsNeeded[distroIds[1]], ShouldNotEqual, 0)
				So(newHostsNeeded[distroIds[2]], ShouldNotEqual, 0)
			})
	})
}

func TestFetchExcessSharedDuration(t *testing.T) {
	Convey("When calling fetchExcessSharedDuration...", t, func() {
		distroOne := "d1"
		distroTwo := "d2"

		Convey("if alternate distros can't handle shared tasks duration "+
			"within the threshold, the shared tasks duration should be "+
			"returned",
			func() {
				distroOneScheduleData := DistroScheduleData{
					numExistingHosts:   2,
					nominalNumNewHosts: 2,
					poolSize:           2,
					taskQueueLength:    2,
					numFreeHosts:       2,
					sharedTasksDuration: map[string]float64{
						distroTwo: 5000,
					},
					runningTasksDuration: 2,
					totalTasksDuration:   2,
				}

				distroTwoScheduleData := DistroScheduleData{
					numExistingHosts:   20,
					nominalNumNewHosts: 0,
					poolSize:           20,
					taskQueueLength:    2,
					numFreeHosts:       2,
					sharedTasksDuration: map[string]float64{
						distroOne: 5000,
					},
					runningTasksDuration: 2,
					totalTasksDuration:   2000,
				}

				maxDurationPerDistroHost := time.Duration(10) * time.Second
				distroScheduleData := map[string]DistroScheduleData{
					distroOne: distroOneScheduleData,
					distroTwo: distroTwoScheduleData,
				}

				// with a max duration per distro host of 10 seconds, and the
				// duration per task on distro two at 100 seconds, we need more
				// hosts for distro one
				sharedDurations := fetchExcessSharedDuration(distroScheduleData,
					distroOne, maxDurationPerDistroHost)

				So(len(sharedDurations), ShouldEqual, 1)

				// the sharedDuration value should equal the value in the
				// alternate distro's map
				So(distroTwoScheduleData.sharedTasksDuration[distroOne],
					ShouldEqual, 5000)
			})

		Convey("if alternate distros can handle shared tasks duration "+
			"within the threshold, no shared tasks duration should be returned",
			func() {
				distroOneScheduleData := DistroScheduleData{
					numExistingHosts:   2,
					nominalNumNewHosts: 2,
					poolSize:           2,
					taskQueueLength:    2,
					numFreeHosts:       2,
					sharedTasksDuration: map[string]float64{
						distroTwo: 5000,
					},
					runningTasksDuration: 2,
					totalTasksDuration:   2,
				}

				distroTwoScheduleData := DistroScheduleData{
					numExistingHosts:   20,
					nominalNumNewHosts: 0,
					poolSize:           20,
					taskQueueLength:    2,
					numFreeHosts:       2,
					sharedTasksDuration: map[string]float64{
						distroOne: 5000,
					},
					runningTasksDuration: 2,
					totalTasksDuration:   2000,
				}

				maxDurationPerDistroHost := time.Duration(100) * time.Second
				distroScheduleData := map[string]DistroScheduleData{
					distroOne: distroOneScheduleData,
					distroTwo: distroTwoScheduleData,
				}

				// with a max duration per distro host of 100 seconds, and the
				// duration per task on distro two at 100 seconds, we don't need
				// any more hosts for distro one
				sharedDurations := fetchExcessSharedDuration(distroScheduleData,
					distroOne, maxDurationPerDistroHost)

				So(len(sharedDurations), ShouldEqual, 0)

				maxDurationPerDistroHost = time.Duration(200) * time.Second
				sharedDurations = fetchExcessSharedDuration(distroScheduleData,
					distroOne, maxDurationPerDistroHost)

				So(len(sharedDurations), ShouldEqual, 0)
			})
	})
}

func TestOrderedScheduleNumNewHosts(t *testing.T) {
	Convey("When calling orderedScheduleNumNewHosts...", t, func() {
		distroOne := "d1"
		distroTwo := "d2"

		Convey("if new hosts are allocated, it should return the number of "+
			"new hosts", func() {

			distroOneScheduleData := DistroScheduleData{
				numExistingHosts:   2,
				nominalNumNewHosts: 3,
				poolSize:           2,
				taskQueueLength:    2,
				numFreeHosts:       2,
				sharedTasksDuration: map[string]float64{
					distroTwo: 5000,
				},
				runningTasksDuration: 2,
				totalTasksDuration:   2,
			}

			distroTwoScheduleData := DistroScheduleData{
				numExistingHosts:   2,
				nominalNumNewHosts: 10,
				poolSize:           2,
				taskQueueLength:    2,
				numFreeHosts:       2,
				sharedTasksDuration: map[string]float64{
					distroOne: 5000,
				},
				runningTasksDuration: 2,
				totalTasksDuration:   2,
			}

			distroScheduleData := map[string]DistroScheduleData{
				distroOne: distroOneScheduleData,
				distroTwo: distroTwoScheduleData,
			}

			So(orderedScheduleNumNewHosts(distroScheduleData, distroOne,
				MaxDurationPerDistroHost, 1.0), ShouldEqual, 3)
			So(orderedScheduleNumNewHosts(distroScheduleData, distroTwo,
				MaxDurationPerDistroHost, 1.0), ShouldEqual, 10)
		})

		Convey("if the distro has no shared tasks, the nominal number of new "+
			"hosts should be returned", func() {
			distroOneScheduleData := DistroScheduleData{
				numExistingHosts:     2,
				nominalNumNewHosts:   0,
				poolSize:             2,
				taskQueueLength:      2,
				numFreeHosts:         2,
				runningTasksDuration: 2,
				totalTasksDuration:   2,
			}

			distroTwoScheduleData := DistroScheduleData{
				numExistingHosts:     2,
				nominalNumNewHosts:   2,
				poolSize:             2,
				taskQueueLength:      2,
				numFreeHosts:         2,
				runningTasksDuration: 2,
				totalTasksDuration:   2,
			}

			distroScheduleData := map[string]DistroScheduleData{
				distroOne: distroOneScheduleData,
				distroTwo: distroTwoScheduleData,
			}
			So(orderedScheduleNumNewHosts(distroScheduleData, distroOne,
				MaxDurationPerDistroHost, 1.0), ShouldEqual, 0)
		})

		Convey("if the distro's max hosts is greater than the number of "+
			"existing hosts, 0 should be returned", func() {
			distroOneScheduleData := DistroScheduleData{
				numExistingHosts:     2,
				nominalNumNewHosts:   0,
				poolSize:             22,
				taskQueueLength:      2,
				numFreeHosts:         2,
				runningTasksDuration: 2,
				totalTasksDuration:   2,
			}

			distroTwoScheduleData := DistroScheduleData{
				numExistingHosts:     2,
				nominalNumNewHosts:   2,
				poolSize:             2,
				taskQueueLength:      2,
				numFreeHosts:         2,
				runningTasksDuration: 2,
				totalTasksDuration:   2,
			}

			distroScheduleData := map[string]DistroScheduleData{
				distroOne: distroOneScheduleData,
				distroTwo: distroTwoScheduleData,
			}
			So(orderedScheduleNumNewHosts(distroScheduleData, distroOne,
				MaxDurationPerDistroHost, 1.0), ShouldEqual, 0)
		})

		Convey("if existing alternate distros can handle the tasks, 0 should "+
			"be returned", func() {
			distroOneScheduleData := DistroScheduleData{
				numExistingHosts:   2,
				nominalNumNewHosts: 0,
				poolSize:           12,
				sharedTasksDuration: map[string]float64{
					distroTwo: 5000,
				},
				taskQueueLength:      20,
				numFreeHosts:         2,
				runningTasksDuration: 200,
				totalTasksDuration:   2000,
			}

			distroTwoScheduleData := DistroScheduleData{
				numExistingHosts:   3,
				nominalNumNewHosts: 2,
				poolSize:           12,
				taskQueueLength:    2,
				sharedTasksDuration: map[string]float64{
					distroOne: 500,
				},
				numFreeHosts:         2,
				runningTasksDuration: 200,
				totalTasksDuration:   500,
			}

			maxDurationPerDistroHost := time.Duration(100) * time.Second
			distroScheduleData := map[string]DistroScheduleData{
				distroOne: distroOneScheduleData,
				distroTwo: distroTwoScheduleData,
			}
			So(orderedScheduleNumNewHosts(distroScheduleData, distroOne,
				maxDurationPerDistroHost, 1.0), ShouldEqual, 0)
		})

		Convey("if existing alternate distros can not handle the tasks, more "+
			"hosts are required - within poolsize", func() {
			distroOneScheduleData := DistroScheduleData{
				numExistingHosts:   5,
				nominalNumNewHosts: 0,
				poolSize:           80,
				taskQueueLength:    40,
				numFreeHosts:       2,
				sharedTasksDuration: map[string]float64{
					distroTwo: 500,
				},
				runningTasksDuration: 100,
				totalTasksDuration:   2,
			}

			distroTwoScheduleData := DistroScheduleData{
				numExistingHosts:   20,
				nominalNumNewHosts: 0,
				poolSize:           20,
				taskQueueLength:    2,
				numFreeHosts:       2,
				sharedTasksDuration: map[string]float64{
					distroOne: 500,
				},
				runningTasksDuration: 200,
				totalTasksDuration:   2000,
			}

			maxDurationPerDistroHost := time.Duration(20) * time.Second
			distroScheduleData := map[string]DistroScheduleData{
				distroOne: distroOneScheduleData,
				distroTwo: distroTwoScheduleData,
			}
			So(orderedScheduleNumNewHosts(distroScheduleData, distroOne,
				maxDurationPerDistroHost, 1.0), ShouldEqual, 30)
		})
	})
}

func TestSortDistrosByNumStaticHosts(t *testing.T) {
	Convey("When calling sortDistrosByNumStaticHosts...", t, func() {
		Convey("distro hosts should be sorted by the number of static hosts", func() {
			getDistro := func(j int) (d distro.Distro) {
				r := []interface{}{}
				for i := 0; i < j; i++ {
					r = append(r, map[interface{}]interface{}{"name": strconv.Itoa(i)})
				}
				d.Id = strconv.Itoa(j)
				d.Provider = evergreen.HostTypeStatic
				d.ProviderSettings = &map[string]interface{}{"hosts": r}
				return d
			}

			order := []int{0, 2, 1, 4, 6, 3, 5}
			distros := make([]distro.Distro, 0, len(order))
			hosts := make([]string, 0, len(order))
			for i, o := range order {
				distros = append(distros, getDistro(o))
				hosts = append(hosts, strconv.Itoa(i))
			}

			newDistros := sortDistrosByNumStaticHosts(distros, hostAllocatorTestConf)

			So(len(distros), ShouldEqual, len(newDistros))
			So(newDistros[0].Id, ShouldEqual, hosts[6])
			So(newDistros[1].Id, ShouldEqual, hosts[5])
			So(newDistros[2].Id, ShouldEqual, hosts[4])
			So(newDistros[3].Id, ShouldEqual, hosts[3])
			So(newDistros[4].Id, ShouldEqual, hosts[2])
			So(newDistros[5].Id, ShouldEqual, hosts[1])
			So(newDistros[6].Id, ShouldEqual, hosts[0])
		})
	})
}

func TestComputeDurationBasedNumNewHosts(t *testing.T) {
	Convey("When calling computeDurationBasedNumNewHosts...", t, func() {

		Convey("when there's an abundance of hosts, no new hosts are needed",
			func() {
				scheduledTasksDuration := 120.
				runningTasksDuration := 120.
				numExistingHosts := 10.
				maxDurationPerHost := time.Duration(200) * time.Second
				numNewHosts := computeDurationBasedNumNewHosts(
					scheduledTasksDuration, runningTasksDuration,
					numExistingHosts, maxDurationPerHost)
				So(numNewHosts, ShouldEqual, 0)
			})

		Convey("when there's an insufficient number of hosts, new hosts are "+
			"needed", func() {
			scheduledTasksDuration := 120.
			runningTasksDuration := 120.
			numExistingHosts := 10.
			maxDurationPerHost := time.Duration(20) * time.Second
			numNewHosts := computeDurationBasedNumNewHosts(
				scheduledTasksDuration, runningTasksDuration,
				numExistingHosts, maxDurationPerHost)
			So(numNewHosts, ShouldEqual, 2)
		})

		Convey("when the durations of existing tasks is short, no new hosts "+
			"are needed", func() {
			scheduledTasksDuration := 12.
			runningTasksDuration := 10.
			numExistingHosts := 10.
			maxDurationPerHost := time.Duration(20) * time.Second
			numNewHosts := computeDurationBasedNumNewHosts(
				scheduledTasksDuration, runningTasksDuration,
				numExistingHosts, maxDurationPerHost)
			So(numNewHosts, ShouldEqual, 0)
		})

		Convey("when the durations of existing tasks is less than the "+
			"maximum duration, exactly one host is needed", func() {
			scheduledTasksDuration := 12.
			runningTasksDuration := 10.
			numExistingHosts := 0.
			maxDurationPerHost := time.Duration(23) * time.Second
			numNewHosts := computeDurationBasedNumNewHosts(
				scheduledTasksDuration, runningTasksDuration,
				numExistingHosts, maxDurationPerHost)
			So(numNewHosts, ShouldEqual, 1)
		})

		Convey("when the durations of existing tasks is equal to the "+
			"maximum duration, exactly one host is needed", func() {
			scheduledTasksDuration := 12.
			runningTasksDuration := 12.
			numExistingHosts := 0.
			maxDurationPerHost := time.Duration(24) * time.Second
			numNewHosts := computeDurationBasedNumNewHosts(
				scheduledTasksDuration, runningTasksDuration,
				numExistingHosts, maxDurationPerHost)
			So(numNewHosts, ShouldEqual, 1)
		})

		Convey("when the durations of existing tasks is slightly more than "+
			"the maximum duration, exactly two hosts are needed", func() {
			scheduledTasksDuration := 12.
			runningTasksDuration := 13.
			numExistingHosts := 0.
			maxDurationPerHost := time.Duration(24) * time.Second
			numNewHosts := computeDurationBasedNumNewHosts(
				scheduledTasksDuration, runningTasksDuration,
				numExistingHosts, maxDurationPerHost)
			So(numNewHosts, ShouldEqual, 2)
		})
	})
}

func TestComputeRunningTasksDuration(t *testing.T) {
	var testTaskDuration time.Duration
	var hostIds []string
	var runningTaskIds []string
	var taskDurations model.ProjectTaskDurations

	Convey("When calling computeRunningTasksDuration...", t, func() {
		// set all variables
		testTaskDuration = time.Duration(4) * time.Minute
		hostIds = []string{"h1", "h2", "h3", "h4", "h5", "h6"}
		runningTaskIds = []string{"t1", "t2", "t3", "t4", "t5", "t6"}

		startTimeOne := time.Now()
		startTimeTwo := startTimeOne.Add(-time.Duration(1) * time.Minute)
		startTimeThree := startTimeOne.Add(-time.Duration(2) * time.Minute)

		remainingDurationOne := (time.Duration(4) * time.Minute).Seconds()
		remainingDurationTwo := (time.Duration(3) * time.Minute).Seconds()
		remainingDurationThree := (time.Duration(2) * time.Minute).Seconds()

		// durations of tasks we know
		taskDurations = model.ProjectTaskDurations{
			TaskDurationByProject: map[string]*model.BuildVariantTaskDurations{
				"": &model.BuildVariantTaskDurations{
					TaskDurationByBuildVariant: map[string]*model.TaskDurations{
						"": &model.TaskDurations{
							TaskDurationByDisplayName: map[string]time.
								Duration{
								"": testTaskDuration,
							},
						},
					},
				},
			},
		}

		So(db.Clear(task.Collection), ShouldBeNil)

		Convey("the total duration of running tasks with similar start times "+
			" should be the total of the remaining time using estimates from "+
			"the project task duration data for running tasks", func() {
			// tasks running on hosts
			runningTasks := []task.Task{
				{Id: runningTaskIds[0], StartTime: startTimeOne},
				{Id: runningTaskIds[1], StartTime: startTimeOne},
				{Id: runningTaskIds[2], StartTime: startTimeOne},
			}

			for _, runningTask := range runningTasks {
				So(runningTask.Insert(), ShouldBeNil)
			}

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				host.Host{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				host.Host{Id: hostIds[3]},
				host.Host{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}

			runningTasksDuration, err :=
				computeRunningTasksDuration(existingDistroHosts, taskDurations)

			So(err, ShouldBeNil)

			// the running task duration should be a total of the remaining
			// duration of running tasks - 3 in this case
			// due to scheduling variables, we allow a 10 second tolerance
			So(runningTasksDuration, ShouldAlmostEqual, remainingDurationOne*3, 10)
		})

		Convey("the total duration of running tasks with different start "+
			"times should be the total of the remaining time using estimates "+
			"from the project task duration data for running tasks", func() {

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				host.Host{Id: hostIds[0], RunningTask: runningTaskIds[0]},
				host.Host{Id: hostIds[1], RunningTask: runningTaskIds[1]},
				host.Host{Id: hostIds[2], RunningTask: runningTaskIds[2]},
				host.Host{Id: hostIds[3], RunningTask: runningTaskIds[3]},
				host.Host{Id: hostIds[4], RunningTask: runningTaskIds[4]},
				host.Host{Id: hostIds[5], RunningTask: runningTaskIds[5]},
			}

			// tasks running on hosts
			runningTasks := []task.Task{
				{Id: runningTaskIds[0], StartTime: startTimeThree},
				{Id: runningTaskIds[1], StartTime: startTimeTwo},
				{Id: runningTaskIds[2], StartTime: startTimeOne},
				{Id: runningTaskIds[3], StartTime: startTimeTwo},
				{Id: runningTaskIds[4], StartTime: startTimeOne},
				{Id: runningTaskIds[5], StartTime: startTimeThree},
			}

			for _, runningTask := range runningTasks {
				So(runningTask.Insert(), ShouldBeNil)
			}

			runningTasksDuration, err :=
				computeRunningTasksDuration(existingDistroHosts, taskDurations)
			So(err, ShouldBeNil)
			// the running task duration should be a total of the remaining
			// duration of running tasks - 6 in this case
			// due to scheduling variables, we allow a 5 second tolerance
			expectedResult := remainingDurationOne*2 + remainingDurationTwo*2 +
				remainingDurationThree*2
			So(runningTasksDuration, ShouldAlmostEqual, expectedResult, 5)
		})

		Convey("the duration of running tasks with unknown running time "+
			"estimates should be ignored", func() {

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				host.Host{Id: hostIds[0], RunningTask: runningTaskIds[0]},
				host.Host{Id: hostIds[1], RunningTask: runningTaskIds[1]},
				host.Host{Id: hostIds[2], RunningTask: runningTaskIds[2]},
			}

			// tasks running on hosts
			runningTasks := []task.Task{
				{Id: runningTaskIds[0], StartTime: startTimeThree, DisplayName: "unknown"},
				{Id: runningTaskIds[1], StartTime: startTimeTwo},
				{Id: runningTaskIds[2], StartTime: startTimeOne, DisplayName: "unknown"},
			}

			for _, runningTask := range runningTasks {
				So(runningTask.Insert(), ShouldBeNil)
			}

			runningTasksDuration, err :=
				computeRunningTasksDuration(existingDistroHosts, taskDurations)
			So(err, ShouldBeNil)
			// only task 1's duration is known
			// due to scheduling variables, we allow a 5 second tolerance
			So(runningTasksDuration, ShouldAlmostEqual, remainingDurationTwo, 5)
		})

		Convey("the duration of running tasks with outliers as running times "+
			"should be ignored", func() {

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				host.Host{Id: hostIds[0], RunningTask: runningTaskIds[0]},
				host.Host{Id: hostIds[1], RunningTask: runningTaskIds[1]},
				host.Host{Id: hostIds[2], RunningTask: runningTaskIds[2]},
			}

			// tasks running on hosts
			runningTasks := []task.Task{
				{Id: runningTaskIds[0], StartTime: startTimeOne},
				{Id: runningTaskIds[1], StartTime: startTimeOne.Add(-time.Duration(4) * time.Hour)},
				{Id: runningTaskIds[2], StartTime: startTimeTwo},
			}

			for _, runningTask := range runningTasks {
				So(runningTask.Insert(), ShouldBeNil)
			}

			runningTasksDuration, err :=
				computeRunningTasksDuration(existingDistroHosts, taskDurations)
			So(err, ShouldBeNil)
			// task 2's duration should be ignored
			// due to scheduling variables, we allow a 5 second tolerance
			expectedResult := remainingDurationOne + remainingDurationTwo
			So(runningTasksDuration, ShouldAlmostEqual, expectedResult, 5)
		})

		Convey("the total duration if there are no running tasks should be "+
			"zero", func() {

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1]},
				host.Host{Id: hostIds[2]},
				host.Host{Id: hostIds[3]},
			}

			runningTasksDuration, err :=
				computeRunningTasksDuration(existingDistroHosts, taskDurations)
			So(err, ShouldBeNil)
			// the running task duration should be a total of the remaining
			// duration of running tasks
			So(runningTasksDuration, ShouldEqual, 0)
		})
	})
}

func TestComputeScheduledTasksDuration(t *testing.T) {
	var expDur time.Duration
	var tasks []string
	var queueItems []model.TaskQueueItem
	var tasksAccountedFor map[string]bool

	Convey("When calling computeScheduledTasksDuration...", t, func() {
		Convey("the total scheduled tasks duration should equal the duration "+
			"of all tasks scheduled, for that distro, in the queue", func() {
			tasks = []string{"t1", "t2", "t3", "t4", "t5", "t6"}
			expDur = time.Duration(180) * time.Minute
			tasksAccountedFor = make(map[string]bool)
			queueItems = []model.TaskQueueItem{
				model.TaskQueueItem{Id: tasks[0], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[1], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[2], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[3], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[4], ExpectedDuration: expDur},
			}

			// construct the data needed by computeScheduledTasksDuration
			scheduledDistroTasksData := &ScheduledDistroTasksData{
				taskQueueItems:    queueItems,
				tasksAccountedFor: tasksAccountedFor,
			}

			scheduledTasksDuration, _ := computeScheduledTasksDuration(
				scheduledDistroTasksData)

			expectedTotalDuration := float64(len(queueItems)) * expDur.Seconds()
			So(scheduledTasksDuration, ShouldEqual, expectedTotalDuration)
		})

		Convey("the map of tasks accounted for should be updated", func() {
			tasks = []string{"t1", "t2", "t3", "t4", "t5", "t6"}
			expDur = time.Duration(180) * time.Minute
			tasksAccountedFor = make(map[string]bool)
			queueItems = []model.TaskQueueItem{
				model.TaskQueueItem{Id: tasks[0], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[1], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[2], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[3], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[4], ExpectedDuration: expDur},
			}

			// construct the data needed by computeScheduledTasksDuration
			scheduledDistroTasksData := &ScheduledDistroTasksData{
				taskQueueItems:    queueItems,
				tasksAccountedFor: tasksAccountedFor,
			}

			computeScheduledTasksDuration(scheduledDistroTasksData)

			expectedTasksAccountedFor := map[string]bool{
				tasks[0]: true,
				tasks[1]: true,
				tasks[2]: true,
				tasks[3]: true,
				tasks[4]: true,
			}

			So(tasksAccountedFor, ShouldResemble, expectedTasksAccountedFor)
		})

		Convey("other distro task queues with the same task should disregard "+
			"the duplicate entry in other queues", func() {
			tasks = []string{"t1", "t2", "t3", "t4", "t5", "t6"}
			expDur = time.Duration(180) * time.Minute
			tasksAccountedFor = make(map[string]bool)
			distroOneQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: tasks[0], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[1], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[2], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[3], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[4], ExpectedDuration: expDur},
			}

			// construct the data needed by computeScheduledTasksDuration
			scheduledDistroTasksData := &ScheduledDistroTasksData{
				taskQueueItems:    distroOneQueueItems,
				tasksAccountedFor: tasksAccountedFor,
			}

			computeScheduledTasksDuration(scheduledDistroTasksData)

			expectedTasksAccountedFor := map[string]bool{
				tasks[0]: true,
				tasks[1]: true,
				tasks[2]: true,
				tasks[3]: true,
				tasks[4]: true,
			}

			So(tasksAccountedFor, ShouldResemble, expectedTasksAccountedFor)

			// task 0 appears in both task queues so it's duration should be
			// ignored. task 5 is new so its duration should be used and the
			// map should be updated to include it
			distroTwoQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: tasks[0], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: tasks[5], ExpectedDuration: expDur},
			}
			expectedTasksAccountedFor[tasks[5]] = true

			// construct the data needed by computeScheduledTasksDuration
			scheduledDistroTasksData = &ScheduledDistroTasksData{
				taskQueueItems:    distroTwoQueueItems,
				tasksAccountedFor: tasksAccountedFor,
			}

			scheduledTasksDuration, _ := computeScheduledTasksDuration(
				scheduledDistroTasksData)

			So(tasksAccountedFor, ShouldResemble, expectedTasksAccountedFor)
			So(scheduledTasksDuration, ShouldEqual, expDur.Seconds())
		})
	})
}

func TestDurationBasedHostAllocator(t *testing.T) {
	var taskIds []string
	var runningTaskIds []string
	var hostIds []string
	var dist distro.Distro
	var testTaskDuration time.Duration
	var taskDurations model.ProjectTaskDurations
	var durationBasedHostAllocator *DurationBasedHostAllocator

	Convey("With a duration based host allocator,"+
		" determining the number of new hosts to spin up", t, func() {

		durationBasedHostAllocator = &DurationBasedHostAllocator{}
		taskIds = []string{"t1", "t2", "t3", "t4", "t5"}
		runningTaskIds = []string{"t1", "t2", "t3", "t4", "t5"}
		hostIds = []string{"h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8", "h9"}
		dist = distro.Distro{Provider: "ec2"}
		testTaskDuration = time.Duration(2) * time.Minute
		taskDurations = model.ProjectTaskDurations{
			TaskDurationByProject: map[string]*model.BuildVariantTaskDurations{
				"": &model.BuildVariantTaskDurations{
					TaskDurationByBuildVariant: map[string]*model.TaskDurations{
						"": &model.TaskDurations{
							TaskDurationByDisplayName: map[string]time.Duration{
								"": testTaskDuration,
							},
						},
					},
				},
			},
		}

		So(db.Clear(task.Collection), ShouldBeNil)

		Convey("if there are no tasks to run, no new hosts should be needed",
			func() {
				hosts := []host.Host{
					host.Host{Id: hostIds[0]},
					host.Host{Id: hostIds[1]},
					host.Host{Id: hostIds[2]},
				}
				dist.PoolSize = len(hosts) + 5

				hostAllocatorData := &HostAllocatorData{
					existingDistroHosts: map[string][]host.Host{
						"": hosts,
					},
					distros: map[string]distro.Distro{
						"": dist,
					},
				}

				tasksAccountedFor := make(map[string]bool)
				distroScheduleData := make(map[string]DistroScheduleData)

				newHosts, err := durationBasedHostAllocator.
					numNewHostsForDistro(hostAllocatorData, dist,
					tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
				So(err, ShouldBeNil)
				So(newHosts, ShouldEqual, 0)
			})

		Convey("if the number of existing hosts equals the max hosts, no new"+
			" hosts can be spawned", func() {
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
				model.TaskQueueItem{Id: taskIds[2]},
				model.TaskQueueItem{Id: taskIds[3]},
			}
			dist.PoolSize = 0

			hostAllocatorData := &HostAllocatorData{
				existingDistroHosts: map[string][]host.Host{},
				distros: map[string]distro.Distro{
					"": dist,
				},
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			newHosts, err := durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
			hosts := []host.Host{
				host.Host{Id: hostIds[0]},
			}
			dist.PoolSize = len(hosts)

			hostAllocatorData = &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
			}

			tasksAccountedFor = make(map[string]bool)
			distroScheduleData = make(map[string]DistroScheduleData)

			newHosts, err = durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
		})

		Convey("if the number of existing hosts exceeds the max hosts, no new"+
			" hosts can be spawned", func() {

			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
				model.TaskQueueItem{Id: taskIds[2]},
				model.TaskQueueItem{Id: taskIds[3]},
			}
			hosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1]},
			}
			dist.PoolSize = 1

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			newHosts, err := durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
		})

		Convey("if the number of tasks to run is less than the number of free"+
			" hosts, no new hosts are needed", func() {
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
			}
			hosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1]},
				host.Host{Id: hostIds[2]},
			}
			dist.PoolSize = len(hosts) + 5

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			newHosts, err := durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)

		})

		Convey("if the number of tasks to run is equal to the number of free"+
			" hosts, no new hosts are needed", func() {
			hosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				host.Host{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				host.Host{Id: hostIds[3]},
			}
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0]},
				model.TaskQueueItem{Id: taskIds[1]},
			}

			dist.PoolSize = len(hosts) + 5

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
				projectTaskDurations: taskDurations,
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				task := task.Task{Id: runningTaskId}
				So(task.Insert(), ShouldBeNil)
			}

			newHosts, err := durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
		})

		Convey("if the number of tasks to run exceeds the number of free"+
			" hosts, new hosts are needed up to the maximum allowed for the"+
			" dist", func() {
			expDur := time.Duration(200) * time.Minute
			// all runnable tasks have an expected duration of expDur (200mins)
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[1], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[2], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[3], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[4], ExpectedDuration: expDur},
			}
			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				host.Host{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				host.Host{Id: hostIds[3]},
				host.Host{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}
			dist.PoolSize = 9

			// In this test:
			//
			// 1. Total distro duration is:
			//		(len(taskQueueItems) * expDur ) +
			//		time left on hosts with running tasks
			// which comes out to:
			//		(5 * 200 * 60) + (60 * 3) ~ 60180 (in seconds)
			//
			// 2. MAX_DURATION_PER_DISTRO = 7200 (2 hours)
			//
			// 3. We have 5 existing hosts
			//
			// Thus, our duration based host allocator will always return 8 -
			// which is greater than what distro.PoolSize-len(existingDistroHosts)
			// will ever return in this situation.
			//
			// Hence, we should always expect to use that minimum.
			//
			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
				projectTaskDurations: taskDurations,
			}
			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				task := task.Task{Id: runningTaskId}
				So(task.Insert(), ShouldBeNil)
			}

			// total running duration here is
			newHosts, err := durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 3)

			dist.PoolSize = 8
			hostAllocatorData = &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
				projectTaskDurations: taskDurations,
			}

			tasksAccountedFor = make(map[string]bool)
			distroScheduleData = make(map[string]DistroScheduleData)

			newHosts, err = durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 3)
			dist.PoolSize = 7
			hostAllocatorData = &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
				projectTaskDurations: taskDurations,
			}

			tasksAccountedFor = make(map[string]bool)
			distroScheduleData = make(map[string]DistroScheduleData)

			newHosts, err = durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 2)

			dist.PoolSize = 6

			hostAllocatorData = &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
				projectTaskDurations: taskDurations,
			}
			tasksAccountedFor = make(map[string]bool)

			newHosts, err = durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 1)
		})

		Convey("if the distro cannot be used to spawn hosts, then no new "+
			"hosts can be spawned", func() {
			expDur := time.Duration(200) * time.Minute
			// all runnable tasks have an expected duration of expDur (200mins)
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[1], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[2], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[3], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[4], ExpectedDuration: expDur},
			}
			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1]},
				host.Host{Id: hostIds[2]},
				host.Host{Id: hostIds[3]},
				host.Host{Id: hostIds[4]},
			}

			dist.PoolSize = 20
			dist.Provider = "static"

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
				projectTaskDurations: taskDurations,
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			newHosts, err := durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
		})

		Convey("if the duration based estimate is less than the maximum "+
			"\nnumber of new hosts allowed for this distro, the estimate of new "+
			"\nhosts should be used", func() {
			expDur := time.Duration(200) * time.Minute
			// all runnable tasks have an expected duration of expDur (200mins)
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[1], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[2], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[3], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[4], ExpectedDuration: expDur},
			}

			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				host.Host{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				host.Host{Id: hostIds[3]},
				host.Host{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}
			dist.PoolSize = 20

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
				projectTaskDurations: taskDurations,
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				task := task.Task{Id: runningTaskId}
				So(task.Insert(), ShouldBeNil)
			}

			newHosts, err := durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 3)
		})

		Convey("if the duration based estimate is less than the maximum "+
			"\nnumber of new hosts allowed for this distro, but greater than "+
			"\nthe difference between the number of runnable tasks and the "+
			"\nnumber of free hosts, that difference should be used", func() {
			expDur := time.Duration(400) * time.Minute
			// all runnable tasks have an expected duration of expDur (200mins)
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[1], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[2], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[3], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[4], ExpectedDuration: expDur},
			}

			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				host.Host{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				host.Host{Id: hostIds[3]},
				host.Host{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}
			dist.PoolSize = 20

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
				projectTaskDurations: taskDurations,
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				task := task.Task{Id: runningTaskId}
				So(task.Insert(), ShouldBeNil)
			}

			// estimates based on data
			// duration estimate: 11
			// max new hosts allowed: 15
			// 'one-host-per-scheduled-task': 3
			newHosts, err := durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 3)
		})

		Convey("if the duration based estimate is less than both the maximum "+
			"\nnumber of new hosts allowed for this distro, and the "+
			"\ndifference between the number of runnable tasks and the "+
			"\nnumber of free hosts, then the duration based estimate should "+
			"be used", func() {
			expDur := time.Duration(180) * time.Minute
			// all runnable tasks have an expected duration of expDur (200mins)
			taskQueueItems := []model.TaskQueueItem{
				model.TaskQueueItem{Id: taskIds[0], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[1], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[2], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[3], ExpectedDuration: expDur},
				model.TaskQueueItem{Id: taskIds[4], ExpectedDuration: expDur},
			}

			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				host.Host{Id: hostIds[0]},
				host.Host{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				host.Host{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				host.Host{Id: hostIds[3]},
				host.Host{Id: hostIds[4], RunningTask: runningTaskIds[2]},
				host.Host{Id: hostIds[5]},
			}
			dist.PoolSize = 20

			hostAllocatorData := &HostAllocatorData{
				taskQueueItems: map[string][]model.TaskQueueItem{
					"": taskQueueItems,
				},
				existingDistroHosts: map[string][]host.Host{
					"": hosts,
				},
				distros: map[string]distro.Distro{
					"": dist,
				},
				projectTaskDurations: taskDurations,
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				task := task.Task{Id: runningTaskId}
				So(task.Insert(), ShouldBeNil)
			}

			// estimates based on data
			// duration estimate: 2
			// max new hosts allowed: 15
			// 'one-host-per-scheduled-task': 3
			newHosts, err := durationBasedHostAllocator.
				numNewHostsForDistro(hostAllocatorData, dist,
				tasksAccountedFor, distroScheduleData, hostAllocatorTestConf)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 2)
		})
	})

}
