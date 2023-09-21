package model

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	oneMs = time.Millisecond
)

// checkDisabled checks that the given task is disabled and logs the expected
// events.
func checkDisabled(t *testing.T, dbTask *task.Task) {
	assert.Equal(t, evergreen.DisabledTaskPriority, dbTask.Priority, "task '%s' should have disabled priority", dbTask.Id)
	assert.False(t, dbTask.Activated, "task '%s' should be deactivated", dbTask.Id)

	events, err := event.FindAllByResourceID(dbTask.Id)
	require.NoError(t, err)

	var loggedDeactivationEvent bool
	var loggedPriorityChangedEvent bool
	for _, e := range events {
		switch e.EventType {
		case event.TaskPriorityChanged:
			loggedPriorityChangedEvent = true
		case event.TaskDeactivated:
			loggedDeactivationEvent = true
		}
	}

	assert.True(t, loggedPriorityChangedEvent, "task '%s' did not log an event indicating its priority was set", dbTask.Id)
	assert.True(t, loggedDeactivationEvent, "task '%s' did not log an event indicating it was deactivated", dbTask.Id)
}

func TestDisableStaleContainerTasks(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, event.EventCollection, build.Collection, VersionCollection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, tsk task.Task){
		"DisablesStaleUnallocatedContainerTask": func(t *testing.T, tsk task.Task) {
			tsk.ActivatedTime = time.Now().Add(-9000 * 24 * time.Hour)
			require.NoError(t, tsk.Insert())

			require.NoError(t, DisableStaleContainerTasks(t.Name()))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			checkDisabled(t, dbTask)
		},
		"DisablesStaleAllocatedContainerTask": func(t *testing.T, tsk task.Task) {
			tsk.ActivatedTime = time.Now().Add(-9000 * 24 * time.Hour)
			tsk.ContainerAllocated = true
			tsk.ContainerAllocatedTime = time.Now().Add(-5000 * 24 * time.Hour)
			require.NoError(t, tsk.Insert())

			require.NoError(t, DisableStaleContainerTasks(t.Name()))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			checkDisabled(t, dbTask)
		},
		"IgnoresFreshContainerTask": func(t *testing.T, tsk task.Task) {
			tsk.ActivatedTime = time.Now()
			require.NoError(t, tsk.Insert())

			require.NoError(t, DisableStaleContainerTasks(t.Name()))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.True(t, dbTask.Activated)
			assert.Zero(t, dbTask.Priority)
		},
		"IgnoresContainerTaskWithStatusOtherThanUndispatched": func(t *testing.T, tsk task.Task) {
			tsk.ActivatedTime = time.Now().Add(-9000 * 24 * time.Hour)
			tsk.Status = evergreen.TaskSucceeded
			require.NoError(t, tsk.Insert())

			require.NoError(t, DisableStaleContainerTasks(t.Name()))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.True(t, dbTask.Activated)
			assert.Zero(t, dbTask.Priority)
		},
		"IgnoresHostTasks": func(t *testing.T, tsk task.Task) {
			tsk.ActivatedTime = time.Now().Add(-9000 * 24 * time.Hour)
			tsk.ExecutionPlatform = task.ExecutionPlatformHost
			require.NoError(t, tsk.Insert())

			require.NoError(t, DisableStaleContainerTasks(t.Name()))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.True(t, dbTask.Activated)
			assert.Zero(t, dbTask.Priority)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, event.EventCollection, build.Collection, VersionCollection))
			versionId := bson.NewObjectId()
			v := &Version{
				Id: versionId.Hex(),
			}
			require.NoError(t, v.Insert())
			b := &build.Build{
				Id:      "build-id",
				Version: v.Id,
			}
			require.NoError(t, b.Insert())
			task := task.Task{
				Id:                "task-id",
				BuildId:           b.Id,
				Version:           v.Id,
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				ExecutionPlatform: task.ExecutionPlatformContainer,
			}
			tCase(t, task)
		})
	}
}

func TestDisableOneTask(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, event.EventCollection, build.Collection, VersionCollection))
	}()

	type disableFunc func(t *testing.T, tsk task.Task) error

	for funcName, disable := range map[string]disableFunc{
		"DisableTasks": func(t *testing.T, tsk task.Task) error {
			return DisableTasks(t.Name(), tsk)
		},
	} {
		t.Run(funcName, func(t *testing.T) {
			for tName, tCase := range map[string]func(t *testing.T, tasks [5]task.Task){
				"DisablesNormalTask": func(t *testing.T, tasks [5]task.Task) {
					require.NoError(t, disable(t, tasks[3]))

					dbTask, err := task.FindOneId(tasks[3].Id)
					require.NoError(t, err)
					require.NotZero(t, dbTask)

					checkDisabled(t, dbTask)
				},
				"DisablesTaskAndDeactivatesItsDependents": func(t *testing.T, tasks [5]task.Task) {
					require.NoError(t, disable(t, tasks[4]))

					dbTask, err := task.FindOneId(tasks[4].Id)
					require.NoError(t, err)
					require.NotZero(t, dbTask)

					checkDisabled(t, dbTask)

					dbDependentTask, err := task.FindOneId(tasks[3].Id)
					require.NoError(t, err)
					require.NotZero(t, dbDependentTask)

					assert.Zero(t, dbDependentTask.Priority, "dependent task should not have been disabled")
					assert.False(t, dbDependentTask.Activated, "dependent task should have been deactivated")
				},
				"DisablesDisplayTaskAndItsExecutionTasks": func(t *testing.T, tasks [5]task.Task) {
					require.NoError(t, disable(t, tasks[0]))

					dbDisplayTask, err := task.FindOneId(tasks[0].Id)
					require.NoError(t, err)
					require.NotZero(t, dbDisplayTask)
					checkDisabled(t, dbDisplayTask)

					dbExecTasks, err := task.FindAll(db.Query(task.ByIds([]string{tasks[1].Id, tasks[2].Id})))
					require.NoError(t, err)
					assert.Len(t, dbExecTasks, 2)

					for _, task := range dbExecTasks {
						checkDisabled(t, &task)
					}
				},
				"DoesNotDisableParentDisplayTask": func(t *testing.T, tasks [5]task.Task) {
					require.NoError(t, disable(t, tasks[1]))

					dbExecTask, err := task.FindOneId(tasks[1].Id)
					require.NoError(t, err)
					require.NotZero(t, dbExecTask)

					checkDisabled(t, dbExecTask)

					dbDisplayTask, err := task.FindOneId(tasks[0].Id)
					require.NoError(t, err)
					require.NotZero(t, dbDisplayTask)

					assert.Zero(t, dbDisplayTask.Priority, "display task is not modified when its execution task is disabled")
					assert.True(t, dbDisplayTask.Activated, "display task is not modified when its execution task is disabled")
				},
			} {
				t.Run(tName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(task.Collection, event.EventCollection, build.Collection, VersionCollection))
					versionId := bson.NewObjectId()
					v := &Version{
						Id: versionId.Hex(),
					}
					require.NoError(t, v.Insert())
					b := &build.Build{
						Id:      "build-id",
						Version: v.Id,
					}
					require.NoError(t, b.Insert())
					tasks := [5]task.Task{
						{Id: "display-task0", DisplayOnly: true, ExecutionTasks: []string{"exec-task1", "exec-task2"}, Activated: true, BuildId: b.Id, Version: v.Id},
						{Id: "exec-task1", DisplayTaskId: utility.ToStringPtr("display-task0"), Activated: true, BuildId: b.Id, Version: v.Id},
						{Id: "exec-task2", DisplayTaskId: utility.ToStringPtr("display-task0"), Activated: true, BuildId: b.Id, Version: v.Id},
						{Id: "task3", Activated: true, DependsOn: []task.Dependency{{TaskId: "task4"}}, BuildId: b.Id, Version: v.Id},
						{Id: "task4", Activated: true, BuildId: b.Id, Version: v.Id},
					}
					for _, task := range tasks {
						require.NoError(t, task.Insert())
					}
					tCase(t, tasks)
				})
			}
		})
	}
}

func TestDisableManyTasks(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, event.EventCollection, build.Collection, VersionCollection))
	}()

	for tName, tCase := range map[string]func(t *testing.T){
		"DisablesIndividualExecutionTasksWithinADisplayTaskAndDoesNotUpdateDisplayTask": func(t *testing.T) {
			dt := task.Task{
				Id:             "display-task",
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec-task1", "exec-task2", "exec-task3"},
				Activated:      true,
				BuildId:        "build-id",
				Version:        "abcdefghijk",
			}
			et1 := task.Task{
				Id:            "exec-task1",
				DisplayTaskId: utility.ToStringPtr(dt.Id),
				Activated:     true,
				BuildId:       "build-id",
				Version:       "abcdefghijk",
			}
			et2 := task.Task{
				Id:            "exec-task2",
				DisplayTaskId: utility.ToStringPtr(dt.Id),
				Activated:     true,
				BuildId:       "build-id",
				Version:       "abcdefghijk",
			}
			et3 := task.Task{
				Id:            "exec-task3",
				DisplayTaskId: utility.ToStringPtr(dt.Id),
				Activated:     true,
				BuildId:       "build-id",
				Version:       "abcdefghijk",
			}
			require.NoError(t, dt.Insert())
			require.NoError(t, et1.Insert())
			require.NoError(t, et2.Insert())
			require.NoError(t, et3.Insert())

			require.NoError(t, DisableTasks(t.Name(), et1, et2))

			dbDisplayTask, err := task.FindOneId(dt.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask)

			assert.Zero(t, dbDisplayTask.Priority, "parent display task priority should not be modified when execution tasks are disabled")
			assert.True(t, dbDisplayTask.Activated, "parent display task should not be deactivated when execution tasks are disabled")

			dbExecTask1, err := task.FindOneId(et1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask1)
			checkDisabled(t, dbExecTask1)

			dbExecTask2, err := task.FindOneId(et2.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask2)
			checkDisabled(t, dbExecTask1)

			dbExecTask3, err := task.FindOneId(et3.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask3)
			assert.Zero(t, dbExecTask3.Priority, "priority of execution task under same parent display task as disabled execution tasks should not be modified")
			assert.True(t, dbExecTask3.Activated, "execution task under same parent display task as disabled execution tasks should not be deactivated")
		},
		"DisablesMixOfExecutionTasksAndDisplayTasks": func(t *testing.T) {
			dt1 := task.Task{
				Id:             "display-task1",
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec-task1", "exec-task2"},
				Activated:      true,
				BuildId:        "build-id",
				Version:        "abcdefghijk",
			}
			dt2 := task.Task{
				Id:             "display-task2",
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec-task3", "exec-task4"},
				Activated:      true,
				BuildId:        "build-id",
				Version:        "abcdefghijk",
			}
			et1 := task.Task{
				Id:            "exec-task1",
				DisplayTaskId: utility.ToStringPtr(dt1.Id),
				Activated:     true,
				BuildId:       "build-id",
				Version:       "abcdefghijk",
			}
			et2 := task.Task{
				Id:            "exec-task2",
				DisplayTaskId: utility.ToStringPtr(dt1.Id),
				Activated:     true,
				BuildId:       "build-id",
				Version:       "abcdefghijk",
			}
			et3 := task.Task{
				Id:            "exec-task3",
				DisplayTaskId: utility.ToStringPtr(dt2.Id),
				Activated:     true,
				BuildId:       "build-id",
				Version:       "abcdefghijk",
			}
			et4 := task.Task{
				Id:            "exec-task4",
				DisplayTaskId: utility.ToStringPtr(dt2.Id),
				Activated:     true,
				BuildId:       "build-id",
				Version:       "abcdefghijk",
			}
			require.NoError(t, dt1.Insert())
			require.NoError(t, dt2.Insert())
			require.NoError(t, et1.Insert())
			require.NoError(t, et2.Insert())
			require.NoError(t, et3.Insert())
			require.NoError(t, et4.Insert())

			require.NoError(t, DisableTasks(t.Name(), et1, et3, dt2))

			dbDisplayTask1, err := task.FindOneId(dt1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask1)

			assert.Zero(t, dbDisplayTask1.Priority, "parent display task priority should not be modified when execution tasks are disabled")
			assert.True(t, dbDisplayTask1.Activated, "parent display task should not be deactivated when execution tasks are disabled")

			dbDisplayTask2, err := task.FindOneId(dt2.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask2)

			checkDisabled(t, dbDisplayTask2)

			dbExecTask1, err := task.FindOneId(et1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask1)
			checkDisabled(t, dbExecTask1)

			dbExecTask2, err := task.FindOneId(et2.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask2)
			assert.Zero(t, dbExecTask2.Priority, "priority of execution task under same parent display task as disabled execution tasks should not be modified")
			assert.True(t, dbExecTask2.Activated, "execution task under same parent display task as disabled execution tasks should not be deactivated")

			dbExecTask3, err := task.FindOneId(et3.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask3)
			checkDisabled(t, dbExecTask3)

			dbExecTask4, err := task.FindOneId(et4.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask4)
			checkDisabled(t, dbExecTask4)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, event.EventCollection, build.Collection, VersionCollection))
			versionId := "abcdefghijk"
			v := &Version{
				Id: versionId,
			}
			require.NoError(t, v.Insert())
			b := &build.Build{
				Id:      "build-id",
				Version: v.Id,
			}
			require.NoError(t, b.Insert())
			tCase(t)
		})
	}
}

func TestSetActiveState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With one task with no dependencies", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, task.OldCollection, VersionCollection, commitqueue.Collection))
		var err error

		displayName := "testName"
		userName := "testUser"
		testTime := time.Now()
		versionId := bson.NewObjectId()
		v := &Version{
			Id: versionId.Hex(),
		}
		b := &build.Build{
			Id:      "buildtest",
			Version: v.Id,
		}
		testTask := &task.Task{
			Id:                "testone",
			DisplayName:       displayName,
			ScheduledTime:     testTime,
			Activated:         false,
			BuildId:           b.Id,
			DistroId:          "arch",
			Version:           v.Id,
			Project:           "p",
			Status:            evergreen.TaskUndispatched,
			CommitQueueMerge:  true,
			Requester:         evergreen.MergeTestRequester,
			TaskGroup:         "tg",
			TaskGroupMaxHosts: 1,
			TaskGroupOrder:    1,
		}
		dependentTask := &task.Task{
			Id:                "dependentTask",
			Activated:         true,
			BuildId:           b.Id,
			Status:            evergreen.TaskFailed,
			DistroId:          "arch",
			Version:           v.Id,
			TaskGroup:         "tg",
			TaskGroupMaxHosts: 1,
			TaskGroupOrder:    2,
			DependsOn: []task.Dependency{
				{
					TaskId: testTask.Id,
					Status: evergreen.TaskSucceeded,
				},
			},
		}
		p := &patch.Patch{
			Id:          versionId,
			Version:     v.Id,
			Status:      evergreen.VersionStarted,
			PatchNumber: 12,
			Alias:       evergreen.CommitQueueAlias,
		}
		cq := commitqueue.CommitQueue{
			ProjectID: "p",
			Queue: []commitqueue.CommitQueueItem{
				{Issue: v.Id, Version: v.Id},
			},
		}

		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(dependentTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		So(p.Insert(), ShouldBeNil)
		So(commitqueue.InsertQueue(&cq), ShouldBeNil)
		Convey("activating the task should set the task state to active and mark the version as activated", func() {
			So(SetActiveState(ctx, "randomUser", true, *testTask), ShouldBeNil)
			testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
			So(err, ShouldBeNil)
			So(testTask.Activated, ShouldBeTrue)
			So(testTask.ScheduledTime, ShouldHappenWithin, oneMs, testTime)
			cq, err := commitqueue.FindOneId("p")
			assert.NoError(t, err)
			assert.Len(t, cq.Queue, 1)

			version, err := VersionFindOneId(testTask.Version)
			So(err, ShouldBeNil)
			So(utility.FromBoolPtr(version.Activated), ShouldBeTrue)
			Convey("deactivating an active task as a normal user should deactivate the task", func() {
				So(SetActiveState(ctx, userName, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldBeFalse)
				dependentTask, err = task.FindOne(db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)
				cq, err := commitqueue.FindOneId("p")
				assert.NoError(t, err)
				assert.Len(t, cq.Queue, 0)
				build, err := build.FindOneId(testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionFailed)
			})
		})
		Convey("when deactivating an active task as evergreen", func() {
			Convey("if the task is activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(ctx, "", true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, "")
				So(SetActiveState(ctx, "", false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
				dependentTask, err = task.FindOne(db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)
				cq, err := commitqueue.FindOneId("p")
				assert.NoError(t, err)
				assert.Len(t, cq.Queue, 0)
				build, err := build.FindOneId(testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionFailed)
			})
			Convey("if the task is activated by stepback user, the task should not deactivate", func() {
				So(SetActiveState(ctx, evergreen.StepbackTaskActivator, true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.StepbackTaskActivator)
				So(SetActiveState(ctx, "", false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, true)
				cq, err := commitqueue.FindOneId("p")
				assert.NoError(t, err)
				assert.Len(t, cq.Queue, 1)
				build, err := build.FindOneId(testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildStarted)
				version, err := VersionFindOneId(testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionStarted)
			})
			Convey("if the task is not activated by evergreen, the task should not deactivate", func() {
				So(SetActiveState(ctx, userName, true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, userName)
				So(SetActiveState(ctx, "", false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, true)
				cq, err := commitqueue.FindOneId("p")
				assert.NoError(t, err)
				assert.Len(t, cq.Queue, 1)
				build, err := build.FindOneId(testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildStarted)
				version, err := VersionFindOneId(testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionStarted)
			})
		})
		Convey("when deactivating an active task a normal user", func() {
			u := "test_user"
			Convey("if the task is activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(ctx, "", true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, "")
				So(SetActiveState(ctx, u, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
				dependentTask, err = task.FindOne(db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)
				cq, err := commitqueue.FindOneId("p")
				assert.NoError(t, err)
				assert.Len(t, cq.Queue, 0)
				build, err := build.FindOneId(testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionFailed)
			})
			Convey("if the task is activated by stepback user, the task should deactivate", func() {
				So(SetActiveState(ctx, evergreen.StepbackTaskActivator, true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.StepbackTaskActivator)
				So(SetActiveState(ctx, u, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
				dependentTask, err = task.FindOne(db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)

				cq, err := commitqueue.FindOneId("p")
				assert.NoError(t, err)
				assert.Len(t, cq.Queue, 0)
				build, err := build.FindOneId(testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionFailed)
			})
			Convey("if the task is not activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(ctx, userName, true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, userName)
				So(SetActiveState(ctx, u, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
				dependentTask, err = task.FindOne(db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)
				cq, err := commitqueue.FindOneId("p")
				assert.NoError(t, err)
				assert.Len(t, cq.Queue, 0)
				build, err := build.FindOneId(testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionFailed)
			})
		})
	})
	Convey("With one task has tasks it depends on", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection))
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
			Id: buildId,
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(testTask.DistroId, ShouldNotEqual, "")

		Convey("activating the task should activate the tasks it depends on", func() {
			So(SetActiveState(ctx, userName, true, testTask), ShouldBeNil)
			depTask, err := task.FindOne(db.Query(task.ById(dep1.Id)))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			depTask, err = task.FindOne(db.Query(task.ById(dep2.Id)))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			Convey("deactivating the task should not deactivate the tasks it depends on", func() {
				So(SetActiveState(ctx, userName, false, testTask), ShouldBeNil)
				depTask, err = task.FindOne(db.Query(task.ById(depTask.Id)))
				So(err, ShouldBeNil)
				So(depTask.Activated, ShouldBeTrue)
			})

		})

		Convey("activating a task with override dependencies set should not activate the tasks it depends on", func() {
			So(testTask.SetOverrideDependencies(userName), ShouldBeNil)

			So(SetActiveState(ctx, userName, true, testTask), ShouldBeNil)
			depTask, err := task.FindOne(db.Query(task.ById(dep1.Id)))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeFalse)

			depTask, err = task.FindOne(db.Query(task.ById(dep2.Id)))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeFalse)
		})
	})

	Convey("with a task that is part of a display task", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection))
		b := &build.Build{
			Id:      "displayBuild",
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
			So(SetActiveState(ctx, "test", true, *dt), ShouldBeNil)
			t1FromDb, err := task.FindOne(db.Query(task.ById(t1.Id)))
			So(err, ShouldBeNil)
			So(t1FromDb.Activated, ShouldBeTrue)
			dtFromDb, err := task.FindOne(db.Query(task.ById(dt.Id)))
			So(err, ShouldBeNil)
			So(dtFromDb.Activated, ShouldBeTrue)
		})
		Convey("that should activate and deactivate", func() {
			dt.DispatchTime = time.Now()
			So(SetActiveState(ctx, "test", true, *dt), ShouldBeNil)
			t1FromDb, err := task.FindOne(db.Query(task.ById(t1.Id)))
			So(err, ShouldBeNil)
			So(t1FromDb.Activated, ShouldBeTrue)
			dtFromDb, err := task.FindOne(db.Query(task.ById(dt.Id)))
			So(err, ShouldBeNil)
			So(dtFromDb.Activated, ShouldBeTrue)

			So(SetActiveState(ctx, "test", false, *t1), ShouldBeNil)
			t1FromDb, err = task.FindOne(db.Query(task.ById(t1.Id)))
			So(err, ShouldBeNil)
			So(t1FromDb.Activated, ShouldBeFalse)
			dtFromDb, err = task.FindOne(db.Query(task.ById(dt.Id)))
			So(err, ShouldBeNil)
			So(dtFromDb.Activated, ShouldBeFalse)
		})
	})
	Convey("with a task that is part of a task group", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection))
		b := &build.Build{
			Id:      "build",
			Version: "version",
		}
		So(b.Insert(), ShouldBeNil)
		taskDef := &task.Task{ // should restart
			Id:                "task1",
			Activated:         true,
			BuildId:           b.Id,
			Status:            evergreen.TaskFailed,
			DistroId:          "arch",
			Version:           "version",
			TaskGroup:         "tg",
			TaskGroupMaxHosts: 1,
			TaskGroupOrder:    1,
		}
		So(taskDef.Insert(), ShouldBeNil)

		taskDef.Id = "task2"
		taskDef.Activated = false
		taskDef.Status = evergreen.TaskUndispatched
		taskDef.TaskGroupOrder = 2
		So(taskDef.Insert(), ShouldBeNil) // should be scheduled

		taskDef.Id = "task4"
		taskDef.TaskGroupOrder = 4
		So(taskDef.Insert(), ShouldBeNil) //should not be activated

		taskDef.Id = "task3"
		taskDef.TaskGroupOrder = 3
		So(taskDef.Insert(), ShouldBeNil) // the task we're activating

		So(SetActiveState(ctx, "test", true, *taskDef), ShouldBeNil)

		taskGroup, err := task.FindTaskGroupFromBuild(b.Id, taskDef.TaskGroup)
		So(err, ShouldBeNil)
		So(taskGroup, ShouldHaveLength, 4)
		for _, t := range taskGroup {
			if t.TaskGroupOrder < 4 {
				So(t.Activated, ShouldBeTrue)
				So(t.Status, ShouldEqual, evergreen.TaskUndispatched)
			} else {
				So(t.Activated, ShouldBeFalse)
			}
			if t.TaskGroupOrder == 1 { // the first task should be restarted
				So(t.Execution, ShouldEqual, 1)
			} else {
				So(t.Execution, ShouldEqual, 0)
			}
		}
	})
	Convey("deactivating an early task group task", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection))
		b := &build.Build{
			Id:      "build",
			Version: "version",
		}
		So(b.Insert(), ShouldBeNil)
		taskDef := &task.Task{
			Id:                "task1",
			Activated:         true,
			BuildId:           b.Id,
			Status:            evergreen.TaskSucceeded,
			DistroId:          "arch",
			Version:           "version",
			TaskGroup:         "tg",
			TaskGroupMaxHosts: 1,
			TaskGroupOrder:    1,
		}
		So(taskDef.Insert(), ShouldBeNil)

		taskDef.Id = "task2"
		taskDef.TaskGroupOrder = 2
		taskDef.Status = evergreen.TaskDispatched
		taskDef.DependsOn = append(taskDef.DependsOn, task.Dependency{TaskId: "task1", Status: evergreen.TaskSucceeded})
		So(taskDef.Insert(), ShouldBeNil) // should not be unscheduled

		taskDef.Id = "task3"
		taskDef.TaskGroupOrder = 3
		taskDef.DependsOn = append(taskDef.DependsOn, task.Dependency{TaskId: "task2", Status: evergreen.TaskSucceeded})
		So(taskDef.Insert(), ShouldBeNil) // task to deactivate

		taskDef.Id = "task4"
		taskDef.TaskGroupOrder = 4
		taskDef.DependsOn = append(taskDef.DependsOn, task.Dependency{TaskId: "task3", Status: evergreen.TaskSucceeded})
		So(taskDef.Insert(), ShouldBeNil) // task should also be deactivated

		taskDef.Id = "task3"
		So(SetActiveState(ctx, "test", false, *taskDef), ShouldBeNil)

		taskGroup, err := task.FindTaskGroupFromBuild(b.Id, taskDef.TaskGroup)
		So(err, ShouldBeNil)
		So(taskGroup, ShouldHaveLength, 4)
		for _, t := range taskGroup {
			if t.TaskGroupOrder >= 3 {
				So(t.Activated, ShouldBeFalse)
			} else {
				So(t.Activated, ShouldBeTrue)
			}
		}
	})
}

func TestActivatePreviousTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With two tasks and a build", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection))
		// create two tasks
		displayName := "testTask"
		v := Version{
			Id: "version",
		}
		b := &build.Build{
			Id:      "testBuild",
			Version: v.Id,
		}

		previousTask := &task.Task{
			Id:                  "one",
			DisplayName:         displayName,
			RevisionOrderNumber: 1,
			Priority:            1,
			Activated:           false,
			BuildId:             b.Id,
			DistroId:            "arch",
			Version:             v.Id,
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
			Version:             v.Id,
		}

		So(v.Insert(), ShouldBeNil)
		So(b.Insert(), ShouldBeNil)
		So(previousTask.Insert(), ShouldBeNil)
		So(currentTask.Insert(), ShouldBeNil)
		Convey("activating a previous task should set the previous task's active field to true", func() {
			So(activatePreviousTask(ctx, currentTask.Id, "", nil, 12), ShouldBeNil)
			t, err := task.FindOne(db.Query(task.ById(previousTask.Id)))
			So(err, ShouldBeNil)
			So(t.Activated, ShouldBeTrue)
			So(t.StepbackDepth, ShouldEqual, 12)
		})
	})
}

func TestDeactivatePreviousTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With two tasks and a build", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection))
		// create two tasks
		displayName := "testTask"
		userName := "user"
		b := &build.Build{
			Id: "testBuild",
		}
		v := &Version{
			Id: "testVersion",
		}
		previouserTask := &task.Task{
			Id:                  "zero",
			DisplayName:         displayName,
			RevisionOrderNumber: 1,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b.Id,
			Version:             v.Id,
			Status:              evergreen.TaskUndispatched,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		activeDependentTask := &task.Task{
			Id:                  "dependentOnZero",
			DisplayName:         "something else",
			RevisionOrderNumber: 1,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b.Id,
			Version:             v.Id,
			Status:              evergreen.TaskUndispatched,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
			DependsOn: []task.Dependency{
				{TaskId: "zero"},
			},
		}
		previousTask := &task.Task{
			Id:                  "one",
			DisplayName:         displayName,
			RevisionOrderNumber: 2,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b.Id,
			Version:             v.Id,
			Status:              evergreen.TaskUndispatched,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		inactiveDependentTask := &task.Task{
			Id:                  "dependentOnOne",
			DisplayName:         "something else",
			RevisionOrderNumber: 2,
			Priority:            1,
			Activated:           false,
			ActivatedBy:         "user",
			BuildId:             b.Id,
			Version:             v.Id,
			Status:              evergreen.TaskUndispatched,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
			DependsOn: []task.Dependency{
				{TaskId: "one"},
			},
		}
		currentTask := &task.Task{
			Id:                  "two",
			DisplayName:         displayName,
			RevisionOrderNumber: 3,
			Status:              evergreen.TaskSucceeded,
			Priority:            1,
			Activated:           true,
			BuildId:             b.Id,
			Version:             v.Id,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(b.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		So(previouserTask.Insert(), ShouldBeNil)
		So(previousTask.Insert(), ShouldBeNil)
		So(currentTask.Insert(), ShouldBeNil)
		So(activeDependentTask.Insert(), ShouldBeNil)
		So(inactiveDependentTask.Insert(), ShouldBeNil)
		Convey("should deactivate previous task", func() {
			So(DeactivatePreviousTasks(ctx, currentTask, userName), ShouldBeNil)
			var err error
			// Deactivates this task even though it has a dependent task, because it's inactive.
			previousTask, err = task.FindOne(db.Query(task.ById(previousTask.Id)))
			So(err, ShouldBeNil)
			So(previousTask.Activated, ShouldBeFalse)

			// Shouldn't deactivate this task because it has an active dependent task.
			previouserTask, err = task.FindOne(db.Query(task.ById(previouserTask.Id)))
			So(err, ShouldBeNil)
			So(previouserTask.Activated, ShouldBeTrue)
		})
	})
	Convey("With a display task", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection))
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
		v1 := &Version{
			Id: "testVersion1",
		}
		v2 := &Version{
			Id: "testVersion2",
		}
		v3 := &Version{
			Id: "testVersion3",
		}
		dt1 := &task.Task{
			Id:                  "displayTaskOld",
			DisplayName:         "displayTask",
			RevisionOrderNumber: 5,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b1.Id,
			Version:             v1.Id,
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
			Version:             v1.Id,
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
			Version:             v2.Id,
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
			Version:             v2.Id,
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
			Version:             v3.Id,
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
			Version:             v3.Id,
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
			Version:             v3.Id,
			Status:              evergreen.TaskStarted,
			Project:             "sample",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(b1.Insert(), ShouldBeNil)
		So(b2.Insert(), ShouldBeNil)
		So(b3.Insert(), ShouldBeNil)
		So(v1.Insert(), ShouldBeNil)
		So(v2.Insert(), ShouldBeNil)
		So(v3.Insert(), ShouldBeNil)
		So(dt1.Insert(), ShouldBeNil)
		So(dt2.Insert(), ShouldBeNil)
		So(dt3.Insert(), ShouldBeNil)
		So(et1.Insert(), ShouldBeNil)
		So(et2.Insert(), ShouldBeNil)
		So(et3.Insert(), ShouldBeNil)
		So(et4.Insert(), ShouldBeNil)
		Convey("deactivating a display task should deactivate its child tasks", func() {
			So(DeactivatePreviousTasks(ctx, dt2, userName), ShouldBeNil)
			dbTask, err := task.FindOne(db.Query(task.ById(dt1.Id)))
			So(err, ShouldBeNil)
			So(dbTask.Activated, ShouldBeFalse)
			dbTask, err = task.FindOne(db.Query(task.ById(et1.Id)))
			So(err, ShouldBeNil)
			So(dbTask.Activated, ShouldBeFalse)
			Convey("but should not touch any tasks that have started", func() {
				dbTask, err = task.FindOne(db.Query(task.ById(dt3.Id)))
				So(err, ShouldBeNil)
				So(dbTask.Activated, ShouldBeTrue)
				dbTask, err = task.FindOne(db.Query(task.ById(et3.Id)))
				So(err, ShouldBeNil)
				So(dbTask.Activated, ShouldBeTrue)
				So(dbTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				dbTask, err = task.FindOne(db.Query(task.ById(et4.Id)))
				So(err, ShouldBeNil)
				So(dbTask.Activated, ShouldBeTrue)
				So(dbTask.Status, ShouldEqual, evergreen.TaskStarted)
			})
		})
	})
}

func TestUpdateBuildStatusForTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type testCase struct {
		tasks []task.Task

		expectedBuildStatus   string
		expectedVersionStatus string
		expectedPatchStatus   string

		expectedBuildActivation   bool
		expectedVersionActivation bool
		expectedPatchActivation   bool
	}

	for name, test := range map[string]testCase{
		"created": {
			tasks: []task.Task{
				{Status: evergreen.TaskUndispatched, Activated: true},
				{Status: evergreen.TaskUndispatched, Activated: true},
			},
			expectedBuildStatus:       evergreen.BuildCreated,
			expectedVersionStatus:     evergreen.VersionCreated,
			expectedPatchStatus:       evergreen.VersionCreated,
			expectedBuildActivation:   true,
			expectedVersionActivation: true,
			expectedPatchActivation:   true,
		},
		"deactivated": {
			tasks: []task.Task{
				{Status: evergreen.TaskUndispatched, Activated: false},
				{Status: evergreen.TaskUndispatched, Activated: false},
			},
			expectedBuildStatus:       evergreen.BuildCreated,
			expectedVersionStatus:     evergreen.VersionCreated,
			expectedPatchStatus:       evergreen.VersionCreated,
			expectedBuildActivation:   false,
			expectedVersionActivation: false,
			expectedPatchActivation:   true, // patch activation is a bit different, since it indicates if the patch has been finalized.
		},
		"started": {
			tasks: []task.Task{
				{Status: evergreen.TaskUndispatched, Activated: true},
				{Status: evergreen.TaskStarted, Activated: true},
			},
			expectedBuildStatus:       evergreen.BuildStarted,
			expectedVersionStatus:     evergreen.VersionStarted,
			expectedPatchStatus:       evergreen.VersionStarted,
			expectedBuildActivation:   true,
			expectedVersionActivation: true,
			expectedPatchActivation:   true,
		},
		"succeeded": {
			tasks: []task.Task{
				{Status: evergreen.TaskSucceeded, Activated: true},
				{Status: evergreen.TaskSucceeded, Activated: true},
			},
			expectedBuildStatus:       evergreen.BuildSucceeded,
			expectedVersionStatus:     evergreen.VersionSucceeded,
			expectedPatchStatus:       evergreen.LegacyPatchSucceeded,
			expectedBuildActivation:   true,
			expectedVersionActivation: true,
			expectedPatchActivation:   true,
		},
		"some unactivated tasks": {
			tasks: []task.Task{
				{Status: evergreen.TaskSucceeded, Activated: true},
				{Status: evergreen.TaskUndispatched, Activated: false},
			},
			expectedBuildStatus:       evergreen.BuildSucceeded,
			expectedVersionStatus:     evergreen.VersionSucceeded,
			expectedPatchStatus:       evergreen.LegacyPatchSucceeded,
			expectedBuildActivation:   true,
			expectedVersionActivation: true,
			expectedPatchActivation:   true,
		},
		"some unactivated but essential tasks": {
			tasks: []task.Task{
				{Status: evergreen.TaskSucceeded, Activated: true},
				{Status: evergreen.TaskUndispatched, Activated: false, IsEssentialToSucceed: true},
			},
			expectedBuildStatus:       evergreen.BuildStarted,
			expectedVersionStatus:     evergreen.VersionStarted,
			expectedPatchStatus:       evergreen.VersionStarted,
			expectedBuildActivation:   true,
			expectedVersionActivation: true,
			expectedPatchActivation:   true,
		},
		"some failed tasks and some unfinished essential tasks": {
			tasks: []task.Task{
				{Status: evergreen.TaskFailed, Activated: true},
				{Status: evergreen.TaskUndispatched, Activated: false, IsEssentialToSucceed: true},
			},
			expectedBuildStatus:       evergreen.BuildFailed,
			expectedVersionStatus:     evergreen.VersionFailed,
			expectedPatchStatus:       evergreen.VersionFailed,
			expectedBuildActivation:   true,
			expectedVersionActivation: true,
			expectedPatchActivation:   true,
		},
		"all blocked tasks": {
			tasks: []task.Task{
				{
					Status:    evergreen.TaskUndispatched,
					Activated: true,
					DependsOn: []task.Dependency{
						{TaskId: "testDepends1", Status: "*", Unattainable: true},
					},
				},
				{
					Status:    evergreen.TaskUndispatched,
					Activated: true,
					DependsOn: []task.Dependency{
						{TaskId: "testDepends1", Status: "*", Unattainable: true},
					},
				},
			},
			expectedBuildStatus:       evergreen.BuildCreated,
			expectedVersionStatus:     evergreen.VersionCreated,
			expectedPatchStatus:       evergreen.VersionCreated,
			expectedBuildActivation:   true,
			expectedVersionActivation: true,
			expectedPatchActivation:   true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection, patch.Collection))

			b := &build.Build{
				Id:        "buildtest",
				Status:    evergreen.BuildCreated,
				Version:   bson.NewObjectId().Hex(),
				Activated: true,
			}
			v := &Version{
				Id:        b.Version,
				Status:    evergreen.VersionCreated,
				Requester: evergreen.PatchVersionRequester,
				Activated: utility.TruePtr(),
			}
			p := &patch.Patch{
				Id:        patch.NewId(v.Id),
				Status:    evergreen.VersionCreated,
				Activated: true,
			}
			require.NoError(t, b.Insert())
			require.NoError(t, v.Insert())
			require.NoError(t, p.Insert())

			for i, tempTask := range test.tasks {
				tempTask.Id = strconv.Itoa(i)
				tempTask.BuildId = b.Id
				tempTask.Version = v.Id
				require.NoError(t, tempTask.Insert())
			}
			// Verify tasks are inserted and found correctly
			tasks, err := task.FindWithFields(task.ByBuildId(b.Id))
			assert.NoError(t, err)
			assert.Len(t, tasks, 2)

			assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &task.Task{Version: v.Id, BuildId: b.Id}))

			b, err = build.FindOneId(b.Id)
			require.NoError(t, err)
			assert.Equal(t, test.expectedBuildStatus, b.Status)
			assert.Equal(t, test.expectedBuildActivation, b.Activated)

			v, err = VersionFindOneId(v.Id)
			require.NoError(t, err)
			assert.Equal(t, test.expectedVersionStatus, v.Status)
			assert.Equal(t, test.expectedVersionActivation, utility.FromBoolPtr(v.Activated))

			p, err = patch.FindOneId(p.Id.Hex())
			require.NoError(t, err)
			assert.Equal(t, test.expectedPatchStatus, p.Status)
			assert.Equal(t, test.expectedPatchActivation, p.Activated)
		})
	}
}

func TestUpdateVersionAndPatchStatusForBuilds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(build.Collection, patch.Collection, task.Collection, VersionCollection))
	uiConfig := evergreen.UIConfig{Url: "http://localhost"}
	require.NoError(t, uiConfig.Set(ctx))

	sender := send.MakeInternalLogger()
	assert.NoError(t, grip.SetSender(sender))
	defer func() {
		assert.NoError(t, grip.SetSender(send.MakeNative()))
	}()

	b := &build.Build{
		Id:        "buildtest",
		Status:    evergreen.BuildFailed,
		Requester: evergreen.GithubPRRequester,
		Version:   "aaaaaaaaaaff001122334455",
		Activated: true,
	}
	p := &patch.Patch{
		Id:              patch.NewId(b.Version),
		Status:          evergreen.VersionFailed,
		GithubPatchData: thirdparty.GithubPatch{HeadOwner: "q"},
	}
	v := &Version{
		Id:        b.Version,
		Status:    evergreen.VersionFailed,
		Requester: evergreen.GithubPRRequester,
	}
	testTask := task.Task{
		Id:        "testone",
		Activated: true,
		BuildId:   b.Id,
		Project:   "sample",
		Status:    evergreen.TaskUndispatched,
		Version:   b.Version,
	}
	anotherTask := task.Task{
		Id:        "two",
		Activated: true,
		BuildId:   b.Id,
		Project:   "sample",
		Status:    evergreen.TaskFailed,
		StartTime: time.Now().Add(-time.Hour),
		Version:   b.Version,
	}

	assert.NoError(t, b.Insert())
	assert.NoError(t, p.Insert())
	assert.NoError(t, v.Insert())
	assert.NoError(t, testTask.Insert())
	assert.NoError(t, anotherTask.Insert())

	assert.NoError(t, UpdateVersionAndPatchStatusForBuilds([]string{b.Id}))
	dbBuild, err := build.FindOneId(b.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildStarted, dbBuild.Status)
	dbPatch, err := patch.FindOneId(p.Id.Hex())
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionStarted, dbPatch.Status)

	err = task.UpdateOne(
		bson.M{task.IdKey: testTask.Id},
		bson.M{"$set": bson.M{task.StatusKey: evergreen.TaskFailed}},
	)
	assert.NoError(t, err)
	assert.NoError(t, UpdateVersionAndPatchStatusForBuilds([]string{b.Id}))
	dbBuild, err = build.FindOneId(b.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildFailed, dbBuild.Status)
	dbPatch, err = patch.FindOneId(p.Id.Hex())
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionFailed, dbPatch.Status)
}

func TestUpdateBuildStatusForTaskReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.EventCollection))
	displayName := "testName"
	b := &build.Build{
		Id:        "buildtest",
		Status:    evergreen.BuildFailed,
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

	assert.NoError(t, b.Insert())
	assert.NoError(t, v.Insert())
	assert.NoError(t, testTask.Insert())
	assert.NoError(t, anotherTask.Insert())

	assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &testTask))
	dbBuild, err := build.FindOneId(b.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildStarted, dbBuild.Status)
	dbVersion, err := VersionFindOneId(v.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionStarted, dbVersion.Status)
	events, err := event.FindAllByResourceID(v.Id)
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	data := events[0].Data.(*event.VersionEventData)
	assert.Equal(t, evergreen.VersionStarted, data.Status)
}

func TestUpdateVersionStatusForGithubChecks(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.EventCollection))
	b1 := build.Build{
		Id:                "b1",
		Status:            evergreen.BuildStarted,
		Version:           "v1",
		Activated:         true,
		IsGithubCheck:     true,
		GithubCheckStatus: evergreen.BuildSucceeded,
	}

	b2 := build.Build{
		Id:        "b2",
		Status:    evergreen.BuildFailed,
		Version:   "v1",
		Activated: true,
	}

	assert.NoError(t, b1.Insert())
	assert.NoError(t, b2.Insert())
	v1 := Version{
		Id:     "v1",
		Status: evergreen.VersionStarted,
	}
	assert.NoError(t, v1.Insert())
	versionStatus, err := updateVersionStatus(&v1)
	assert.NoError(t, err)
	assert.Equal(t, versionStatus, v1.Status) // version status hasn't changed

	events, err := event.FindAllByResourceID("v1")
	assert.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, events[0].EventType, event.VersionGithubCheckFinished)
}

func TestUpdateVersionStatus(t *testing.T) {
	type testCase struct {
		builds []build.Build

		expectedVersionStatus     string
		expectedVersionAborted    bool
		expectedVersionActivation bool
	}

	for name, test := range map[string]testCase{
		"VersionCreatedForAllUnactivatedBuilds": {
			builds: []build.Build{
				{Status: evergreen.BuildCreated},
				{Status: evergreen.BuildCreated},
			},
			expectedVersionStatus:     evergreen.VersionCreated,
			expectedVersionAborted:    false,
			expectedVersionActivation: false,
		},
		"VersionStartedForMixOfSucceededBuildAndBuildWithUnfinishedEssentialTasks": {
			builds: []build.Build{
				{Status: evergreen.BuildCreated, Activated: true, HasUnfinishedEssentialTask: true},
				{Status: evergreen.BuildSucceeded, Activated: true},
			},
			expectedVersionStatus:     evergreen.VersionStarted,
			expectedVersionAborted:    false,
			expectedVersionActivation: true,
		},
		"VersionStartedForMixOfFailedBuildAndBuildWithUnfinishedEssentialTasks": {
			builds: []build.Build{
				{Status: evergreen.BuildCreated, HasUnfinishedEssentialTask: true},
				{Status: evergreen.BuildFailed, Activated: true},
			},
			expectedVersionStatus:     evergreen.VersionFailed,
			expectedVersionAborted:    false,
			expectedVersionActivation: true,
		},
		"VersionStartedForMixOfFinishedAndUnfinishedBuilds": {
			builds: []build.Build{
				{Status: evergreen.BuildStarted, Activated: true},
				{Status: evergreen.BuildSucceeded, Activated: true},
			},
			expectedVersionStatus:     evergreen.VersionStarted,
			expectedVersionAborted:    false,
			expectedVersionActivation: true,
		},
		"VersionAbortedForAbortedBuild": {
			builds: []build.Build{
				{Status: evergreen.BuildFailed, Activated: true, Aborted: true},
				{Status: evergreen.BuildSucceeded, Activated: true},
			},
			expectedVersionStatus:     evergreen.VersionFailed,
			expectedVersionAborted:    true,
			expectedVersionActivation: true,
		},
		"VersionFinishedForAllFinishedBuilds": {
			builds: []build.Build{
				{Status: evergreen.BuildFailed, Activated: true},
				{Status: evergreen.BuildSucceeded, Activated: true},
			},
			expectedVersionStatus:     evergreen.VersionFailed,
			expectedVersionAborted:    false,
			expectedVersionActivation: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.EventCollection))
			v := &Version{
				Id:        bson.NewObjectId().Hex(),
				Status:    evergreen.VersionCreated,
				Activated: utility.TruePtr(),
			}
			require.NoError(t, v.Insert())
			for i, b := range test.builds {
				b.Id = strconv.Itoa(i)
				b.Version = v.Id
				require.NoError(t, b.Insert())
			}

			status, err := updateVersionStatus(v)
			require.NoError(t, err)
			assert.Equal(t, test.expectedVersionStatus, status)

			dbVersion, err := VersionFindOneId(v.Id)
			require.NoError(t, err)
			assert.Equal(t, test.expectedVersionStatus, dbVersion.Status)
			assert.Equal(t, test.expectedVersionAborted, dbVersion.Aborted)
			assert.Equal(t, test.expectedVersionActivation, utility.FromBoolPtr(dbVersion.Activated))
		})
	}
}

func TestUpdateBuildAndVersionStatusForTaskAbort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection, event.EventCollection))
	displayName := "testName"
	b1 := &build.Build{
		Id:        "buildtest1",
		Status:    evergreen.BuildStarted,
		Version:   "abc",
		Activated: true,
	}
	b2 := &build.Build{
		Id:        "buildtest2",
		Status:    evergreen.BuildSucceeded,
		Version:   "abc",
		Activated: true,
	}
	v := &Version{
		Id:     b1.Version,
		Status: evergreen.VersionStarted,
	}
	testTask := task.Task{
		Id:          "testone",
		DisplayName: displayName,
		Activated:   true,
		BuildId:     b1.Id,
		Project:     "sample",
		Status:      evergreen.TaskStarted,
		Version:     b1.Version,
	}
	anotherTask := task.Task{
		Id:          "two",
		DisplayName: displayName,
		Activated:   true,
		BuildId:     b2.Id,
		Project:     "sample",
		Status:      evergreen.TaskSucceeded,
		StartTime:   time.Now().Add(-time.Hour),
		Version:     b2.Version,
	}

	assert.NoError(t, b1.Insert())
	assert.NoError(t, b2.Insert())
	assert.NoError(t, v.Insert())
	assert.NoError(t, testTask.Insert())
	assert.NoError(t, anotherTask.Insert())

	assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &testTask))
	dbBuild1, err := build.FindOneId(b1.Id)
	assert.NoError(t, err)
	assert.Equal(t, false, dbBuild1.Aborted)
	dbBuild2, err := build.FindOneId(b2.Id)
	assert.NoError(t, err)
	assert.Equal(t, false, dbBuild2.Aborted)
	dbVersion, err := VersionFindOneId(v.Id)
	assert.NoError(t, err)
	assert.Equal(t, false, dbVersion.Aborted)

	// abort started task
	assert.NoError(t, testTask.SetAborted(task.AbortInfo{}))
	assert.NoError(t, testTask.MarkFailed())
	assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &testTask))
	dbBuild1, err = build.FindOneId(b1.Id)
	assert.NoError(t, err)
	assert.Equal(t, true, dbBuild1.Aborted)
	assert.Equal(t, evergreen.BuildFailed, dbBuild1.Status)
	dbBuild2, err = build.FindOneId(b2.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildSucceeded, dbBuild2.Status)
	assert.Equal(t, false, dbBuild2.Aborted)
	dbVersion, err = VersionFindOneId(v.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionFailed, dbVersion.Status)
	assert.Equal(t, true, dbVersion.Aborted)

	// restart aborted task
	assert.NoError(t, testTask.Archive())
	assert.NoError(t, testTask.MarkUnscheduled())
	assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &testTask))
	dbBuild1, err = build.FindOneId(b1.Id)
	assert.NoError(t, err)
	assert.Equal(t, false, dbBuild1.Aborted)
	assert.Equal(t, evergreen.BuildCreated, dbBuild1.Status)
	dbBuild2, err = build.FindOneId(b2.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildSucceeded, dbBuild2.Status)
	assert.Equal(t, false, dbBuild2.Aborted)
	dbVersion, err = VersionFindOneId(v.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionStarted, dbVersion.Status)
	assert.Equal(t, false, dbVersion.Aborted)
}

func TestGetBuildStatus(t *testing.T) {
	// The build shouldn't start until a task starts running.
	buildTasks := []task.Task{
		{Status: evergreen.TaskUndispatched},
		{Status: evergreen.TaskUndispatched},
	}
	buildStatus := getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildCreated, buildStatus.status)
	assert.Equal(t, false, buildStatus.allTasksBlocked)

	// Any started tasks should start the build.
	buildTasks = []task.Task{
		{Status: evergreen.TaskUndispatched, Activated: true},
		{Status: evergreen.TaskStarted},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildStarted, buildStatus.status)
	assert.Equal(t, false, buildStatus.allTasksBlocked)

	// Unactivated tasks shouldn't prevent the build from completing.
	buildTasks = []task.Task{
		{Status: evergreen.TaskUndispatched, Activated: false},
		{Status: evergreen.TaskFailed},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildFailed, buildStatus.status)
	assert.Equal(t, false, buildStatus.allTasksBlocked)

	// Blocked tasks shouldn't prevent the build from completing.
	buildTasks = []task.Task{
		{Status: evergreen.TaskUndispatched,
			DependsOn: []task.Dependency{{Unattainable: true}}},
		{Status: evergreen.TaskSucceeded},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildSucceeded, buildStatus.status)
	assert.Equal(t, false, buildStatus.allTasksBlocked)

	buildTasks = []task.Task{
		{
			Status:    evergreen.TaskUndispatched,
			DependsOn: []task.Dependency{{Unattainable: true}},
			Activated: true,
		},
		{Status: evergreen.TaskFailed},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildFailed, buildStatus.status)
	assert.Equal(t, false, buildStatus.allTasksBlocked)

	// Blocked tasks that are overriding dependencies should prevent the build from being completed.
	buildTasks = []task.Task{
		{
			Status:               evergreen.TaskUndispatched,
			DependsOn:            []task.Dependency{{Unattainable: true}},
			OverrideDependencies: true,
			Activated:            true,
		},
		{Status: evergreen.TaskSucceeded},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildStarted, buildStatus.status)
	assert.Equal(t, false, buildStatus.allTasksBlocked)

	// Builds with only blocked tasks should stay as created.
	buildTasks = []task.Task{
		{Status: evergreen.TaskUndispatched,
			DependsOn: []task.Dependency{{Unattainable: true}}},
		{Status: evergreen.TaskUndispatched,
			DependsOn: []task.Dependency{{Unattainable: true}}},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildCreated, buildStatus.status)
	assert.Equal(t, true, buildStatus.allTasksBlocked)

}

func TestGetVersionStatus(t *testing.T) {
	t.Run("VersionCreatedForAllCreatedButInactiveBuilds", func(t *testing.T) {
		versionBuilds := []build.Build{
			{Status: evergreen.BuildCreated},
			{Status: evergreen.BuildCreated},
		}
		activated, status := getVersionActivationAndStatus(versionBuilds)
		assert.Equal(t, evergreen.VersionCreated, status)
		assert.False(t, activated) // false because no task is activated
	})

	t.Run("VersionCreatedForAllCreatedBuildsWithSomeUnfinishedEssentialTasks", func(t *testing.T) {
		versionBuilds := []build.Build{
			{Status: evergreen.BuildCreated, HasUnfinishedEssentialTask: true},
			{Status: evergreen.BuildCreated},
		}
		activated, status := getVersionActivationAndStatus(versionBuilds)
		assert.Equal(t, evergreen.VersionCreated, status)
		assert.False(t, activated)
	})

	t.Run("VersionCreatedForAllCreatedAndPartialActiveBuilds", func(t *testing.T) {
		// Any activated build implies that the version is activated
		versionBuilds := []build.Build{
			{Status: evergreen.BuildCreated, Activated: true},
			{Status: evergreen.BuildCreated},
		}
		activated, status := getVersionActivationAndStatus(versionBuilds)
		assert.Equal(t, evergreen.VersionCreated, status)
		assert.True(t, activated)
	})

	t.Run("VersionStartedForAtLeastOneStartedBuild", func(t *testing.T) {
		versionBuilds := []build.Build{
			{Status: evergreen.BuildCreated},
			{Status: evergreen.BuildStarted, Activated: true},
		}
		activated, status := getVersionActivationAndStatus(versionBuilds)
		assert.Equal(t, evergreen.VersionStarted, status)
		assert.True(t, activated)
	})

	t.Run("VersionStartedForMixOfFinishedAndCreatedBuilds", func(t *testing.T) {
		versionBuilds := []build.Build{
			{Status: evergreen.BuildCreated, Activated: true},
			{Status: evergreen.BuildFailed, Activated: true},
		}
		activated, status := getVersionActivationAndStatus(versionBuilds)
		assert.Equal(t, evergreen.VersionStarted, status)
		assert.True(t, activated)
	})

	t.Run("VersionFailedForMixOfFailedBuildAndBuildsWithUnfinishedEssentialTasks", func(t *testing.T) {
		versionBuilds := []build.Build{
			{Status: evergreen.BuildCreated, HasUnfinishedEssentialTask: true},
			{Status: evergreen.BuildFailed, Activated: true},
		}
		activated, status := getVersionActivationAndStatus(versionBuilds)
		assert.Equal(t, evergreen.VersionFailed, status)
		assert.True(t, activated)
	})

	t.Run("VersionFailedForMixOfFinishedAndUnactivatedBuilds", func(t *testing.T) {
		versionBuilds := []build.Build{
			{Status: evergreen.BuildCreated, Activated: false},
			{Status: evergreen.BuildFailed, Activated: true},
		}
		activated, status := getVersionActivationAndStatus(versionBuilds)
		assert.Equal(t, evergreen.VersionFailed, status)
		assert.True(t, activated)
	})
}

func TestUpdateVersionGithubStatus(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionCollection, event.EventCollection))
	versionID := "v1"
	v := &Version{Id: versionID}
	require.NoError(t, v.Insert())

	builds := []build.Build{
		{IsGithubCheck: true, Status: evergreen.BuildSucceeded},
		{IsGithubCheck: false, Status: evergreen.BuildCreated},
	}

	assert.NoError(t, updateVersionGithubStatus(v, builds))

	e, err := event.FindUnprocessedEvents(-1)
	assert.NoError(t, err)
	require.Len(t, e, 1)
}

func TestUpdateBuildGithubStatus(t *testing.T) {
	require.NoError(t, db.ClearCollections(build.Collection, event.EventCollection))
	buildID := "b1"
	b := &build.Build{Id: buildID}
	require.NoError(t, b.Insert())

	tasks := []task.Task{
		{IsGithubCheck: true, Status: evergreen.TaskSucceeded},
		{IsGithubCheck: false, Status: evergreen.TaskUndispatched},
	}

	assert.NoError(t, updateBuildGithubStatus(b, tasks))

	b, err := build.FindOneId(buildID)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildSucceeded, b.GithubCheckStatus)

	e, err := event.FindUnprocessedEvents(-1)
	assert.NoError(t, err)
	require.Len(t, e, 1)
}

func TestTaskStatusImpactedByFailedTest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.Clear(ProjectRefCollection))
	projRef := &ProjectRef{
		Id: "p1",
	}
	assert.NoError(t, projRef.Insert())

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
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
			}
			v = &Version{
				Id:         b.Version,
				Identifier: "p1",
				Status:     evergreen.VersionStarted,
			}
			testTask = &task.Task{
				Id:          "testone",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Project:     "p1",
				Version:     b.Version,
				HostId:      "myHost",
			}
			taskHost := &host.Host{
				Id:          "myHost",
				RunningTask: testTask.Id,
			}
			pp := &ParserProject{
				Id:         b.Version,
				Identifier: utility.ToStringPtr("p1"),
			}
			detail = &apimodels.TaskEndDetail{
				Status: evergreen.TaskSucceeded,
			}
			pRef := ProjectRef{Id: "p1"}
			pConfig := ProjectConfig{Id: "p1"}
			require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection, host.Collection,
				ProjectRefCollection, ProjectConfigCollection, ParserProjectCollection))
			So(pRef.Insert(), ShouldBeNil)
			So(pConfig.Insert(), ShouldBeNil)
			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
			So(pp.Insert(), ShouldBeNil)
			So(taskHost.Insert(ctx), ShouldBeNil)
		}

		Convey("task should not fail if there are no failed test", func() {
			reset()
			testTask.ResultsService = testresult.TestResultsServiceLocal
			So(MarkEnd(ctx, settings, testTask, "", time.Now(), detail, true), ShouldBeNil)

			v, err := VersionFindOneId(v.Id)
			So(err, ShouldBeNil)
			So(v.Status, ShouldEqual, evergreen.VersionSucceeded)

			b, err := build.FindOneId(b.Id)
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildSucceeded)

			taskData, err := task.FindOne(db.Query(task.ById(testTask.Id)))
			So(err, ShouldBeNil)
			So(taskData.Status, ShouldEqual, evergreen.TaskSucceeded)
		})

		Convey("task should fail if there are failing tests", func() {
			reset()
			testTask.ResultsService = testresult.TestResultsServiceLocal
			testTask.ResultsFailed = true
			So(MarkEnd(ctx, settings, testTask, "", time.Now(), detail, true), ShouldBeNil)

			v, err := VersionFindOneId(v.Id)
			So(err, ShouldBeNil)
			So(v.Status, ShouldEqual, evergreen.VersionFailed)

			b, err := build.FindOneId(b.Id)
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildFailed)

			taskData, err := task.FindOne(db.Query(task.ById(testTask.Id)))
			So(err, ShouldBeNil)
			So(taskData.Status, ShouldEqual, evergreen.TaskFailed)
			So(taskData.Details.Type, ShouldEqual, evergreen.CommandTypeTest)
			So(taskData.Details.Description, ShouldEqual, evergreen.TaskDescriptionResultsFailed)
		})

		Convey("incomplete versions report updates", func() {
			reset()
			b2 := &build.Build{
				Id:        "buildtest2",
				Version:   "abc",
				Activated: false,
				Status:    evergreen.BuildCreated,
			}
			So(b2.Insert(), ShouldBeNil)
			detail.Status = evergreen.TaskFailed
			So(MarkEnd(ctx, settings, testTask, "", time.Now(), detail, true), ShouldBeNil)

			v, err := VersionFindOneId(v.Id)
			So(err, ShouldBeNil)
			So(v.Status, ShouldEqual, evergreen.VersionFailed)

			b, err := build.FindOneId(b.Id)
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildFailed)
		})
	})
}

func TestMarkEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(task.Collection, build.Collection, host.Collection,
		VersionCollection, ProjectRefCollection, ParserProjectCollection))

	displayName := "testName"
	userName := "testUser"
	b := &build.Build{
		Id:      "buildtest",
		Status:  evergreen.BuildStarted,
		Version: "abc",
	}
	v := &Version{
		Id:         b.Version,
		Identifier: "p1",
		Status:     evergreen.VersionStarted,
	}
	projRef := &ProjectRef{
		Id: "p1",
	}
	testTask := task.Task{
		Id:          "testone",
		DisplayName: displayName,
		Activated:   true,
		BuildId:     b.Id,
		Project:     "p1",
		Status:      evergreen.TaskStarted,
		Version:     b.Version,
		HostId:      "taskHost",
	}
	taskHost := host.Host{
		Id:          "taskHost",
		RunningTask: testTask.Id,
	}
	dependentTask := task.Task{
		Id:        "dependentTask",
		Activated: true,
		BuildId:   b.Id,
		Project:   "p1",
		Status:    evergreen.TaskUndispatched,
		Version:   b.Version,
		DependsOn: []task.Dependency{
			{TaskId: testTask.Id},
		},
	}
	pp := &ParserProject{
		Id:         b.Version,
		Identifier: utility.ToStringPtr("sample"),
	}

	require.NoError(projRef.Insert())
	require.NoError(b.Insert())
	require.NoError(testTask.Insert())
	require.NoError(v.Insert())
	require.NoError(pp.Insert())
	require.NoError(dependentTask.Insert())
	require.NoError(taskHost.Insert(ctx))

	details := apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
	}
	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	assert.NoError(MarkEnd(ctx, settings, &testTask, userName, time.Now(), &details, false))

	b, err := build.FindOneId(b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildFailed, b.Status)

	dbDependentTask, err := task.FindOneId(dependentTask.Id)
	require.NoError(err)
	require.NotZero(dbDependentTask)
	require.Len(dbDependentTask.DependsOn, 1)
	assert.Equal(testTask.Id, dbDependentTask.DependsOn[0].TaskId)
	assert.True(dbDependentTask.DependsOn[0].Finished, "dependency should be marked finished")

	Convey("with a task that is part of a display task", t, func() {
		p := &Project{
			Identifier: "sample",
		}
		b := &build.Build{
			Id:      "displayBuild",
			Project: p.Identifier,
			Version: "version1",
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
			Version:   "version1",
			HostId:    taskHost.Id,
		}
		t2 := &task.Task{
			Id:        "execTask2",
			Activated: true,
			BuildId:   b.Id,
			Status:    evergreen.TaskStarted,
			Version:   "version1",
			HostId:    taskHost.Id,
		}
		So(t1.Insert(), ShouldBeNil)
		So(t2.Insert(), ShouldBeNil)

		detail := &apimodels.TaskEndDetail{
			Status: evergreen.TaskSucceeded,
		}
		endTime := time.Now().Round(time.Second)
		So(MarkEnd(ctx, settings, t1, "test", endTime, detail, false), ShouldBeNil)
		t1FromDb, err := task.FindOne(db.Query(task.ById(t1.Id)))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskSucceeded)
		dtFromDb, err := task.FindOne(db.Query(task.ById(dt.Id)))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskSucceeded)

		// Ensure that calling MarkEnd on a non-aborted finished task returns early
		// by checking that its finish_time hasn't changed
		So(MarkEnd(ctx, settings, t1, "test", time.Now().Add(time.Minute), detail, false), ShouldBeNil)
		t1FromDb, err = task.FindOne(db.Query(task.ById(t1.Id)))
		So(err, ShouldBeNil)
		So(t1FromDb.FinishTime, ShouldEqual, endTime)

		// Ensure that calling MarkEnd on an aborted finished task does not return early.
		endTime = time.Now().Round(time.Second)
		So(AbortTask(ctx, t2.Id, "testUser"), ShouldBeNil)
		t2FromDb, err := task.FindOne(db.Query(task.ById(t2.Id)))
		So(err, ShouldBeNil)
		So(t2FromDb.FinishTime, ShouldEqual, time.Time{})
		So(MarkEnd(ctx, settings, t2FromDb, "test", endTime, &t2FromDb.Details, false), ShouldBeNil)
		t2FromDb, err = task.FindOne(db.Query(task.ById(t2.Id)))
		So(err, ShouldBeNil)
		So(t2FromDb.FinishTime, ShouldEqual, endTime)
	})
}

func TestMarkEndWithTaskGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		HostId:            "h1",
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
	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	for name, test := range map[string]func(*testing.T){
		"NotResetWhenFinished": func(t *testing.T) {
			assert.NoError(t, MarkEnd(ctx, settings, runningTask, "test", time.Now(), detail, false))
			runningTaskDB, err := task.FindOneId(runningTask.Id)
			assert.NoError(t, err)
			assert.NotNil(t, runningTaskDB)
			assert.Equal(t, evergreen.TaskFailed, runningTaskDB.Status)
		},
		"ResetWhenFinished": func(t *testing.T) {
			assert.NoError(t, runningTask.SetResetWhenFinished())
			assert.NoError(t, MarkEnd(ctx, settings, runningTask, "test", time.Now(), detail, false))

			runningTaskDB, err := task.FindOneId(runningTask.Id)
			assert.NoError(t, err)
			assert.NotNil(t, runningTaskDB)
			assert.NotEqual(t, evergreen.TaskFailed, runningTaskDB.Status)
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(distro.Collection, host.Collection, task.Collection, task.OldCollection,
				build.Collection, VersionCollection, ParserProjectCollection, ProjectRefCollection))
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
			assert.NoError(h.Insert(ctx))
			b := build.Build{
				Id:      "b",
				Version: "abc",
			}
			v := &Version{
				Id:     b.Version,
				Status: evergreen.VersionStarted,
			}
			pp := &ParserProject{}
			err := util.UnmarshalYAMLWithFallback([]byte(sampleProjYmlTaskGroups), &pp)
			assert.NoError(err)
			pp.Id = b.Version
			assert.NoError(pp.Insert())
			assert.NoError(b.Insert())
			assert.NoError(v.Insert())

			d := distro.Distro{
				Id: "my_distro",
				PlannerSettings: distro.PlannerSettings{
					Version: evergreen.PlannerVersionTunable,
				},
			}
			assert.NoError(d.Insert(ctx))

			test(t)
		})
	}
}

func TestTryResetTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	Convey("With a task that does not exist", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection))
		So(TryResetTask(ctx, settings, "id", "username", "", nil), ShouldNotBeNil)
	})
	Convey("With a task, a build, version and a project", t, func() {
		Convey("resetting a task without a max number of executions", func() {
			require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection))

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
			dependentTask := &task.Task{
				Id:        "testthree",
				Activated: true,
				BuildId:   b.Id,
				Execution: 1,
				Project:   "sample",
				Version:   b.Version,
				DependsOn: []task.Dependency{
					{TaskId: testTask.Id, Status: evergreen.TaskSucceeded, Finished: true},
				},
			}
			detail := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}

			var err error

			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(otherTask.Insert(), ShouldBeNil)
			So(dependentTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
			Convey("should reset and add a task to the old tasks collection", func() {
				So(TryResetTask(ctx, settings, testTask.Id, userName, "", detail), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
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

				// Task's dependency should be marked as unfinished.
				dbDependentTask, err := task.FindOneId(dependentTask.Id)
				So(err, ShouldBeNil)
				So(dbDependentTask, ShouldNotBeNil)
				So(len(dbDependentTask.DependsOn), ShouldEqual, 1)
				So(dbDependentTask.DependsOn[0].TaskId, ShouldEqual, testTask.Id)
				So(dbDependentTask.DependsOn[0].Finished, ShouldBeFalse)
			})
			Convey("with a container task", func() {
				containerTask := &task.Task{
					Id:                          "container_task",
					DisplayName:                 displayName,
					Activated:                   false,
					BuildId:                     b.Id,
					Execution:                   1,
					Project:                     "sample",
					Status:                      evergreen.TaskSucceeded,
					Version:                     b.Version,
					ExecutionPlatform:           task.ExecutionPlatformContainer,
					PodID:                       "pod_id",
					ContainerAllocationAttempts: 2,
				}
				So(containerTask.Insert(), ShouldBeNil)

				Convey("should reset task state specific to containers", func() {
					So(TryResetTask(ctx, settings, containerTask.Id, userName, "source", detail), ShouldBeNil)

					dbTask, err := task.FindOneId(containerTask.Id)
					So(err, ShouldBeNil)
					So(dbTask.Details, ShouldResemble, apimodels.TaskEndDetail{})
					So(dbTask.Status, ShouldEqual, evergreen.TaskUndispatched)
					So(dbTask.FinishTime, ShouldResemble, utility.ZeroTime)
					So(dbTask.Activated, ShouldBeTrue)
					So(dbTask.ContainerAllocationAttempts, ShouldEqual, 0)
					So(dbTask.PodID, ShouldBeZeroValue)
					oldTask, err := task.FindOneOldByIdAndExecution(dbTask.Id, 1)
					So(err, ShouldBeNil)
					So(oldTask, ShouldNotBeNil)
					So(oldTask.Execution, ShouldEqual, 1)
					So(oldTask.Details, ShouldResemble, *detail)
					So(oldTask.FinishTime, ShouldNotResemble, utility.ZeroTime)
					So(oldTask.ContainerAllocationAttempts, ShouldEqual, 2)
					So(oldTask.PodID, ShouldEqual, containerTask.PodID)
				})
			})
		})
		Convey("resetting a task with a max number of executions", func() {
			require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection))
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
			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
			So(anotherTask.Insert(), ShouldBeNil)

			commitQueueMax := settings.CommitQueue.MaxSystemFailedTaskRetries
			commitQueueTask := &task.Task{
				Id:          "commit_queue_task",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Execution:   commitQueueMax,
				Project:     "sample",
				Status:      evergreen.TaskSystemFailed,
				Version:     b.Version,
				Requester:   evergreen.MergeTestRequester,
			}
			So(commitQueueTask.Insert(), ShouldBeNil)

			var err error

			Convey("should reset if ui package tries to reset", func() {
				So(TryResetTask(ctx, settings, testTask.Id, userName, evergreen.UIPackage, detail), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
			})
			Convey("should not reset if an origin other than the ui package tries to reset", func() {
				So(TryResetTask(ctx, settings, testTask.Id, userName, "", detail), ShouldBeNil)
				testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldNotResemble, *detail)
				So(testTask.Status, ShouldNotEqual, detail.Status)
			})
			Convey("should reset and use detail information if the UI package passes in a detail ", func() {
				So(TryResetTask(ctx, settings, anotherTask.Id, userName, evergreen.UIPackage, detail), ShouldBeNil)
				a, err := task.FindOne(db.Query(task.ById(anotherTask.Id)))
				So(err, ShouldBeNil)
				So(a.Details, ShouldResemble, apimodels.TaskEndDetail{})
				So(a.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(a.FinishTime, ShouldResemble, utility.ZeroTime)
			})
			Convey("merge tasks not reset if they've reached the admin setting limit", func() {
				So(TryResetTask(ctx, settings, commitQueueTask.Id, userName, "", detail), ShouldBeNil)
				commitQueueTask, err = task.FindOne(db.Query(task.ById(commitQueueTask.Id)))
				So(err, ShouldBeNil)
				So(commitQueueTask.Details, ShouldNotResemble, *detail)
				So(commitQueueTask.Status, ShouldNotEqual, detail.Status)
			})
		})
	})

	Convey("with a display task", t, func() {
		b := &build.Build{
			Id:      "displayBuild",
			Project: "sample",
			Version: "version1",
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

		So(TryResetTask(ctx, settings, dt.Id, "user", "test", nil), ShouldBeNil)
		t1FromDb, err := task.FindOne(db.Query(task.ById(t1.Id)))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskUndispatched)
		dtFromDb, err := task.FindOne(db.Query(task.ById(dt.Id)))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskUndispatched)
	})
}

func TestTryResetTaskWithTaskGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(host.Collection, build.Collection, VersionCollection, distro.Collection))
	assert := assert.New(t)
	require := require.New(t)

	h := &host.Host{
		Id:          "h1",
		RunningTask: "say-hi",
	}
	assert.NoError(h.Insert(ctx))
	b := build.Build{
		Id:      "b",
		Version: "abc",
	}
	v := &Version{
		Id:     b.Version,
		Status: evergreen.VersionStarted,
	}
	assert.NoError(b.Insert())
	assert.NoError(v.Insert())
	d := &distro.Distro{
		Id: "my_distro",
		PlannerSettings: distro.PlannerSettings{
			Version: evergreen.PlannerVersionLegacy,
		},
	}
	assert.NoError(d.Insert(ctx))

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}

	for name, test := range map[string]func(*testing.T, *task.Task, string){
		"NotFinished": func(t *testing.T, t1 *task.Task, t2Id string) {
			assert.NoError(TryResetTask(ctx, settings, t2Id, "user", "test", nil))
			err := TryResetTask(ctx, settings, t1.Id, "user", evergreen.UIPackage, nil)
			require.Error(err)
			assert.Contains(err.Error(), "cannot reset task in this status")
		},
		"CanResetTaskGroup": func(t *testing.T, t1 *task.Task, t2Id string) {
			assert.NoError(t1.MarkFailed())
			assert.NoError(TryResetTask(ctx, settings, t2Id, "user", "test", nil))

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a task and a build", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection))
		displayName := "testName"
		userName := "testUser"
		b := &build.Build{
			Id: "buildtest",
		}
		v := &Version{
			Id: "versiontest",
		}
		testTask := &task.Task{
			Id:          "testone",
			DisplayName: displayName,
			Activated:   false,
			BuildId:     b.Id,
			Status:      evergreen.TaskStarted,
			Version:     v.Id,
		}
		finishedTask := &task.Task{
			Id:          "another",
			DisplayName: displayName,
			Activated:   false,
			BuildId:     b.Id,
			Status:      evergreen.TaskFailed,
		}
		So(b.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(finishedTask.Insert(), ShouldBeNil)
		var err error
		Convey("with a task that has started, aborting a task should work", func() {
			So(AbortTask(ctx, testTask.Id, userName), ShouldBeNil)
			testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
			So(err, ShouldBeNil)
			So(testTask.Activated, ShouldEqual, false)
			So(testTask.Aborted, ShouldEqual, true)
		})
		Convey("a task that is finished should error when aborting", func() {
			So(AbortTask(ctx, finishedTask.Id, userName), ShouldNotBeNil)
		})
		Convey("a display task should abort its execution tasks", func() {
			dt := task.Task{
				Id:             "dt",
				DisplayOnly:    true,
				ExecutionTasks: []string{"et1", "et2"},
				Status:         evergreen.TaskStarted,
				BuildId:        b.Id,
				Version:        v.Id,
			}
			So(dt.Insert(), ShouldBeNil)
			et1 := task.Task{
				Id:      "et1",
				Status:  evergreen.TaskStarted,
				BuildId: b.Id,
				Version: v.Id,
			}
			So(et1.Insert(), ShouldBeNil)
			et2 := task.Task{
				Id:      "et2",
				Status:  evergreen.TaskFailed,
				BuildId: b.Id,
				Version: v.Id,
			}
			So(et2.Insert(), ShouldBeNil)

			So(AbortTask(ctx, dt.Id, userName), ShouldBeNil)
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
func TestTryDequeueAndAbortBlockedCommitQueueItem(t *testing.T) {
	assert.NoError(t, db.ClearCollections(patch.Collection, VersionCollection, task.Collection, build.Collection, commitqueue.Collection))
	patchID := "aabbccddeeff001122334455"
	v := &Version{
		Id:     patchID,
		Status: evergreen.VersionStarted,
	}

	p := &patch.Patch{
		Id:          patch.NewId(patchID),
		Version:     v.Id,
		Status:      evergreen.VersionStarted,
		PatchNumber: 12,
		Alias:       evergreen.CommitQueueAlias,
	}
	b := build.Build{
		Id:      "my-build",
		Version: v.Id,
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
		{Issue: patchID, PatchId: patchID, Source: commitqueue.SourceDiff, Version: patchID},
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

	removed, err := tryDequeueAndAbortCommitQueueItem(p, *cq, t1.Id, "some merge error", evergreen.User)
	assert.NoError(t, err)
	require.NotZero(t, removed)
	assert.Equal(t, p.Id.Hex(), removed.PatchId)

	cq, err = commitqueue.FindOneId("my-project")
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

func TestTryDequeueAndAbortCommitQueueItem(t *testing.T) {
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
		Status:  evergreen.VersionStarted,
	}
	b := build.Build{
		Id:      "my-build",
		Version: v.Id,
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
		{Issue: versionId.Hex(), PatchId: versionId.Hex(), Source: commitqueue.SourceDiff, Version: v.Id},
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

	removed, err := tryDequeueAndAbortCommitQueueItem(p, *cq, t1.Id, "some merge error", evergreen.User)
	assert.NoError(t, err)
	require.NotZero(t, removed)
	assert.Equal(t, p.Id.Hex(), removed.PatchId)

	cq, err = commitqueue.FindOneId("my-project")
	assert.NoError(t, err)
	assert.Equal(t, cq.FindItem("12"), -1)
	assert.Len(t, cq.Queue, 1)

	// check that all tasks are now in the correct state
	tasks, err := task.FindAll(task.All)
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

func TestDequeueAndRestartForFirstItemInBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(VersionCollection, patch.Collection, build.Collection, task.Collection, commitqueue.Collection, task.OldCollection))
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
	require.NoError(t, t1.Insert())
	t2 := task.Task{
		Id:               "2",
		Version:          v2.Hex(),
		BuildId:          "2",
		Project:          "p",
		Status:           evergreen.TaskFailed,
		Requester:        evergreen.MergeTestRequester,
		CommitQueueMerge: true,
	}
	require.NoError(t, t2.Insert())
	t3 := task.Task{
		Id:               "3",
		Version:          v3.Hex(),
		BuildId:          "3",
		Project:          "p",
		Status:           evergreen.TaskUndispatched,
		Requester:        evergreen.MergeTestRequester,
		CommitQueueMerge: true,
		DependsOn: []task.Dependency{
			{TaskId: t2.Id, Status: "*", Finished: true},
		},
	}
	require.NoError(t, t3.Insert())
	t4 := task.Task{
		Id:        "4",
		Version:   v3.Hex(),
		BuildId:   "3",
		Project:   "p",
		Status:    evergreen.TaskSucceeded,
		Requester: evergreen.MergeTestRequester,
	}
	require.NoError(t, t4.Insert())
	b1 := build.Build{
		Id:      "1",
		Version: v1.Hex(),
	}
	require.NoError(t, b1.Insert())
	b2 := build.Build{
		Id:      "2",
		Version: v2.Hex(),
	}
	require.NoError(t, b2.Insert())
	b3 := build.Build{
		Id:      "3",
		Version: v3.Hex(),
	}
	require.NoError(t, b3.Insert())
	p1 := patch.Patch{
		Id:      v1,
		Alias:   evergreen.CommitQueueAlias,
		Version: v1.Hex(),
	}
	require.NoError(t, p1.Insert())
	p2 := patch.Patch{
		Id:      v2,
		Alias:   evergreen.CommitQueueAlias,
		Version: v2.Hex(),
	}
	require.NoError(t, p2.Insert())
	p3 := patch.Patch{
		Id:      v3,
		Alias:   evergreen.CommitQueueAlias,
		Version: v3.Hex(),
	}
	p4 := patch.Patch{
		Id:    mgobson.NewObjectId(),
		Alias: evergreen.CommitQueueAlias,
	}
	require.NoError(t, p3.Insert())
	version1 := Version{
		Id: v1.Hex(),
	}
	require.NoError(t, version1.Insert())
	version2 := Version{
		Id: v2.Hex(),
	}
	require.NoError(t, version2.Insert())
	version3 := Version{
		Id: v3.Hex(),
	}
	require.NoError(t, version3.Insert())
	cq := commitqueue.CommitQueue{
		ProjectID: "p",
		Queue: []commitqueue.CommitQueueItem{
			{Issue: v1.Hex(), PatchId: p1.Id.Hex(), Version: v1.Hex()},
			{Issue: v2.Hex(), PatchId: p2.Id.Hex(), Version: v2.Hex()},
			{Issue: v3.Hex(), PatchId: p3.Id.Hex(), Version: v3.Hex()},
			{Issue: p4.Id.Hex(), PatchId: p4.Id.Hex()},
		},
	}
	require.NoError(t, commitqueue.InsertQueue(&cq))

	assert.NoError(t, DequeueAndRestartForTask(ctx, &cq, &t2, message.GithubStateFailure, "", ""))
	dbCq, err := commitqueue.FindOneId(cq.ProjectID)
	assert.NoError(t, err)
	require.Len(t, dbCq.Queue, 3)
	assert.Equal(t, v1.Hex(), dbCq.Queue[0].Issue)
	assert.Equal(t, v3.Hex(), dbCq.Queue[1].Issue)
	assert.Equal(t, p4.Id.Hex(), dbCq.Queue[2].Issue)
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
	require.Len(t, dbTask3.DependsOn, 1)
	assert.Equal(t, t1.Id, dbTask3.DependsOn[0].TaskId)
	assert.False(t, dbTask3.DependsOn[0].Finished)
	dbTask4, err := task.FindOneId(t4.Id)
	assert.NoError(t, err)
	assert.Equal(t, 1, dbTask4.Execution)
}

func TestDequeueAndRestartForItemInMiddleOfBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(VersionCollection, patch.Collection, build.Collection, task.Collection, commitqueue.Collection, task.OldCollection))
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
	require.NoError(t, t1.Insert())
	t2 := task.Task{
		Id:               "2",
		Version:          v2.Hex(),
		BuildId:          "2",
		Project:          "p",
		Status:           evergreen.TaskFailed,
		Requester:        evergreen.MergeTestRequester,
		CommitQueueMerge: true,
	}
	require.NoError(t, t2.Insert())
	t3 := task.Task{
		Id:               "3",
		Version:          v3.Hex(),
		BuildId:          "3",
		Project:          "p",
		Status:           evergreen.TaskUndispatched,
		Requester:        evergreen.MergeTestRequester,
		CommitQueueMerge: true,
		DependsOn: []task.Dependency{
			{TaskId: t2.Id, Status: "*", Finished: true},
		},
	}
	require.NoError(t, t3.Insert())
	t4 := task.Task{
		Id:        "4",
		Version:   v3.Hex(),
		BuildId:   "3",
		Project:   "p",
		Status:    evergreen.TaskSucceeded,
		Requester: evergreen.MergeTestRequester,
	}
	require.NoError(t, t4.Insert())
	b1 := build.Build{
		Id:      "1",
		Version: v1.Hex(),
	}
	require.NoError(t, b1.Insert())
	b2 := build.Build{
		Id:      "2",
		Version: v2.Hex(),
	}
	require.NoError(t, b2.Insert())
	b3 := build.Build{
		Id:      "3",
		Version: v3.Hex(),
	}
	require.NoError(t, b3.Insert())
	p1 := patch.Patch{
		Id:      v1,
		Alias:   evergreen.CommitQueueAlias,
		Version: v1.Hex(),
	}
	require.NoError(t, p1.Insert())
	p2 := patch.Patch{
		Id:      v2,
		Alias:   evergreen.CommitQueueAlias,
		Version: v2.Hex(),
	}
	require.NoError(t, p2.Insert())
	p3 := patch.Patch{
		Id:      v3,
		Alias:   evergreen.CommitQueueAlias,
		Version: v3.Hex(),
	}
	p4 := patch.Patch{
		Id:    mgobson.NewObjectId(),
		Alias: evergreen.CommitQueueAlias,
	}
	require.NoError(t, p3.Insert())
	version1 := Version{
		Id: v1.Hex(),
	}
	require.NoError(t, version1.Insert())
	version2 := Version{
		Id: v2.Hex(),
	}
	require.NoError(t, version2.Insert())
	version3 := Version{
		Id: v3.Hex(),
	}
	require.NoError(t, version3.Insert())
	cq := commitqueue.CommitQueue{
		ProjectID: "p",
		Queue: []commitqueue.CommitQueueItem{
			{Issue: v1.Hex(), PatchId: p1.Id.Hex(), Version: v1.Hex()},
			{Issue: v2.Hex(), PatchId: p2.Id.Hex(), Version: v2.Hex()},
			{Issue: v3.Hex(), PatchId: p3.Id.Hex(), Version: v3.Hex()},
			{Issue: p4.Id.Hex(), PatchId: p4.Id.Hex()},
		},
	}
	require.NoError(t, commitqueue.InsertQueue(&cq))

	removed, err := DequeueAndRestartForVersion(ctx, &cq, cq.ProjectID, v2.Hex(), "user", "reason")
	assert.NoError(t, err)
	require.NotZero(t, removed)
	assert.Equal(t, v2.Hex(), removed.Issue)

	dbCq, err := commitqueue.FindOneId(cq.ProjectID)
	assert.NoError(t, err)
	require.Len(t, dbCq.Queue, 3)
	assert.Equal(t, v1.Hex(), dbCq.Queue[0].Issue)
	assert.Equal(t, v3.Hex(), dbCq.Queue[1].Issue)
	assert.Equal(t, p4.Id.Hex(), dbCq.Queue[2].Issue)
	dbTask1, err := task.FindOneId(t1.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0, dbTask1.Execution)
	assert.Equal(t, t1.Status, dbTask1.Status)
	dbTask2, err := task.FindOneId(t2.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0, dbTask2.Execution)
	assert.Equal(t, t1.Status, dbTask1.Status)
	dbTask3, err := task.FindOneId(t3.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0, dbTask3.Execution)
	assert.Equal(t, evergreen.TaskUndispatched, dbTask3.Status)
	require.Len(t, dbTask3.DependsOn, 1)
	assert.Equal(t, t1.Id, dbTask3.DependsOn[0].TaskId)
	assert.False(t, dbTask3.DependsOn[0].Finished)
	dbTask4, err := task.FindOneId(t4.Id)
	assert.NoError(t, err)
	assert.Equal(t, 1, dbTask4.Execution)
	assert.Equal(t, evergreen.TaskUndispatched, dbTask4.Status)
}

func TestMarkStart(t *testing.T) {
	Convey("With a task, build and version", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection))
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
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)

		Convey("when calling MarkStart, the task, version and build should be updated", func() {
			updates := StatusChanges{}
			err := MarkStart(testTask, &updates)
			So(updates.BuildNewStatus, ShouldBeEmpty)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(err, ShouldBeNil)
			testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskStarted)
			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
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
		t1FromDb, err := task.FindOne(db.Query(task.ById(t1.Id)))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskStarted)
		dtFromDb, err := task.FindOne(db.Query(task.ById(dt.Id)))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskStarted)
	})
}

func TestMarkDispatched(t *testing.T) {
	Convey("With a task, build and version", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection))
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
			So(MarkHostTaskDispatched(testTask, sampleHost), ShouldBeNil)
			var err error
			testTask, err = task.FindOne(db.Query(task.ById(testTask.Id)))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskDispatched)
			So(testTask.HostId, ShouldEqual, "testHost")
			So(testTask.DistroId, ShouldEqual, "distroId")
			So(testTask.AgentVersion, ShouldEqual, "testAgentVersion")
		})
	})
}

func TestGetStepback(t *testing.T) {
	Convey("When the project has a stepback policy set to true", t, func() {
		require.NoError(t, db.ClearCollections(ProjectRefCollection, ParserProjectCollection, task.Collection, build.Collection, VersionCollection))

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
		pp := &ParserProject{}
		err := util.UnmarshalYAMLWithFallback([]byte(config), &pp)
		assert.NoError(t, err)
		pp.Id = "version_id"
		assert.NoError(t, pp.Insert())

		ver := &Version{
			Id:         "version_id",
			Identifier: "p1",
		}
		So(ver.Insert(), ShouldBeNil)
		projRef := &ProjectRef{
			Id: "p1",
		}
		So(projRef.Insert(), ShouldBeNil)
		Convey("if the project ref overrides the settings", func() {
			testTask := &task.Task{Id: "t1", DisplayName: "nil", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			projRef.StepbackDisabled = utility.TruePtr()
			So(projRef.Upsert(), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeFalse)
			})
		})
		Convey("if the task does not override the setting", func() {
			testTask := &task.Task{Id: "t1", DisplayName: "nil", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with true", func() {
			testTask := &task.Task{Id: "t2", DisplayName: "true", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with false", func() {
			testTask := &task.Task{Id: "t3", DisplayName: "false", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeFalse)
			})
		})

		Convey("if the buildvariant does not override the setting", func() {
			testTask := &task.Task{Id: "t4", DisplayName: "bvnil", BuildVariant: "sbnil", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with true", func() {
			testTask := &task.Task{Id: "t5", DisplayName: "bvtrue", BuildVariant: "sbtrue", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with false", func() {
			testTask := &task.Task{Id: "t6", DisplayName: "bvfalse", BuildVariant: "sbfalse", Project: projRef.Id, Version: ver.Id}
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection))
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
	systemFailTask := &task.Task{
		Id:        "systemFail",
		Activated: true,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
		Details:   apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem},
		Version:   b.Version,
	}
	successfulTask := &task.Task{
		Id:        "taskThatSucceeded",
		Activated: true,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskSucceeded,
		Version:   b.Version,
	}
	inLargerRangeTask := &task.Task{
		Id:        "taskOutsideOfSmallTimeRange",
		Activated: true,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 11, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
		Details:   apimodels.TaskEndDetail{Type: "test"},
		Version:   b.Version,
	}
	setupFailTask := &task.Task{
		Id:        "setupFailed",
		Activated: true,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
		Details:   apimodels.TaskEndDetail{Type: "setup"},
		Version:   b.Version,
	}
	ranInRangeTask := &task.Task{
		Id:         "ranInRange",
		Activated:  true,
		BuildId:    b.Id,
		Execution:  1,
		Project:    "sample",
		StartTime:  time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		FinishTime: time.Date(2017, time.June, 12, 12, 30, 0, 0, time.Local),
		Status:     evergreen.TaskFailed,
		Details:    apimodels.TaskEndDetail{Type: "test"},
		Version:    b.Version,
	}
	startedOutOfRangeTask := &task.Task{
		Id:         "startedOutOfRange",
		Activated:  true,
		BuildId:    b.Id,
		Execution:  1,
		Project:    "sample",
		StartTime:  time.Date(2017, time.June, 11, 10, 0, 0, 0, time.Local),
		FinishTime: time.Date(2017, time.June, 12, 12, 30, 0, 0, time.Local),
		Status:     evergreen.TaskFailed,
		Details:    apimodels.TaskEndDetail{Type: "test"},
		Version:    b.Version,
	}
	assert.NoError(b.Insert())
	assert.NoError(v.Insert())
	assert.NoError(systemFailTask.Insert())
	assert.NoError(successfulTask.Insert())
	assert.NoError(inLargerRangeTask.Insert())
	assert.NoError(setupFailTask.Insert())
	assert.NoError(ranInRangeTask.Insert())
	assert.NoError(startedOutOfRangeTask.Insert())

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

	results, err := RestartFailedTasks(ctx, opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Equal(3, len(results.ItemsRestarted))
	restarted := []string{inLargerRangeTask.Id, ranInRangeTask.Id, startedOutOfRangeTask.Id}
	assert.EqualValues(restarted, results.ItemsRestarted)

	opts.IncludeTestFailed = true
	opts.IncludeSysFailed = true
	results, err = RestartFailedTasks(ctx, opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Equal(4, len(results.ItemsRestarted))
	restarted = []string{systemFailTask.Id, inLargerRangeTask.Id, ranInRangeTask.Id, startedOutOfRangeTask.Id}
	assert.EqualValues(restarted, results.ItemsRestarted)

	opts.IncludeTestFailed = false
	opts.IncludeSysFailed = false
	opts.IncludeSetupFailed = true
	results, err = RestartFailedTasks(ctx, opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Equal(1, len(results.ItemsRestarted))
	assert.Equal("setupFailed", results.ItemsRestarted[0])

	// Test restarting all tasks but with a smaller time range
	opts.StartTime = time.Date(2017, time.June, 12, 11, 0, 0, 0, time.Local)
	opts.DryRun = false
	opts.IncludeTestFailed = false
	opts.IncludeSysFailed = false
	opts.IncludeSetupFailed = false
	results, err = RestartFailedTasks(ctx, opts)
	assert.NoError(err)
	assert.Equal(0, len(results.ItemsErrored))
	assert.Equal(4, len(results.ItemsRestarted))
	restarted = []string{systemFailTask.Id, setupFailTask.Id, ranInRangeTask.Id, startedOutOfRangeTask.Id}
	assert.EqualValues(restarted, results.ItemsRestarted)
	dbTask, err := task.FindOne(db.Query(task.ById(systemFailTask.Id)))
	assert.NoError(err)
	assert.Equal(dbTask.Status, evergreen.TaskUndispatched)
	assert.True(dbTask.Execution > 1)
	dbTask, err = task.FindOne(db.Query(task.ById(successfulTask.Id)))
	assert.NoError(err)
	assert.Equal(dbTask.Status, evergreen.TaskSucceeded)
	assert.Equal(1, dbTask.Execution)
	dbTask, err = task.FindOne(db.Query(task.ById(inLargerRangeTask.Id)))
	assert.NoError(err)
	assert.Equal(dbTask.Status, evergreen.TaskFailed)
	assert.Equal(1, dbTask.Execution)
	dbTask, err = task.FindOne(db.Query(task.ById(setupFailTask.Id)))
	assert.NoError(err)
	assert.Equal(dbTask.Status, evergreen.TaskUndispatched)
	assert.Equal(2, dbTask.Execution)
}

func TestFailedTaskRestartWithDisplayTasksAndTaskGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection))
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
	results, err := RestartFailedTasks(ctx, opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Equal(2, len(results.ItemsRestarted)) // not all are included in items restarted
	// but all tasks are restarted
	dbTask, err := task.FindOne(db.Query(task.ById(testTask1.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(db.Query(task.ById(testTask2.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(db.Query(task.ById(testTask3.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(db.Query(task.ById(testTask4.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(db.Query(task.ById(testTask5.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
}

func TestStepback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection))

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
	assert.NoError(doStepback(ctx, t3))
	dbTask, err := task.FindOne(db.Query(task.ById(t2.Id)))
	assert.NoError(err)
	assert.True(dbTask.Activated)

	// test stepping back a display task
	assert.NoError(doStepback(ctx, dt3))
	dbTask, err = task.FindOne(db.Query(task.ById(dt2.Id)))
	assert.NoError(err)
	assert.True(dbTask.Activated)
	dbTask, err = task.FindOne(db.Query(task.ById(dt2.Id)))
	assert.NoError(err)
	assert.True(dbTask.Activated)
}

func TestStepbackWithGenerators(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection))
	v1 := &Version{
		Id: "v1",
	}
	v2 := &Version{
		Id: "v2",
	}
	b1 := &build.Build{
		Id:        "build1",
		Status:    evergreen.BuildStarted,
		Version:   "v1",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	t1Success := &task.Task{
		Id:                  "t1_success",
		DistroId:            "test",
		DisplayName:         "task",
		Activated:           true,
		BuildId:             b1.Id,
		BuildVariant:        "bv1_name",
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             "v1",
	}
	t2Success := &task.Task{
		Id:                  "t2_success",
		DistroId:            "test",
		DisplayName:         "other_task",
		Activated:           true,
		BuildId:             b1.Id,
		BuildVariant:        "bv1_name",
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             "v1",
	}
	depTask := &task.Task{
		Id:                  "my_dep",
		DistroId:            "test",
		DisplayName:         "task",
		Activated:           false,
		BuildId:             b1.Id,
		BuildVariant:        "bv1_name",
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskUndispatched,
		RevisionOrderNumber: 2,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             "v2",
	}

	genPrevious := &task.Task{
		Id:                  "previous_gen",
		DistroId:            "test",
		DisplayName:         "other_task_gen",
		Activated:           false,
		BuildId:             b1.Id,
		BuildVariant:        "bv1_name",
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskUndispatched,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             "v2",
		GenerateTask:        true,
		DependsOn: []task.Dependency{
			{
				TaskId: "my_dep",
				Status: evergreen.TaskSucceeded,
			},
		},
	}
	t1 := &task.Task{
		Id:                  "t1",
		DistroId:            "test",
		DisplayName:         "task",
		Activated:           false,
		BuildId:             b1.Id,
		BuildVariant:        "bv1_name",
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		GeneratedBy:         "not-important",
		Status:              evergreen.TaskUndispatched,
		RevisionOrderNumber: 2,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             "v2",
	}

	taskToStepback := &task.Task{
		Id:                  "t3",
		DistroId:            "test",
		DisplayName:         "task",
		Activated:           true,
		BuildId:             b1.Id,
		BuildVariant:        "bv1_name",
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		GeneratedBy:         "not-important",
		Status:              evergreen.TaskFailed,
		RevisionOrderNumber: 3,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             "v3",
	}
	genToStepback := &task.Task{
		Id:                  "other_task_gen1",
		DistroId:            "test",
		DisplayName:         "other_task_gen",
		Activated:           true,
		BuildId:             b1.Id,
		BuildVariant:        "bv1_name",
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 2,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             "v2",
		GenerateTask:        true,
	}
	taskToStepback2 := &task.Task{
		Id:                  "t4",
		DistroId:            "test",
		DisplayName:         "other_task",
		Activated:           true,
		BuildId:             b1.Id,
		BuildVariant:        "bv1_name",
		Execution:           1,
		Project:             "sample",
		StartTime:           time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		GeneratedBy:         "other_task_gen1",
		Status:              evergreen.TaskFailed,
		RevisionOrderNumber: 3,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             "v3",
	}
	assert.NoError(t, b1.Insert())
	assert.NoError(t, t1.Insert())
	assert.NoError(t, genToStepback.Insert())
	assert.NoError(t, taskToStepback.Insert())
	assert.NoError(t, taskToStepback2.Insert())
	assert.NoError(t, t1Success.Insert())
	assert.NoError(t, t2Success.Insert())
	assert.NoError(t, depTask.Insert())
	assert.NoError(t, genPrevious.Insert())
	assert.NoError(t, v1.Insert())
	assert.NoError(t, v2.Insert())

	// test stepping back where an existing generated task needs to be activated
	assert.NoError(t, doStepback(ctx, taskToStepback))
	dbTask, err := task.FindOne(db.Query(task.ById(t1.Id)))
	assert.NoError(t, err)
	assert.True(t, dbTask.Activated)

	// test stepping back where the generator needs to be activated
	assert.NoError(t, doStepback(ctx, taskToStepback2))
	dbTask, err = task.FindOne(db.Query(task.ById(genPrevious.Id)))
	assert.NoError(t, err)
	assert.True(t, dbTask.Activated)
	assert.Equal(t, dbTask.GeneratedTasksToActivate[taskToStepback2.BuildVariant], []string{taskToStepback2.DisplayName})
	// verify dependency is activated as well
	dbTask, err = task.FindOne(db.Query(task.ById(depTask.Id)))
	assert.NoError(t, err)
	assert.True(t, dbTask.Activated)
}

func TestMarkEndRequiresAllTasksToFinishToUpdateBuildStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(task.Collection, build.Collection, host.Collection, VersionCollection, ParserProjectCollection, event.EventCollection, ProjectRefCollection))

	projRef := &ProjectRef{
		Id: "sample",
	}
	require.NoError(projRef.Insert())
	v := &Version{
		Id:         "sample_version",
		Identifier: "sample",
		Requester:  evergreen.RepotrackerVersionRequester,
		Status:     evergreen.VersionStarted,
	}
	require.NoError(v.Insert())

	pp := ParserProject{
		Id:         "sample_version",
		Identifier: utility.ToStringPtr("sample"),
	}
	require.NoError(pp.Insert())
	taskHost := host.Host{
		Id:          "myHost",
		RunningTask: "testone",
	}
	assert.NoError(taskHost.Insert(ctx))
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
		HostId:      taskHost.Id,
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
		HostId:      taskHost.Id,
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
		HostId:      taskHost.Id,
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
		HostId:      taskHost.Id,
	}
	assert.True(exeTask1.IsPartOfDisplay())
	assert.NoError(exeTask1.Insert())

	b := &build.Build{
		Id:        buildID,
		Status:    evergreen.BuildStarted,
		Activated: true,
		Version:   v.Id,
	}
	require.NoError(b.Insert())
	assert.False(b.IsFinished())

	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   evergreen.CommandTypeSystem,
	}

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}

	assert.NoError(MarkEnd(ctx, settings, testTask, "", time.Now(), details, false))
	var err error
	v, err = VersionFindOneId(v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, v.Status)

	b, err = build.FindOneId(b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildStarted, b.Status)

	assert.NoError(MarkEnd(ctx, settings, anotherTask, "", time.Now(), details, false))
	v, err = VersionFindOneId(v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, v.Status)

	b, err = build.FindOneId(b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildStarted, b.Status)

	assert.NoError(MarkEnd(ctx, settings, exeTask0, "", time.Now(), details, false))
	v, err = VersionFindOneId(v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, v.Status)

	b, err = build.FindOneId(b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildStarted, b.Status)

	exeTask1.DisplayTask = nil
	assert.NoError(err)
	assert.NoError(MarkEnd(ctx, settings, exeTask1, "", time.Now(), details, false))
	v, err = VersionFindOneId(v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionFailed, v.Status)

	b, err = build.FindOneId(b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildFailed, b.Status)

	e, err := event.FindUnprocessedEvents(-1)
	assert.NoError(err)
	assert.Len(e, 7)
}

func TestMarkEndRequiresAllTasksToFinishToUpdateBuildStatusWithCompileTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(task.Collection, build.Collection, host.Collection, VersionCollection, event.EventCollection))
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
		HostId:      "myHost",
	}
	assert.NoError(testTask.Insert())
	taskHost := host.Host{
		Id:          "myHost",
		RunningTask: testTask.Id,
	}
	assert.NoError(taskHost.Insert(ctx))
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
	}
	require.NoError(b.Insert())

	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   "test",
	}

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}

	assert.NoError(MarkEnd(ctx, settings, &testTask, "", time.Now(), details, false))
	var err error
	v, err = VersionFindOneId(v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionFailed, v.Status)

	b, err = build.FindOneId(b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildFailed, b.Status)

	e, err := event.FindUnprocessedEvents(-1)
	assert.NoError(err)
	assert.Len(e, 4)
}

func TestMarkEndWithBlockedDependenciesTriggersNotifications(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(task.Collection, build.Collection, host.Collection, VersionCollection, event.EventCollection))

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
		HostId:      "myHost",
	}
	assert.NoError(testTask.Insert())
	taskHost := host.Host{
		Id:          "myHost",
		RunningTask: testTask.Id,
	}
	assert.NoError(taskHost.Insert(ctx))
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
	}
	require.NoError(b.Insert())

	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   "test",
	}
	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	assert.NoError(MarkEnd(ctx, settings, &testTask, "", time.Now(), details, false))

	var err error
	v, err = VersionFindOneId(v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionFailed, v.Status)

	b, err = build.FindOneId(b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildFailed, b.Status)

	e, err := event.FindUnprocessedEvents(-1)
	assert.NoError(err)
	assert.Len(e, 4)
}

func TestClearAndResetStrandedHostTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection, VersionCollection))
	assert := assert.New(t)

	tasks := []task.Task{
		{
			Id:            "t",
			Status:        evergreen.TaskStarted,
			Activated:     true,
			ActivatedTime: time.Now(),
			BuildId:       "b",
			Version:       "version",
			HostId:        "h1",
		},
		{
			Id:            "t2",
			Status:        evergreen.TaskSucceeded,
			Activated:     true,
			ActivatedTime: time.Now(),
			BuildId:       "b2",
			Version:       "version",
		},
		{
			Id:            "unschedulableTask",
			Status:        evergreen.TaskStarted,
			Activated:     true,
			ActivatedTime: time.Now().Add(-task.UnschedulableThreshold - time.Minute),
			BuildId:       "b2",
			Version:       "version2",
			HostId:        "h1",
			Requester:     evergreen.PatchVersionRequester,
		},
		{
			Id:            "dependencyTask",
			Status:        evergreen.TaskUndispatched,
			Activated:     true,
			ActivatedTime: time.Now(),
			BuildId:       "b2",
			Version:       "version2",
			DependsOn: []task.Dependency{
				{
					TaskId: "unschedulableTask",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
		{
			Id:             "displayTask",
			DisplayName:    "displayTask",
			BuildId:        "b2",
			Version:        "version2",
			Project:        "project",
			Activated:      false,
			DisplayOnly:    true,
			ExecutionTasks: []string{"unschedulableTask"},
			Status:         evergreen.TaskStarted,
			DispatchTime:   time.Now(),
		},
		{
			Id:            "t3",
			Status:        evergreen.TaskStarted,
			Activated:     true,
			ActivatedTime: time.Now(),
			BuildId:       "b3",
			Version:       "version3",
			Execution:     evergreen.MaxTaskExecution,
		},
	}
	for _, tsk := range tasks {
		require.NoError(t, tsk.Insert())
	}

	h := &host.Host{
		Id:          "h1",
		RunningTask: "t",
	}
	assert.NoError(h.Insert(ctx))

	b := build.Build{
		Id:      "b",
		Version: "version",
	}
	assert.NoError(b.Insert())
	v := Version{
		Id: b.Version,
	}
	assert.NoError(v.Insert())

	b2 := build.Build{
		Id:      "b2",
		Version: "version2",
	}
	assert.NoError(b2.Insert())
	v2 := Version{
		Id: b2.Version,
	}
	assert.NoError(v2.Insert())

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	assert.NoError(ClearAndResetStrandedHostTask(ctx, settings, h))

	runningTask, err := task.FindOne(db.Query(task.ById("t")))
	require.NoError(t, err)
	assert.Equal(evergreen.TaskUndispatched, runningTask.Status)

	foundBuild, err := build.FindOneId("b")
	require.NoError(t, err)
	assert.Equal(evergreen.BuildCreated, foundBuild.Status)

	foundVersion, err := VersionFindOneId(b.Version)
	require.NoError(t, err)
	assert.Equal(evergreen.VersionCreated, foundVersion.Status)

	h.RunningTask = "unschedulableTask"
	assert.NoError(ClearAndResetStrandedHostTask(ctx, settings, h))

	unschedulableTask, err := task.FindOne(db.Query(task.ById("unschedulableTask")))
	require.NoError(t, err)
	assert.Equal(evergreen.TaskFailed, unschedulableTask.Status)

	dependencyTask, err := task.FindOne(db.Query(task.ById("dependencyTask")))
	require.NoError(t, err)
	assert.True(dependencyTask.DependsOn[0].Unattainable)
	assert.True(dependencyTask.DependsOn[0].Finished)

	dt, err := task.FindOne(db.Query(task.ById("displayTask")))
	require.NoError(t, err)
	assert.Equal(dt.Status, evergreen.TaskFailed)
	assert.Equal(dt.Details, task.GetSystemFailureDetails(evergreen.TaskDescriptionStranded))

	foundBuild, err = build.FindOneId("b2")
	require.NoError(t, err)
	assert.Equal(evergreen.BuildFailed, foundBuild.Status)

	foundVersion, err = VersionFindOneId(b2.Version)
	require.NoError(t, err)
	assert.Equal(evergreen.VersionFailed, foundVersion.Status)

	h.RunningTask = "t2"
	assert.NoError(resetTask(ctx, "t2", ""))
	assert.NoError(ClearAndResetStrandedHostTask(ctx, settings, h))
	foundTask, err := task.FindOne(db.Query(task.ById("t2")))
	require.NoError(t, err)
	// The task should not have been reset twice.
	assert.Equal(foundTask.Execution, 1)
}

func TestClearAndResetStaleStrandedHostTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection))
	assert := assert.New(t)

	runningTask := &task.Task{
		Id:            "t",
		Status:        evergreen.TaskStarted,
		Activated:     true,
		ActivatedTime: utility.ZeroTime,
		BuildId:       "b",
		Version:       "version",
		HostId:        "h1",
	}
	assert.NoError(runningTask.Insert())

	h := &host.Host{
		Id:          "h1",
		RunningTask: "t",
	}
	assert.NoError(h.Insert(ctx))

	b := build.Build{Id: "b"}
	assert.NoError(b.Insert())
	v := Version{
		Id: b.Version,
	}
	assert.NoError(v.Insert())

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	assert.NoError(ClearAndResetStrandedHostTask(ctx, settings, h))
	runningTask, err := task.FindOne(db.Query(task.ById("t")))
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, runningTask.Status)
	assert.Equal("system", runningTask.Details.Type)
}

func TestClearAndResetStrandedHostTaskFailedOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection, VersionCollection))

	dispTask := &task.Task{
		Id:             "dt",
		Status:         evergreen.TaskStarted,
		Version:        "version",
		Activated:      true,
		ActivatedTime:  time.Now(),
		BuildId:        "b",
		ExecutionTasks: []string{"et1", "et2"},
		DisplayOnly:    true,
	}

	execTask1 := &task.Task{
		Id:            "et1",
		Status:        evergreen.TaskStarted,
		Version:       "version",
		Activated:     true,
		ActivatedTime: time.Now(),
		BuildId:       "b",
		HostId:        "h1",
	}

	execTask2 := &task.Task{
		Id:            "et2",
		Status:        evergreen.TaskSucceeded,
		Version:       "version",
		Activated:     true,
		ActivatedTime: time.Now(),
		BuildId:       "b",
	}
	assert.NoError(t, dispTask.Insert())
	assert.NoError(t, execTask1.Insert())
	assert.NoError(t, execTask2.Insert())

	h := &host.Host{
		Id:          "h1",
		RunningTask: "et1",
	}
	assert.NoError(t, h.Insert(ctx))

	b := build.Build{
		Id:      "b",
		Version: "version",
	}
	assert.NoError(t, b.Insert())
	v := Version{
		Id: "version",
	}
	assert.NoError(t, v.Insert())
	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	assert.NoError(t, ClearAndResetStrandedHostTask(ctx, settings, h))
	restartedDisplayTask, err := task.FindOne(db.Query(task.ById("dt")))
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskUndispatched, restartedDisplayTask.Status)
	assert.Equal(t, 1, restartedDisplayTask.Execution)
	restartedExecutionTask, err := task.FindOne(db.Query(task.ById("et1")))
	assert.NoError(t, err)
	assert.Equal(t, 1, restartedExecutionTask.Execution)
	assert.Equal(t, 1, restartedExecutionTask.LatestParentExecution)
	assert.Equal(t, evergreen.TaskUndispatched, restartedExecutionTask.Status)
	nonRestartedExecutionTask, err := task.FindOne(db.Query(task.ById("et2")))
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskSucceeded, nonRestartedExecutionTask.Status)
	assert.Equal(t, 0, nonRestartedExecutionTask.Execution)
	assert.Equal(t, 1, restartedExecutionTask.LatestParentExecution)

	oldRestartedExecutionTask, err := task.FindOneOld(task.ById(fmt.Sprintf("%v_%v", execTask1.Id, 0)))
	assert.NoError(t, err)
	assert.NotNil(t, oldRestartedExecutionTask)
	assert.Equal(t, evergreen.TaskFailed, oldRestartedExecutionTask.Status)
	assert.Equal(t, 0, oldRestartedExecutionTask.Execution)
}

func TestMarkUnallocatableContainerTasksSystemFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection, event.EventCollection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, tsk task.Task, b build.Build, v Version){
		"SystemFailsTaskWithNoRemainingAllocationAttempts": func(t *testing.T, tsk task.Task, b build.Build, v Version) {
			require.NoError(t, tsk.Insert())
			require.NoError(t, MarkUnallocatableContainerTasksSystemFailed(ctx, settings, []string{tsk.Id}))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.True(t, dbTask.IsFinished(), "task that has used up its container allocation attempts should be finished")

			dbBuild, err := build.FindOneId(b.Id)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.True(t, dbBuild.IsFinished(), "build with finished task should have updated status")

			dbVersion, err := VersionFindOneId(v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.VersionFailed, dbVersion.Status, "version with finished task should have updated status")
		},
		"NoopsWithTaskThatHasRemainingAllocationAttempts": func(t *testing.T, tsk task.Task, b build.Build, v Version) {
			tsk.ContainerAllocationAttempts = 0
			require.NoError(t, tsk.Insert())
			require.NoError(t, MarkUnallocatableContainerTasksSystemFailed(ctx, settings, []string{tsk.Id}))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.IsFinished(), "task with remaining container allocation attempts should not be finished")

			dbBuild, err := build.FindOneId(b.Id)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.Equal(t, b.Status, dbBuild.Status, "build status should not be changed because task should not be finished")

			dbVersion, err := VersionFindOneId(v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, v.Status, dbVersion.Status, "version status should not be changed because task should not be finished")
		},
		"SystemFailsSubsetOfTasksWithNoRemainingAllocationAttempts": func(t *testing.T, tsk0 task.Task, b build.Build, v Version) {
			require.NoError(t, tsk0.Insert())
			tsk1 := tsk0
			tsk1.Id = "other_task_id"
			tsk1.ContainerAllocationAttempts = 0
			require.NoError(t, tsk1.Insert())

			require.NoError(t, MarkUnallocatableContainerTasksSystemFailed(ctx, settings, []string{tsk0.Id, tsk1.Id}))

			dbTask0, err := task.FindOneId(tsk0.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask0)
			assert.True(t, dbTask0.IsFinished())

			dbTask1, err := task.FindOneId(tsk1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask1)
			assert.False(t, dbTask1.IsFinished())
		},
		"NoopsWithHostTask": func(t *testing.T, tsk task.Task, b build.Build, v Version) {
			tsk.ExecutionPlatform = task.ExecutionPlatformHost
			require.NoError(t, tsk.Insert())

			require.NoError(t, MarkUnallocatableContainerTasksSystemFailed(ctx, settings, []string{tsk.Id}))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.IsFinished())
		},
		"NoopsWithNonexistentTasks": func(t *testing.T, tsk task.Task, b build.Build, v Version) {
			require.NoError(t, MarkUnallocatableContainerTasksSystemFailed(ctx, settings, []string{tsk.Id}))

			dbTask, err := task.FindOneId(tsk.Id)
			assert.NoError(t, err)
			assert.Zero(t, dbTask)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, build.Collection, pod.Collection, VersionCollection, event.EventCollection))
			v := Version{
				Id:     "version_id",
				Status: evergreen.VersionStarted,
			}
			require.NoError(t, v.Insert())
			b := build.Build{
				Id:      "build_id",
				Version: v.Id,
				Status:  evergreen.BuildStarted,
			}
			require.NoError(t, b.Insert())
			taskPod := pod.Pod{
				ID: "myPod",
			}
			require.NoError(t, taskPod.Insert())
			tsk := task.Task{
				Id:                          "task_id",
				Execution:                   1,
				BuildId:                     b.Id,
				Version:                     v.Id,
				Status:                      evergreen.TaskUndispatched,
				ExecutionPlatform:           task.ExecutionPlatformContainer,
				Activated:                   true,
				ContainerAllocated:          true,
				ContainerAllocationAttempts: 100,
				PodID:                       taskPod.ID,
			}
			tCase(t, tsk, b, v)
		})
	}
}

func TestClearAndResetExecTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		HostId:        "h1",
	}
	assert.NoError(t, dispTask.Insert())
	assert.NoError(t, execTask.Insert())

	h := &host.Host{
		Id:          "h1",
		RunningTask: "et",
	}
	assert.NoError(t, h.Insert(ctx))

	b := build.Build{
		Id:      "b",
		Version: "version",
	}
	assert.NoError(t, b.Insert())
	v := Version{
		Id: "version",
	}
	assert.NoError(t, v.Insert())

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}

	assert.NoError(t, ClearAndResetStrandedHostTask(ctx, settings, h))
	restartedDisplayTask, err := task.FindOne(db.Query(task.ById("dt")))
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskUndispatched, restartedDisplayTask.Status)
	restartedExecutionTask, err := task.FindOne(db.Query(task.ById("et")))
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskUndispatched, restartedExecutionTask.Status)
}

func TestClearAndResetStrandedContainerTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	defer func() {
		assert.NoError(t, db.ClearCollections(pod.Collection, task.Collection, task.OldCollection, build.Collection, VersionCollection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, p pod.Pod, tsk task.Task){
		"SuccessfullyUpdatesPodAndRestartsTask": func(t *testing.T, p pod.Pod, tsk task.Task) {
			require.NoError(t, p.Insert())
			require.NoError(t, tsk.Insert())

			require.NoError(t, ClearAndResetStrandedContainerTask(ctx, settings, &p))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskID)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskExecution)

			dbArchivedTask, err := task.FindOneOldByIdAndExecution(tsk.Id, 1)
			require.NoError(t, err)
			require.NotZero(t, dbArchivedTask, "should have archived the old task execution")
			assert.Equal(t, evergreen.TaskFailed, dbArchivedTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbArchivedTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionStranded, dbArchivedTask.Details.Description)
			assert.False(t, utility.IsZeroTime(dbArchivedTask.FinishTime))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask, "should have created a new task execution")
			assert.Equal(t, evergreen.TaskUndispatched, dbTask.Status)
			assert.True(t, dbTask.Activated)
			assert.False(t, dbTask.ContainerAllocated)
			assert.Zero(t, dbTask.ContainerAllocatedTime)

			dbBuild, err := build.FindOneId(tsk.BuildId)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.Equal(t, evergreen.BuildCreated, dbBuild.Status, "build status should be updated for restarted task")

			dbVersion, err := VersionFindOneId(tsk.Version)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.VersionCreated, dbVersion.Status, "version status should be updated for restarted task")
		},
		"ResetsParentDisplayTaskForStrandedExecutionTask": func(t *testing.T, p pod.Pod, tsk task.Task) {
			otherExecTask := task.Task{
				Id:        "execution_task_id",
				Status:    evergreen.TaskStarted,
				Activated: true,
			}
			require.NoError(t, otherExecTask.Insert())
			dt := task.Task{
				Id:             "display_task_id",
				DisplayOnly:    true,
				ExecutionTasks: []string{tsk.Id, otherExecTask.Id},
				Status:         evergreen.TaskStarted,
				BuildId:        tsk.BuildId,
				Version:        tsk.Version,
			}
			require.NoError(t, dt.Insert())
			tsk.DisplayTaskId = utility.ToStringPtr(dt.Id)
			require.NoError(t, tsk.Insert())
			require.NoError(t, p.Insert())

			require.NoError(t, ClearAndResetStrandedContainerTask(ctx, settings, &p))

			dbDisplayTask, err := task.FindOneId(dt.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask)
			assert.True(t, dbDisplayTask.ResetFailedWhenFinished, "display task should reset failed when other exec task finishes running")

			dbArchivedTask, err := task.FindOneOldByIdAndExecution(tsk.Id, 1)
			assert.NoError(t, err)
			assert.Zero(t, dbArchivedTask, "execution task should not be archived until display task can reset")

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, 1, dbTask.Execution, "current task execution should still be the stranded one")
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionStranded, dbTask.Details.Description)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))

			dbOtherExecTask, err := task.FindOneId(otherExecTask.Id)
			require.NoError(t, err)
			require.NotZero(t, dbOtherExecTask)
			assert.Equal(t, dbOtherExecTask.Status, evergreen.TaskStarted, "other execution task should still be running")
		},
		"ClearsAlreadyFinishedTaskFromPod": func(t *testing.T, p pod.Pod, tsk task.Task) {
			const status = evergreen.TaskSucceeded
			tsk.Status = status
			require.NoError(t, tsk.Insert())
			require.NoError(t, p.Insert())

			require.NoError(t, ClearAndResetStrandedContainerTask(ctx, settings, &p))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskID)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskExecution)

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, status, dbTask.Status)
		},
		"FailsWithConflictingDBAndInMemoryRunningTasks": func(t *testing.T, p pod.Pod, tsk task.Task) {
			const runningTask = "some_other_task"
			p.TaskRuntimeInfo.RunningTaskID = runningTask
			require.NoError(t, p.Insert())
			p.TaskRuntimeInfo.RunningTaskID = tsk.Id
			require.NoError(t, tsk.Insert())

			assert.Error(t, ClearAndResetStrandedContainerTask(ctx, settings, &p))
		},
		"ClearsNonexistentTaskFromPod": func(t *testing.T, p pod.Pod, tsk task.Task) {
			p.TaskRuntimeInfo.RunningTaskID = "nonexistent_task"
			require.NoError(t, p.Insert())

			require.NoError(t, ClearAndResetStrandedContainerTask(ctx, settings, &p))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskID)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskExecution)
		},
		"NoopsForPodNotRunningAnyTask": func(t *testing.T, p pod.Pod, tsk task.Task) {
			p.TaskRuntimeInfo.RunningTaskID = ""
			p.TaskRuntimeInfo.RunningTaskExecution = 0
			require.NoError(t, p.Insert())

			require.NoError(t, ClearAndResetStrandedContainerTask(ctx, settings, &p))
			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskID)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskExecution)
		},
		"FailsTaskThatHitsUnschedulableThresholdWithoutRestartingIt": func(t *testing.T, p pod.Pod, tsk task.Task) {
			require.NoError(t, p.Insert())
			tsk.ActivatedTime = time.Now().Add(-10 * task.UnschedulableThreshold)
			require.NoError(t, tsk.Insert())

			require.NoError(t, ClearAndResetStrandedContainerTask(ctx, settings, &p))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskID)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskExecution)

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, dbTask.Execution, "current task execution should still be the stranded one")
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionStranded, dbTask.Details.Description)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))

			// TODO (EVG-17033): if a stranded task hits the unschedulable
			// threshold, it should refuse to restart the task, but the build
			// and version statuses should still be updated to reflect the
			// stranded task. The portion of the test below this checking the
			// updated build and version should pass.

			// dbBuild, err := build.FindOneId(tsk.BuildId)
			// require.NoError(t, err)
			// require.NotZero(t, dbBuild)
			// assert.Equal(t, evergreen.BuildFailed, dbBuild.Status, "build status should be updated for unrestartable stranded task")
			//
			// dbVersion, err := VersionFindOneId(tsk.Version)
			// require.NoError(t, err)
			// require.NotZero(t, dbVersion)
			// assert.Equal(t, evergreen.VersionFailed, dbVersion.Status, "version status should be updated for unrestartable stranded task")
		},
		"FailsTaskThatHitsMaxExecutionRestartsWithoutRestartingIt": func(t *testing.T, p pod.Pod, tsk task.Task) {
			const execNum = evergreen.MaxTaskExecution + 1
			tsk.Execution = execNum
			p.TaskRuntimeInfo.RunningTaskExecution = execNum
			require.NoError(t, p.Insert())
			require.NoError(t, tsk.Insert())

			require.NoError(t, ClearAndResetStrandedContainerTask(ctx, settings, &p))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskID)
			assert.Zero(t, dbPod.TaskRuntimeInfo.RunningTaskExecution)

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			assert.Equal(t, execNum, dbTask.Execution, "current task execution should still be the stranded one")
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionStranded, dbTask.Details.Description)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))

			// TODO (EVG-17033): if a stranded task hits the max execution, it
			// should refuse to restart the task, but the build and version
			// statuses should still be updated to reflect the stranded task.
			// The portion of the test below this checking the updated build and
			// version should pass.

			// dbBuild, err := build.FindOneId(tsk.BuildId)
			// require.NoError(t, err)
			// require.NotZero(t, dbBuild)
			// assert.Equal(t, evergreen.BuildFailed, dbBuild.Status, "build status should be updated for unrestartable stranded task")
			//
			// dbVersion, err := VersionFindOneId(tsk.Version)
			// require.NoError(t, err)
			// require.NotZero(t, dbVersion)
			// assert.Equal(t, evergreen.VersionFailed, dbVersion.Status, "version status should be updated for unrestartable stranded task")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(pod.Collection, task.Collection, task.OldCollection, build.Collection, VersionCollection))
			b := build.Build{
				Id:     "build_id",
				Status: evergreen.BuildStarted,
			}
			require.NoError(t, b.Insert())
			v := Version{
				Id:     "version_id",
				Status: evergreen.VersionStarted,
			}
			require.NoError(t, v.Insert())
			tsk := task.Task{
				Id:                     "task_id",
				Execution:              1,
				ExecutionPlatform:      task.ExecutionPlatformContainer,
				ContainerAllocated:     true,
				ContainerAllocatedTime: time.Now(),
				Status:                 evergreen.TaskStarted,
				Activated:              true,
				ActivatedTime:          time.Now(),
				BuildId:                b.Id,
				Version:                v.Id,
				PodID:                  "pod_id",
			}
			p := pod.Pod{
				ID: "pod_id",
				TaskRuntimeInfo: pod.TaskRuntimeInfo{
					RunningTaskID:        tsk.Id,
					RunningTaskExecution: tsk.Execution,
				},
			}
			tCase(t, p, tsk)
		})
	}
}

func TestResetStaleTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, tsk task.Task){
		"SuccessfullyRestartsStaleTask": func(t *testing.T, tsk task.Task) {
			require.NoError(t, tsk.Insert())

			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbArchivedTask, err := task.FindOneOldByIdAndExecution(tsk.Id, 1)
			require.NoError(t, err)
			require.NotZero(t, dbArchivedTask, "should have archived the old task execution")
			assert.Equal(t, evergreen.TaskFailed, dbArchivedTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbArchivedTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionHeartbeat, dbArchivedTask.Details.Description)
			assert.True(t, dbArchivedTask.Details.TimedOut)
			assert.False(t, utility.IsZeroTime(dbArchivedTask.FinishTime))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask, "should have created a new task execution")
			assert.Equal(t, evergreen.TaskUndispatched, dbTask.Status)
			assert.True(t, dbTask.Activated)
			assert.False(t, dbTask.ContainerAllocated)
			assert.Zero(t, dbTask.ContainerAllocatedTime)

			dbBuild, err := build.FindOneId(tsk.BuildId)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.Equal(t, evergreen.BuildCreated, dbBuild.Status, "build status should be updated for restarted task")

			dbVersion, err := VersionFindOneId(tsk.Version)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.VersionCreated, dbVersion.Status, "version status should be updated for restarted task")
		},
		"SuccessfullySystemFailsAbortedTask": func(t *testing.T, tsk task.Task) {
			tsk.Aborted = true
			require.NoError(t, tsk.Insert())
			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbArchivedTask, err := task.FindOneOldByIdAndExecution(tsk.Id, 1)
			require.NoError(t, err)
			require.Zero(t, dbArchivedTask, "should not have archived the aborted task")

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionAborted, dbTask.Details.Description)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))
			assert.False(t, dbTask.ContainerAllocated)
			assert.Zero(t, dbTask.ContainerAllocatedTime)
		},
		"ResetsParentDisplayTaskForStaleExecutionTask": func(t *testing.T, tsk task.Task) {
			otherExecTask := task.Task{
				Id:        "execution_task_id",
				Status:    evergreen.TaskStarted,
				Activated: true,
			}
			require.NoError(t, otherExecTask.Insert())
			dt := task.Task{
				Id:             "display_task_id",
				DisplayOnly:    true,
				ExecutionTasks: []string{tsk.Id, otherExecTask.Id},
				Status:         evergreen.TaskStarted,
				BuildId:        tsk.BuildId,
				Version:        tsk.Version,
			}
			require.NoError(t, dt.Insert())
			tsk.DisplayTaskId = utility.ToStringPtr(dt.Id)
			require.NoError(t, tsk.Insert())

			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbDisplayTask, err := task.FindOneId(dt.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask)
			assert.True(t, dbDisplayTask.ResetFailedWhenFinished, "display task should reset failed when other exec task finishes running")

			dbArchivedTask, err := task.FindOneOldByIdAndExecution(tsk.Id, 1)
			assert.NoError(t, err)
			assert.Zero(t, dbArchivedTask, "execution task should not be archived until display task can reset")

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, 1, dbTask.Execution, "current task execution should still be the stranded one")
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionHeartbeat, dbTask.Details.Description)
			assert.True(t, dbTask.Details.TimedOut)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))

			dbOtherExecTask, err := task.FindOneId(otherExecTask.Id)
			require.NoError(t, err)
			require.NotZero(t, dbOtherExecTask)
			assert.Equal(t, dbOtherExecTask.Status, evergreen.TaskStarted, "other execution task should still be running")
		},
		"FailsStaleTaskThatHitsUnschedulableThresholdWithoutRestartingIt": func(t *testing.T, tsk task.Task) {
			tsk.ActivatedTime = time.Now().Add(-10 * task.UnschedulableThreshold)
			require.NoError(t, tsk.Insert())

			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, dbTask.Execution, "current task execution should still be the stranded one")
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionHeartbeat, dbTask.Details.Description)
			assert.True(t, dbTask.Details.TimedOut)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))

			// TODO (EVG-17033): if a stale task hits the unschedulable
			// threshold, it should refuse to restart the task, but the build
			// and version statuses should still be updated to reflect the
			// stranded task. The portion of the test below this checking the
			// updated build and version should pass.

			// dbBuild, err := build.FindOneId(tsk.BuildId)
			// require.NoError(t, err)
			// require.NotZero(t, dbBuild)
			// assert.Equal(t, evergreen.BuildFailed, dbBuild.Status, "build status should be updated for unrestartable stale task")
			//
			// dbVersion, err := VersionFindOneId(tsk.Version)
			// require.NoError(t, err)
			// require.NotZero(t, dbVersion)
			// assert.Equal(t, evergreen.VersionFailed, dbVersion.Status, "version status should be updated for unrestartable stale task")
		},
		"FailsStaleTaskThatHitsMaxExecutionRestartsWithoutRestartingIt": func(t *testing.T, tsk task.Task) {
			const execNum = evergreen.MaxTaskExecution + 1
			tsk.Execution = execNum
			require.NoError(t, tsk.Insert())

			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			assert.Equal(t, execNum, dbTask.Execution, "current task execution should still be the stranded one")
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionHeartbeat, dbTask.Details.Description)
			assert.True(t, dbTask.Details.TimedOut)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))

			// TODO (EVG-17033): if a stale task hits the max execution, it
			// should refuse to restart the task, but the build and version
			// statuses should still be updated to reflect the stranded task.
			// The portion of the test below this checking the updated build and
			// version should pass.

			// dbBuild, err := build.FindOneId(tsk.BuildId)
			// require.NoError(t, err)
			// require.NotZero(t, dbBuild)
			// assert.Equal(t, evergreen.BuildFailed, dbBuild.Status, "build status should be updated for unrestartable stale task")
			//
			// dbVersion, err := VersionFindOneId(tsk.Version)
			// require.NoError(t, err)
			// require.NotZero(t, dbVersion)
			// assert.Equal(t, evergreen.VersionFailed, dbVersion.Status, "version status should be updated for unrestartable stale task")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(pod.Collection, task.Collection, task.OldCollection, build.Collection, VersionCollection))
			b := build.Build{
				Id:     "build_id",
				Status: evergreen.BuildStarted,
			}
			require.NoError(t, b.Insert())
			v := Version{
				Id:     "version_id",
				Status: evergreen.VersionStarted,
			}
			require.NoError(t, v.Insert())
			taskPod := pod.Pod{
				ID: "pod_id",
			}
			require.NoError(t, taskPod.Insert())
			tsk := task.Task{
				Id:                     "task_id",
				Execution:              1,
				ExecutionPlatform:      task.ExecutionPlatformContainer,
				ContainerAllocated:     true,
				ContainerAllocatedTime: time.Now(),
				Status:                 evergreen.TaskStarted,
				Activated:              true,
				ActivatedTime:          time.Now(),
				LastHeartbeat:          time.Now().Add(-30 * time.Hour),
				BuildId:                b.Id,
				Version:                v.Id,
				PodID:                  taskPod.ID,
			}
			tCase(t, tsk)
		})
	}
}

func TestMarkEndWithNoResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, build.Collection, host.Collection, VersionCollection, event.EventCollection))

	testTask1 := task.Task{
		Id:              "t1",
		Status:          evergreen.TaskStarted,
		Activated:       true,
		ActivatedTime:   time.Now(),
		BuildId:         "b",
		Version:         "v",
		MustHaveResults: true,
		HostId:          "hostId",
	}
	assert.NoError(t, testTask1.Insert())
	taskHost := host.Host{
		Id:          "hostId",
		RunningTask: testTask1.Id,
	}
	assert.NoError(t, taskHost.Insert(ctx))
	testTask2 := task.Task{
		Id:              "t2",
		Status:          evergreen.TaskStarted,
		Activated:       true,
		ActivatedTime:   time.Now(),
		BuildId:         "b",
		Version:         "v",
		MustHaveResults: true,
		ResultsService:  testresult.TestResultsServiceLocal,
		HostId:          "hostId",
	}
	assert.NoError(t, testTask2.Insert())
	b := build.Build{
		Id:      "b",
		Version: "v",
	}
	assert.NoError(t, b.Insert())
	v := &Version{
		Id:        "v",
		Requester: evergreen.RepotrackerVersionRequester,
		Status:    evergreen.VersionStarted,
	}
	assert.NoError(t, v.Insert())
	pp := ParserProject{
		Id:         v.Id,
		Identifier: utility.ToStringPtr("sample"),
	}
	assert.NoError(t, pp.Insert())
	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskSucceeded,
		Type:   "test",
	}

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	err := MarkEnd(ctx, settings, &testTask1, "", time.Now(), details, false)
	assert.NoError(t, err)
	dbTask, err := task.FindOneId(testTask1.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
	assert.Equal(t, evergreen.TaskDescriptionNoResults, dbTask.Details.Description)

	err = MarkEnd(ctx, settings, &testTask2, "", time.Now(), details, false)
	assert.NoError(t, err)
	dbTask, err = task.FindOneId(testTask2.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskSucceeded, dbTask.Status)
}

func TestDisplayTaskUpdates(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, event.EventCollection))
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
	dt3 := task.Task{
		Id:          "dt3",
		DisplayOnly: true,
		Status:      evergreen.TaskUndispatched,
		Activated:   false,
		ExecutionTasks: []string{
			"task11",
			"task12",
		},
	}
	assert.NoError(dt3.Insert())
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
		Status:    evergreen.TaskDispatched,
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
	task11 := task.Task{
		Id:        "task11",
		Activated: true,
		StartTime: time.Time{},
	}
	task12 := task.Task{
		Id:         "task12",
		Activated:  true,
		StartTime:  time.Date(2000, 0, 0, 0, 44, 0, 0, time.Local),
		FinishTime: time.Date(2000, 0, 0, 1, 0, 1, 0, time.Local),
	}
	assert.NoError(task11.Insert())
	assert.NoError(task12.Insert())

	// test that updating the status + activated from execution tasks works
	assert.NoError(UpdateDisplayTaskForTask(&task1))
	dbTask, err := task.FindOne(db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.True(dbTask.Details.TimedOut)
	assert.True(dbTask.Activated)
	assert.Equal(11*time.Minute, dbTask.TimeTaken)
	assert.Equal(task2.StartTime, dbTask.StartTime)
	assert.Equal(task4.FinishTime, dbTask.FinishTime)

	// test that you can't update a display task
	assert.Error(UpdateDisplayTaskForTask(&dt))

	// test that a display task with a finished + unfinished task is "started"
	assert.NoError(UpdateDisplayTaskForTask(&task5))
	dbTask, err = task.FindOne(db.Query(task.ById(dt2.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)
	assert.Zero(dbTask.FinishTime)

	// check that the updates above logged an event for the first one
	events, err := event.Find(event.TaskEventsForId(dt.Id))
	assert.NoError(err)
	assert.Len(events, 1)
	events, err = event.Find(event.TaskEventsForId(dt2.Id))
	assert.NoError(err)
	assert.Len(events, 0)

	// a blocked execution task + unblocked unfinshed tasks should still be "started"
	assert.NoError(UpdateDisplayTaskForTask(&task7))
	dbTask, err = task.FindOne(db.Query(task.ById(blockedDt.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	// a blocked execution task should not contribute to the status
	assert.NoError(task10.MarkFailed())
	assert.NoError(UpdateDisplayTaskForTask(&task8))
	dbTask, err = task.FindOne(db.Query(task.ById(blockedDt.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)

	// a display task should not set its start time to any exec tasks that have zero start time
	assert.NoError(UpdateDisplayTaskForTask(&task11))
	dbTask, err = task.FindOne(db.Query(task.ById(dt3.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(task12.StartTime, dbTask.StartTime)
}

func TestDisplayTaskUpdateNoUndispatched(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, event.EventCollection))
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
	assert.NoError(UpdateDisplayTaskForTask(&task1))
	dbTask, err := task.FindOne(db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	events, err := event.Find(event.TaskEventsForId(dt.Id))
	assert.NoError(err)
	assert.Len(events, 0)
}

func TestDisplayTaskDelayedRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection))
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
		Id:      "b",
		Version: "version",
	}
	assert.NoError(b.Insert())
	v := Version{
		Id: "version",
	}
	assert.NoError(v.Insert())

	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}

	// request that the task restarts when it's done
	assert.NoError(dt.SetResetWhenFinished())
	dbTask, err := task.FindOne(db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.True(dbTask.ResetWhenFinished)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	// end the final task so that it restarts
	assert.NoError(checkResetDisplayTask(ctx, settings, &dt))
	dbTask, err = task.FindOne(db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask2, err := task.FindOne(db.Query(task.ById(task2.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask2.Status)

	oldTask, err := task.FindOneOld(task.ById("dt_0"))
	assert.NoError(err)
	assert.NotNil(oldTask)
}

func TestAbortedTaskDelayedRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, host.Collection, build.Collection, VersionCollection))
	task1 := task.Task{
		Id:                "task1",
		BuildId:           "b",
		Version:           "version",
		Status:            evergreen.TaskStarted,
		Aborted:           true,
		ResetWhenFinished: true,
		Activated:         true,
		HostId:            "hostId",
	}
	assert.NoError(t, task1.Insert())
	taskHost := host.Host{
		Id: "hostId",
	}
	assert.NoError(t, taskHost.Insert(ctx))
	b := build.Build{
		Id:      "b",
		Version: "version",
	}
	assert.NoError(t, b.Insert())
	v := Version{
		Id: "version",
	}
	assert.NoError(t, v.Insert())

	detail := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
	}
	settings := &evergreen.Settings{
		CommitQueue: evergreen.CommitQueueConfig{
			MaxSystemFailedTaskRetries: 2,
		},
	}
	assert.NoError(t, MarkEnd(ctx, settings, &task1, "test", time.Now(), detail, false))
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

	assert.NoError(UpdateDisplayTaskForTask(&execTask0))
	dbTask, err := task.FindOne(db.Query(task.ById(dt.Id)))
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

	assert.NoError(UpdateDisplayTaskForTask(&execTask0))
	dbTask, err := task.FindOne(db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.Equal(evergreen.CommandTypeSetup, dbTask.Details.Type)
	assert.True(dbTask.Activated)
}

func TestEvalStepbackDeactivatePrevious(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, ProjectRefCollection, distro.Collection, build.Collection, VersionCollection))

	proj := ProjectRef{
		Id: "proj",
	}
	require.NoError(t, proj.Insert())
	d := distro.Distro{
		Id: "distro",
	}
	require.NoError(t, d.Insert(ctx))
	v := Version{
		Id:        "sample_version",
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
		Activated:           true,
		RevisionOrderNumber: 2,
		DispatchTime:        utility.ZeroTime,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             v.Id,
	}
	assert.NoError(stepbackTask.Insert())
	b2 := build.Build{
		Id:           "b2",
		BuildVariant: "bv",
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
		Requester:           evergreen.TriggerRequester,
		Version:             v.Id,
	}
	b3 := build.Build{
		Id:           "b3",
		BuildVariant: "bv",
	}
	assert.NoError(b3.Insert())

	// Should not unschedule previous tasks if the requester is not repotracker.
	assert.NoError(evalStepback(ctx, &finishedTask, "", evergreen.TaskSucceeded, true))
	checkTask, err := task.FindOneId(stepbackTask.Id)
	assert.NoError(err)
	assert.True(checkTask.Activated)

	finishedTask.Requester = evergreen.RepotrackerVersionRequester
	assert.NoError(evalStepback(ctx, &finishedTask, "", evergreen.TaskSucceeded, true))
	checkTask, err = task.FindOneId(stepbackTask.Id)
	assert.NoError(err)
	assert.False(checkTask.Activated)
}

func TestEvalStepback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, ProjectRefCollection, ParserProjectCollection, distro.Collection, build.Collection, VersionCollection))
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
	require.NoError(t, d.Insert(ctx))
	v := Version{
		Id:        "sample_version",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	require.NoError(t, v.Insert())
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(yml), &pp)
	assert.NoError(err)
	pp.Id = v.Id
	assert.NoError(pp.Insert())
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
	}
	assert.NoError(b3.Insert())

	// should not step back if there was never a successful task
	assert.NoError(evalStepback(ctx, &finishedTask, "", evergreen.TaskFailed, false))
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
	}
	assert.NoError(b1.Insert())
	assert.NoError(evalStepback(ctx, &finishedTask, "", evergreen.TaskFailed, false))
	checkTask, err = task.FindOneId(stepbackTask.Id)
	require.NoError(t, err)
	assert.True(checkTask.Activated)

	// Generated task should step back its generator.
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
	}
	assert.NoError(b5.Insert())
	// Ensure system failure doesn't cause a stepback unless we're already stepping back.
	assert.NoError(evalStepback(ctx, &generated, "", evergreen.TaskSystemFailed, false))
	checkTask, err = task.FindOneId(stepbackTask.Id)
	assert.NoError(err)
	assert.False(checkTask.Activated)

	// System failure steps back since activated by stepback (and steps back generator).
	generated.ActivatedBy = evergreen.StepbackTaskActivator
	assert.NoError(evalStepback(ctx, &generated, "", evergreen.TaskSystemFailed, false))
	checkTask, err = task.FindOneId(stepbackTask.Id)
	assert.NoError(err)
	assert.True(checkTask.Activated)
}

func TestEvalStepbackTaskGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(task.Collection, ParserProjectCollection, VersionCollection, build.Collection, event.EventCollection, ProjectRefCollection))
	v1 := Version{
		Id:        "v1",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	v2 := Version{
		Id:        "prev_v1",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	v3 := Version{
		Id:        "prev_success_v1",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	pp := ParserProject{
		Stepback: utility.TruePtr(),
	}
	pp.Id = "v1"
	require.NoError(t, pp.Insert())
	pp.Id = "prev_v1"
	require.NoError(t, pp.Insert())
	pp.Id = "prev_success_v1"
	require.NoError(t, pp.Insert())
	require.NoError(t, db.InsertMany(VersionCollection, v1, v2, v3))
	p1 := ProjectRef{
		Id: "p1",
	}
	require.NoError(t, p1.Insert())
	b1 := build.Build{
		Id: "prev_b1",
	}
	b2 := build.Build{
		Id: "prev_b2",
	}
	require.NoError(t, db.InsertMany(build.Collection, b1, b2))
	t1 := task.Task{
		Id:                  "t1",
		Project:             p1.Id,
		BuildId:             "b1",
		Version:             "v1",
		TaskGroup:           "my_group",
		TaskGroupMaxHosts:   1,
		BuildVariant:        "bv",
		DisplayName:         "first",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskSucceeded,
		Activated:           true,
		RevisionOrderNumber: 3,
	}
	t2 := task.Task{
		Id:                  "t2",
		Project:             p1.Id,
		BuildId:             "b1",
		Version:             "v1",
		TaskGroup:           "my_group",
		TaskGroupMaxHosts:   1,
		BuildVariant:        "bv",
		DisplayName:         "second",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskFailed,
		Activated:           true,
		RevisionOrderNumber: 3,
	}
	t3 := task.Task{
		Id:                  "t3",
		BuildId:             "b1",
		Version:             "v1",
		Project:             p1.Id,
		TaskGroup:           "my_group",
		TaskGroupMaxHosts:   1,
		BuildVariant:        "bv",
		DisplayName:         "third",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskUndispatched,
		Activated:           true,
		RevisionOrderNumber: 3,
	}
	prevT1 := task.Task{
		Id:                  "prev_t1",
		BuildId:             "prev_b1",
		Version:             "prev_v1",
		Project:             p1.Id,
		TaskGroup:           "my_group",
		BuildVariant:        "bv",
		DisplayName:         "first",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskUndispatched,
		Activated:           false,
		RevisionOrderNumber: 2,
	}
	prevT2 := task.Task{
		Id:                  "prev_t2",
		BuildId:             "prev_b1",
		Version:             "prev_v1",
		Project:             p1.Id,
		TaskGroup:           "my_group",
		BuildVariant:        "bv",
		DisplayName:         "second",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskUndispatched,
		Activated:           false,
		RevisionOrderNumber: 2,
	}
	prevT3 := task.Task{
		Id:                  "prev_t3",
		BuildId:             "prev_b1",
		Version:             "prev_v1",
		Project:             p1.Id,
		TaskGroup:           "my_group",
		BuildVariant:        "bv",
		DisplayName:         "third",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskUndispatched,
		Activated:           false,
		RevisionOrderNumber: 2,
	}
	prevSuccessT1 := task.Task{
		Id:                  "prev_success_t1",
		BuildId:             "prev_success_b1",
		Version:             "prev_success_v1",
		Project:             p1.Id,
		TaskGroup:           "my_group",
		BuildVariant:        "bv",
		DisplayName:         "first",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskSucceeded,
		Activated:           true,
		RevisionOrderNumber: 1,
	}
	prevSuccessT2 := task.Task{
		Id:                  "prev_success_t2",
		BuildId:             "prev_success_b1",
		Version:             "prev_success_v1",
		Project:             p1.Id,
		TaskGroup:           "my_group",
		BuildVariant:        "bv",
		DisplayName:         "second",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskSucceeded,
		Activated:           true,
		RevisionOrderNumber: 1,
	}
	prevSuccessT3 := task.Task{
		Id:                  "prev_success_t3",
		BuildId:             "prev_success_b1",
		Version:             "prev_success_v1",
		Project:             p1.Id,
		TaskGroup:           "my_group",
		BuildVariant:        "bv",
		DisplayName:         "third",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskSucceeded,
		Activated:           true,
		RevisionOrderNumber: 1,
	}
	assert.NoError(t, db.InsertMany(task.Collection, t1, t2, t3, prevT1, prevT2, prevT3, prevSuccessT1, prevSuccessT2, prevSuccessT3))
	assert.NoError(t, evalStepback(ctx, &t2, "", evergreen.TaskFailed, false))

	// verify only the previous t1 and t2 are stepped back
	prevT1FromDb, err := task.FindOneId(prevT1.Id)
	assert.NoError(t, err)
	assert.True(t, prevT1FromDb.Activated)
	prevT2FromDb, err := task.FindOneId(prevT2.Id)
	assert.NoError(t, err)
	assert.True(t, prevT2FromDb.Activated)
	prevT3FromDb, err := task.FindOneId(prevT3.Id)
	assert.NoError(t, err)
	assert.False(t, prevT3FromDb.Activated)

	// stepping back t3 should now also stepback t3 and not error on earlier activated tasks
	assert.NoError(t, evalStepback(ctx, &t3, "", evergreen.TaskFailed, false))
	prevT3FromDb, err = task.FindOneId(prevT3.Id)
	assert.NoError(t, err)
	assert.True(t, prevT3FromDb.Activated)
}

func TestUpdateBlockedDependencies(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(VersionCollection, task.Collection, build.Collection, event.EventCollection))

	v := Version{Id: "version0"}
	require.NoError(v.Insert())
	b0 := build.Build{
		Id:      "build0",
		Version: v.Id,
		Status:  evergreen.BuildStarted,
	}
	require.NoError(b0.Insert())
	b1 := build.Build{
		Id:      "build1",
		Version: v.Id,
		Status:  evergreen.BuildCreated,
	}
	require.NoError(b1.Insert())
	tasks := []task.Task{
		{
			Id:      "t0",
			Version: v.Id,
			BuildId: b0.Id,
			Status:  evergreen.TaskFailed,
		},
		{
			Id:      "t1",
			Version: v.Id,
			BuildId: b0.Id,
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
			Version:     v.Id,
			BuildId:     b0.Id,
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
			Version: v.Id,
			BuildId: b0.Id,
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
			Version: v.Id,
			BuildId: b1.Id,
			DependsOn: []task.Dependency{
				{
					TaskId: "t3",
					Status: evergreen.TaskSucceeded,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id:      "t5",
			Version: v.Id,
			BuildId: b1.Id,
			DependsOn: []task.Dependency{
				{
					TaskId: "t0",
					Status: evergreen.TaskSucceeded,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
	}
	for _, t := range tasks {
		assert.NoError(t.Insert())
	}
	execTask := task.Task{
		Id: "t2-execution",
		DependsOn: []task.Dependency{
			{
				TaskId: "t1",
				Status: evergreen.TaskSucceeded,
			},
		},
		BuildId:     b0.Id,
		DisplayTask: &tasks[2],
	}
	assert.NoError(execTask.Insert())

	assert.NoError(UpdateBlockedDependencies(&tasks[0]))

	dbTask1, err := task.FindOneId(tasks[1].Id)
	assert.NoError(err)
	assert.Len(dbTask1.DependsOn, 2)
	assert.True(dbTask1.DependsOn[0].Unattainable)
	assert.True(dbTask1.DependsOn[1].Unattainable) // this task has duplicates which are also marked

	dbTask2, err := task.FindOneId(tasks[2].Id)
	assert.NoError(err)
	assert.True(dbTask2.DependsOn[0].Unattainable)

	dbTask3, err := task.FindOneId(tasks[3].Id)
	assert.NoError(err)
	assert.True(dbTask3.DependsOn[0].Unattainable)

	// We don't traverse past t3 which was already unattainable == true
	dbTask4, err := task.FindOneId(tasks[4].Id)
	assert.NoError(err)
	assert.False(dbTask4.DependsOn[0].Unattainable)

	// update more than one dependency (t1 and t5)
	dbTask5, err := task.FindOneId(tasks[5].Id)
	assert.NoError(err)
	assert.True(dbTask5.DependsOn[0].Unattainable)

	dbExecTask, err := task.FindOneId(execTask.Id)
	assert.NoError(err)
	assert.True(dbExecTask.DependsOn[0].Unattainable)

	dbBuild0, err := build.FindOneId(b0.Id)
	require.NoError(err)
	require.NotZero(dbBuild0)
	assert.Equal(evergreen.BuildFailed, dbBuild0.Status, "build status with failed and blocked tasks should be updated")

	dbBuild1, err := build.FindOneId(b1.Id)
	require.NoError(err)
	require.NotZero(dbBuild1)
	assert.Equal(evergreen.BuildCreated, dbBuild1.Status, "build status should not need to be updated")

	dbVersion, err := VersionFindOneId(v.Id)
	require.NoError(err)
	require.NotZero(dbVersion)
	assert.Equal(evergreen.VersionFailed, dbVersion.Status, "version status with all finished or blocked tasks should be updated")

	// one event inserted for every updated task, one for the updated build, and
	// one for the updated version.
	events, err := event.Find(db.Q{})
	assert.NoError(err)
	assert.Len(events, 6)

}

func TestUpdateUnblockedDependencies(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, build.Collection, VersionCollection))
	v := Version{Id: "v"}
	b := build.Build{Id: "build0", Version: v.Id}
	b2 := build.Build{Id: "build2", Version: v.Id, AllTasksBlocked: true}
	tasks := []task.Task{
		{Id: "t0", BuildId: b.Id, Version: v.Id},
		{Id: "t1", BuildId: b.Id, Version: v.Id, Status: evergreen.TaskFailed},
		{
			Id:      "t2",
			Version: v.Id,
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
			Version: v.Id,
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
			Version: v.Id,
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
			Version: v.Id,
			BuildId: b.Id,
			DependsOn: []task.Dependency{
				{
					TaskId:       "t4",
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id:      "t6",
			Version: v.Id,
			BuildId: b2.Id,
			DependsOn: []task.Dependency{
				{
					TaskId:       "t0",
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id:      "t7",
			Version: v.Id,
			BuildId: b2.Id,
			Status:  evergreen.TaskDispatched,
		},
	}

	for _, t := range tasks {
		assert.NoError(t.Insert())
	}

	assert.NoError(v.Insert())
	assert.NoError(b.Insert())
	assert.NoError(b2.Insert())

	assert.NoError(UpdateUnblockedDependencies(&tasks[0]))

	// this task should still be marked blocked because t1 is unattainable
	dbTask2, err := task.FindOneId(tasks[2].Id)
	assert.NoError(err)
	assert.False(dbTask2.DependsOn[0].Unattainable)
	assert.True(dbTask2.DependsOn[1].Unattainable)

	dbTask3, err := task.FindOneId(tasks[3].Id)
	assert.NoError(err)
	assert.False(dbTask3.DependsOn[0].Unattainable)

	dbTask4, err := task.FindOneId(tasks[4].Id)
	assert.NoError(err)
	assert.False(dbTask4.DependsOn[0].Unattainable)

	// We don't traverse past the t4 which was already unattainable == false
	dbTask5, err := task.FindOneId(tasks[5].Id)
	assert.NoError(err)
	assert.True(dbTask5.DependsOn[0].Unattainable)

	// Unblocking a dependent task should unblock its build
	dbTask6, err := task.FindOneId(tasks[6].Id)
	assert.NoError(err)
	assert.False(dbTask6.DependsOn[0].Unattainable)
	dbBuild2, err := build.FindOneId(b2.Id)
	assert.NoError(err)
	assert.False(dbBuild2.AllTasksBlocked)
}

type TaskConnectorAbortTaskSuite struct {
	suite.Suite
}

func TestDBTaskConnectorAbortTaskSuite(t *testing.T) {
	s := new(TaskConnectorAbortTaskSuite)
	suite.Run(t, s)
}

func (s *TaskConnectorAbortTaskSuite) TestAbort() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(db.ClearCollections(task.Collection, user.Collection, build.Collection, VersionCollection))
	taskToAbort := task.Task{Id: "task1", Status: evergreen.TaskStarted, BuildId: "b1", Version: "v1"}
	s.NoError(taskToAbort.Insert())
	s.NoError((&build.Build{Id: "b1"}).Insert())
	s.NoError((&Version{Id: "v1"}).Insert())
	u := user.DBUser{
		Id: "user1",
	}
	s.NoError(u.Insert())
	err := AbortTask(ctx, "task1", "user1")
	s.NoError(err)
	foundTask, err := task.FindOneId("task1")
	s.NoError(err)
	s.Equal("user1", foundTask.AbortInfo.User)
	s.Equal(true, foundTask.Aborted)
}

func (s *TaskConnectorAbortTaskSuite) TestAbortFail() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(db.ClearCollections(task.Collection, user.Collection))
	taskToAbort := task.Task{Id: "task1", Status: evergreen.TaskStarted}
	s.NoError(taskToAbort.Insert())
	u := user.DBUser{
		Id: "user1",
	}
	s.NoError(u.Insert())
	err := AbortTask(ctx, "task1", "user1")
	s.Error(err)
}
