package scheduler

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
)

var taskDurationEstimatorTestConf = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskDurationEstimatorTestConf))
	grip.CatchError(grip.SetSender(testutil.SetupTestSender(taskDurationEstimatorTestConf.Scheduler.LogFile)))
}

func TestDBTaskDurationEstimator(t *testing.T) {
	var taskDurationEstimator *DBTaskDurationEstimator
	var displayNames []string
	var buildVariants []string
	var projects []string
	var taskIds []string
	var timeTaken []time.Duration
	var runnableTasks []task.Task

	Convey("With a DBTaskDurationEstimator...", t,
		func() {
			taskDurationEstimator = &DBTaskDurationEstimator{}
			taskIds = []string{"t1", "t2", "t3", "t4", "t5", "t6",
				"t7", "t8", "t9"}
			displayNames = []string{"dn1", "dn2", "dn3", "dn4", "dn5", "dn6"}
			buildVariants = []string{"bv1", "bv2", "bv3", "bv4", "bv5", "bv6"}
			projects = []string{"p1", "p2", "p3", "p4", "p5", "p6"}
			timeTaken = []time.Duration{
				time.Duration(1) * time.Minute, time.Duration(2) * time.Minute,
				time.Duration(3) * time.Minute, time.Duration(4) * time.Minute,
				time.Duration(5) * time.Minute, time.Duration(6) * time.Minute,
			}

			Convey("the expected task duration for tasks with different "+
				"display names, same project and buildvariant, should be "+
				"distinct",
				func() {
					runnableTasks = []task.Task{
						// task 0/7 are from the same project and buildvariant
						// and have different display names - expected duration
						// should not be averaged between these two
						{
							Id:           taskIds[0],
							DisplayName:  displayNames[0],
							BuildVariant: buildVariants[0],
							TimeTaken:    timeTaken[0],
							Project:      projects[0],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[0]),
							FinishTime:   time.Now(),
						},
						{
							Id:           taskIds[7],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[0],
							TimeTaken:    timeTaken[4],
							Project:      projects[0],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[4]),
							FinishTime:   time.Now(),
						},
					}

					So(db.Clear(task.Collection), ShouldBeNil)

					// insert all the test tasks
					for _, testTask := range runnableTasks {
						testutil.HandleTestingErr(testTask.Insert(), t, "failed to "+
							"insert task")
					}

					taskDurations, err := taskDurationEstimator.
						GetExpectedDurations(runnableTasks)

					So(err, ShouldEqual, nil)

					projectDurations := taskDurations.TaskDurationByProject
					// we only have 1 project for all runnable tasks
					So(len(projectDurations), ShouldEqual, 1)

					bvDurs := projectDurations[projects[0]].
						TaskDurationByBuildVariant
					// we only have 1 buildvariant for all runnable tasks
					So(len(bvDurs), ShouldEqual, 1)

					tkDurs := bvDurs[buildVariants[0]].TaskDurationByDisplayName
					// we have 2 runnable tasks
					So(len(tkDurs), ShouldEqual, 2)

					// ensure the expected durations are as expected
					So(tkDurs[displayNames[0]], ShouldEqual, timeTaken[0])
					So(tkDurs[displayNames[1]], ShouldEqual, timeTaken[4])
				},
			)

			Convey("the expected task duration for tasks with same display "+
				"name same project but different buildvariant should be "+
				"distinct",
				func() {
					runnableTasks = []task.Task{
						// task 1/4 are from the same project but from different
						// buildvariants and have the same display names -
						// expected duration should not be averaged between
						// these two
						{
							Id:           taskIds[1],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[3],
							TimeTaken:    timeTaken[2],
							Project:      projects[2],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[2]),
							FinishTime:   time.Now(),
						},
						{
							Id:           taskIds[4],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[4],
							TimeTaken:    timeTaken[0],
							Project:      projects[2],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[0]),
							FinishTime:   time.Now(),
						},
					}

					So(db.Clear(task.Collection), ShouldBeNil)

					// insert all the test tasks
					for _, testTask := range runnableTasks {
						testutil.HandleTestingErr(testTask.Insert(), t, "failed to "+
							" insert task")
					}

					taskDurations, err := taskDurationEstimator.
						GetExpectedDurations(runnableTasks)

					So(err, ShouldEqual, nil)

					projectDurations := taskDurations.TaskDurationByProject
					// we only have 1 project for all runnable tasks
					So(len(projectDurations), ShouldEqual, 1)

					bvDurs := projectDurations[projects[2]].
						TaskDurationByBuildVariant
					// we have 2 buildvariants for project 2
					So(len(bvDurs), ShouldEqual, 2)

					tkDurs := bvDurs[buildVariants[3]].TaskDurationByDisplayName
					// we have 1 runnable task for buildvariant 3
					So(len(tkDurs), ShouldEqual, 1)

					// ensure the expected duration is as expected
					So(tkDurs[displayNames[1]], ShouldEqual, timeTaken[2])

					tkDurs = bvDurs[buildVariants[4]].TaskDurationByDisplayName
					// we have 1 runnable task for buildvariant 4
					So(len(tkDurs), ShouldEqual, 1)

					// ensure the expected duration is as expected
					So(tkDurs[displayNames[1]], ShouldEqual, timeTaken[0])
				},
			)

			Convey("the expected task duration for tasks with same display "+
				"name same buildvariant and from same project should be "+
				"averaged",
				func() {
					runnableTasks = []task.Task{
						// task 2/3 are from the same buildvariant, have the
						// same display name amd are from same projects -
						// expected duration should be average across those
						// projects
						{
							Id:           taskIds[3],
							DisplayName:  displayNames[3],
							BuildVariant: buildVariants[3],
							TimeTaken:    timeTaken[3],
							Project:      projects[3],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[3]),
							FinishTime:   time.Now(),
						},
						{
							Id:           taskIds[4],
							DisplayName:  displayNames[3],
							BuildVariant: buildVariants[3],
							TimeTaken:    timeTaken[4],
							Project:      projects[3],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[4]),
							FinishTime:   time.Now(),
						},
					}

					So(db.Clear(task.Collection), ShouldBeNil)

					// insert all the test tasks
					for _, testTask := range runnableTasks {
						testutil.HandleTestingErr(testTask.Insert(), t, "failed to "+
							"insert task")
					}

					taskDurations, err := taskDurationEstimator.
						GetExpectedDurations(runnableTasks)

					So(err, ShouldEqual, nil)

					projectDurations := taskDurations.TaskDurationByProject
					// we have 1 project for all runnable tasks
					So(len(projectDurations), ShouldEqual, 1)

					// check the results for project 4
					bvDurs := projectDurations[projects[3]].
						TaskDurationByBuildVariant
					// we have 1 buildvariant for project 3
					So(len(bvDurs), ShouldEqual, 1)

					tkDurs := bvDurs[buildVariants[3]].TaskDurationByDisplayName
					// we have 1 task duration for display name tasks
					// for buildvariant 3
					So(len(tkDurs), ShouldEqual, 1)

					// ensure the expected duration is as expected
					expDur := (timeTaken[3] + timeTaken[4]) / 2
					So(tkDurs[displayNames[3]], ShouldEqual, expDur)
				},
			)

			Convey("the expected task duration for tasks with same display "+
				"name same buildvariant but from different project should be "+
				"distinct",
				func() {
					runnableTasks = []task.Task{
						// task 4/5 are from the same buildvariant, have the
						// same display name but are from different projects -
						// expected duration should be distinct across those
						// projects
						{
							Id:           taskIds[4],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[1],
							TimeTaken:    timeTaken[4],
							Project:      projects[4],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[4]),
							FinishTime:   time.Now(),
						},
						{
							Id:           taskIds[5],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[1],
							TimeTaken:    timeTaken[5],
							Project:      projects[5],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[5]),
							FinishTime:   time.Now(),
						},
					}

					So(db.Clear(task.Collection), ShouldBeNil)

					// insert all the test tasks
					for _, testTask := range runnableTasks {
						testutil.HandleTestingErr(testTask.Insert(), t, "failed to "+
							"insert task")
					}

					taskDurations, err := taskDurationEstimator.
						GetExpectedDurations(runnableTasks)

					So(err, ShouldEqual, nil)

					projectDurations := taskDurations.TaskDurationByProject
					// we have 2 projects for all runnable tasks
					So(len(projectDurations), ShouldEqual, 2)

					// check the results for project 4
					bvDurs := projectDurations[projects[4]].
						TaskDurationByBuildVariant
					// we have 1 buildvariant for project 4
					So(len(bvDurs), ShouldEqual, 1)

					tkDurs := bvDurs[buildVariants[1]].TaskDurationByDisplayName
					// we have 1 runnable task for buildvariant 1
					So(len(tkDurs), ShouldEqual, 1)

					// ensure the expected duration is as expected
					So(tkDurs[displayNames[1]], ShouldEqual, timeTaken[4])

					// check the results for project 5
					bvDurs = projectDurations[projects[5]].
						TaskDurationByBuildVariant
					// we have 1 buildvariant for project 4
					So(len(bvDurs), ShouldEqual, 1)

					tkDurs = bvDurs[buildVariants[1]].TaskDurationByDisplayName
					// we have 1 runnable task for buildvariant 1
					So(len(tkDurs), ShouldEqual, 1)

					// ensure the expected duration is as expected
					So(tkDurs[displayNames[1]], ShouldEqual, timeTaken[5])
				},
			)

			Convey("the expected task duration for tasks should only pick up "+
				"tasks that are completed",
				func() {
					runnableTasks = []task.Task{
						// task 4/5 are from the same buildvariant, have the
						// same display name and are from the same project -
						// however task 4 is not yet dispatched so shouldn't
						// count toward the expected duration
						{
							Id:           taskIds[4],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[1],
							TimeTaken:    timeTaken[4],
							Project:      projects[4],
							Status:       evergreen.TaskUndispatched,
							StartTime:    time.Now().Add(-timeTaken[4]),
							FinishTime:   time.Now(),
						},
						{
							Id:           taskIds[5],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[1],
							TimeTaken:    timeTaken[5],
							Project:      projects[4],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[5]),
							FinishTime:   time.Now(),
						},
					}

					So(db.Clear(task.Collection), ShouldBeNil)

					// insert all the test tasks
					for _, testTask := range runnableTasks {
						testutil.HandleTestingErr(testTask.Insert(), t, "failed to "+
							"insert task")
					}

					taskDurations, err := taskDurationEstimator.
						GetExpectedDurations(runnableTasks)

					So(err, ShouldEqual, nil)

					projectDurations := taskDurations.TaskDurationByProject
					// we have 1 project for all runnable tasks
					So(len(projectDurations), ShouldEqual, 1)

					// check the results for project 4
					bvDurs := projectDurations[projects[4]].
						TaskDurationByBuildVariant

					// we have 1 buildvariant for project 4
					So(len(bvDurs), ShouldEqual, 1)

					tkDurs := bvDurs[buildVariants[1]].TaskDurationByDisplayName
					// we have 1 runnable task for buildvariant 1
					// since task 4 is undispatched
					So(len(tkDurs), ShouldEqual, 1)

					// ensure the expected duration is that of task 5
					So(tkDurs[displayNames[1]], ShouldEqual, timeTaken[5])
				},
			)

			Convey("the expected task duration for tasks should only pick up "+
				"tasks that are completed within the specified window",
				func() {
					runnableTasks = []task.Task{
						// task 4/5 are from the same buildvariant, have the
						// same display name and are from the same project -
						// however task 4 did not finish within the specified
						// window and should thus, not count toward the expected
						//  duration
						{
							Id:           taskIds[4],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[1],
							TimeTaken:    timeTaken[4],
							Project:      projects[4],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[4]),
							FinishTime: time.Now().
								Add(-model.TaskCompletionEstimateWindow).
								Add(-model.TaskCompletionEstimateWindow),
						},
						{
							Id:           taskIds[5],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[1],
							TimeTaken:    timeTaken[3],
							Project:      projects[4],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[3]),
							FinishTime:   time.Now(),
						},
					}

					So(db.Clear(task.Collection), ShouldBeNil)

					// insert all the test tasks
					for _, testTask := range runnableTasks {
						testutil.HandleTestingErr(testTask.Insert(), t, "failed to "+
							"insert task")
					}

					taskDurations, err := taskDurationEstimator.
						GetExpectedDurations(runnableTasks)

					So(err, ShouldEqual, nil)

					projectDurations := taskDurations.TaskDurationByProject
					// we have 1 project for all runnable tasks
					So(len(projectDurations), ShouldEqual, 1)

					// check the results for project 4
					bvDurs := projectDurations[projects[4]].
						TaskDurationByBuildVariant

					// we have 1 buildvariant for project 4
					So(len(bvDurs), ShouldEqual, 1)

					tkDurs := bvDurs[buildVariants[1]].TaskDurationByDisplayName
					// we have 1 task for buildvariant 1
					// since task 4 is did not finish within the window
					So(len(tkDurs), ShouldEqual, 1)

					// ensure the expected duration is that of task 5
					So(tkDurs[displayNames[1]], ShouldEqual, timeTaken[3])
				},
			)

			Convey("the expected task duration for tasks where several "+
				"projects that contain a mix of completed/uncompleted, for \n"+
				"several buildvariants/tasks should ignore uncompleted tasks "+
				"and average tasks with the same display name",
				func() {
					runnableTasks = []task.Task{
						// task 0/7 are from the same project and buildvariant
						// and have different display names - expected duration
						// should thus be averaged between these two - even
						// though task 0 succeeded and 7 failed
						{
							Id:           taskIds[0],
							DisplayName:  displayNames[0],
							BuildVariant: buildVariants[0],
							TimeTaken:    timeTaken[0],
							Project:      projects[0],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[0]),
							FinishTime:   time.Now(),
						},
						{
							Id:           taskIds[7],
							DisplayName:  displayNames[0],
							BuildVariant: buildVariants[0],
							TimeTaken:    timeTaken[4],
							Project:      projects[0],
							Status:       evergreen.TaskFailed,
							StartTime:    time.Now().Add(-timeTaken[4]),
							FinishTime:   time.Now(),
						},
						// task 1 is the only runnable task with this project,
						// buildvariant & display name
						// should be exactly equal to its time taken
						{
							Id:           taskIds[1],
							DisplayName:  displayNames[1],
							BuildVariant: buildVariants[1],
							TimeTaken:    timeTaken[1],
							Project:      projects[1],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[1]),
							FinishTime:   time.Now(),
						},
						// task 2 is the only runnable task with this project,
						// buildvariant & display name - so the expected
						// duration should be exactly equal to its time taken
						{
							Id:           taskIds[2],
							DisplayName:  displayNames[2],
							BuildVariant: buildVariants[2],
							TimeTaken:    timeTaken[2],
							Project:      projects[2],
							Status:       evergreen.TaskFailed,
							StartTime:    time.Now().Add(-timeTaken[2]),
							FinishTime:   time.Now(),
						},
						// task 3/6 are from the same project and buildvariant
						// but have different display names - expected duration
						// should thus not be averaged between these two
						{
							Id:           taskIds[3],
							DisplayName:  displayNames[3],
							BuildVariant: buildVariants[3],
							TimeTaken:    timeTaken[3],
							Project:      projects[3],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[3]),
							FinishTime:   time.Now(),
						},
						{
							Id:           taskIds[6],
							DisplayName:  displayNames[4],
							BuildVariant: buildVariants[3],
							TimeTaken:    timeTaken[5],
							Project:      projects[3],
							Status:       evergreen.TaskSucceeded,
							StartTime:    time.Now().Add(-timeTaken[5]),
							FinishTime:   time.Now(),
						},
						// task 4/5 are from the same project and buildvariant
						// and have the same display name - expected duration
						// should thus be averaged between these two
						{
							Id:           taskIds[4],
							DisplayName:  displayNames[4],
							BuildVariant: buildVariants[4],
							TimeTaken:    timeTaken[4],
							Project:      projects[4],
							Status:       evergreen.TaskFailed,
							StartTime:    time.Now().Add(-timeTaken[4]),
							FinishTime:   time.Now(),
						},
						{
							Id:           taskIds[5],
							DisplayName:  displayNames[4],
							BuildVariant: buildVariants[4],
							TimeTaken:    timeTaken[5],
							Project:      projects[4],
							Status:       evergreen.TaskFailed,
							StartTime:    time.Now().Add(-timeTaken[5]),
							FinishTime:   time.Now(),
						},
						// task 9 is undispatched so shouldn't
						// count toward the expected duration for
						// project/buildvariant/display name 4
						{
							Id:           taskIds[8],
							DisplayName:  displayNames[4],
							BuildVariant: buildVariants[4],
							TimeTaken:    timeTaken[5],
							Project:      projects[4],
							Status:       evergreen.TaskUndispatched,
							StartTime:    time.Now().Add(-timeTaken[5]),
							FinishTime:   time.Now(),
						},
					}

					So(db.Clear(task.Collection), ShouldBeNil)

					// insert all the test tasks
					for _, testTask := range runnableTasks {
						testutil.HandleTestingErr(testTask.Insert(), t, "failed to "+
							"insert task")
					}

					taskDurations, err := taskDurationEstimator.
						GetExpectedDurations(runnableTasks)
					testutil.HandleTestingErr(err, t, "failed to get task "+
						"durations")
					projectDurations := taskDurations.
						TaskDurationByProject

					// we have five projects for all runnable tasks
					So(len(projectDurations), ShouldEqual, 5)
					taskDuration := projectDurations[projects[0]].
						TaskDurationByBuildVariant[buildVariants[0]].
						TaskDurationByDisplayName[displayNames[0]]
					So(taskDuration, ShouldEqual,
						(timeTaken[0]+timeTaken[4])/2)
					taskDuration = projectDurations[projects[1]].
						TaskDurationByBuildVariant[buildVariants[1]].
						TaskDurationByDisplayName[displayNames[1]]
					So(taskDuration, ShouldEqual, timeTaken[1])
					taskDuration = projectDurations[projects[2]].
						TaskDurationByBuildVariant[buildVariants[2]].
						TaskDurationByDisplayName[displayNames[2]]
					So(taskDuration, ShouldEqual, timeTaken[2])
					taskDuration = projectDurations[projects[3]].
						TaskDurationByBuildVariant[buildVariants[3]].
						TaskDurationByDisplayName[displayNames[3]]
					So(taskDuration, ShouldEqual, timeTaken[3])
					taskDuration = projectDurations[projects[4]].
						TaskDurationByBuildVariant[buildVariants[4]].
						TaskDurationByDisplayName[displayNames[4]]
					// should be avearge of tasks 4/5
					So(taskDuration, ShouldEqual,
						(timeTaken[4]+timeTaken[5])/2)
				},
			)
		},
	)
}
