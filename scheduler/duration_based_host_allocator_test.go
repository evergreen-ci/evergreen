package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDurationBasedNewHostsNeeded(t *testing.T) {
	/*
		Note that this is a functional test and its validity relies on the
		values of:
		1. MaxDurationPerDistroHost
		2. SharedTasksAllocationProportion
	*/

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

		distroSlice := []distro.Distro{
			{
				Id:       distroIds[0],
				Provider: "static",
				PoolSize: 5,
			},
			{
				Id:       distroIds[1],
				Provider: "ec2",
				PoolSize: 10,
			},
			{
				Id:       distroIds[2],
				Provider: "ec2",
				PoolSize: 12,
			},
		}

		taskDurations := map[string]time.Duration{
			taskIds[0]: expDurs[0],
			taskIds[1]: expDurs[1],
			taskIds[2]: expDurs[2],
			taskIds[3]: expDurs[3],
			taskIds[4]: expDurs[4],
			taskIds[5]: expDurs[5],
			taskIds[6]: expDurs[6],
		}

		hosts := [][]host.Host{
			{
				{Id: hostIds[0]},
				{Id: hostIds[1]},
				{Id: hostIds[2]},
			},
			{
				{Id: hostIds[3]},
				{Id: hostIds[4]},
				{Id: hostIds[5]},
			},
			{},
		}

		distroQueueInfo := DistroQueueInfo{
			Distro:        distroSlice[0],
			TaskDurations: taskDurations,
		}

		Convey("ensure that the distro schedule data is used to spin "+
			"up new hosts if needed",
			func() {
				hostAllocatorData := HostAllocatorData{
					ExistingHosts:   hosts[0],
					Distro:          distroSlice[0],
					DistroQueueInfo: distroQueueInfo,
				}

				// integration test of duration based host allocator
				newHostsNeeded, err := DurationBasedHostAllocator(ctx, hostAllocatorData)

				So(err, ShouldBeNil)

				// only distros with only static hosts should be zero
				So(newHostsNeeded, ShouldEqual, 0)
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

		Convey("when the durations of existing tasks is only slightly more than "+
			"the maximum duration, exactly one host is needed", func() {
			scheduledTasksDuration := 12.
			runningTasksDuration := 13.
			numExistingHosts := 0.
			maxDurationPerHost := time.Duration(24) * time.Second
			numNewHosts := computeDurationBasedNumNewHosts(
				scheduledTasksDuration, runningTasksDuration,
				numExistingHosts, maxDurationPerHost)
			So(numNewHosts, ShouldEqual, 1)
		})
	})
}

func TestComputeRunningTasksDuration(t *testing.T) {
	var hostIds []string
	var runningTaskIds []string

	Convey("When calling computeRunningTasksDuration...", t, func() {
		// set all variables
		hostIds = []string{"h1", "h2", "h3", "h4", "h5", "h6"}
		runningTaskIds = []string{"t1", "t2", "t3", "t4", "t5", "t6"}

		startTimeOne := time.Now()
		startTimeTwo := startTimeOne.Add(-time.Duration(1) * time.Minute)
		startTimeThree := startTimeOne.Add(-time.Duration(2) * time.Minute)

		remainingDurationOne := 4 * time.Minute
		remainingDurationTwo := 3 * time.Minute
		remainingDurationThree := 2 * time.Minute

		remainingDurationOneSecs := remainingDurationOne.Seconds()
		remainingDurationTwoSecs := remainingDurationTwo.Seconds()

		So(db.Clear(task.Collection), ShouldBeNil)

		Convey("the total duration of running tasks with similar start times "+
			"should be the total of the remaining time using estimates from "+
			"the project task duration data for running tasks", func() {
			// tasks running on hosts
			runningTasks := []task.Task{
				{Id: runningTaskIds[0], StartTime: startTimeOne, DurationPrediction: util.CachedDurationValue{Value: remainingDurationOne}},
				{Id: runningTaskIds[1], StartTime: startTimeOne, DurationPrediction: util.CachedDurationValue{Value: remainingDurationOne}},
				{Id: runningTaskIds[2], StartTime: startTimeOne, DurationPrediction: util.CachedDurationValue{Value: remainingDurationOne}},
			}

			for _, runningTask := range runningTasks {
				So(runningTask.Insert(), ShouldBeNil)
			}

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
				{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}

			runningTasksDuration, err := computeRunningTasksDuration(existingDistroHosts)

			So(err, ShouldBeNil)

			// the running task duration should be a total of the remaining
			// duration of running tasks - 3 in this case
			// due to scheduling variables, we allow a 10 second tolerance
			So(runningTasksDuration, ShouldAlmostEqual, remainingDurationOneSecs*3, 10)
		})

		Convey("the total duration of running tasks with different start "+
			"times should be the total of the remaining time using estimates "+
			"from the project task duration data for running tasks", func() {

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				{Id: hostIds[0], RunningTask: runningTaskIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[1]},
				{Id: hostIds[2], RunningTask: runningTaskIds[2]},
				{Id: hostIds[3], RunningTask: runningTaskIds[3]},
				{Id: hostIds[4], RunningTask: runningTaskIds[4]},
				{Id: hostIds[5], RunningTask: runningTaskIds[5]},
			}

			// tasks running on hosts
			runningTasks := []task.Task{
				{Id: runningTaskIds[0], StartTime: startTimeThree, DurationPrediction: util.CachedDurationValue{Value: remainingDurationThree + time.Second}},
				{Id: runningTaskIds[1], StartTime: startTimeTwo, DurationPrediction: util.CachedDurationValue{Value: remainingDurationTwo}},
				{Id: runningTaskIds[2], StartTime: startTimeOne, DurationPrediction: util.CachedDurationValue{Value: remainingDurationOne}},
				{Id: runningTaskIds[3], StartTime: startTimeTwo, DurationPrediction: util.CachedDurationValue{Value: remainingDurationTwo}},
				{Id: runningTaskIds[4], StartTime: startTimeOne, DurationPrediction: util.CachedDurationValue{Value: remainingDurationOne}},
				{Id: runningTaskIds[5], StartTime: startTimeThree, DurationPrediction: util.CachedDurationValue{Value: remainingDurationThree + time.Second}},
			}

			for _, runningTask := range runningTasks {
				So(runningTask.Insert(), ShouldBeNil)
			}

			runningTasksDuration, err := computeRunningTasksDuration(existingDistroHosts)
			So(err, ShouldBeNil)
			// the running task duration should be a total of the remaining
			// duration of running tasks - 6 in this case
			// due to scheduling variables, we allow a 5 second tolerance
			totalRunTime := 2 * (remainingDurationOne + remainingDurationTwo + remainingDurationThree)
			timeSpent := 2 * (time.Since(startTimeOne) + time.Since(startTimeTwo) + time.Since(startTimeThree))
			expectedResult := (totalRunTime - timeSpent).Seconds()
			So(runningTasksDuration, ShouldAlmostEqual, expectedResult, 5)
		})

		Convey("the duration of running tasks with unknown running time "+
			"estimates should be accounted for", func() {

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				{Id: hostIds[0], RunningTask: runningTaskIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[1]},
				{Id: hostIds[2], RunningTask: runningTaskIds[2]},
			}

			// tasks running on hosts
			runningTasks := []task.Task{
				{Id: runningTaskIds[0], StartTime: startTimeThree, DisplayName: "unknown", DurationPrediction: util.CachedDurationValue{Value: 0}},
				{Id: runningTaskIds[1], StartTime: startTimeTwo, DurationPrediction: util.CachedDurationValue{Value: remainingDurationTwo}},
				{Id: runningTaskIds[2], StartTime: startTimeOne, DisplayName: "unknown", DurationPrediction: util.CachedDurationValue{Value: 0}},
			}

			for _, runningTask := range runningTasks {
				So(runningTask.Insert(), ShouldBeNil)
			}

			runningTasksDuration, err := computeRunningTasksDuration(existingDistroHosts)
			So(err, ShouldBeNil)
			// only task 1's duration is known, so the others should use the default.
			expectedDur := remainingDurationTwoSecs + float64((2*10*time.Minute)/time.Second)
			So(runningTasksDuration, ShouldAlmostEqual, expectedDur, 200)
		})

		Convey("the duration of running tasks with outliers as running times "+
			"should be ignored", func() {

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				{Id: hostIds[0], RunningTask: runningTaskIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[1]},
				{Id: hostIds[2], RunningTask: runningTaskIds[2]},
			}

			// tasks running on hosts
			runningTasks := []task.Task{
				{Id: runningTaskIds[0], StartTime: startTimeOne, DurationPrediction: util.CachedDurationValue{Value: remainingDurationOne}},
				{Id: runningTaskIds[1], StartTime: startTimeOne.Add(-4 * time.Hour), DurationPrediction: util.CachedDurationValue{Value: remainingDurationOne}},
				{Id: runningTaskIds[2], StartTime: startTimeTwo, DurationPrediction: util.CachedDurationValue{Value: remainingDurationTwo}},
			}

			for _, runningTask := range runningTasks {
				So(runningTask.Insert(), ShouldBeNil)
			}

			runningTasksDuration, err := computeRunningTasksDuration(existingDistroHosts)
			So(err, ShouldBeNil)
			// task 2's duration should be ignored
			// due to scheduling variables, we allow a 5 second tolerance
			expectedResult := (remainingDurationOne + remainingDurationTwo) - (time.Since(startTimeOne) + time.Since(startTimeTwo))
			So(runningTasksDuration, ShouldAlmostEqual, expectedResult.Seconds(), 5)
		})

		Convey("the total duration if there are no running tasks should be "+
			"zero", func() {

			// running tasks have a time to completion of about 1 minute
			existingDistroHosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1]},
				{Id: hostIds[2]},
				{Id: hostIds[3]},
			}

			runningTasksDuration, err := computeRunningTasksDuration(existingDistroHosts)
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
	taskDurations := make(map[string]time.Duration)

	var tasksAccountedFor map[string]bool

	Convey("When calling computeScheduledTasksDuration...", t, func() {
		Convey("the total scheduled tasks duration should equal the duration "+
			"of all tasks scheduled, for that distro, in the queue", func() {
			tasks = []string{"t1", "t2", "t3", "t4", "t5", "t6"}
			expDur = time.Duration(180) * time.Minute
			tasksAccountedFor = make(map[string]bool)

			taskDurations = map[string]time.Duration{
				tasks[0]: expDur,
				tasks[1]: expDur,
				tasks[2]: expDur,
				tasks[3]: expDur,
				tasks[4]: expDur,
			}

			// construct the data needed by computeScheduledTasksDuration
			scheduledDistroTasksData := &ScheduledDistroTasksData{
				tasksAccountedFor: tasksAccountedFor,
				taskDurations:     taskDurations,
			}

			scheduledTasksDuration, _ := computeScheduledTasksDuration(
				scheduledDistroTasksData)

			expectedTotalDuration := float64(len(taskDurations)) * expDur.Seconds()
			So(scheduledTasksDuration, ShouldEqual, expectedTotalDuration)
		})

		Convey("the map of tasks accounted for should be updated", func() {
			tasks = []string{"t1", "t2", "t3", "t4", "t5", "t6"}
			expDur = time.Duration(180) * time.Minute
			tasksAccountedFor = make(map[string]bool)

			taskDurations = map[string]time.Duration{
				tasks[0]: expDur,
				tasks[1]: expDur,
				tasks[2]: expDur,
				tasks[3]: expDur,
				tasks[4]: expDur,
			}

			// construct the data needed by computeScheduledTasksDuration
			scheduledDistroTasksData := &ScheduledDistroTasksData{
				taskDurations:     taskDurations,
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

			distroOneTaskDurations := map[string]time.Duration{
				tasks[0]: expDur,
				tasks[1]: expDur,
				tasks[2]: expDur,
				tasks[3]: expDur,
				tasks[4]: expDur,
			}

			// construct the data needed by computeScheduledTasksDuration
			scheduledDistroTasksData := &ScheduledDistroTasksData{
				taskDurations:     distroOneTaskDurations,
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
			// distroTwoQueueItems := []model.TaskQueueItem{
			// 	{Id: tasks[0], ExpectedDuration: expDur},
			// 	{Id: tasks[5], ExpectedDuration: expDur},
			// }
			distroTwoTaskDurations := map[string]time.Duration{
				tasks[0]: expDur,
				tasks[5]: expDur,
			}
			expectedTasksAccountedFor[tasks[5]] = true

			// construct the data needed by computeScheduledTasksDuration
			scheduledDistroTasksData = &ScheduledDistroTasksData{
				taskDurations:     distroTwoTaskDurations,
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a duration based host allocator,"+
		" determining the number of new hosts to spin up", t, func() {

		taskIds = []string{"t1", "t2", "t3", "t4", "t5"}
		runningTaskIds = []string{"t1", "t2", "t3", "t4", "t5"}
		hostIds = []string{"h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8", "h9"}
		dist = distro.Distro{Provider: "ec2"}

		So(db.Clear(task.Collection), ShouldBeNil)

		Convey("if there are no tasks to run, no new hosts should be needed",
			func() {
				hosts := []host.Host{
					{Id: hostIds[0]},
					{Id: hostIds[1]},
					{Id: hostIds[2]},
				}
				dist.PoolSize = len(hosts) + 5

				hostAllocatorData := &HostAllocatorData{
					ExistingHosts: hosts,
					Distro:        dist,
				}

				tasksAccountedFor := make(map[string]bool)
				distroScheduleData := make(map[string]DistroScheduleData)

				newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
					tasksAccountedFor, distroScheduleData)
				So(err, ShouldBeNil)
				So(newHosts, ShouldEqual, 0)
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

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
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

			tasksAccountedFor = make(map[string]bool)
			distroScheduleData = make(map[string]DistroScheduleData)

			newHosts, err = durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
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

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
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
				{Id: hostIds[1]},
				{Id: hostIds[2]},
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

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)

		})

		Convey("if the number of tasks to run is equal to the number of free"+
			" hosts, no new hosts are needed", func() {
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
			}

			var duration time.Duration
			taskDurations := map[string]time.Duration{
				taskIds[0]: duration,
				taskIds[1]: duration,
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

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				t := task.Task{Id: runningTaskId}
				So(t.Insert(), ShouldBeNil)
			}

			newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
		})

		Convey("if the number of tasks to run exceeds the number of free"+
			" hosts, new hosts are needed up to the maximum allowed for the"+
			" dist", func() {
			expDur := time.Duration(200) * time.Minute
			taskDurations := map[string]time.Duration{
				taskIds[0]: expDur,
				taskIds[1]: expDur,
				taskIds[2]: expDur,
				taskIds[3]: expDur,
				taskIds[4]: expDur,
			}
			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
				{Id: hostIds[4], RunningTask: runningTaskIds[2]},
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

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}
			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				t := task.Task{Id: runningTaskId}
				So(t.Insert(), ShouldBeNil)
			}

			// total running duration here is
			newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 3)

			dist.PoolSize = 8

			distroQueueInfo = DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
			}

			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			tasksAccountedFor = make(map[string]bool)
			distroScheduleData = make(map[string]DistroScheduleData)

			newHosts, err = durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 3)
			dist.PoolSize = 7

			distroQueueInfo = DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
			}

			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			tasksAccountedFor = make(map[string]bool)
			distroScheduleData = make(map[string]DistroScheduleData)

			newHosts, err = durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 2)

			dist.PoolSize = 6

			distroQueueInfo = DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
			}

			hostAllocatorData = &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			tasksAccountedFor = make(map[string]bool)

			newHosts, err = durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 1)
		})

		Convey("if the distro cannot be used to spawn hosts, then no new "+
			"hosts can be spawned", func() {
			expDur := time.Duration(200) * time.Minute
			taskDurations := map[string]time.Duration{
				taskIds[0]: expDur,
				taskIds[1]: expDur,
				taskIds[2]: expDur,
				taskIds[3]: expDur,
				taskIds[4]: expDur,
			}

			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1]},
				{Id: hostIds[2]},
				{Id: hostIds[3]},
				{Id: hostIds[4]},
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

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 0)
		})

		Convey("if the duration based estimate is less than the maximum "+
			"\nnumber of new hosts allowed for this distro, the estimate of new "+
			"\nhosts should be used", func() {
			expDur := time.Duration(200) * time.Minute
			taskDurations := map[string]time.Duration{
				taskIds[0]: expDur,
				taskIds[1]: expDur,
				taskIds[2]: expDur,
				taskIds[3]: expDur,
				taskIds[4]: expDur,
			}

			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
				{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}
			dist.PoolSize = 20

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				t := task.Task{Id: runningTaskId}
				So(t.Insert(), ShouldBeNil)
			}

			newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 3)
		})

		Convey("if the duration based estimate is less than the maximum "+
			"\nnumber of new hosts allowed for this distro, but greater than "+
			"\nthe difference between the number of runnable tasks and the "+
			"\nnumber of free hosts, that difference should be used", func() {
			expDur := time.Duration(400) * time.Minute
			// all runnable tasks have an expected duration of expDur (200mins)
			taskDurations := map[string]time.Duration{
				taskIds[0]: expDur,
				taskIds[1]: expDur,
				taskIds[2]: expDur,
				taskIds[3]: expDur,
				taskIds[4]: expDur,
			}

			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
				{Id: hostIds[4], RunningTask: runningTaskIds[2]},
			}
			dist.PoolSize = 20

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				t := task.Task{Id: runningTaskId}
				So(t.Insert(), ShouldBeNil)
			}

			// estimates based on data
			// duration estimate: 11
			// max new hosts allowed: 15
			// 'one-host-per-scheduled-task': 3
			newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
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
			taskDurations := map[string]time.Duration{
				taskIds[0]: expDur,
				taskIds[1]: expDur,
				taskIds[2]: expDur,
				taskIds[3]: expDur,
				taskIds[4]: expDur,
			}

			// running tasks have a time to completion of about 1 minute
			hosts := []host.Host{
				{Id: hostIds[0]},
				{Id: hostIds[1], RunningTask: runningTaskIds[0]},
				{Id: hostIds[2], RunningTask: runningTaskIds[1]},
				{Id: hostIds[3]},
				{Id: hostIds[4], RunningTask: runningTaskIds[2]},
				{Id: hostIds[5]},
			}
			dist.PoolSize = 20

			distroQueueInfo := DistroQueueInfo{
				Distro:        dist,
				TaskDurations: taskDurations,
			}

			hostAllocatorData := &HostAllocatorData{
				Distro:          dist,
				ExistingHosts:   hosts,
				DistroQueueInfo: distroQueueInfo,
			}

			tasksAccountedFor := make(map[string]bool)
			distroScheduleData := make(map[string]DistroScheduleData)

			// tasks running on hosts
			for _, runningTaskId := range runningTaskIds {
				t := task.Task{Id: runningTaskId}
				So(t.Insert(), ShouldBeNil)
			}

			// estimates based on data
			// duration estimate: 2
			// max new hosts allowed: 15
			// 'one-host-per-scheduled-task': 3
			newHosts, err := durationNumNewHostsForDistro(ctx, hostAllocatorData,
				tasksAccountedFor, distroScheduleData)
			So(err, ShouldBeNil)
			So(newHosts, ShouldEqual, 2)
		})
	})

}
