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
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

var (
	oneMs = time.Millisecond
)

func TestSetActiveState(t *testing.T) {
	Convey("With one task with no dependencies", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, task.OldCollection, VersionCollection),
			"Error clearing task and build collections")
		var err error

		displayName := "testName"
		userName := "testUser"
		testTime := time.Now()
		v := &Version{
			Id: "version",
		}
		b := &build.Build{
			Id:      "buildtest",
			Version: "version",
		}
		testTask := &task.Task{
			Id:            "testone",
			DisplayName:   displayName,
			ScheduledTime: testTime,
			Activated:     false,
			BuildId:       b.Id,
			DistroId:      "arch",
			Version:       "version",
		}
		b.Tasks = []build.TaskCache{{Id: testTask.Id}}

		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		Convey("activating the task should set the task state to active", func() {
			So(SetActiveState(testTask, "randomUser", true), ShouldBeNil)
			testTask, err = task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Activated, ShouldBeTrue)
			So(testTask.ScheduledTime, ShouldHappenWithin, oneMs, testTime)

			var testBuild *build.Build
			testBuild, err = build.FindOneId(b.Id)
			So(err, ShouldBeNil)
			So(testBuild.Tasks[0].Activated, ShouldBeTrue)

			Convey("deactivating an active task as a normal user should deactivate the task", func() {
				So(SetActiveState(testTask, userName, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(testTask.Activated, ShouldBeFalse)

				var testBuild *build.Build
				testBuild, err = build.FindOneId(b.Id)
				So(err, ShouldBeNil)
				So(testBuild.Tasks[0].Activated, ShouldBeFalse)
			})
		})
		Convey("when deactivating an active task as evergreen", func() {
			Convey("if the task is activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(testTask, evergreen.DefaultTaskActivator, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.DefaultTaskActivator)
				So(SetActiveState(testTask, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
			})
			Convey("if the task is activated by stepback user, the task should not deactivate", func() {
				So(SetActiveState(testTask, evergreen.StepbackTaskActivator, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.StepbackTaskActivator)
				So(SetActiveState(testTask, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, true)
			})
			Convey("if the task is not activated by evergreen, the task should not deactivate", func() {
				So(SetActiveState(testTask, userName, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, userName)
				So(SetActiveState(testTask, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, true)
			})

		})
		Convey("when deactivating an active task a normal user", func() {
			u := "test_user"
			Convey("if the task is activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(testTask, evergreen.DefaultTaskActivator, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.DefaultTaskActivator)
				So(SetActiveState(testTask, u, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
			})
			Convey("if the task is activated by stepback user, the task should deactivate", func() {
				So(SetActiveState(testTask, evergreen.StepbackTaskActivator, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.StepbackTaskActivator)
				So(SetActiveState(testTask, u, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
			})
			Convey("if the task is not activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(testTask, userName, true), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, userName)
				So(SetActiveState(testTask, u, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
			})
		})
	})
	Convey("With one task has tasks it depends on", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection),
			"Error clearing task and build collections")
		displayName := "testName"
		userName := "testUser"
		testTime := time.Now()
		taskId := "t1"
		buildId := "b1"
		distroId := "d1"
		v := &Version{
			Id:     "version",
			Status: evergreen.VersionStarted,
		}
		dep1 := &task.Task{
			Id:            "t2",
			ScheduledTime: testTime,
			BuildId:       buildId,
			DistroId:      distroId,
			Version:       "version",
		}
		dep2 := &task.Task{
			Id:            "t3",
			ScheduledTime: testTime,
			BuildId:       buildId,
			DistroId:      distroId,
			Version:       "version",
		}
		So(v.Insert(), ShouldBeNil)
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
			Version: "version",
		}

		b := &build.Build{
			Id:    buildId,
			Tasks: []build.TaskCache{{Id: taskId}, {Id: "t2"}, {Id: "t3"}},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(testTask.DistroId, ShouldNotEqual, "")

		Convey("activating the task should activate the tasks it depends on", func() {
			So(SetActiveState(&testTask, userName, true), ShouldBeNil)
			depTask, err := task.FindOne(task.ById(dep1.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			depTask, err = task.FindOne(task.ById(dep2.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			Convey("deactivating the task should not deactive the tasks it depends on", func() {
				So(SetActiveState(&testTask, userName, false), ShouldBeNil)
				depTask, err = task.FindOne(task.ById(depTask.Id))
				So(err, ShouldBeNil)
				So(depTask.Activated, ShouldBeTrue)
			})

		})

		Convey("activating a task with override dependencies set should not activate the tasks it depends on", func() {
			So(testTask.SetOverrideDependencies(userName), ShouldBeNil)

			So(SetActiveState(&testTask, userName, true), ShouldBeNil)
			depTask, err := task.FindOne(task.ById(dep1.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeFalse)

			depTask, err = task.FindOne(task.ById(dep2.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeFalse)
		})
	})

	Convey("with a task that is part of a display task", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection),
			"Error clearing task and build collections")
		b := &build.Build{
			Id: "displayBuild",
			Tasks: []build.TaskCache{
				{Id: "displayTask", Activated: false, Status: evergreen.TaskUndispatched},
			},
			Version: "version",
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
			Version:        "version",
		}
		So(dt.Insert(), ShouldBeNil)
		t1 := &task.Task{
			Id:        "execTask",
			Activated: false,
			BuildId:   b.Id,
			Status:    evergreen.TaskUndispatched,
			Version:   "version",
		}
		So(t1.Insert(), ShouldBeNil)
		Convey("that should not restart", func() {
			So(SetActiveState(dt, "test", true), ShouldBeNil)
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
		Convey("that should restart", func() {
			dt.DispatchTime = time.Now()
			So(SetActiveState(dt, "test", true), ShouldBeNil)
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
	})
}

func TestActivatePreviousTask(t *testing.T) {
	Convey("With two tasks and a build", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection),
			"Error clearing task and build collections")
		// create two tasks
		displayName := "testTask"
		b := &build.Build{
			Id:      "testBuild",
			Version: "version",
		}
		previousTask := &task.Task{
			Id:                  "one",
			DisplayName:         displayName,
			RevisionOrderNumber: 1,
			Priority:            1,
			Activated:           false,
			BuildId:             b.Id,
			DistroId:            "arch",
			Version:             "version",
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
			Version:             "version",
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

func TestUpdateBuildStatusForTaskReset(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.AllLogCollection),
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
		Status: evergreen.VersionFailed,
	}
	testTask := task.Task{
		Id:          "testone",
		DisplayName: displayName,
		Activated:   true,
		BuildId:     b.Id,
		Project:     "sample",
		Status:      evergreen.TaskUndispatched,
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
			Status:    evergreen.TaskFailed,
			Activated: true,
		},
		{
			Id:        anotherTask.Id,
			Status:    evergreen.TaskFailed,
			Activated: true,
		},
	}

	assert.NoError(t, b.Insert())
	assert.NoError(t, v.Insert())
	assert.NoError(t, testTask.Insert())
	assert.NoError(t, anotherTask.Insert())

	updates := StatusChanges{}
	assert.NoError(t, UpdateBuildAndVersionStatusForTask(testTask.Id, &updates))
	dbVersion, err := VersionFindOneId(v.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionStarted, dbVersion.Status)
	events, err := event.FindAllByResourceID(v.Id)
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	data := events[0].Data.(*event.VersionEventData)
	assert.Equal(t, evergreen.VersionStarted, data.Status)
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
				Config: "identifier: sample",
			}
			testTask = &task.Task{
				Id:          "testone",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Project:     "sample",
				Version:     b.Version,
			}
			detail = &apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
				Logs: &apimodels.TaskLogs{
					AgentLogURLs:  []apimodels.LogInfo{{Command: "foo1", URL: "agent"}},
					TaskLogURLs:   []apimodels.LogInfo{{Command: "foo2", URL: "task"}},
					SystemLogURLs: []apimodels.LogInfo{{Command: "foo3", URL: "system"}},
				},
			}

			require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection),
				"Error clearing collections")
			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
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
		"Error clearing collections")

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
		Config: "identifier: sample",
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

func TestMarkEndWithTaskGroup(t *testing.T) {
	runningTask := &task.Task{
		Id:                "say-hi-123",
		DisplayName:       "say-hi",
		Status:            evergreen.TaskStarted,
		Activated:         true,
		ActivatedTime:     time.Now(),
		BuildId:           "b",
		TaskGroup:         "my_task_group",
		TaskGroupMaxHosts: 1,
		TaskGroupOrder:    1,
		Project:           "my_project",
		DistroId:          "my_distro",
		Version:           "abc",
		BuildVariant:      "a_variant",
	}
	otherTask := &task.Task{
		Id:                "say-bye-123",
		DisplayName:       "say-hi",
		Status:            evergreen.TaskSucceeded,
		Activated:         true,
		BuildId:           "b",
		TaskGroup:         "my_task_group",
		TaskGroupMaxHosts: 1,
		TaskGroupOrder:    2,
		Project:           "my_project",
		DistroId:          "my_distro",
		Version:           "abc",
		BuildVariant:      "a_variant",
	}
	detail := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
	}
	updates := StatusChanges{}
	for name, test := range map[string]func(*testing.T){
		"NotResetWhenFinished": func(t *testing.T) {
			assert.NoError(t, MarkEnd(runningTask, "test", time.Now(), detail, false, &updates))
			runningTaskDB, err := task.FindOneId(runningTask.Id)
			assert.NoError(t, err)
			assert.NotNil(t, runningTaskDB)
			assert.Equal(t, evergreen.TaskFailed, runningTaskDB.Status)
		},
		"ResetWhenFinished": func(t *testing.T) {
			assert.NoError(t, runningTask.SetResetWhenFinished())
			assert.NoError(t, MarkEnd(runningTask, "test", time.Now(), detail, false, &updates))

			runningTaskDB, err := task.FindOneId(runningTask.Id)
			assert.NoError(t, err)
			assert.NotNil(t, runningTaskDB)
			assert.NotEqual(t, evergreen.TaskFailed, runningTaskDB.Status)
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(distro.Collection, host.Collection, task.Collection, task.OldCollection,
				build.Collection, VersionCollection, ParserProjectCollection, ProjectRefCollection), t, "error clearing collection")
			assert := assert.New(t)
			runningTask.ResetWhenFinished = false
			runningTask.Status = evergreen.TaskStarted
			otherTask.Status = evergreen.TaskSucceeded
			assert.NoError(runningTask.Insert())
			assert.NoError(otherTask.Insert())
			pRef := &ProjectRef{Id: "my_project"}
			assert.NoError(pRef.Insert())
			h := &host.Host{
				Id:          "h1",
				RunningTask: "say-hi",
			}
			assert.NoError(h.Insert())
			b := build.Build{
				Id:      "b",
				Version: "abc",
				Tasks: []build.TaskCache{
					{
						Id:        "say-hi-123",
						Status:    evergreen.TaskStarted,
						Activated: true,
					},
					{
						Id:        "say-bye-123",
						Status:    evergreen.TaskFailed,
						Activated: true,
					},
				},
			}
			v := &Version{
				Id:     b.Version,
				Status: evergreen.VersionStarted,
				Config: sampleProjYmlTaskGroups,
			}
			assert.NoError(b.Insert())
			assert.NoError(v.Insert())
			d := distro.Distro{
				Id: "my_distro",
				PlannerSettings: distro.PlannerSettings{
					Version: evergreen.PlannerVersionTunable,
				},
			}
			assert.NoError(d.Insert())

			test(t)
		})
	}
}

func TestTryResetTask(t *testing.T) {
	Convey("With a task that does not exist", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection), "Error clearing task collection")
		So(TryResetTask("id", "username", "", nil), ShouldNotBeNil)
	})
	Convey("With a task, a build, version and a project", t, func() {
		Convey("resetting a task without a max number of executions", func() {
			require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection),
				"Error clearing task and build collections")

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
				Config: "identifier: sample",
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
				So(testTask.FinishTime, ShouldResemble, utility.ZeroTime)
				So(testTask.Activated, ShouldBeTrue)
				oldTaskId := fmt.Sprintf("%v_%v", testTask.Id, 1)
				oldTask, err := task.FindOneOld(task.ById(oldTaskId))
				So(err, ShouldBeNil)
				So(oldTask, ShouldNotBeNil)
				So(oldTask.Execution, ShouldEqual, 1)
				So(oldTask.Details, ShouldResemble, *detail)
				So(oldTask.FinishTime, ShouldNotResemble, utility.ZeroTime)

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
				Config: "identifier: sample",
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

			Convey("should reset if ui package tries to reset", func() {
				So(TryResetTask(testTask.Id, userName, evergreen.UIPackage, detail), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
			})
			Convey("should not reset if an origin other than the ui package tries to reset", func() {
				So(TryResetTask(testTask.Id, userName, "", detail), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldResemble, *detail)
				So(testTask.Status, ShouldEqual, detail.Status)
				So(testTask.FinishTime, ShouldNotResemble, utility.ZeroTime)
			})
			Convey("should reset and use detail information if the UI package passes in a detail ", func() {
				So(TryResetTask(anotherTask.Id, userName, evergreen.UIPackage, detail), ShouldBeNil)
				a, err := task.FindOne(task.ById(anotherTask.Id))
				So(err, ShouldBeNil)
				So(a.Details, ShouldResemble, apimodels.TaskEndDetail{})
				So(a.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(a.FinishTime, ShouldResemble, utility.ZeroTime)
			})
		})
	})

	Convey("with a display task", t, func() {
		b := &build.Build{
			Id:      "displayBuild",
			Project: "sample",
			Version: "version1",
			Tasks: []build.TaskCache{
				{Id: "displayTask", Activated: false, Status: evergreen.TaskSucceeded},
			},
		}
		So(b.Insert(), ShouldBeNil)
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
			Config: "identifier: sample",
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

func TestTryResetTaskWithTaskGroup(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection,
		build.Collection, VersionCollection, distro.Collection), t, "error clearing collection")
	assert := assert.New(t)
	require := require.New(t)

	h := &host.Host{
		Id:          "h1",
		RunningTask: "say-hi",
	}
	assert.NoError(h.Insert())
	b := build.Build{
		Id:      "b",
		Version: "abc",
		Tasks: []build.TaskCache{
			{
				Id:        "say-hi-123",
				Status:    evergreen.TaskStarted,
				Activated: true,
			},
			{
				Id:        "say-bye-123",
				Status:    evergreen.TaskFailed,
				Activated: true,
			},
		},
	}
	v := &Version{
		Id:     b.Version,
		Status: evergreen.VersionStarted,
		Config: sampleProjYmlTaskGroups,
	}
	assert.NoError(b.Insert())
	assert.NoError(v.Insert())
	d := &distro.Distro{
		Id: "my_distro",
		PlannerSettings: distro.PlannerSettings{
			Version: evergreen.PlannerVersionLegacy,
		},
	}
	assert.NoError(d.Insert())

	for name, test := range map[string]func(*testing.T, *task.Task, string){
		"NotFinished": func(t *testing.T, t1 *task.Task, t2Id string) {
			assert.NoError(TryResetTask(t2Id, "user", "test", nil))
			err := TryResetTask(t1.Id, "user", evergreen.UIPackage, nil)
			require.Error(err)
			assert.Contains(err.Error(), "cannot reset task in this status")
		},
		"CanResetTaskGroup": func(t *testing.T, t1 *task.Task, t2Id string) {
			assert.NoError(t1.MarkFailed())
			assert.NoError(TryResetTask(t2Id, "user", "test", nil))

			var err error
			t1, err = task.FindOneId(t1.Id)
			assert.NoError(err)
			assert.NotNil(t1)
			assert.Equal(evergreen.TaskUndispatched, t1.Status)
			t2, err := task.FindOneId(t2Id)
			assert.NoError(err)
			assert.NotNil(t2)
			assert.Equal(evergreen.TaskUndispatched, t2.Status)
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(db.ClearCollections(task.Collection, task.OldCollection))
			runningTask := &task.Task{
				Id:                "say-hi-123",
				DisplayName:       "say-hi",
				Status:            evergreen.TaskStarted,
				Activated:         true,
				ActivatedTime:     time.Now(),
				BuildId:           "b",
				TaskGroup:         "my_task_group",
				TaskGroupMaxHosts: 1,
				Project:           "my_project",
				DistroId:          "my_distro",
				Version:           "abc",
				BuildVariant:      "a_variant",
			}
			otherTask := &task.Task{
				Id:                "say-bye-123",
				DisplayName:       "say-bye",
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				BuildId:           "b",
				TaskGroup:         "my_task_group",
				TaskGroupMaxHosts: 1,
				Project:           "my_project",
				DistroId:          "my_distro",
				Version:           "abc",
				BuildVariant:      "a_variant",
			}
			assert.NoError(runningTask.Insert())
			assert.NoError(otherTask.Insert())
			assert.NoError(runningTask.MarkStart(time.Now()))
			t1 := *runningTask
			test(t, &t1, otherTask.Id)
		})
	}
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
func TestTryDequeueAndAbortBlockedCommitQueueVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(patch.Collection, VersionCollection, task.Collection, build.Collection, commitqueue.Collection))
	patchID := "aabbccddeeff001122334455"
	v := &Version{
		Id:     patchID,
		Status: evergreen.VersionStarted,
	}

	p := &patch.Patch{
		Id:          patch.NewId(patchID),
		Version:     v.Id,
		Status:      evergreen.PatchStarted,
		PatchNumber: 12,
		Alias:       evergreen.CommitQueueAlias,
	}
	b := build.Build{
		Id:      "my-build",
		Version: v.Id,
		Tasks:   []build.TaskCache{{Id: "t1", Activated: true}},
	}
	t1 := &task.Task{
		Id:               "t1",
		Activated:        true,
		Status:           evergreen.TaskFailed,
		Version:          v.Id,
		BuildId:          b.Id,
		CommitQueueMerge: true,
	}

	q := []commitqueue.CommitQueueItem{
		{Issue: patchID, Source: commitqueue.SourceDiff, Version: patchID},
		{Issue: "42"},
	}
	cq := &commitqueue.CommitQueue{
		ProjectID: "my-project",
		Queue:     q,
	}
	assert.NoError(t, v.Insert())
	assert.NoError(t, p.Insert())
	assert.NoError(t, b.Insert())
	assert.NoError(t, t1.Insert())
	assert.NoError(t, commitqueue.InsertQueue(cq))

	pRef := &ProjectRef{Id: cq.ProjectID}

	assert.NoError(t, tryDequeueAndAbortCommitQueueVersion(&task.Task{Id: "t1", Version: v.Id, Project: pRef.Id}, *cq, evergreen.User))
	cq, err := commitqueue.FindOneId("my-project")
	assert.NoError(t, err)
	assert.Equal(t, cq.FindItem(patchID), -1)
	assert.Len(t, cq.Queue, 1)

	mergeTask, err := task.FindMergeTaskForVersion(patchID)
	assert.NoError(t, err)
	assert.Equal(t, mergeTask.Priority, int64(-1))
	assert.False(t, mergeTask.Activated)
	p, err = patch.FindOne(patch.ByVersion(patchID))
	assert.NoError(t, err)
	assert.NotNil(t, p)
}

func TestTryDequeueAndAbortCommitQueueVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(patch.Collection, VersionCollection, task.Collection, build.Collection, commitqueue.Collection))

	versionId := bson.NewObjectId()
	v := &Version{
		Id:     versionId.Hex(),
		Status: evergreen.VersionStarted,
	}
	p := &patch.Patch{
		Id:      versionId,
		Version: v.Id,
		Alias:   evergreen.CommitQueueAlias,
		Status:  evergreen.PatchStarted,
	}
	b := build.Build{
		Id:      "my-build",
		Version: v.Id,
		Tasks: []build.TaskCache{
			{Id: "merge"},
		},
	}
	t1 := &task.Task{
		Id:        "t1",
		Activated: true,
		Status:    evergreen.TaskFailed,
		Version:   v.Id,
		BuildId:   b.Id,
	}
	t2 := &task.Task{
		Id:        "t2",
		Activated: true,
		Status:    evergreen.TaskUndispatched,
		Version:   v.Id,
		BuildId:   b.Id,
	}
	t3 := &task.Task{
		Id:        "t3",
		Activated: true,
		Status:    evergreen.TaskStarted,
		Version:   v.Id,
		BuildId:   b.Id,
	}
	t4 := task.Task{
		Id:        "t4",
		Activated: true,
		Status:    evergreen.TaskDispatched,
		Version:   v.Id,
		BuildId:   b.Id,
	}
	m := task.Task{
		Id:               "merge",
		Status:           evergreen.TaskUndispatched,
		Activated:        true,
		CommitQueueMerge: true,
		Version:          v.Id,
		BuildId:          b.Id,
	}
	q := []commitqueue.CommitQueueItem{
		{Issue: v.Id, Source: commitqueue.SourceDiff, Version: v.Id},
		{Issue: "42", Source: commitqueue.SourceDiff},
	}
	cq := &commitqueue.CommitQueue{ProjectID: "my-project", Queue: q}
	assert.NoError(t, v.Insert())
	assert.NoError(t, p.Insert())
	assert.NoError(t, b.Insert())
	assert.NoError(t, t1.Insert())
	assert.NoError(t, t2.Insert())
	assert.NoError(t, t3.Insert())
	assert.NoError(t, t4.Insert())
	assert.NoError(t, m.Insert())
	assert.NoError(t, commitqueue.InsertQueue(cq))

	pRef := &ProjectRef{Id: cq.ProjectID}

	assert.NoError(t, tryDequeueAndAbortCommitQueueVersion(&task.Task{Id: "t1", Version: v.Id, Project: pRef.Id}, *cq, evergreen.User))
	cq, err := commitqueue.FindOneId("my-project")
	assert.NoError(t, err)
	assert.Equal(t, cq.FindItem("12"), -1)
	assert.Len(t, cq.Queue, 1)

	// check that all tasks are now in the correct state
	tasks, err := task.FindAll(db.Q{})
	assert.NoError(t, err)
	aborted := 0
	finished := 0
	for _, thisTask := range tasks {
		if thisTask.Aborted {
			aborted++
		}
		if thisTask.Status == evergreen.TaskFailed {
			finished++
		}
		if thisTask.Status == evergreen.TaskUndispatched {
			assert.False(t, thisTask.Activated)
		}
	}
	assert.Equal(t, 2, aborted)
	assert.Equal(t, 1, finished)
	p, err = patch.FindOne(patch.ByVersion(versionId.Hex()))
	assert.NoError(t, err)
	assert.NotNil(t, p)
}

func TestDequeueAndRestart(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection, patch.Collection, build.Collection, task.Collection, commitqueue.Collection, task.OldCollection))
	v1 := bson.NewObjectId()
	v2 := bson.NewObjectId()
	v3 := bson.NewObjectId()
	t1 := task.Task{
		Id:               "1",
		Version:          v1.Hex(),
		BuildId:          "1",
		Project:          "p",
		Status:           evergreen.TaskSucceeded,
		Requester:        evergreen.MergeTestRequester,
		CommitQueueMerge: true,
	}
	assert.NoError(t, t1.Insert())
	t2 := task.Task{
		Id:               "2",
		Version:          v2.Hex(),
		BuildId:          "2",
		Project:          "p",
		Status:           evergreen.TaskFailed,
		Requester:        evergreen.MergeTestRequester,
		CommitQueueMerge: true,
	}
	assert.NoError(t, t2.Insert())
	t3 := task.Task{
		Id:               "3",
		Version:          v3.Hex(),
		BuildId:          "3",
		Project:          "p",
		Status:           evergreen.TaskUndispatched,
		Requester:        evergreen.MergeTestRequester,
		CommitQueueMerge: true,
		DependsOn: []task.Dependency{
			{TaskId: t2.Id, Status: "*"},
		},
	}
	assert.NoError(t, t3.Insert())
	t4 := task.Task{
		Id:        "4",
		Version:   v3.Hex(),
		BuildId:   "3",
		Project:   "p",
		Status:    evergreen.TaskSucceeded,
		Requester: evergreen.MergeTestRequester,
	}
	assert.NoError(t, t4.Insert())
	b1 := build.Build{
		Id:      "1",
		Version: v1.Hex(),
		Tasks: []build.TaskCache{
			{Id: t1.Id},
		},
	}
	assert.NoError(t, b1.Insert())
	b2 := build.Build{
		Id:      "2",
		Version: v2.Hex(),
		Tasks: []build.TaskCache{
			{Id: t2.Id},
		},
	}
	assert.NoError(t, b2.Insert())
	b3 := build.Build{
		Id:      "3",
		Version: v3.Hex(),
		Tasks: []build.TaskCache{
			{Id: t3.Id},
			{Id: t4.Id},
		},
	}
	assert.NoError(t, b3.Insert())
	p1 := patch.Patch{
		Id:      v1,
		Alias:   evergreen.CommitQueueAlias,
		Version: v1.Hex(),
	}
	assert.NoError(t, p1.Insert())
	p2 := patch.Patch{
		Id:      v2,
		Alias:   evergreen.CommitQueueAlias,
		Version: v2.Hex(),
	}
	assert.NoError(t, p2.Insert())
	p3 := patch.Patch{
		Id:      v3,
		Alias:   evergreen.CommitQueueAlias,
		Version: v3.Hex(),
	}
	assert.NoError(t, p3.Insert())
	version1 := Version{
		Id: v1.Hex(),
	}
	assert.NoError(t, version1.Insert())
	version2 := Version{
		Id: v2.Hex(),
	}
	assert.NoError(t, version2.Insert())
	version3 := Version{
		Id: v3.Hex(),
	}
	assert.NoError(t, version3.Insert())
	cq := commitqueue.CommitQueue{
		ProjectID: "p",
		Queue: []commitqueue.CommitQueueItem{
			{Issue: v1.Hex(), Version: v1.Hex()},
			{Issue: v2.Hex(), Version: v2.Hex()},
			{Issue: v3.Hex(), Version: v3.Hex()},
		},
	}
	assert.NoError(t, commitqueue.InsertQueue(&cq))

	assert.NoError(t, DequeueAndRestart(&t2, "", ""))
	dbCq, err := commitqueue.FindOneId(cq.ProjectID)
	assert.NoError(t, err)
	assert.Len(t, dbCq.Queue, 2)
	assert.Equal(t, v1.Hex(), dbCq.Queue[0].Issue)
	assert.Equal(t, v3.Hex(), dbCq.Queue[1].Issue)
	dbTask1, err := task.FindOneId(t1.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0, dbTask1.Execution)
	dbTask2, err := task.FindOneId(t2.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0, dbTask2.Execution)
	dbTask3, err := task.FindOneId(t3.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0, dbTask3.Execution)
	assert.Equal(t, evergreen.TaskUndispatched, dbTask3.Status)
	assert.Len(t, dbTask3.DependsOn, 1)
	assert.Equal(t, t1.Id, dbTask3.DependsOn[0].TaskId)
	dbTask4, err := task.FindOneId(t4.Id)
	assert.NoError(t, err)
	assert.Equal(t, 1, dbTask4.Execution)
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
			Config: "identifier: sample",
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
		b := &build.Build{
			Id:      "displayBuild",
			Project: "sample",
			Version: "version1",
			Tasks: []build.TaskCache{
				{Id: "displayTask", Activated: false, Status: evergreen.TaskUndispatched},
			},
		}
		So(b.Insert(), ShouldBeNil)
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
			Config: "identifier: sample",
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
			Config: "identifier: sample",
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
			sampleHost := &host.Host{
				Id: "testHost",
				Distro: distro.Distro{
					Id: "distroId",
				},
				AgentRevision: "testAgentVersion",
			}
			So(MarkTaskDispatched(testTask, sampleHost), ShouldBeNil)
			var err error
			testTask, err = task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskDispatched)
			So(testTask.HostId, ShouldEqual, "testHost")
			So(testTask.DistroId, ShouldEqual, "distroId")
			So(testTask.AgentVersion, ShouldEqual, "testAgentVersion")
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
		Config: "identifier: sample",
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

func TestFailedTaskRestartWithDisplayTasksAndTaskGroup(t *testing.T) {
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
		Config: "identifier: sample",
	}
	testTask1 := &task.Task{
		Id:                "taskGroup1",
		Activated:         false,
		BuildId:           b.Id,
		Execution:         1,
		Project:           "sample",
		StartTime:         time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:            evergreen.TaskFailed,
		Details:           apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem},
		TaskGroup:         "myTaskGroup",
		TaskGroupMaxHosts: 1,
		Version:           b.Version,
	}
	testTask2 := &task.Task{
		Id:                "taskGroup2",
		Activated:         false,
		BuildId:           b.Id,
		Execution:         1,
		Project:           "sample",
		StartTime:         time.Date(2017, time.June, 13, 12, 0, 0, 0, time.Local),
		Status:            evergreen.TaskFailed,
		Details:           apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem},
		TaskGroup:         "myTaskGroup",
		TaskGroupMaxHosts: 1,
		Version:           b.Version,
	}
	testTask3 := &task.Task{
		Id:        "dt1",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
		Details:   apimodels.TaskEndDetail{Type: "test"},
		Version:   b.Version,
	}
	testTask4 := &task.Task{
		Id:        "dt2",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
		Details:   apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem},
		Version:   b.Version,
	}
	testTask5 := &task.Task{
		Id:             "dt",
		Activated:      false,
		BuildId:        b.Id,
		Execution:      1,
		Project:        "sample",
		StartTime:      time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:         evergreen.TaskFailed,
		Details:        apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem},
		DisplayOnly:    true,
		ExecutionTasks: []string{testTask3.Id, testTask4.Id},
		Version:        b.Version,
	}
	b.Tasks = []build.TaskCache{
		{Id: testTask1.Id},
		{Id: testTask2.Id},
		{Id: testTask3.Id},
		{Id: testTask4.Id},
		{Id: testTask5.Id},
	}
	assert.NoError(b.Insert())
	assert.NoError(v.Insert())
	assert.NoError(testTask1.Insert())
	assert.NoError(testTask2.Insert())
	assert.NoError(testTask3.Insert())
	assert.NoError(testTask4.Insert())
	assert.NoError(testTask5.Insert())

	opts := RestartOptions{
		IncludeTestFailed:  false,
		IncludeSysFailed:   true,
		IncludeSetupFailed: false,
		StartTime:          time.Date(2017, time.June, 12, 11, 0, 0, 0, time.Local),
		EndTime:            time.Date(2017, time.June, 12, 13, 0, 0, 0, time.Local),
		User:               userName,
	}

	// test that all of these tasks are restarted, even though some are out of range/wrong type, because of the group
	results, err := RestartFailedTasks(opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Equal(2, len(results.ItemsRestarted)) // not all are included in items restarted
	// but all tasks are restarted
	dbTask, err := task.FindOne(task.ById(testTask1.Id))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(task.ById(testTask2.Id))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(task.ById(testTask3.Id))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(task.ById(testTask4.Id))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(task.ById(testTask5.Id))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
}

func TestStepback(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection),
		"Error clearing task and build collections")

	v1 := &Version{
		Id: "v1",
	}
	v2 := &Version{
		Id: "v2",
	}
	v3 := &Version{
		Id: "v3",
	}
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
		Version:             "v1",
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
		Version:             "v2",
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
		Version:             "v3",
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
		Version:             "v1",
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
		Version:             "v2",
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
		Version:             "v3",
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
	assert.NoError(v1.Insert())
	assert.NoError(v2.Insert())
	assert.NoError(v3.Insert())
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

	require.NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.AllLogCollection))

	v := &Version{
		Id:         "sample_version",
		Identifier: "sample",
		Requester:  evergreen.RepotrackerVersionRequester,
		Config:     "identifier: sample",
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
		Type:   evergreen.CommandTypeSystem,
	}

	updates := StatusChanges{}
	assert.NoError(MarkEnd(testTask, "", time.Now(), details, false, &updates))
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

	e, err := event.FindUnprocessedEvents(evergreen.DefaultEventProcessingLimit)
	assert.NoError(err)
	assert.Len(e, 10)
}

func TestMarkEndRequiresAllTasksToFinishToUpdateBuildStatusWithCompileTask(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.AllLogCollection))
	v := &Version{
		Id:        "sample_version",
		Requester: evergreen.RepotrackerVersionRequester,
		Config:    "identifier: sample",
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
	assert.True(b.Tasks[1].Blocked)

	e, err := event.FindUnprocessedEvents(evergreen.DefaultEventProcessingLimit)
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
		Config:    "identifier: sample",
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

	e, err := event.FindUnprocessedEvents(evergreen.DefaultEventProcessingLimit)
	assert.NoError(err)
	assert.Len(e, 3)
}

func TestClearAndResetStrandedTask(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection, VersionCollection), t, "error clearing collection")
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
		Version: "version",
	}
	assert.NoError(b.Insert())
	v := Version{
		Id: b.Version,
	}
	assert.NoError(v.Insert())

	assert.NoError(ClearAndResetStrandedTask(h))
	runningTask, err := task.FindOne(task.ById("t"))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, runningTask.Status)
}

func TestMarkEndWithNoResults(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.AllLogCollection, testresult.Collection))
	testTask1 := task.Task{
		Id:              "t1",
		Status:          evergreen.TaskStarted,
		Activated:       true,
		ActivatedTime:   time.Now(),
		BuildId:         "b",
		Version:         "v",
		MustHaveResults: true,
	}
	assert.NoError(t, testTask1.Insert())
	testTask2 := task.Task{
		Id:              "t2",
		Status:          evergreen.TaskStarted,
		Activated:       true,
		ActivatedTime:   time.Now(),
		BuildId:         "b",
		Version:         "v",
		MustHaveResults: true,
	}
	assert.NoError(t, testTask2.Insert())
	b := build.Build{
		Id:      "b",
		Tasks:   []build.TaskCache{{Id: "t1"}, {Id: "t2"}},
		Version: "v",
	}
	assert.NoError(t, b.Insert())
	v := &Version{
		Id:        "v",
		Requester: evergreen.RepotrackerVersionRequester,
		Status:    evergreen.VersionStarted,
		Config:    "identifier: sample",
	}
	assert.NoError(t, v.Insert())
	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskSucceeded,
		Type:   "test",
	}
	updates := StatusChanges{}

	err := MarkEnd(&testTask1, "", time.Now(), details, false, &updates)
	assert.NoError(t, err)
	dbTask, err := task.FindOneId(testTask1.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
	assert.Equal(t, evergreen.TaskDescriptionNoResults, dbTask.Details.Description)

	results := testresult.TestResult{
		ID:     mgobson.NewObjectId(),
		TaskID: testTask2.Id,
	}
	assert.NoError(t, results.Insert())
	err = MarkEnd(&testTask2, "", time.Now(), details, false, &updates)
	assert.NoError(t, err)
	dbTask, err = task.FindOneId(testTask2.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskSucceeded, dbTask.Status)
}

func TestClearAndResetStaleStrandedTask(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection), t, "error clearing collection")
	assert := assert.New(t)
	require := require.New(t)

	runningTask := &task.Task{
		Id:            "t",
		Status:        evergreen.TaskStarted,
		Activated:     true,
		ActivatedTime: utility.ZeroTime,
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

func TestClearAndResetExecTask(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection, VersionCollection))

	dispTask := &task.Task{
		Id:             "dt",
		Status:         evergreen.TaskStarted,
		Version:        "version",
		Activated:      true,
		ActivatedTime:  time.Now(),
		BuildId:        "b",
		ExecutionTasks: []string{"et"},
		DisplayOnly:    true,
	}

	execTask := &task.Task{
		Id:            "et",
		Status:        evergreen.TaskStarted,
		Version:       "version",
		Activated:     true,
		ActivatedTime: time.Now(),
		BuildId:       "b",
	}
	assert.NoError(t, dispTask.Insert())
	assert.NoError(t, execTask.Insert())

	h := &host.Host{
		Id:          "h1",
		RunningTask: "et",
	}
	assert.NoError(t, h.Insert())

	b := build.Build{
		Id: "b",
		Tasks: []build.TaskCache{
			build.TaskCache{Id: "dt"},
		},
		Version: "version",
	}
	assert.NoError(t, b.Insert())
	v := Version{
		Id: "version",
	}
	assert.NoError(t, v.Insert())

	assert.NoError(t, ClearAndResetStrandedTask(h))
	restartedDisplayTask, err := task.FindOne(task.ById("dt"))
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskUndispatched, restartedDisplayTask.Status)
	restartedExecutionTask, err := task.FindOne(task.ById("et"))
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskUndispatched, restartedExecutionTask.Status)
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
	blockedDt := task.Task{
		Id:          "blockedDt",
		DisplayOnly: true,
		Status:      evergreen.TaskUndispatched,
		Activated:   false,
		ExecutionTasks: []string{
			"task7",
			"task8",
			"task9",
			"task10",
		},
	}
	assert.NoError(blockedDt.Insert())
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
	task7 := task.Task{
		Id:        "task7",
		Activated: true,
		Status:    evergreen.TaskSucceeded,
	}
	assert.NoError(task7.Insert())
	task8 := task.Task{
		Id:        "task8",
		Activated: true,
		Status:    evergreen.TaskUndispatched,
		DependsOn: []task.Dependency{{TaskId: "task9", Unattainable: true}},
	}
	assert.NoError(task8.Insert())
	task9 := task.Task{
		Id:        "task9",
		Activated: true,
		Status:    evergreen.TaskFailed,
	}
	assert.NoError(task9.Insert())
	task10 := task.Task{
		Id:        "task10",
		Activated: true,
		Status:    evergreen.TaskUndispatched,
	}
	assert.NoError(task10.Insert())

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

	// test that a display task with a finished + unstarted task is "started"
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

	// a blocked execution task + unblocked unfinshed tasks should still be "started"
	assert.NoError(UpdateDisplayTask(&blockedDt))
	dbTask, err = task.FindOne(task.ById(blockedDt.Id))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	// a blocked execution task should not contribute to the status
	assert.NoError(task10.MarkFailed())
	assert.NoError(UpdateDisplayTask(&blockedDt))
	dbTask, err = task.FindOne(task.ById(blockedDt.Id))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
}

func TestDisplayTaskUpdateNoUndispatched(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, event.AllLogCollection), "error clearing collection")
	assert := assert.New(t)
	dt := task.Task{
		Id:          "dt",
		DisplayOnly: true,
		Status:      evergreen.TaskStarted,
		Activated:   true,
		ExecutionTasks: []string{
			"task1",
			"task2",
		},
	}
	assert.NoError(dt.Insert())
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
		Id:        "task2",
		Status:    evergreen.TaskStarted,
		StartTime: time.Date(2000, 0, 0, 0, 30, 0, 0, time.Local), // this should end up as the start time for dt1
	}
	assert.NoError(task2.Insert())

	// test that updating the status + activated from execution tasks shows started
	assert.NoError(UpdateDisplayTask(&dt))
	dbTask, err := task.FindOne(task.ById(dt.Id))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	events, err := event.Find(event.AllLogCollection, event.TaskEventsForId(dt.Id))
	assert.NoError(err)
	assert.Len(events, 0)
}

func TestDisplayTaskDelayedRestart(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection), "error clearing collection")
	assert := assert.New(t)
	dt := task.Task{
		Id:          "dt",
		DisplayOnly: true,
		Status:      evergreen.TaskStarted,
		Activated:   true,
		BuildId:     "b",
		Version:     "version",
		ExecutionTasks: []string{
			"task1",
			"task2",
		},
	}
	assert.NoError(dt.Insert())
	task1 := task.Task{
		Id:      "task1",
		BuildId: "b",
		Version: "version",
		Status:  evergreen.TaskSucceeded,
	}
	assert.NoError(task1.Insert())
	task2 := task.Task{
		Id:      "task2",
		BuildId: "b",
		Version: "version",
		Status:  evergreen.TaskSucceeded,
	}
	assert.NoError(task2.Insert())
	b := build.Build{
		Id: "b",
		Tasks: []build.TaskCache{
			{Id: "dt", Status: evergreen.TaskStarted, Activated: true},
		},
		Version: "version",
	}
	assert.NoError(b.Insert())
	v := Version{
		Id: "version",
	}
	assert.NoError(v.Insert())

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

func TestAbortedTaskDelayedRestart(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection), "error clearing collection")
	task1 := task.Task{
		Id:                "task1",
		BuildId:           "b",
		Version:           "version",
		Status:            evergreen.TaskStarted,
		Aborted:           true,
		ResetWhenFinished: true,
		Activated:         true,
	}
	assert.NoError(t, task1.Insert())
	b := build.Build{
		Id: "b",
		Tasks: []build.TaskCache{
			{Id: task1.Id, Status: evergreen.TaskStarted, Activated: true},
		},
		Version: "version",
	}
	assert.NoError(t, b.Insert())
	v := Version{
		Id:     "version",
		Config: `_id: v`,
	}
	assert.NoError(t, v.Insert())

	detail := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
	}
	updates := StatusChanges{}
	assert.NoError(t, MarkEnd(&task1, "test", time.Now(), detail, false, &updates))
	newTask, err := task.FindOneId(task1.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskUndispatched, newTask.Status)
	assert.Equal(t, 1, newTask.Execution)
	oldTask, err := task.FindOneOld(task.ById("task1_0"))
	assert.NoError(t, err)
	assert.True(t, oldTask.Aborted)
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
		Id: "proj",
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
		DispatchTime:        utility.ZeroTime,
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
		DispatchTime:        utility.ZeroTime,
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
		DispatchTime:        utility.ZeroTime,
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
		DispatchTime:        utility.ZeroTime,
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

func TestUpdateBlockedDependencies(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection))

	b := build.Build{Id: "build0"}
	tasks := []task.Task{
		{
			Id:      "t0",
			BuildId: b.Id,
			Status:  evergreen.TaskFailed,
		},
		{
			Id:      "t1",
			BuildId: b.Id,
			DependsOn: []task.Dependency{
				{
					TaskId: "t0",
					Status: evergreen.TaskSucceeded,
				},
				{
					TaskId: "t0",
					Status: evergreen.TaskSucceeded,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id:          "t2",
			BuildId:     b.Id,
			Status:      evergreen.TaskUndispatched,
			DisplayOnly: true,
			DependsOn: []task.Dependency{
				{
					TaskId: "t1",
					Status: evergreen.TaskSucceeded,
				},
			},
			ExecutionTasks: []string{"t2-execution"},
		},
		{
			Id:      "t3",
			BuildId: b.Id,
			DependsOn: []task.Dependency{
				{
					TaskId:       "t2",
					Status:       evergreen.TaskSucceeded,
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id:      "t4",
			BuildId: b.Id,
			DependsOn: []task.Dependency{
				{
					TaskId: "t3",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
		{
			Id:      "t5",
			BuildId: b.Id,
			DependsOn: []task.Dependency{
				{
					TaskId: "t0",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
	}
	for _, t := range tasks {
		assert.NoError(t.Insert())
		b.Tasks = append(b.Tasks, build.TaskCache{Id: t.Id, Blocked: t.Blocked()})
	}
	execTask := task.Task{
		Id: "t2-execution",
		DependsOn: []task.Dependency{
			{
				TaskId: "t1",
				Status: evergreen.TaskSucceeded,
			},
		},
		BuildId:     b.Id,
		DisplayTask: &tasks[2],
	}
	assert.NoError(execTask.Insert())
	assert.NoError(b.Insert())

	assert.NoError(UpdateBlockedDependencies(&tasks[0]))
	dbBuild, err := build.FindOneId(b.Id)
	assert.NoError(err)
	require.Len(t, dbBuild.Tasks, 6)
	assert.False(dbBuild.Tasks[0].Blocked)

	dbTask1, err := task.FindOneId(tasks[1].Id)
	assert.NoError(err)
	assert.Len(dbTask1.DependsOn, 2)
	assert.True(dbTask1.DependsOn[0].Unattainable)
	assert.True(dbTask1.DependsOn[1].Unattainable) // this task has duplicates which are also marked
	assert.True(dbBuild.Tasks[1].Blocked)

	dbTask2, err := task.FindOneId(tasks[2].Id)
	assert.NoError(err)
	assert.True(dbTask2.DependsOn[0].Unattainable)
	assert.True(dbBuild.Tasks[2].Blocked)

	dbTask3, err := task.FindOneId(tasks[3].Id)
	assert.NoError(err)
	assert.True(dbTask3.DependsOn[0].Unattainable)
	assert.True(dbBuild.Tasks[3].Blocked)

	// We don't traverse past t3 which was already unattainable == true
	dbTask4, err := task.FindOneId(tasks[4].Id)
	assert.NoError(err)
	assert.False(dbTask4.DependsOn[0].Unattainable)
	assert.False(dbBuild.Tasks[4].Blocked)

	// update more than one dependency (t1 and t5)
	dbTask5, err := task.FindOneId(tasks[5].Id)
	assert.NoError(err)
	assert.True(dbTask5.DependsOn[0].Unattainable)
	assert.True(dbBuild.Tasks[5].Blocked)

	dbExecTask, err := task.FindOneId(execTask.Id)
	assert.NoError(err)
	assert.True(dbExecTask.DependsOn[0].Unattainable)
}

func TestUpdateUnblockedDependencies(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection))
	b := build.Build{Id: "build0"}
	tasks := []task.Task{
		{Id: "t0", BuildId: b.Id},
		{Id: "t1", BuildId: b.Id, Status: evergreen.TaskFailed},
		{
			Id:      "t2",
			BuildId: b.Id,
			DependsOn: []task.Dependency{
				{
					TaskId:       "t0",
					Unattainable: true,
				},
				{
					TaskId:       "t1",
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id:      "t3",
			BuildId: b.Id,
			DependsOn: []task.Dependency{
				{
					TaskId:       "t0",
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id:      "t4",
			BuildId: b.Id,
			DependsOn: []task.Dependency{
				{
					TaskId:       "t3",
					Unattainable: false,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id:      "t5",
			BuildId: b.Id,
			DependsOn: []task.Dependency{
				{
					TaskId:       "t4",
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
	}

	for _, t := range tasks {
		assert.NoError(t.Insert())
		b.Tasks = append(b.Tasks, build.TaskCache{Id: t.Id, Blocked: t.Blocked()})
	}
	assert.NoError(b.Insert())

	assert.NoError(UpdateUnblockedDependencies(&tasks[0], false, ""))
	dbBuild, err := build.FindOneId(b.Id)
	assert.NoError(err)
	require.Len(t, dbBuild.Tasks, 6)
	assert.False(dbBuild.Tasks[0].Blocked)
	assert.False(dbBuild.Tasks[1].Blocked)

	// this task should still be marked blocked because t1 is unattainable
	dbTask2, err := task.FindOneId(tasks[2].Id)
	assert.NoError(err)
	assert.False(dbTask2.DependsOn[0].Unattainable)
	assert.True(dbTask2.DependsOn[1].Unattainable)
	assert.True(dbBuild.Tasks[2].Blocked)

	dbTask3, err := task.FindOneId(tasks[3].Id)
	assert.NoError(err)
	assert.False(dbTask3.DependsOn[0].Unattainable)
	assert.False(dbBuild.Tasks[3].Blocked)

	dbTask4, err := task.FindOneId(tasks[4].Id)
	assert.NoError(err)
	assert.False(dbTask4.DependsOn[0].Unattainable)
	assert.False(dbBuild.Tasks[4].Blocked)

	// We don't traverse past the t4 which was already unattainable == false
	dbTask5, err := task.FindOneId(tasks[5].Id)
	assert.NoError(err)
	assert.True(dbTask5.DependsOn[0].Unattainable)
	assert.True(dbBuild.Tasks[5].Blocked)
}
