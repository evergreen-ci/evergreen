package model

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	oneMs = time.Millisecond
)

func TestSetActiveState(t *testing.T) {
	Convey("With one task with no dependencies", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, task.OldCollection),
			"Error clearing task and build collections")
		var err error

		displayName := "testName"
		userName := "testUser"
		testTime := time.Now()
		b := &build.Build{
			Id: "buildtest",
		}
		testTask := &task.Task{
			Id:            "testone",
			DisplayName:   displayName,
			ScheduledTime: testTime,
			Activated:     false,
			BuildId:       b.Id,
			DistroId:      "arch",
		}
		b.Tasks = []build.TaskCache{{Id: testTask.Id}}

		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)

		Convey("activating the task should set the task state to active", func() {
			So(SetActiveState(testTask.Id, "randomUser", true), ShouldBeNil)
			testTask, err = task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Activated, ShouldBeTrue)
			So(testTask.ScheduledTime, ShouldHappenWithin, oneMs, testTime)

			Convey("deactivating an active task as a normal user should deactivate the task", func() {
				So(SetActiveState(testTask.Id, userName, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(testTask.Activated, ShouldBeFalse)
			})
		})
		Convey("when deactivating an active task as evergreen", func() {
			Convey("if the task is activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(testTask.Id, evergreen.DefaultTaskActivator, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.DefaultTaskActivator)
				So(SetActiveState(testTask.Id, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
			})
			Convey("if the task is activated by stepback user, the task should not deactivate", func() {
				So(SetActiveState(testTask.Id, evergreen.StepbackTaskActivator, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.StepbackTaskActivator)
				So(SetActiveState(testTask.Id, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, true)
			})
			Convey("if the task is not activated by evergreen, the task should not deactivate", func() {
				So(SetActiveState(testTask.Id, userName, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, userName)
				So(SetActiveState(testTask.Id, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, true)
			})

		})
		Convey("when deactivating an active task a normal user", func() {
			u := "test_user"
			Convey("if the task is activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(testTask.Id, evergreen.DefaultTaskActivator, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.DefaultTaskActivator)
				So(SetActiveState(testTask.Id, u, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
			})
			Convey("if the task is activated by stepback user, the task should deactivate", func() {
				So(SetActiveState(testTask.Id, evergreen.StepbackTaskActivator, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.StepbackTaskActivator)
				So(SetActiveState(testTask.Id, u, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
			})
			Convey("if the task is not activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(testTask.Id, userName, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, userName)
				So(SetActiveState(testTask.Id, u, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
			})

		})
	})
	Convey("With one task has tasks it depends on", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection),
			"Error clearing task and build collections")
		displayName := "testName"
		userName := "testUser"
		testTime := time.Now()
		taskId := "t1"
		buildId := "b1"
		distroId := "d1"

		dep1 := &task.Task{
			Id:            "t2",
			ScheduledTime: testTime,
			BuildId:       buildId,
			DistroId:      distroId,
		}
		dep2 := &task.Task{
			Id:            "t3",
			ScheduledTime: testTime,
			BuildId:       buildId,
			DistroId:      distroId,
		}
		So(dep1.Insert(), ShouldBeNil)
		So(dep2.Insert(), ShouldBeNil)

		testTask := task.Task{
			Id:          taskId,
			DisplayName: displayName,
			Activated:   false,
			DistroId:    "arch",
			BuildId:     buildId,
			DependsOn: []task.Dependency{
				{
					TaskId: "t2",
					Status: evergreen.TaskSucceeded,
				},
				{
					TaskId: "t3",
					Status: evergreen.TaskSucceeded,
				},
			},
		}

		b := &build.Build{
			Id:    buildId,
			Tasks: []build.TaskCache{{Id: taskId}, {Id: "t2"}, {Id: "t3"}},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(testTask.DistroId, ShouldNotEqual, "")

		Convey("activating the task should activate the tasks it depends on", func() {
			So(SetActiveState(testTask.Id, userName, true), ShouldBeNil)
			depTask, err := task.FindOne(task.ById(dep1.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			depTask, err = task.FindOne(task.ById(dep2.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			Convey("deactivating the task should not deactive the tasks it depends on", func() {
				So(SetActiveState(testTask.Id, userName, false), ShouldBeNil)
				depTask, err = task.FindOne(task.ById(depTask.Id))
				So(err, ShouldBeNil)
				So(depTask.Activated, ShouldBeTrue)
			})

		})

		Convey("activating a task with override dependencies set should not activate the tasks it depends on", func() {
			So(testTask.SetOverrideDependencies(userName), ShouldBeNil)

			So(SetActiveState(testTask.Id, userName, true), ShouldBeNil)
			depTask, err := task.FindOne(task.ById(dep1.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeFalse)

			depTask, err = task.FindOne(task.ById(dep2.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeFalse)
		})
	})

	Convey("with a task that is part of a display task", t, func() {
		b := &build.Build{
			Id: "displayBuild",
			Tasks: []build.TaskCache{
				{Id: "displayTask", Activated: false, Status: evergreen.TaskUndispatched},
			},
		}
		So(b.Insert(), ShouldBeNil)
		dt := &task.Task{
			Id:             "displayTask",
			Activated:      false,
			BuildId:        b.Id,
			Status:         evergreen.TaskUndispatched,
			DisplayOnly:    true,
			ExecutionTasks: []string{"execTask"},
			DistroId:       "arch",
		}
		So(dt.Insert(), ShouldBeNil)
		t1 := &task.Task{
			Id:        "execTask",
			Activated: false,
			BuildId:   b.Id,
			Status:    evergreen.TaskUndispatched,
		}
		So(t1.Insert(), ShouldBeNil)

		So(SetActiveState(dt.Id, "test", true), ShouldBeNil)
		t1FromDb, err := task.FindOne(task.ById(t1.Id))
		So(err, ShouldBeNil)
		So(t1FromDb.Activated, ShouldBeTrue)
		dtFromDb, err := task.FindOne(task.ById(dt.Id))
		So(err, ShouldBeNil)
		So(dtFromDb.Activated, ShouldBeTrue)
		dbBuild, err := build.FindOne(build.ById(b.Id))
		So(err, ShouldBeNil)
		So(dbBuild.Tasks[0].Activated, ShouldBeTrue)
	})
}

func TestActivatePreviousTask(t *testing.T) {
	Convey("With two tasks and a build", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection),
			"Error clearing task and build collections")
		// create two tasks
		displayName := "testTask"
		b := &build.Build{
			Id: "testBuild",
		}
		previousTask := &task.Task{
			Id:                  "one",
			DisplayName:         displayName,
			RevisionOrderNumber: 1,
			Priority:            1,
			Activated:           false,
			BuildId:             b.Id,
			DistroId:            "arch",
		}
		currentTask := &task.Task{
			Id:                  "two",
			DisplayName:         displayName,
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
			Priority:            1,
			Activated:           true,
			BuildId:             b.Id,
			DistroId:            "arch",
		}
		tc := []build.TaskCache{
			{
				DisplayName: displayName,
				Id:          previousTask.Id,
			},
			{
				DisplayName: displayName,
				Id:          currentTask.Id,
			},
		}
		b.Tasks = tc
		So(b.Insert(), ShouldBeNil)
		So(previousTask.Insert(), ShouldBeNil)
		So(currentTask.Insert(), ShouldBeNil)
		Convey("activating a previous task should set the previous task's active field to true", func() {
			So(ActivatePreviousTask(currentTask.Id, ""), ShouldBeNil)
			t, err := task.FindOne(task.ById(previousTask.Id))
			So(err, ShouldBeNil)
			So(t.Activated, ShouldBeTrue)
		})
	})
}

func TestDeactivatePreviousTask(t *testing.T) {
	Convey("With two tasks and a build", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection),
			"Error clearing task and build collections")
		// create two tasks
		displayName := "testTask"
		userName := "user"
		b := &build.Build{
			Id: "testBuild",
		}
		previousTask := &task.Task{
			Id:                  "one",
			DisplayName:         displayName,
			RevisionOrderNumber: 1,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b.Id,
			Status:              evergreen.TaskUndispatched,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		currentTask := &task.Task{
			Id:                  "two",
			DisplayName:         displayName,
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
			Priority:            1,
			Activated:           true,
			BuildId:             b.Id,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		tc := []build.TaskCache{
			{
				DisplayName: displayName,
				Id:          previousTask.Id,
			},
			{
				DisplayName: displayName,
				Id:          currentTask.Id,
			},
		}
		b.Tasks = tc
		So(b.Insert(), ShouldBeNil)
		So(previousTask.Insert(), ShouldBeNil)
		So(currentTask.Insert(), ShouldBeNil)
		Convey("activating a previous task should set the previous task's active field to true", func() {
			So(DeactivatePreviousTasks(currentTask, userName), ShouldBeNil)
			var err error
			previousTask, err = task.FindOne(task.ById(previousTask.Id))
			So(err, ShouldBeNil)
			So(previousTask.Activated, ShouldBeFalse)
		})
	})
	Convey("With a display task", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection),
			"Error clearing task and build collections")
		userName := "user"
		b1 := &build.Build{
			Id: "testBuild1",
		}
		b2 := &build.Build{
			Id: "testBuild2",
		}
		b3 := &build.Build{
			Id: "testBuild3",
		}
		dt1 := &task.Task{
			Id:                  "displayTaskOld",
			DisplayName:         "displayTask",
			RevisionOrderNumber: 5,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b1.Id,
			Status:              evergreen.TaskUndispatched,
			Project:             "sample",
			DisplayOnly:         true,
			ExecutionTasks:      []string{"execTaskOld"},
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		et1 := &task.Task{
			Id:                  "execTaskOld",
			DisplayName:         "execTask",
			RevisionOrderNumber: 5,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b1.Id,
			Status:              evergreen.TaskUndispatched,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		dt2 := &task.Task{
			Id:                  "displayTaskNew",
			DisplayName:         "displayTask",
			RevisionOrderNumber: 10,
			Status:              evergreen.TaskSucceeded,
			Priority:            1,
			Activated:           true,
			BuildId:             b2.Id,
			Project:             "sample",
			DisplayOnly:         true,
			ExecutionTasks:      []string{"execTaskNew"},
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		et2 := &task.Task{
			Id:                  "execTaskNew",
			DisplayName:         "execTask",
			RevisionOrderNumber: 10,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b2.Id,
			Status:              evergreen.TaskSucceeded,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		dt3 := &task.Task{
			Id:                  "displayTaskMulti",
			DisplayName:         "displayTask",
			RevisionOrderNumber: 4,
			Status:              evergreen.TaskStarted,
			Priority:            1,
			Activated:           true,
			BuildId:             b3.Id,
			Project:             "sample",
			DisplayOnly:         true,
			ExecutionTasks:      []string{"execTask1", "execTask2"},
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		et3 := &task.Task{
			Id:                  "execTask1",
			DisplayName:         "execTask1",
			RevisionOrderNumber: 4,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b3.Id,
			Status:              evergreen.TaskUndispatched,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		et4 := &task.Task{
			Id:                  "execTask2",
			DisplayName:         "execTask2",
			RevisionOrderNumber: 4,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b3.Id,
			Status:              evergreen.TaskStarted,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		b1.Tasks = []build.TaskCache{
			{
				DisplayName: dt1.DisplayName,
				Id:          dt1.Id,
				Activated:   true,
			},
		}
		b2.Tasks = []build.TaskCache{
			{
				DisplayName: dt2.DisplayName,
				Id:          dt2.Id,
				Activated:   true,
			},
		}
		b3.Tasks = []build.TaskCache{
			{
				DisplayName: dt3.DisplayName,
				Id:          dt3.Id,
				Activated:   true,
			},
		}
		So(b1.Insert(), ShouldBeNil)
		So(b2.Insert(), ShouldBeNil)
		So(b3.Insert(), ShouldBeNil)
		So(dt1.Insert(), ShouldBeNil)
		So(dt2.Insert(), ShouldBeNil)
		So(dt3.Insert(), ShouldBeNil)
		So(et1.Insert(), ShouldBeNil)
		So(et2.Insert(), ShouldBeNil)
		So(et3.Insert(), ShouldBeNil)
		So(et4.Insert(), ShouldBeNil)
		Convey("deactivating a display task should deactivate its child tasks", func() {
			So(DeactivatePreviousTasks(dt2, userName), ShouldBeNil)
			dbTask, err := task.FindOne(task.ById(dt1.Id))
			So(err, ShouldBeNil)
			So(dbTask.Activated, ShouldBeFalse)
			dbTask, err = task.FindOne(task.ById(et1.Id))
			So(err, ShouldBeNil)
			So(dbTask.Activated, ShouldBeFalse)
			dbBuild, err := build.FindOne(build.ById(b1.Id))
			So(err, ShouldBeNil)
			So(dbBuild.Tasks[0].Activated, ShouldBeFalse)
			Convey("but should not touch any tasks that have started", func() {
				dbTask, err = task.FindOne(task.ById(dt3.Id))
				So(err, ShouldBeNil)
				So(dbTask.Activated, ShouldBeTrue)
				dbTask, err = task.FindOne(task.ById(et3.Id))
				So(err, ShouldBeNil)
				So(dbTask.Activated, ShouldBeTrue)
				So(dbTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				dbTask, err = task.FindOne(task.ById(et4.Id))
				So(err, ShouldBeNil)
				So(dbTask.Activated, ShouldBeTrue)
				So(dbTask.Status, ShouldEqual, evergreen.TaskStarted)
				dbBuild, err := build.FindOne(build.ById(b3.Id))
				So(err, ShouldBeNil)
				So(dbBuild.Tasks[0].Activated, ShouldBeTrue)
			})
		})
	})
}

func TestUpdateBuildStatusForTask(t *testing.T) {
	Convey("With two tasks and a build", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection),
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:        "buildtest",
			Status:    evergreen.BuildStarted,
			Version:   "abc",
			Activated: true,
		}
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		testTask := task.Task{
			Id:          "testone",
			DisplayName: displayName,
			Activated:   false,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskFailed,
			StartTime:   time.Now().Add(-time.Hour),
			Version:     b.Version,
		}
		anotherTask := task.Task{
			Id:          "two",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskFailed,
			StartTime:   time.Now().Add(-time.Hour),
			Version:     b.Version,
		}

		b.Tasks = []build.TaskCache{
			{
				Id:        testTask.Id,
				Status:    evergreen.TaskStarted,
				Activated: true,
			},
			{
				Id:        anotherTask.Id,
				Status:    evergreen.TaskFailed,
				Activated: true,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(anotherTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		Convey("updating the build for a task should update the build's status and the version's status", func() {
			var err error
			updates := StatusChanges{}
			So(UpdateBuildAndVersionStatusForTask(testTask.Id, &updates), ShouldBeNil)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(updates.VersionNewStatus, ShouldEqual, evergreen.VersionFailed)
			So(updates.VersionComplete, ShouldBeTrue)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildFailed)
			So(updates.BuildComplete, ShouldBeTrue)

			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildFailed)
			v, err = VersionFindOne(VersionById(v.Id))
			So(err, ShouldBeNil)
			So(v.Status, ShouldEqual, evergreen.VersionFailed)
		})
	})
}

func TestTaskStatusImpactedByFailedTest(t *testing.T) {
	Convey("With a successful task one failed test should result in a task failure", t, func() {
		displayName := "testName"

		var (
			b        *build.Build
			v        *Version
			testTask *task.Task
			detail   *apimodels.TaskEndDetail
		)

		reset := func() {
			b = &build.Build{
				Id:        "buildtest",
				Version:   "abc",
				Activated: true,
				Tasks: []build.TaskCache{
					{
						Id:        "testone",
						Activated: true,
					},
				},
			}
			v = &Version{
				Id:     b.Version,
				Status: evergreen.VersionStarted,
			}
			testTask = &task.Task{
				Id:          "testone",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Project:     "sample",
				Version:     b.Version,
			}
			ref := &ProjectRef{
				Identifier: "sample",
			}
			detail = &apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
				Logs: &apimodels.TaskLogs{
					AgentLogURLs:  []apimodels.LogInfo{{Command: "foo1", URL: "agent"}},
					TaskLogURLs:   []apimodels.LogInfo{{Command: "foo2", URL: "task"}},
					SystemLogURLs: []apimodels.LogInfo{{Command: "foo3", URL: "system"}},
				},
			}

			require.NoError(t, db.ClearCollections(ProjectRefCollection, task.Collection, build.Collection, VersionCollection),
				"Error clearing task and build collections")
			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
			So(ref.Insert(), ShouldBeNil)
		}

		Convey("task should not fail if there are no failed test, also logs should be updated", func() {
			reset()
			updates := StatusChanges{}
			So(MarkEnd(testTask, "", time.Now(), detail, true, &updates), ShouldBeNil)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(updates.VersionNewStatus, ShouldEqual, evergreen.VersionSucceeded)
			So(updates.VersionComplete, ShouldBeTrue)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildSucceeded)
			So(updates.BuildComplete, ShouldBeTrue)

			taskData, err := task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(taskData.Status, ShouldEqual, evergreen.TaskSucceeded)
			So(reflect.DeepEqual(taskData.Logs, detail.Logs), ShouldBeTrue)
			buildCache, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(buildCache.Status, ShouldEqual, evergreen.TaskSucceeded)
			for _, t := range buildCache.Tasks {
				So(t.Status, ShouldEqual, evergreen.TaskSucceeded)
			}

		})

		Convey("task should not fail if there are only passing or silently failing tests", func() {
			reset()
			updates := StatusChanges{}
			err := testTask.SetResults([]task.TestResult{
				{
					Status: evergreen.TestSilentlyFailedStatus,
				},
				{
					Status: evergreen.TestSucceededStatus,
				},
				{
					Status: evergreen.TestSilentlyFailedStatus,
				},
			})
			So(err, ShouldBeNil)
			So(MarkEnd(testTask, "", time.Now(), detail, true, &updates), ShouldBeNil)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(updates.VersionNewStatus, ShouldEqual, evergreen.VersionSucceeded)
			So(updates.VersionComplete, ShouldBeTrue)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildSucceeded)
			So(updates.BuildComplete, ShouldBeTrue)

			taskData, err := task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(taskData.Status, ShouldEqual, evergreen.TaskSucceeded)
			buildCache, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(buildCache.Status, ShouldEqual, evergreen.TaskSucceeded)
			for _, t := range buildCache.Tasks {
				So(t.Status, ShouldEqual, evergreen.TaskSucceeded)
			}
		})

		Convey("task should fail if there is one failed test", func() {
			reset()
			err := testTask.SetResults([]task.TestResult{
				{
					Status: evergreen.TestFailedStatus,
				},
			})
			updates := StatusChanges{}

			So(err, ShouldBeNil)
			detail.Status = evergreen.TaskFailed
			So(MarkEnd(testTask, "", time.Now(), detail, true, &updates), ShouldBeNil)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(updates.VersionNewStatus, ShouldEqual, evergreen.VersionFailed)
			So(updates.VersionComplete, ShouldBeTrue)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildFailed)
			So(updates.BuildComplete, ShouldBeTrue)

			taskData, err := task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(taskData.Status, ShouldEqual, evergreen.TaskFailed)
			buildCache, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(buildCache.Status, ShouldEqual, evergreen.TaskFailed)
		})

		Convey("test failures should update the task cache", func() {
			reset()
			err := testTask.SetResults([]task.TestResult{
				{
					Status: evergreen.TestFailedStatus,
				},
			})
			updates := StatusChanges{}
			So(err, ShouldBeNil)
			detail.Status = evergreen.TaskFailed
			So(MarkEnd(testTask, "", time.Now(), detail, true, &updates), ShouldBeNil)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(updates.VersionNewStatus, ShouldEqual, evergreen.VersionFailed)
			So(updates.VersionComplete, ShouldBeTrue)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildFailed)
			So(updates.BuildComplete, ShouldBeTrue)

			updates = StatusChanges{}
			So(UpdateBuildAndVersionStatusForTask(testTask.Id, &updates), ShouldBeNil)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(updates.VersionNewStatus, ShouldEqual, evergreen.VersionFailed)
			So(updates.VersionComplete, ShouldBeTrue)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildFailed)
			So(updates.BuildComplete, ShouldBeTrue)
			buildCache, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(buildCache.Status, ShouldEqual, evergreen.TaskFailed)

			var hasFailedTask bool
			for _, t := range buildCache.Tasks {
				if t.Status == evergreen.TaskFailed {
					hasFailedTask = true
				}
			}
			So(hasFailedTask, ShouldBeTrue)
		})
		Convey("incomplete versions report updates", func() {
			reset()
			b2 := &build.Build{
				Id:        "buildtest2",
				Version:   "abc",
				Activated: false,
				Status:    evergreen.BuildCreated,
				Tasks: []build.TaskCache{
					{
						Id:     "testone2",
						Status: evergreen.TaskUndispatched,
					},
				},
			}
			So(b2.Insert(), ShouldBeNil)
			err := testTask.SetResults([]task.TestResult{
				{
					Status: evergreen.TestFailedStatus,
				},
			})
			So(err, ShouldBeNil)
			updates := StatusChanges{}
			detail.Status = evergreen.TaskFailed
			So(MarkEnd(testTask, "", time.Now(), detail, true, &updates), ShouldBeNil)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(updates.VersionNewStatus, ShouldEqual, evergreen.VersionFailed)
			So(updates.VersionComplete, ShouldBeTrue)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildFailed)
			So(updates.BuildComplete, ShouldBeTrue)
		})
	})
}

func TestMarkEnd(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection),
		"Error clearing task and build collections")

	ref := &ProjectRef{
		Identifier: "sample",
	}
	assert.NoError(ref.Insert())

	displayName := "testName"
	userName := "testUser"
	b := &build.Build{
		Id:      "buildtest",
		Status:  evergreen.BuildStarted,
		Version: "abc",
	}
	v := &Version{
		Id:     b.Version,
		Status: evergreen.VersionStarted,
	}
	testTask := task.Task{
		Id:          "testone",
		DisplayName: displayName,
		Activated:   true,
		BuildId:     b.Id,
		Project:     "sample",
		Status:      evergreen.TaskStarted,
		Version:     b.Version,
	}

	b.Tasks = []build.TaskCache{
		{
			Id:        testTask.Id,
			Status:    evergreen.TaskStarted,
			Activated: true,
		},
	}
	assert.NoError(b.Insert())
	assert.NoError(testTask.Insert())
	assert.NoError(v.Insert())
	updates := StatusChanges{}
	details := apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
	}
	assert.NoError(MarkEnd(&testTask, userName, time.Now(), &details, false, &updates))
	assert.Equal(evergreen.BuildFailed, updates.BuildNewStatus)

	Convey("with a task that is part of a display task", t, func() {
		p := &Project{
			Identifier: "sample",
		}
		b := &build.Build{
			Id:      "displayBuild",
			Project: p.Identifier,
			Version: "version1",
			Tasks: []build.TaskCache{
				{Id: "displayTask", Activated: true, Status: evergreen.TaskStarted},
			},
		}
		So(b.Insert(), ShouldBeNil)
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		So(v.Insert(), ShouldBeNil)
		dt := &task.Task{
			Id:             "displayTask",
			Activated:      true,
			BuildId:        b.Id,
			Status:         evergreen.TaskStarted,
			DisplayOnly:    true,
			ExecutionTasks: []string{"execTask"},
		}
		So(dt.Insert(), ShouldBeNil)
		t1 := &task.Task{
			Id:        "execTask",
			Activated: true,
			BuildId:   b.Id,
			Status:    evergreen.TaskStarted,
		}
		So(t1.Insert(), ShouldBeNil)

		detail := &apimodels.TaskEndDetail{
			Status: evergreen.TaskSucceeded,
		}
		So(MarkEnd(t1, "test", time.Now(), detail, false, &updates), ShouldBeNil)
		t1FromDb, err := task.FindOne(task.ById(t1.Id))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskSucceeded)
		dtFromDb, err := task.FindOne(task.ById(dt.Id))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskSucceeded)
		dbBuild, err := build.FindOne(build.ById(b.Id))
		So(err, ShouldBeNil)
		So(dbBuild.Tasks[0].Status, ShouldEqual, evergreen.TaskSucceeded)
	})
}

func TestTryResetTask(t *testing.T) {
	Convey("With a task, a build, version and a project", t, func() {
		Convey("resetting a task without a max number of executions", func() {
			require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection, ProjectRefCollection),
				"Error clearing task and build collections")
			p := &ProjectRef{Identifier: "sample"}
			So(p.Insert(), ShouldBeNil)

			displayName := "testName"
			userName := "testUser"
			b := &build.Build{
				Id:      "buildtest",
				Status:  evergreen.BuildSucceeded,
				Version: "abc",
			}
			v := &Version{
				Id:     b.Version,
				Status: evergreen.VersionStarted,
			}
			testTask := &task.Task{
				Id:          "testone",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Execution:   1,
				Project:     "sample",
				Status:      evergreen.TaskSucceeded,
				Version:     b.Version,
			}
			otherTask := &task.Task{
				Id:          "testtwo",
				DisplayName: "foo",
				Activated:   true,
				BuildId:     b.Id,
				Execution:   1,
				Project:     "sample",
				Status:      evergreen.TaskSucceeded,
				Version:     b.Version,
			}
			detail := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}

			b.Tasks = []build.TaskCache{
				{
					Id:        testTask.Id,
					Activated: false,
				},
				{
					Id:        otherTask.Id,
					Activated: true,
				},
			}

			var err error

			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(otherTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
			Convey("should reset and add a task to the old tasks collection", func() {
				So(TryResetTask(testTask.Id, userName, "", detail), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldResemble, apimodels.TaskEndDetail{})
				So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(testTask.FinishTime, ShouldResemble, util.ZeroTime)
				So(testTask.Activated, ShouldBeTrue)
				oldTaskId := fmt.Sprintf("%v_%v", testTask.Id, 1)
				oldTask, err := task.FindOneOld(task.ById(oldTaskId))
				So(err, ShouldBeNil)
				So(oldTask, ShouldNotBeNil)
				So(oldTask.Execution, ShouldEqual, 1)
				So(oldTask.Details, ShouldResemble, *detail)
				So(oldTask.FinishTime, ShouldNotResemble, util.ZeroTime)

				// should also reset the build status to "started"
				buildFromDb, err := build.FindOne(build.ById(b.Id))
				So(err, ShouldBeNil)
				So(buildFromDb.Status, ShouldEqual, evergreen.BuildStarted)
				So(buildFromDb.Tasks[0].Activated, ShouldBeTrue)
			})

		})
		Convey("resetting a task with a max number of excutions", func() {
			require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection),
				"Error clearing task and build collections")
			displayName := "testName"
			userName := "testUser"
			b := &build.Build{
				Id:      "buildtest",
				Status:  evergreen.BuildStarted,
				Version: "abc",
			}
			v := &Version{
				Id:     b.Version,
				Status: evergreen.VersionStarted,
			}
			testTask := &task.Task{
				Id:          "testone",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Execution:   evergreen.MaxTaskExecution,
				Project:     "sample",
				Status:      evergreen.TaskSucceeded,
				Version:     b.Version,
			}
			detail := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}
			anotherTask := &task.Task{
				Id:          "two",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Execution:   evergreen.MaxTaskExecution,
				Project:     "sample",
				Status:      evergreen.TaskSucceeded,
				Version:     b.Version,
			}
			b.Tasks = []build.TaskCache{
				{
					Id: testTask.Id,
				},
				{
					Id: anotherTask.Id,
				},
			}
			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
			So(anotherTask.Insert(), ShouldBeNil)

			var err error

			Convey("should not reset if an origin other than the ui package tries to reset", func() {
				So(TryResetTask(testTask.Id, userName, "", detail), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldResemble, *detail)
				So(testTask.Status, ShouldEqual, detail.Status)
				So(testTask.FinishTime, ShouldNotResemble, util.ZeroTime)
			})
			Convey("should reset and use detail information if the UI package passes in a detail ", func() {
				So(TryResetTask(anotherTask.Id, userName, evergreen.UIPackage, detail), ShouldBeNil)
				a, err := task.FindOne(task.ById(anotherTask.Id))
				So(err, ShouldBeNil)
				So(a.Details, ShouldResemble, apimodels.TaskEndDetail{})
				So(a.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(a.FinishTime, ShouldResemble, util.ZeroTime)
			})
		})
	})

	Convey("with a display task", t, func() {
		p := &Project{
			Identifier: "sample",
		}
		b := &build.Build{
			Id:      "displayBuild",
			Project: p.Identifier,
			Version: "version1",
			Tasks: []build.TaskCache{
				{Id: "displayTask", Activated: false, Status: evergreen.TaskSucceeded},
			},
		}
		So(b.Insert(), ShouldBeNil)
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		So(v.Insert(), ShouldBeNil)
		dt := &task.Task{
			Id:             "displayTask",
			Activated:      true,
			BuildId:        b.Id,
			Status:         evergreen.TaskSucceeded,
			DisplayOnly:    true,
			ExecutionTasks: []string{"execTask"},
			Version:        b.Version,
		}
		So(dt.Insert(), ShouldBeNil)
		t1 := &task.Task{
			Id:        "execTask",
			Activated: true,
			BuildId:   b.Id,
			Status:    evergreen.TaskSucceeded,
			Version:   b.Version,
		}
		So(t1.Insert(), ShouldBeNil)

		So(TryResetTask(dt.Id, "user", "test", nil), ShouldBeNil)
		t1FromDb, err := task.FindOne(task.ById(t1.Id))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskUndispatched)
		dtFromDb, err := task.FindOne(task.ById(dt.Id))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskUndispatched)
		dbBuild, err := build.FindOne(build.ById(b.Id))
		So(err, ShouldBeNil)
		So(dbBuild.Tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
	})
}

func TestAbortTask(t *testing.T) {
	Convey("With a task and a build", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection),
			"Error clearing task, build, and version collections")
		displayName := "testName"
		userName := "testUser"
		b := &build.Build{
			Id: "buildtest",
		}
		testTask := &task.Task{
			Id:          "testone",
			DisplayName: displayName,
			Activated:   false,
			BuildId:     b.Id,
			Status:      evergreen.TaskStarted,
		}
		finishedTask := &task.Task{
			Id:          "another",
			DisplayName: displayName,
			Activated:   false,
			BuildId:     b.Id,
			Status:      evergreen.TaskFailed,
		}
		b.Tasks = []build.TaskCache{
			{
				Id: testTask.Id,
			},
			{
				Id: finishedTask.Id,
			},
			{
				Id: "dt",
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(finishedTask.Insert(), ShouldBeNil)
		var err error
		Convey("with a task that has started, aborting a task should work", func() {
			So(AbortTask(testTask.Id, userName), ShouldBeNil)
			testTask, err = task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Activated, ShouldEqual, false)
			So(testTask.Aborted, ShouldEqual, true)
		})
		Convey("a task that is finished should error when aborting", func() {
			So(AbortTask(finishedTask.Id, userName), ShouldNotBeNil)
		})
		Convey("a display task should abort its execution tasks", func() {
			dt := task.Task{
				Id:             "dt",
				DisplayOnly:    true,
				ExecutionTasks: []string{"et1", "et2"},
				Status:         evergreen.TaskStarted,
				BuildId:        b.Id,
			}
			So(dt.Insert(), ShouldBeNil)
			et1 := task.Task{
				Id:      "et1",
				Status:  evergreen.TaskStarted,
				BuildId: b.Id,
			}
			So(et1.Insert(), ShouldBeNil)
			et2 := task.Task{
				Id:      "et2",
				Status:  evergreen.TaskFailed,
				BuildId: b.Id,
			}
			So(et2.Insert(), ShouldBeNil)

			So(AbortTask(dt.Id, userName), ShouldBeNil)
			dbTask, err := task.FindOneId(dt.Id)
			So(err, ShouldBeNil)
			So(dbTask.Aborted, ShouldBeTrue)
			dbTask, err = task.FindOneId(et1.Id)
			So(err, ShouldBeNil)
			So(dbTask.Aborted, ShouldBeTrue)
			dbTask, err = task.FindOneId(et2.Id)
			So(err, ShouldBeNil)
			So(dbTask.Aborted, ShouldBeFalse)
		})
	})

}
func TestMarkStart(t *testing.T) {
	Convey("With a task, build and version", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection),
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildCreated,
			Version: "abc",
		}
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionCreated,
		}
		testTask := &task.Task{
			Id:          "testTask",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskUndispatched,
			Version:     b.Version,
		}

		b.Tasks = []build.TaskCache{
			{
				Id:     testTask.Id,
				Status: evergreen.TaskUndispatched,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)

		Convey("when calling MarkStart, the task, version and build should be updated", func() {
			updates := StatusChanges{}
			err := MarkStart(testTask, &updates)
			So(updates.BuildNewStatus, ShouldBeEmpty)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(err, ShouldBeNil)
			testTask, err = task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskStarted)
			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			So(b.Tasks, ShouldNotBeNil)
			So(len(b.Tasks), ShouldEqual, 1)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskStarted)
			v, err = VersionFindOne(VersionById(v.Id))
			So(err, ShouldBeNil)
			So(v.Status, ShouldEqual, evergreen.VersionStarted)
		})
	})

	Convey("with a task that is part of a display task", t, func() {
		p := &Project{
			Identifier: "sample",
		}
		b := &build.Build{
			Id:      "displayBuild",
			Project: p.Identifier,
			Version: "version1",
			Tasks: []build.TaskCache{
				{Id: "displayTask", Activated: false, Status: evergreen.TaskUndispatched},
			},
		}
		So(b.Insert(), ShouldBeNil)
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		So(v.Insert(), ShouldBeNil)
		dt := &task.Task{
			Id:             "displayTask",
			Activated:      true,
			BuildId:        b.Id,
			Status:         evergreen.TaskUndispatched,
			Version:        v.Id,
			DisplayOnly:    true,
			ExecutionTasks: []string{"execTask"},
		}
		So(dt.Insert(), ShouldBeNil)
		t1 := &task.Task{
			Id:        "execTask",
			Activated: true,
			BuildId:   b.Id,
			Version:   v.Id,
			Status:    evergreen.TaskUndispatched,
		}
		So(t1.Insert(), ShouldBeNil)

		So(MarkStart(t1, &StatusChanges{}), ShouldBeNil)
		t1FromDb, err := task.FindOne(task.ById(t1.Id))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskStarted)
		dtFromDb, err := task.FindOne(task.ById(dt.Id))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskStarted)
		dbBuild, err := build.FindOne(build.ById(b.Id))
		So(err, ShouldBeNil)
		So(dbBuild.Tasks[0].Status, ShouldEqual, evergreen.TaskStarted)
	})
}

func TestMarkUndispatched(t *testing.T) {
	Convey("With a task, build and version", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection),
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildStarted,
			Version: "abc",
		}
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		testTask := &task.Task{
			Id:          "testTask",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskStarted,
			Version:     b.Version,
		}

		b.Tasks = []build.TaskCache{
			{
				Id:     testTask.Id,
				Status: evergreen.TaskStarted,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		Convey("when calling MarkStart, the task, version and build should be updated", func() {
			var err error
			So(MarkTaskUndispatched(testTask), ShouldBeNil)
			testTask, err = task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Tasks, ShouldNotBeNil)
			So(len(b.Tasks), ShouldEqual, 1)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
		})
	})
}

func TestMarkDispatched(t *testing.T) {
	Convey("With a task, build and version", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection),
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildCreated,
			Version: "abc",
		}
		testTask := &task.Task{
			Id:          "testTask",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskUndispatched,
			Version:     b.Version,
		}

		b.Tasks = []build.TaskCache{
			{
				Id:     testTask.Id,
				Status: evergreen.TaskUndispatched,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		Convey("when calling MarkStart, the task, version and build should be updated", func() {
			So(MarkTaskDispatched(testTask, "testHost", "distroId"), ShouldBeNil)
			var err error
			testTask, err = task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskDispatched)
			So(testTask.HostId, ShouldEqual, "testHost")
			So(testTask.DistroId, ShouldEqual, "distroId")
			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Tasks, ShouldNotBeNil)
			So(len(b.Tasks), ShouldEqual, 1)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskDispatched)
		})
	})
}

func TestGetStepback(t *testing.T) {
	Convey("When the project has a stepback policy set to true", t, func() {
		require.NoError(t, db.ClearCollections(ProjectRefCollection, task.Collection, build.Collection, VersionCollection),
			"Error clearing collections")

		config := `
stepback: true
tasks:
 - name: true
   stepback: true
 - name: false
   stepback: false
buildvariants:
 - name: sbnil
 - name: sbtrue
   stepback: true
 - name: sbfalse
   stepback: false
`
		ver := &Version{
			Id:     "version_id",
			Config: config,
		}
		So(ver.Insert(), ShouldBeNil)

		Convey("if the task does not override the setting", func() {
			testTask := &task.Task{Id: "t1", DisplayName: "nil", Project: "sample", Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with true", func() {
			testTask := &task.Task{Id: "t2", DisplayName: "true", Project: "sample", Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with false", func() {
			testTask := &task.Task{Id: "t3", DisplayName: "false", Project: "sample", Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeFalse)
			})
		})

		Convey("if the buildvariant does not override the setting", func() {
			testTask := &task.Task{Id: "t4", DisplayName: "bvnil", BuildVariant: "sbnil", Project: "sample", Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with true", func() {
			testTask := &task.Task{Id: "t5", DisplayName: "bvtrue", BuildVariant: "sbtrue", Project: "sample", Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with false", func() {
			testTask := &task.Task{Id: "t6", DisplayName: "bvfalse", BuildVariant: "sbfalse", Project: "sample", Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeFalse)
			})
		})

	})
}

func TestFailedTaskRestart(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection),
		"Error clearing task and build collections")
	userName := "testUser"
	b := &build.Build{
		Id:      "buildtest",
		Status:  evergreen.BuildStarted,
		Version: "abc",
	}
	v := &Version{
		Id:     b.Version,
		Status: evergreen.VersionStarted,
	}
	testTask1 := &task.Task{
		Id:        "taskToRestart",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
		Details:   apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem},
		Version:   b.Version,
	}
	testTask2 := &task.Task{
		Id:        "taskThatSucceeded",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskSucceeded,
		Version:   b.Version,
	}
	testTask3 := &task.Task{
		Id:        "taskOutsideOfTimeRange",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 11, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
		Details:   apimodels.TaskEndDetail{Type: "test"},
		Version:   b.Version,
	}
	testTask4 := &task.Task{
		Id:        "setupFailed",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
		Details:   apimodels.TaskEndDetail{Type: "setup"},
		Version:   b.Version,
	}
	p := &ProjectRef{
		Identifier: "sample",
	}

	b.Tasks = []build.TaskCache{
		{
			Id: testTask1.Id,
		},
		{
			Id: testTask2.Id,
		},
		{
			Id: testTask3.Id,
		},
		{
			Id: testTask4.Id,
		},
	}
	assert.NoError(b.Insert())
	assert.NoError(v.Insert())
	assert.NoError(testTask1.Insert())
	assert.NoError(testTask2.Insert())
	assert.NoError(testTask3.Insert())
	assert.NoError(testTask4.Insert())
	assert.NoError(p.Insert())

	// test a dry run
	opts := RestartOptions{
		DryRun:             true,
		IncludeTestFailed:  true,
		IncludeSysFailed:   false,
		IncludeSetupFailed: false,
		StartTime:          time.Date(2017, time.June, 11, 11, 0, 0, 0, time.Local),
		EndTime:            time.Date(2017, time.June, 12, 13, 0, 0, 0, time.Local),
		User:               userName,
	}

	results, err := RestartFailedTasks(opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Equal(1, len(results.ItemsRestarted))
	assert.Equal("taskOutsideOfTimeRange", results.ItemsRestarted[0])

	opts.IncludeTestFailed = true
	opts.IncludeSysFailed = true
	results, err = RestartFailedTasks(opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Equal(2, len(results.ItemsRestarted))
	assert.Equal("taskToRestart", results.ItemsRestarted[0])

	opts.IncludeTestFailed = false
	opts.IncludeSysFailed = false
	opts.IncludeSetupFailed = true
	results, err = RestartFailedTasks(opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Equal(1, len(results.ItemsRestarted))
	assert.Equal("setupFailed", results.ItemsRestarted[0])

	// test restarting all tasks
	opts.StartTime = time.Date(2017, time.June, 12, 11, 0, 0, 0, time.Local)
	opts.DryRun = false
	opts.IncludeTestFailed = false
	opts.IncludeSysFailed = false
	opts.IncludeSetupFailed = false
	results, err = RestartFailedTasks(opts)
	assert.NoError(err)
	assert.Equal(0, len(results.ItemsErrored))
	assert.Equal(2, len(results.ItemsRestarted))
	assert.Equal(testTask1.Id, results.ItemsRestarted[0])
	dbTask, err := task.FindOne(task.ById(testTask1.Id))
	assert.NoError(err)
	assert.Equal(dbTask.Status, evergreen.TaskUndispatched)
	assert.True(dbTask.Execution > 1)
	dbTask, err = task.FindOne(task.ById(testTask2.Id))
	assert.NoError(err)
	assert.Equal(dbTask.Status, evergreen.TaskSucceeded)
	assert.Equal(1, dbTask.Execution)
	dbTask, err = task.FindOne(task.ById(testTask3.Id))
	assert.NoError(err)
	assert.Equal(dbTask.Status, evergreen.TaskFailed)
	assert.Equal(1, dbTask.Execution)
	dbTask, err = task.FindOne(task.ById(testTask4.Id))
	assert.NoError(err)
	assert.Equal(dbTask.Status, evergreen.TaskUndispatched)
	assert.Equal(2, dbTask.Execution)
}

func TestStepback(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection),
		"Error clearing task and build collections")
	b1 := &build.Build{
		Id:        "build1",
		Status:    evergreen.BuildStarted,
		Version:   "v1",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	b2 := &build.Build{
		Id:        "build2",
		Status:    evergreen.BuildStarted,
		Version:   "v2",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	b3 := &build.Build{
		Id:        "build3",
		Status:    evergreen.BuildStarted,
		Version:   "v3",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	t1 := &task.Task{
		Id:                  "t1",
		DistroId:            "test",
		DisplayName:         "task",
		Activated:           true,
		BuildId:             b1.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	t2 := &task.Task{
		Id:                  "t2",
		DistroId:            "test",
		DisplayName:         "task",
		Activated:           false,
		BuildId:             b2.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskInactive,
		RevisionOrderNumber: 2,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	t3 := &task.Task{
		Id:                  "t3",
		DistroId:            "test",
		DisplayName:         "task",
		Activated:           true,
		BuildId:             b2.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskFailed,
		RevisionOrderNumber: 3,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	dt1 := &task.Task{
		Id:                  "dt1",
		DistroId:            "test",
		DisplayName:         "displayTask",
		Activated:           true,
		BuildId:             b1.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
		DisplayOnly:         true,
		ExecutionTasks:      []string{"et1"},
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	dt2 := &task.Task{
		Id:                  "dt2",
		DistroId:            "test",
		DisplayName:         "displayTask",
		Activated:           false,
		BuildId:             b2.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskInactive,
		RevisionOrderNumber: 2,
		DisplayOnly:         true,
		ExecutionTasks:      []string{"et2"},
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	dt3 := &task.Task{
		Id:                  "dt3",
		DistroId:            "test",
		DisplayName:         "displayTask",
		Activated:           true,
		BuildId:             b2.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskFailed,
		RevisionOrderNumber: 3,
		DisplayOnly:         true,
		ExecutionTasks:      []string{"et3"},
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	et1 := &task.Task{
		Id:                  "et1",
		DistroId:            "test",
		DisplayName:         "execTask",
		Activated:           true,
		BuildId:             b1.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	et2 := &task.Task{
		Id:                  "et2",
		DistroId:            "test",
		DisplayName:         "execTask",
		Activated:           false,
		BuildId:             b2.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskInactive,
		RevisionOrderNumber: 2,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	et3 := &task.Task{
		Id:                  "et3",
		DistroId:            "test",
		DisplayName:         "execTask",
		Activated:           true,
		BuildId:             b3.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskFailed,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	p := &ProjectRef{
		Identifier: "sample",
	}

	b1.Tasks = []build.TaskCache{
		{
			Id: t1.Id,
		},
		{
			Id: dt1.Id,
		},
	}
	b2.Tasks = []build.TaskCache{
		{
			Id: t2.Id,
		},
		{
			Id: dt2.Id,
		},
	}
	b3.Tasks = []build.TaskCache{
		{
			Id: t3.Id,
		},
		{
			Id: dt3.Id,
		},
	}
	assert.NoError(b1.Insert())
	assert.NoError(b2.Insert())
	assert.NoError(b3.Insert())
	assert.NoError(t1.Insert())
	assert.NoError(t2.Insert())
	assert.NoError(t3.Insert())
	assert.NoError(et1.Insert())
	assert.NoError(et2.Insert())
	assert.NoError(et3.Insert())
	assert.NoError(dt1.Insert())
	assert.NoError(dt2.Insert())
	assert.NoError(dt3.Insert())
	assert.NoError(p.Insert())

	// test stepping back a regular task
	assert.NoError(doStepback(t3))
	dbTask, err := task.FindOne(task.ById(t2.Id))
	assert.NoError(err)
	assert.True(dbTask.Activated)

	// test stepping back a display task
	assert.NoError(doStepback(dt3))
	dbTask, err = task.FindOne(task.ById(dt2.Id))
	assert.NoError(err)
	assert.True(dbTask.Activated)
	dbTask, err = task.FindOne(task.ById(dt2.Id))
	assert.NoError(err)
	assert.True(dbTask.Activated)
}

func TestMarkEndRequiresAllTasksToFinishToUpdateBuildStatus(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection, ProjectRefCollection, event.AllLogCollection))
	ref := ProjectRef{
		Identifier: "sample",
	}
	require.NoError(ref.Insert())
	v := &Version{
		Id:         "sample_version",
		Identifier: "sample",
		Requester:  evergreen.RepotrackerVersionRequester,
		Status:     evergreen.VersionStarted,
	}
	require.NoError(v.Insert())

	buildID := "buildtest"
	testTask := &task.Task{
		Id:          "testone",
		DisplayName: "test 1",
		Activated:   false,
		BuildId:     buildID,
		Project:     "sample",
		Status:      evergreen.TaskStarted,
		StartTime:   time.Now().Add(-time.Hour),
		Version:     v.Id,
	}
	assert.NoError(testTask.Insert())
	anotherTask := &task.Task{
		Id:          "two",
		DisplayName: "test 2",
		Activated:   true,
		BuildId:     buildID,
		Project:     "sample",
		Status:      evergreen.TaskStarted,
		StartTime:   time.Now().Add(-time.Hour),
		Version:     v.Id,
	}
	assert.NoError(anotherTask.Insert())
	displayTask := &task.Task{
		Id:             "three",
		DisplayName:    "display task",
		Activated:      true,
		DisplayOnly:    true,
		BuildId:        buildID,
		Project:        "sample",
		Status:         evergreen.TaskStarted,
		StartTime:      time.Now().Add(-time.Hour),
		ExecutionTasks: []string{"exe0", "exe1"},
		Version:        v.Id,
	}
	assert.NoError(displayTask.Insert())
	exeTask0 := &task.Task{
		Id:          "exe0",
		DisplayName: "execution 0",
		Activated:   true,
		BuildId:     buildID,
		Project:     "sample",
		Status:      evergreen.TaskStarted,
		StartTime:   time.Now().Add(-time.Hour),
		Version:     v.Id,
	}
	assert.True(exeTask0.IsPartOfDisplay())
	assert.NoError(exeTask0.Insert())
	exeTask1 := &task.Task{
		Id:          "exe1",
		DisplayName: "execution 1",
		Activated:   true,
		BuildId:     buildID,
		Project:     "sample",
		Status:      evergreen.TaskStarted,
		StartTime:   time.Now().Add(-time.Hour),
		Version:     v.Id,
	}
	assert.True(exeTask1.IsPartOfDisplay())
	assert.NoError(exeTask1.Insert())

	b := &build.Build{
		Id:        buildID,
		Status:    evergreen.BuildStarted,
		Activated: true,
		Version:   v.Id,
		Tasks: []build.TaskCache{
			{
				Id:        testTask.Id,
				Status:    evergreen.TaskStarted,
				Activated: true,
			},
			{
				Id:        anotherTask.Id,
				Status:    evergreen.TaskStarted,
				Activated: true,
			},
			{
				Id:        displayTask.Id,
				Status:    evergreen.TaskStarted,
				Activated: true,
			},
		},
	}
	require.NoError(b.Insert())
	assert.False(b.IsFinished())

	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   "system",
	}

	updates := StatusChanges{}
	assert.NoError(MarkEnd(testTask, "", time.Now(), details, false, &updates))
	assert.Empty(updates.BuildNewStatus)
	assert.False(updates.BuildComplete)
	assert.Empty(updates.VersionNewStatus)
	assert.False(updates.VersionComplete)
	b, err := build.FindOneId(buildID)
	assert.NoError(err)
	tasks, err := task.Find(task.ByVersion(b.Version).WithFields(task.BuildIdKey, task.StatusKey, task.ActivatedKey, task.DependsOnKey))
	require.NoError(err)
	complete, _, err := b.AllUnblockedTasksFinished(tasks)
	assert.NoError(err)
	assert.False(complete)

	updates = StatusChanges{}
	assert.NoError(MarkEnd(anotherTask, "", time.Now(), details, false, &updates))
	assert.Empty(updates.BuildNewStatus)
	assert.False(updates.BuildComplete)
	assert.Empty(updates.VersionNewStatus)
	assert.False(updates.VersionComplete)
	b, err = build.FindOneId(buildID)
	assert.NoError(err)
	tasks, err = task.Find(task.ByVersion(b.Version).WithFields(task.BuildIdKey, task.StatusKey, task.ActivatedKey, task.DependsOnKey))
	require.NoError(err)
	complete, _, err = b.AllUnblockedTasksFinished(tasks)
	assert.NoError(err)
	assert.False(complete)

	updates = StatusChanges{}
	assert.NoError(MarkEnd(exeTask0, "", time.Now(), details, false, &updates))
	assert.Empty(updates.BuildNewStatus)
	assert.False(updates.BuildComplete)
	assert.Empty(updates.VersionNewStatus)
	assert.False(updates.VersionComplete)
	b, err = build.FindOneId(buildID)
	assert.NoError(err)
	tasks, err = task.Find(task.ByVersion(b.Version).WithFields(task.BuildIdKey, task.StatusKey, task.ActivatedKey, task.DependsOnKey))
	require.NoError(err)
	complete, _, err = b.AllUnblockedTasksFinished(tasks)
	assert.NoError(err)
	assert.False(complete)

	exeTask1.DisplayTask = nil
	assert.NoError(err)
	updates = StatusChanges{}
	assert.NoError(MarkEnd(exeTask1, "", time.Now(), details, false, &updates))
	assert.Equal(evergreen.BuildFailed, updates.BuildNewStatus)
	assert.True(updates.BuildComplete)
	assert.Equal(evergreen.VersionFailed, updates.VersionNewStatus)
	assert.True(updates.VersionComplete)
	b, err = build.FindOneId(buildID)
	assert.NoError(err)
	tasks, err = task.Find(task.ByVersion(b.Version).WithFields(task.BuildIdKey, task.StatusKey, task.ActivatedKey, task.DependsOnKey))
	require.NoError(err)
	complete, _, err = b.AllUnblockedTasksFinished(tasks)
	assert.NoError(err)
	assert.True(complete)

	e, err := event.FindUnprocessedEvents()
	assert.NoError(err)
	assert.Len(e, 7)
}

func TestMarkEndRequiresAllTasksToFinishToUpdateBuildStatusWithCompileTask(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.AllLogCollection))
	v := &Version{
		Id:        "sample_version",
		Requester: evergreen.RepotrackerVersionRequester,
		Status:    evergreen.VersionStarted,
	}
	require.NoError(v.Insert())

	buildID := "buildtest"
	testTask := task.Task{
		Id:          "testone",
		DisplayName: "compile",
		Activated:   true,
		BuildId:     buildID,
		Project:     "sample",
		Status:      evergreen.TaskStarted,
		StartTime:   time.Now().Add(-time.Hour),
		Version:     v.Id,
	}
	require.NoError(testTask.Insert())
	anotherTask := task.Task{
		Id:          "two",
		Activated:   true,
		DisplayName: "test 2",
		BuildId:     buildID,
		Project:     "sample",
		Status:      evergreen.TaskUndispatched,
		StartTime:   time.Now().Add(-time.Hour),
		DependsOn: []task.Dependency{
			{
				TaskId: testTask.Id,
				Status: evergreen.TaskSucceeded,
			},
		},
		Version: v.Id,
	}
	require.NoError(anotherTask.Insert())

	b := &build.Build{
		Id:        buildID,
		Status:    evergreen.BuildStarted,
		Activated: true,
		Version:   v.Id,
		Tasks: []build.TaskCache{
			{
				Id:          testTask.Id,
				DisplayName: "compile",
				Status:      evergreen.TaskStarted,
				Activated:   true,
			},
			{
				Id:        anotherTask.Id,
				Activated: true,
				Status:    evergreen.TaskStarted,
			},
		},
	}
	require.NoError(b.Insert())

	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   "test",
	}
	updates := StatusChanges{}
	assert.NoError(MarkEnd(&testTask, "", time.Now(), details, false, &updates))
	assert.Equal(evergreen.BuildFailed, updates.BuildNewStatus)
	assert.True(updates.BuildComplete)
	assert.Equal(evergreen.VersionFailed, updates.VersionNewStatus)
	assert.True(updates.VersionComplete)
	b, err := build.FindOneId(buildID)
	assert.NoError(err)
	tasks, err := task.Find(task.ByVersion(b.Version).WithFields(task.BuildIdKey, task.StatusKey, task.ActivatedKey, task.DependsOnKey))
	require.NoError(err)
	complete, _, err := b.AllUnblockedTasksFinished(tasks)
	assert.True(complete)
	assert.NoError(err)
	assert.True(b.IsFinished())

	e, err := event.FindUnprocessedEvents()
	assert.NoError(err)
	assert.Len(e, 3)
}

func TestMarkEndWithBlockedDependenciesTriggersNotifications(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.AllLogCollection))

	v := &Version{
		Id:        "sample_version",
		Requester: evergreen.RepotrackerVersionRequester,
		Status:    evergreen.VersionStarted,
	}
	require.NoError(v.Insert())

	buildID := "buildtest"
	testTask := task.Task{
		Id:          "testone",
		DisplayName: "dothings",
		Activated:   true,
		BuildId:     buildID,
		Project:     "sample",
		Status:      evergreen.TaskStarted,
		StartTime:   time.Now().Add(-time.Hour),
		Version:     v.Id,
	}
	require.NoError(testTask.Insert())
	anotherTask := task.Task{
		Id:          "two",
		DisplayName: "test 2",
		BuildId:     buildID,
		Project:     "sample",
		Activated:   true,
		Status:      evergreen.TaskUndispatched,
		StartTime:   time.Now().Add(-time.Hour),
		DependsOn: []task.Dependency{
			{
				TaskId: testTask.Id,
				Status: evergreen.TaskSucceeded,
			},
		},
		Version: v.Id,
	}
	require.NoError(anotherTask.Insert())

	b := &build.Build{
		Id:        buildID,
		Status:    evergreen.BuildStarted,
		Activated: true,
		Version:   v.Id,
		Tasks: []build.TaskCache{
			{
				Id:        testTask.Id,
				Status:    evergreen.TaskStarted,
				Activated: true,
			},
			{
				Id:        anotherTask.Id,
				Activated: true,
				Status:    evergreen.TaskStarted,
			},
		},
	}
	require.NoError(b.Insert())

	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   "test",
	}
	updates := StatusChanges{}
	assert.NoError(MarkEnd(&testTask, "", time.Now(), details, false, &updates))
	assert.Equal(evergreen.BuildFailed, updates.BuildNewStatus)
	assert.True(updates.BuildComplete)
	assert.Equal(evergreen.VersionFailed, updates.VersionNewStatus)
	assert.True(updates.VersionComplete)
	b, err := build.FindOneId(buildID)
	assert.NoError(err)
	tasks, err := task.Find(task.ByVersion(b.Version).WithFields(task.BuildIdKey, task.StatusKey, task.ActivatedKey, task.DependsOnKey))
	require.NoError(err)
	complete, _, err := b.AllUnblockedTasksFinished(tasks)
	assert.True(complete)
	assert.NoError(err)
	assert.True(b.IsFinished())

	e, err := event.FindUnprocessedEvents()
	assert.NoError(err)
	assert.Len(e, 3)
}

func TestClearAndResetStrandedTask(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection), t, "error clearing collection")
	assert := assert.New(t)

	runningTask := &task.Task{
		Id:            "t",
		Status:        evergreen.TaskStarted,
		Activated:     true,
		ActivatedTime: time.Now(),
		BuildId:       "b",
	}
	assert.NoError(runningTask.Insert())

	h := &host.Host{
		Id:          "h1",
		RunningTask: "t",
	}
	assert.NoError(h.Insert())

	b := build.Build{
		Id: "b",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t",
			},
		},
	}
	assert.NoError(b.Insert())

	assert.NoError(ClearAndResetStrandedTask(h))
	runningTask, err := task.FindOne(task.ById("t"))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, runningTask.Status)
}

func TestClearAndResetStaleStrandedTask(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection), t, "error clearing collection")
	assert := assert.New(t)
	require := require.New(t)

	runningTask := &task.Task{
		Id:            "t",
		Status:        evergreen.TaskStarted,
		Activated:     true,
		ActivatedTime: util.ZeroTime,
		BuildId:       "b",
	}
	assert.NoError(runningTask.Insert())

	h := &host.Host{
		Id:          "h1",
		RunningTask: "t",
	}
	assert.NoError(h.Insert())

	b := build.Build{
		Id: "b",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t",
			},
		},
	}
	assert.NoError(b.Insert())

	assert.NoError(ClearAndResetStrandedTask(h))
	runningTask, err := task.FindOne(task.ById("t"))
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, runningTask.Status)
	assert.Equal("system", runningTask.Details.Type)

	updatedBuild, err := build.FindOneId("b")
	assert.NoError(err)
	require.NotNil(updatedBuild)
	require.Len(updatedBuild.Tasks, 1)
	assert.Equal("t", updatedBuild.Tasks[0].Id)
	assert.Equal(evergreen.TaskFailed, updatedBuild.Tasks[0].Status)
}

func TestDisplayTaskUpdates(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, event.AllLogCollection), "error clearing collection")
	assert := assert.New(t)
	dt := task.Task{
		Id:          "dt",
		DisplayOnly: true,
		Status:      evergreen.TaskUndispatched,
		Activated:   false,
		ExecutionTasks: []string{
			"task1",
			"task2",
			"task3",
			"task4",
		},
	}
	assert.NoError(dt.Insert())
	dt2 := task.Task{
		Id:          "dt2",
		DisplayOnly: true,
		Status:      evergreen.TaskUndispatched,
		Activated:   false,
		ExecutionTasks: []string{
			"task5",
			"task6",
		},
	}
	assert.NoError(dt2.Insert())
	task1 := task.Task{
		Id:     "task1",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Status:   evergreen.TaskFailed,
			TimedOut: true,
		},
		TimeTaken:  3 * time.Minute,
		StartTime:  time.Date(2000, 0, 0, 1, 1, 1, 0, time.Local),
		FinishTime: time.Date(2000, 0, 0, 1, 9, 1, 0, time.Local),
	}
	assert.NoError(task1.Insert())
	task2 := task.Task{
		Id:         "task2",
		Status:     evergreen.TaskSucceeded,
		TimeTaken:  2 * time.Minute,
		StartTime:  time.Date(2000, 0, 0, 0, 30, 0, 0, time.Local), // this should end up as the start time for dt1
		FinishTime: time.Date(2000, 0, 0, 1, 0, 5, 0, time.Local),
	}
	assert.NoError(task2.Insert())
	task3 := task.Task{
		Id:         "task3",
		Activated:  true,
		Status:     evergreen.TaskSystemUnresponse,
		TimeTaken:  5 * time.Minute,
		StartTime:  time.Date(2000, 0, 0, 0, 44, 0, 0, time.Local),
		FinishTime: time.Date(2000, 0, 0, 1, 0, 1, 0, time.Local),
	}
	assert.NoError(task3.Insert())
	task4 := task.Task{
		Id:         "task4",
		Activated:  true,
		Status:     evergreen.TaskSystemUnresponse,
		TimeTaken:  1 * time.Minute,
		StartTime:  time.Date(2000, 0, 0, 1, 0, 20, 0, time.Local),
		FinishTime: time.Date(2000, 0, 0, 1, 22, 0, 0, time.Local), // this should end up as the end time for dt1
	}
	assert.NoError(task4.Insert())
	task5 := task.Task{
		Id:        "task5",
		Activated: true,
		Status:    evergreen.TaskUndispatched,
	}
	assert.NoError(task5.Insert())
	task6 := task.Task{
		Id:        "task6",
		Activated: true,
		Status:    evergreen.TaskSucceeded,
	}
	assert.NoError(task6.Insert())

	// test that updating the status + activated from execution tasks works
	assert.NoError(UpdateDisplayTask(&dt))
	dbTask, err := task.FindOne(task.ById(dt.Id))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.True(dbTask.Details.TimedOut)
	assert.True(dbTask.Activated)
	assert.Equal(11*time.Minute, dbTask.TimeTaken)
	assert.Equal(task2.StartTime, dbTask.StartTime)
	assert.Equal(task4.FinishTime, dbTask.FinishTime)

	// test that you can't update an execution task
	assert.Error(UpdateDisplayTask(&task1))

	// test that a display task with a finished + unstarted task is "scheduled"
	assert.NoError(UpdateDisplayTask(&dt2))
	dbTask, err = task.FindOne(task.ById(dt2.Id))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	// check that the updates above logged an event for the first one
	events, err := event.Find(event.AllLogCollection, event.TaskEventsForId(dt.Id))
	assert.NoError(err)
	assert.Len(events, 1)
	events, err = event.Find(event.AllLogCollection, event.TaskEventsForId(dt2.Id))
	assert.NoError(err)
	assert.Len(events, 0)
}

func TestDisplayTaskDelayedRestart(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection), "error clearing collection")
	assert := assert.New(t)
	dt := task.Task{
		Id:          "dt",
		DisplayOnly: true,
		Status:      evergreen.TaskStarted,
		Activated:   true,
		BuildId:     "b",
		ExecutionTasks: []string{
			"task1",
			"task2",
		},
	}
	assert.NoError(dt.Insert())
	task1 := task.Task{
		Id:      "task1",
		BuildId: "b",
		Status:  evergreen.TaskSucceeded,
	}
	assert.NoError(task1.Insert())
	task2 := task.Task{
		Id:      "task2",
		BuildId: "b",
		Status:  evergreen.TaskSucceeded,
	}
	assert.NoError(task2.Insert())
	b := build.Build{
		Id: "b",
		Tasks: []build.TaskCache{
			{Id: "dt", Status: evergreen.TaskStarted, Activated: true},
		},
	}
	assert.NoError(b.Insert())

	// request that the task restarts when it's done
	assert.NoError(dt.SetResetWhenFinished())
	dbTask, err := task.FindOne(task.ById(dt.Id))
	assert.NoError(err)
	assert.True(dbTask.ResetWhenFinished)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	// end the final task so that it restarts
	assert.NoError(checkResetDisplayTask(&dt))
	dbTask, err = task.FindOne(task.ById(dt.Id))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask2, err := task.FindOne(task.ById(task2.Id))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask2.Status)

	oldTask, err := task.FindOneOld(task.ById("dt_0"))
	assert.NoError(err)
	assert.NotNil(oldTask)
}

func TestDisplayTaskFailedExecTasks(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))
	dt := task.Task{
		Id:             "task",
		DisplayOnly:    true,
		Status:         evergreen.TaskUndispatched,
		ExecutionTasks: []string{"exec0", "exec1"},
	}
	assert.NoError(dt.Insert())
	execTask0 := task.Task{
		Id:        "exec0",
		Activated: true,
		Status:    evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Status: evergreen.TaskFailed,
			Type:   evergreen.CommandTypeSystem,
		}}
	assert.NoError(execTask0.Insert())

	execTask1 := task.Task{Id: "exec1", Status: evergreen.TaskUndispatched}
	assert.NoError(execTask1.Insert())

	assert.NoError(UpdateDisplayTask(&dt))
	dbTask, err := task.FindOne(task.ById(dt.Id))
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.Equal(evergreen.CommandTypeSystem, dbTask.Details.Type)
	assert.True(dbTask.Activated)
}

func TestDisplayTaskFailedAndSucceededExecTasks(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))
	dt := task.Task{
		Id:             "task",
		DisplayOnly:    true,
		Status:         evergreen.TaskUndispatched,
		ExecutionTasks: []string{"exec0", "exec1"},
	}
	assert.NoError(dt.Insert())
	execTask0 := task.Task{
		Id:        "exec0",
		Activated: true,
		Status:    evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Status: evergreen.TaskFailed,
			Type:   evergreen.CommandTypeSetup,
		},
	}
	assert.NoError(execTask0.Insert())

	execTask1 := task.Task{Id: "exec1", Activated: true, Status: evergreen.TaskSucceeded}
	assert.NoError(execTask1.Insert())

	assert.NoError(UpdateDisplayTask(&dt))
	dbTask, err := task.FindOne(task.ById(dt.Id))
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.Equal(evergreen.CommandTypeSetup, dbTask.Details.Type)
	assert.True(dbTask.Activated)
}

func TestEvalStepback(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, ProjectRefCollection, distro.Collection, build.Collection, VersionCollection))
	yml := `
stepback: true
buildvariants:
- name: "bv"
  run_on: distro
  tasks:
  - name: task
  - name: generator
tasks:
- name: task
- name: generator
  `
	proj := ProjectRef{
		Identifier: "proj",
	}
	require.NoError(t, proj.Insert())
	d := distro.Distro{
		Id: "distro",
	}
	require.NoError(t, d.Insert())
	v := Version{
		Id:        "sample_version",
		Config:    yml,
		Requester: evergreen.RepotrackerVersionRequester,
	}
	require.NoError(t, v.Insert())
	stepbackTask := task.Task{
		Id:                  "t2",
		BuildId:             "b2",
		Status:              evergreen.TaskUndispatched,
		BuildVariant:        "bv",
		DisplayName:         "task",
		Project:             "proj",
		Activated:           false,
		RevisionOrderNumber: 2,
		DispatchTime:        util.ZeroTime,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             v.Id,
	}
	assert.NoError(stepbackTask.Insert())
	b2 := build.Build{
		Id:           "b2",
		BuildVariant: "bv",
		Tasks:        []build.TaskCache{{Id: "t2"}},
	}
	assert.NoError(b2.Insert())
	finishedTask := task.Task{
		Id:                  "t3",
		BuildId:             "b3",
		Status:              evergreen.TaskUndispatched,
		BuildVariant:        "bv",
		DisplayName:         "task",
		Project:             "proj",
		Activated:           true,
		RevisionOrderNumber: 3,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             v.Id,
	}
	assert.NoError(finishedTask.Insert())
	b3 := build.Build{
		Id:           "b3",
		BuildVariant: "bv",
		Tasks:        []build.TaskCache{{Id: "t3"}},
	}
	assert.NoError(b3.Insert())

	// should not step back if there was never a successful task
	assert.NoError(evalStepback(&finishedTask, "", evergreen.TaskFailed, false))
	checkTask, err := task.FindOneId(stepbackTask.Id)
	assert.NoError(err)
	assert.False(checkTask.Activated)

	// should step back if there is one
	prevComplete := task.Task{
		Id:                  "t1",
		BuildId:             "b1",
		Status:              evergreen.TaskSucceeded,
		BuildVariant:        "bv",
		DisplayName:         "task",
		Project:             "proj",
		Activated:           true,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             v.Id,
	}
	assert.NoError(prevComplete.Insert())
	b1 := build.Build{
		Id:           "b1",
		BuildVariant: "bv",
		Tasks:        []build.TaskCache{{Id: "t1"}, {Id: "g1"}},
	}
	assert.NoError(b1.Insert())
	assert.NoError(evalStepback(&finishedTask, "", evergreen.TaskFailed, false))
	checkTask, err = task.FindOneId(stepbackTask.Id)
	require.NoError(t, err)
	assert.True(checkTask.Activated)

	// generated task should step back its generator
	prevComplete = task.Task{
		Id:                  "g1",
		BuildId:             "b1",
		Status:              evergreen.TaskSucceeded,
		BuildVariant:        "bv",
		DisplayName:         "generator",
		Project:             "proj",
		Activated:           true,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             v.Id,
	}
	assert.NoError(prevComplete.Insert())
	stepbackTask = task.Task{
		Id:                  "g4",
		BuildId:             "b4",
		Status:              evergreen.TaskUndispatched,
		BuildVariant:        "bv",
		DisplayName:         "generator",
		Project:             "proj",
		Activated:           false,
		RevisionOrderNumber: 4,
		DispatchTime:        util.ZeroTime,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             v.Id,
	}
	assert.NoError(stepbackTask.Insert())
	b4 := build.Build{
		Id:           "b4",
		BuildVariant: "bv",
		Tasks:        []build.TaskCache{{Id: "g4"}},
	}
	assert.NoError(b4.Insert())
	generator := task.Task{
		Id:                  "g5",
		BuildId:             "b5",
		Status:              evergreen.TaskSucceeded,
		BuildVariant:        "bv",
		DisplayName:         "generator",
		Project:             "proj",
		Activated:           true,
		RevisionOrderNumber: 5,
		DispatchTime:        util.ZeroTime,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             v.Id,
	}
	assert.NoError(generator.Insert())
	generated := task.Task{
		Id:                  "t5",
		BuildId:             "b5",
		Status:              evergreen.TaskFailed,
		BuildVariant:        "bv",
		DisplayName:         "task",
		Project:             "proj",
		Activated:           true,
		RevisionOrderNumber: 5,
		GeneratedBy:         "g5",
		DispatchTime:        util.ZeroTime,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             v.Id,
	}
	assert.NoError(generated.Insert())
	b5 := build.Build{
		Id:           "b5",
		BuildVariant: "bv",
		Tasks:        []build.TaskCache{{Id: "g5"}},
	}
	assert.NoError(b5.Insert())
	assert.NoError(evalStepback(&generated, "", evergreen.TaskFailed, false))
	checkTask, err = task.FindOneId(stepbackTask.Id)
	assert.NoError(err)
	assert.True(checkTask.Activated)
}
