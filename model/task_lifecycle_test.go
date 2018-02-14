package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

var (
	oneMs = time.Millisecond
)

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func TestSetActiveState(t *testing.T) {
	Convey("With one task with no dependencies", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, task.OldCollection), t,
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
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		userName := "testUser"
		testTime := time.Now()
		taskId := "t1"
		buildId := "b1"

		dep1 := &task.Task{
			Id:            "t2",
			ScheduledTime: testTime,
			BuildId:       buildId,
		}
		dep2 := &task.Task{
			Id:            "t3",
			ScheduledTime: testTime,
			BuildId:       buildId,
		}
		So(dep1.Insert(), ShouldBeNil)
		So(dep2.Insert(), ShouldBeNil)

		testTask := task.Task{
			Id:          taskId,
			DisplayName: displayName,
			Activated:   false,
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
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection), t,
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
		}
		currentTask := &task.Task{
			Id:                  "two",
			DisplayName:         displayName,
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
			Priority:            1,
			Activated:           true,
			BuildId:             b.Id,
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
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection), t,
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
			So(DeactivatePreviousTasks(currentTask.Id, userName), ShouldBeNil)
			var err error
			previousTask, err = task.FindOne(task.ById(previousTask.Id))
			So(err, ShouldBeNil)
			So(previousTask.Activated, ShouldBeFalse)
		})
	})
	Convey("With a display task", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection), t,
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
			So(DeactivatePreviousTasks(dt2.Id, userName), ShouldBeNil)
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
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildStarted,
			Version: "abc",
		}
		v := &version.Version{
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
		}
		anotherTask := task.Task{
			Id:          "two",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskFailed,
		}

		b.Tasks = []build.TaskCache{
			{
				Id:     testTask.Id,
				Status: evergreen.TaskSucceeded,
			},
			{
				Id:     anotherTask.Id,
				Status: evergreen.TaskFailed,
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
			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildFailed)
			v, err = version.FindOne(version.ById(v.Id))
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
			v        *version.Version
			testTask *task.Task
			p        *Project
			detail   *apimodels.TaskEndDetail
		)

		reset := func() {
			b = &build.Build{
				Id:      "buildtest",
				Version: "abc",
				Tasks: []build.TaskCache{
					{Id: "testone"},
				},
			}
			v = &version.Version{
				Id:     b.Version,
				Status: evergreen.VersionStarted,
			}
			testTask = &task.Task{
				Id:          "testone",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Project:     "sample",
			}
			p = &Project{
				Identifier: "sample",
			}
			detail = &apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
			}

			testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
				"Error clearing task and build collections")
			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
		}

		Convey("task should not fail if there are no failed test", func() {
			reset()
			updates := StatusChanges{}
			So(MarkEnd(testTask, "", time.Now(), detail, p, true, &updates), ShouldBeNil)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildSucceeded)

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
			So(MarkEnd(testTask, "", time.Now(), detail, p, true, &updates), ShouldBeNil)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildSucceeded)

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
			So(MarkEnd(testTask, "", time.Now(), detail, p, true, &updates), ShouldBeNil)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildFailed)

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
			So(MarkEnd(testTask, "", time.Now(), detail, p, true, &updates), ShouldBeNil)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildFailed)

			So(UpdateBuildAndVersionStatusForTask(testTask.Id, &updates), ShouldBeNil)
			So(updates.BuildNewStatus, ShouldEqual, evergreen.BuildFailed)
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
	})
}

func TestMarkEnd(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection, version.Collection),
		"Error clearing task and build collections")

	displayName := "testName"
	userName := "testUser"
	b := &build.Build{
		Id:      "buildtest",
		Status:  evergreen.BuildStarted,
		Version: "abc",
	}
	p := &Project{
		Identifier: "sample",
	}
	v := &version.Version{
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
	}

	b.Tasks = []build.TaskCache{
		{
			Id:     testTask.Id,
			Status: evergreen.TaskStarted,
		},
	}
	assert.NoError(b.Insert())
	assert.NoError(testTask.Insert())
	assert.NoError(v.Insert())
	updates := StatusChanges{}
	details := apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
	}
	assert.NoError(MarkEnd(&testTask, userName, time.Now(), &details, p, false, &updates))
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
		v := &version.Version{
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
			Status: evergreen.TaskFailed,
			Type:   "system",
		}
		So(MarkEnd(t1, "test", time.Now(), detail, p, false, &updates), ShouldBeNil)
		t1FromDb, err := task.FindOne(task.ById(t1.Id))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskFailed)
		dtFromDb, err := task.FindOne(task.ById(dt.Id))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskSystemFailed)
		dbBuild, err := build.FindOne(build.ById(b.Id))
		So(err, ShouldBeNil)
		So(dbBuild.Tasks[0].Status, ShouldEqual, evergreen.TaskSystemFailed)
	})
}

func TestTryResetTask(t *testing.T) {
	Convey("With a task, a build, version and a project", t, func() {
		Convey("resetting a task without a max number of executions", func() {
			testutil.HandleTestingErr(db.ClearCollections(task.Collection, task.OldCollection, build.Collection, version.Collection), t,
				"Error clearing task and build collections")
			displayName := "testName"
			userName := "testUser"
			b := &build.Build{
				Id:      "buildtest",
				Status:  evergreen.BuildStarted,
				Version: "abc",
			}
			v := &version.Version{
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
			}
			p := &Project{
				Identifier: "sample",
			}
			detail := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}

			b.Tasks = []build.TaskCache{
				{
					Id: testTask.Id,
				},
			}

			var err error

			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
			Convey("should reset and add a task to the old tasks collection", func() {
				So(TryResetTask(testTask.Id, userName, "", p, detail), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldResemble, apimodels.TaskEndDetail{})
				So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(testTask.FinishTime, ShouldResemble, util.ZeroTime)
				oldTaskId := fmt.Sprintf("%v_%v", testTask.Id, 1)
				fmt.Println(oldTaskId)
				oldTask, err := task.FindOneOld(task.ById(oldTaskId))
				So(err, ShouldBeNil)
				So(oldTask, ShouldNotBeNil)
				So(oldTask.Execution, ShouldEqual, 1)
				So(oldTask.Details, ShouldResemble, *detail)
				So(oldTask.FinishTime, ShouldNotResemble, util.ZeroTime)
			})

		})
		Convey("resetting a task with a max number of excutions", func() {
			testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
				"Error clearing task and build collections")
			displayName := "testName"
			userName := "testUser"
			b := &build.Build{
				Id:      "buildtest",
				Status:  evergreen.BuildStarted,
				Version: "abc",
			}
			v := &version.Version{
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
			}
			p := &Project{
				Identifier: "sample",
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
				So(TryResetTask(testTask.Id, userName, "", p, detail), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldResemble, *detail)
				So(testTask.Status, ShouldEqual, detail.Status)
				So(testTask.FinishTime, ShouldNotResemble, util.ZeroTime)
			})
			Convey("should reset and use detail information if the UI package passes in a detail ", func() {
				So(TryResetTask(anotherTask.Id, userName, evergreen.UIPackage, p, detail), ShouldBeNil)
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
		v := &version.Version{
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
		}
		So(dt.Insert(), ShouldBeNil)
		t1 := &task.Task{
			Id:        "execTask",
			Activated: true,
			BuildId:   b.Id,
			Status:    evergreen.TaskSucceeded,
		}
		So(t1.Insert(), ShouldBeNil)

		So(TryResetTask(dt.Id, "user", "test", p, nil), ShouldBeNil)
		t1FromDb, err := task.FindOne(task.ById(t1.Id))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskUndispatched)
		dtFromDb, err := task.FindOne(task.ById(dt.Id))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskUnstarted)
		dbBuild, err := build.FindOne(build.ById(b.Id))
		So(err, ShouldBeNil)
		So(dbBuild.Tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
	})
}

func TestAbortTask(t *testing.T) {
	Convey("With a task and a build", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
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
	})

}
func TestMarkStart(t *testing.T) {
	Convey("With a task, build and version", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildCreated,
			Version: "abc",
		}
		v := &version.Version{
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
			err := MarkStart(testTask.Id, &updates)
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
			v, err = version.FindOne(version.ById(v.Id))
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
		v := &version.Version{
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

		So(MarkStart(t1.Id, &StatusChanges{}), ShouldBeNil)
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
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildStarted,
			Version: "abc",
		}
		v := &version.Version{
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
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
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

func TestGetstepback(t *testing.T) {
	Convey("When the project has a stepback policy set to true", t, func() {
		_true, _false := true, false
		project := &Project{
			Stepback: true,
			BuildVariants: []BuildVariant{
				{
					Name: "sbnil",
				},
				{
					Name:     "sbtrue",
					Stepback: &_true,
				},
				{
					Name:     "sbfalse",
					Stepback: &_false,
				},
			},
			Tasks: []ProjectTask{
				{Name: "nil"},
				{Name: "true", Stepback: &_true},
				{Name: "false", Stepback: &_false},
				{Name: "bvnil"},
				{Name: "bvtrue"},
				{Name: "bvfalse"},
			},
		}

		Convey("if the task does not override the setting", func() {
			testTask := &task.Task{Id: "t1", DisplayName: "nil"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with true", func() {
			testTask := &task.Task{Id: "t2", DisplayName: "true"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with false", func() {
			testTask := &task.Task{Id: "t3", DisplayName: "false"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeFalse)
			})
		})

		Convey("if the buildvariant does not override the setting", func() {
			testTask := &task.Task{Id: "t4", DisplayName: "bvnil", BuildVariant: "sbnil"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with true", func() {
			testTask := &task.Task{Id: "t5", DisplayName: "bvtrue", BuildVariant: "sbtrue"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with false", func() {
			testTask := &task.Task{Id: "t6", DisplayName: "bvfalse", BuildVariant: "sbfalse"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeFalse)
			})
		})

	})
}

func TestFailedTaskRestart(t *testing.T) {
	assert := assert.New(t)
	testutil.HandleTestingErr(db.ClearCollections(task.Collection, task.OldCollection, build.Collection, version.Collection), t,
		"Error clearing task and build collections")
	userName := "testUser"
	b := &build.Build{
		Id:      "buildtest",
		Status:  evergreen.BuildStarted,
		Version: "abc",
	}
	v := &version.Version{
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
		Details:   apimodels.TaskEndDetail{Type: "system"},
	}
	testTask2 := &task.Task{
		Id:        "taskThatSucceeded",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskSucceeded,
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
	}
	assert.NoError(b.Insert())
	assert.NoError(v.Insert())
	assert.NoError(testTask1.Insert())
	assert.NoError(testTask2.Insert())
	assert.NoError(testTask3.Insert())
	assert.NoError(p.Insert())

	// test a dry run getting only red or purple tasks
	opts := RestartTaskOptions{
		DryRun:     true,
		OnlyRed:    true,
		OnlyPurple: false,
		StartTime:  time.Date(2017, time.June, 11, 11, 0, 0, 0, time.Local),
		EndTime:    time.Date(2017, time.June, 12, 13, 0, 0, 0, time.Local),
		User:       userName,
	}

	results, err := RestartFailedTasks(opts)
	assert.NoError(err)
	assert.Nil(results.TasksErrored)
	assert.Equal(1, len(results.TasksRestarted))
	assert.Equal("taskOutsideOfTimeRange", results.TasksRestarted[0])

	opts.OnlyRed = false
	opts.OnlyPurple = true
	results, err = RestartFailedTasks(opts)
	assert.NoError(err)
	assert.Nil(results.TasksErrored)
	assert.Equal(1, len(results.TasksRestarted))
	assert.Equal("taskToRestart", results.TasksRestarted[0])

	// test restarting all tasks
	opts.StartTime = time.Date(2017, time.June, 12, 11, 0, 0, 0, time.Local)
	opts.DryRun = false
	opts.OnlyRed = false
	opts.OnlyPurple = false
	results, err = RestartFailedTasks(opts)
	assert.NoError(err)
	assert.Equal(0, len(results.TasksErrored))
	assert.Equal(1, len(results.TasksRestarted))
	assert.Equal(testTask1.Id, results.TasksRestarted[0])
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
}

func TestStepback(t *testing.T) {
	assert := assert.New(t)
	testutil.HandleTestingErr(db.ClearCollections(task.Collection, task.OldCollection, build.Collection, version.Collection), t,
		"Error clearing task and build collections")
	b1 := &build.Build{
		Id:      "build1",
		Status:  evergreen.BuildStarted,
		Version: "v1",
	}
	b2 := &build.Build{
		Id:      "build2",
		Status:  evergreen.BuildStarted,
		Version: "v2",
	}
	b3 := &build.Build{
		Id:      "build3",
		Status:  evergreen.BuildStarted,
		Version: "v3",
	}
	t1 := &task.Task{
		Id:                  "t1",
		DisplayName:         "task",
		Activated:           true,
		BuildId:             b1.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
	}
	t2 := &task.Task{
		Id:                  "t2",
		DisplayName:         "task",
		Activated:           false,
		BuildId:             b2.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskInactive,
		RevisionOrderNumber: 2,
	}
	t3 := &task.Task{
		Id:                  "t3",
		DisplayName:         "task",
		Activated:           true,
		BuildId:             b2.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskFailed,
		RevisionOrderNumber: 3,
	}
	dt1 := &task.Task{
		Id:                  "dt1",
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
	}
	dt2 := &task.Task{
		Id:                  "dt2",
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
	}
	dt3 := &task.Task{
		Id:                  "dt3",
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
	}
	et1 := &task.Task{
		Id:                  "et1",
		DisplayName:         "execTask",
		Activated:           true,
		BuildId:             b1.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
	}
	et2 := &task.Task{
		Id:                  "et2",
		DisplayName:         "execTask",
		Activated:           false,
		BuildId:             b2.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskInactive,
		RevisionOrderNumber: 2,
	}
	et3 := &task.Task{
		Id:                  "et3",
		DisplayName:         "execTask",
		Activated:           true,
		BuildId:             b3.Id,
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskFailed,
		RevisionOrderNumber: 1,
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
