package model

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	oneMs = time.Millisecond
)

// checkDisabled checks that the given task is disabled and logs the expected
// events.
func checkDisabled(t *testing.T, dbTask *task.Task) {
	assert.Equal(t, evergreen.DisabledTaskPriority, dbTask.Priority, "task '%s' should have disabled priority", dbTask.Id)
	assert.False(t, dbTask.Activated, "task '%s' should be deactivated", dbTask.Id)

	events, err := event.FindAllByResourceID(t.Context(), dbTask.Id)
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

func requireTaskFromDB(ctx context.Context, t *testing.T, id string) *task.Task {
	dbTask, err := task.FindOneId(ctx, id)
	require.NoError(t, err)
	require.NotZero(t, dbTask)
	return dbTask
}

func TestDisableOneTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, event.EventCollection, build.Collection, VersionCollection))
	}()

	type disableFunc func(t *testing.T, tsk task.Task) error

	for funcName, disable := range map[string]disableFunc{
		"DisableTasks": func(t *testing.T, tsk task.Task) error {
			return DisableTasks(ctx, t.Name(), tsk)
		},
	} {
		t.Run(funcName, func(t *testing.T) {
			for tName, tCase := range map[string]func(t *testing.T, tasks [5]task.Task){
				"DisablesNormalTask": func(t *testing.T, tasks [5]task.Task) {
					require.NoError(t, disable(t, tasks[3]))

					dbTask, err := task.FindOneId(ctx, tasks[3].Id)
					require.NoError(t, err)
					require.NotZero(t, dbTask)

					checkDisabled(t, dbTask)
				},
				"DisablesTaskAndDeactivatesItsDependents": func(t *testing.T, tasks [5]task.Task) {
					require.NoError(t, disable(t, tasks[4]))

					dbTask, err := task.FindOneId(ctx, tasks[4].Id)
					require.NoError(t, err)
					require.NotZero(t, dbTask)

					checkDisabled(t, dbTask)

					dbDependentTask, err := task.FindOneId(ctx, tasks[3].Id)
					require.NoError(t, err)
					require.NotZero(t, dbDependentTask)

					assert.Zero(t, dbDependentTask.Priority, "dependent task should not have been disabled")
					assert.False(t, dbDependentTask.Activated, "dependent task should have been deactivated")
				},
				"DisablesDisplayTaskAndItsExecutionTasks": func(t *testing.T, tasks [5]task.Task) {
					require.NoError(t, disable(t, tasks[0]))

					dbDisplayTask, err := task.FindOneId(ctx, tasks[0].Id)
					require.NoError(t, err)
					require.NotZero(t, dbDisplayTask)
					checkDisabled(t, dbDisplayTask)

					dbExecTasks, err := task.FindAll(ctx, db.Query(task.ByIds([]string{tasks[1].Id, tasks[2].Id})))
					require.NoError(t, err)
					assert.Len(t, dbExecTasks, 2)

					for _, task := range dbExecTasks {
						checkDisabled(t, &task)
					}
				},
				"DoesNotDisableParentDisplayTask": func(t *testing.T, tasks [5]task.Task) {
					require.NoError(t, disable(t, tasks[1]))

					dbExecTask, err := task.FindOneId(ctx, tasks[1].Id)
					require.NoError(t, err)
					require.NotZero(t, dbExecTask)

					checkDisabled(t, dbExecTask)

					dbDisplayTask, err := task.FindOneId(ctx, tasks[0].Id)
					require.NoError(t, err)
					require.NotZero(t, dbDisplayTask)

					assert.Zero(t, dbDisplayTask.Priority, "display task is not modified when its execution task is disabled")
					assert.True(t, dbDisplayTask.Activated, "display task is not modified when its execution task is disabled")
				},
			} {
				t.Run(tName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(task.Collection, event.EventCollection, build.Collection, VersionCollection))
					versionId := primitive.NewObjectID()
					v := &Version{
						Id: versionId.Hex(),
					}
					require.NoError(t, v.Insert(t.Context()))
					b := &build.Build{
						Id:      "build-id",
						Version: v.Id,
					}
					require.NoError(t, b.Insert(t.Context()))
					tasks := [5]task.Task{
						{Id: "display-task0", DisplayOnly: true, ExecutionTasks: []string{"exec-task1", "exec-task2"}, Activated: true, BuildId: b.Id, Version: v.Id},
						{Id: "exec-task1", DisplayTaskId: utility.ToStringPtr("display-task0"), Activated: true, BuildId: b.Id, Version: v.Id},
						{Id: "exec-task2", DisplayTaskId: utility.ToStringPtr("display-task0"), Activated: true, BuildId: b.Id, Version: v.Id},
						{Id: "task3", Activated: true, DependsOn: []task.Dependency{{TaskId: "task4"}}, BuildId: b.Id, Version: v.Id},
						{Id: "task4", Activated: true, BuildId: b.Id, Version: v.Id},
					}
					for _, task := range tasks {
						require.NoError(t, task.Insert(t.Context()))
					}
					tCase(t, tasks)
				})
			}
		})
	}
}

func TestDisableManyTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
			require.NoError(t, dt.Insert(t.Context()))
			require.NoError(t, et1.Insert(t.Context()))
			require.NoError(t, et2.Insert(t.Context()))
			require.NoError(t, et3.Insert(t.Context()))

			require.NoError(t, DisableTasks(ctx, t.Name(), et1, et2))

			dbDisplayTask, err := task.FindOneId(ctx, dt.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask)

			assert.Zero(t, dbDisplayTask.Priority, "parent display task priority should not be modified when execution tasks are disabled")
			assert.True(t, dbDisplayTask.Activated, "parent display task should not be deactivated when execution tasks are disabled")

			dbExecTask1, err := task.FindOneId(ctx, et1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask1)
			checkDisabled(t, dbExecTask1)

			dbExecTask2, err := task.FindOneId(ctx, et2.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask2)
			checkDisabled(t, dbExecTask1)

			dbExecTask3, err := task.FindOneId(ctx, et3.Id)
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
			require.NoError(t, dt1.Insert(t.Context()))
			require.NoError(t, dt2.Insert(t.Context()))
			require.NoError(t, et1.Insert(t.Context()))
			require.NoError(t, et2.Insert(t.Context()))
			require.NoError(t, et3.Insert(t.Context()))
			require.NoError(t, et4.Insert(t.Context()))

			require.NoError(t, DisableTasks(ctx, t.Name(), et1, et3, dt2))

			dbDisplayTask1, err := task.FindOneId(ctx, dt1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask1)

			assert.Zero(t, dbDisplayTask1.Priority, "parent display task priority should not be modified when execution tasks are disabled")
			assert.True(t, dbDisplayTask1.Activated, "parent display task should not be deactivated when execution tasks are disabled")

			dbDisplayTask2, err := task.FindOneId(ctx, dt2.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask2)

			checkDisabled(t, dbDisplayTask2)

			dbExecTask1, err := task.FindOneId(ctx, et1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask1)
			checkDisabled(t, dbExecTask1)

			dbExecTask2, err := task.FindOneId(ctx, et2.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask2)
			assert.Zero(t, dbExecTask2.Priority, "priority of execution task under same parent display task as disabled execution tasks should not be modified")
			assert.True(t, dbExecTask2.Activated, "execution task under same parent display task as disabled execution tasks should not be deactivated")

			dbExecTask3, err := task.FindOneId(ctx, et3.Id)
			require.NoError(t, err)
			require.NotZero(t, dbExecTask3)
			checkDisabled(t, dbExecTask3)

			dbExecTask4, err := task.FindOneId(ctx, et4.Id)
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
			require.NoError(t, v.Insert(t.Context()))
			b := &build.Build{
				Id:      "build-id",
				Version: v.Id,
			}
			require.NoError(t, b.Insert(t.Context()))
			tCase(t)
		})
	}
}

func TestSetActiveState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With one task with no dependencies", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, task.OldCollection, VersionCollection))
		var err error

		displayName := "testName"
		userName := "testUser"
		testTime := time.Now()
		versionId := mgobson.NewObjectId()
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
			Requester:         evergreen.GithubMergeRequester,
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

		So(b.Insert(t.Context()), ShouldBeNil)
		So(testTask.Insert(t.Context()), ShouldBeNil)
		So(dependentTask.Insert(t.Context()), ShouldBeNil)
		So(v.Insert(t.Context()), ShouldBeNil)
		So(p.Insert(t.Context()), ShouldBeNil)
		Convey("activating the task should set the task state to active and mark the version as activated", func() {
			So(SetActiveState(ctx, "randomUser", true, *testTask), ShouldBeNil)
			testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
			So(err, ShouldBeNil)
			So(testTask.Activated, ShouldBeTrue)
			So(testTask.ScheduledTime, ShouldHappenWithin, oneMs, testTime)

			version, err := VersionFindOneId(t.Context(), testTask.Version)
			So(err, ShouldBeNil)
			So(utility.FromBoolPtr(version.Activated), ShouldBeTrue)
			Convey("deactivating an active task as a normal user should deactivate the task", func() {
				So(SetActiveState(ctx, userName, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldBeFalse)
				dependentTask, err = task.FindOne(ctx, db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)

				build, err := build.FindOneId(t.Context(), testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(t.Context(), testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionFailed)
			})
		})
		Convey("when deactivating an active task as evergreen", func() {
			Convey("if the task is activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(ctx, "", true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, "")
				So(SetActiveState(ctx, "", false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
				dependentTask, err = task.FindOne(ctx, db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)
				build, err := build.FindOneId(t.Context(), testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(t.Context(), testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionFailed)
			})
			Convey("if the task is activated by stepback user, the task should not deactivate", func() {
				So(SetActiveState(ctx, evergreen.StepbackTaskActivator, true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.StepbackTaskActivator)
				So(SetActiveState(ctx, evergreen.APIServerTaskActivator, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, true)
				build, err := build.FindOneId(t.Context(), testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildStarted)
				version, err := VersionFindOneId(t.Context(), testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionStarted)
			})
			Convey("if the task is not activated by evergreen, the task should not deactivate", func() {
				So(SetActiveState(ctx, userName, true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, userName)
				So(SetActiveState(ctx, evergreen.APIServerTaskActivator, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, true)
				build, err := build.FindOneId(t.Context(), testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildStarted)
				version, err := VersionFindOneId(t.Context(), testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionStarted)
			})
		})
		Convey("when deactivating an active task a normal user", func() {
			u := "test_user"
			Convey("if the task is activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(ctx, "", true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, "")
				So(SetActiveState(ctx, u, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
				dependentTask, err = task.FindOne(ctx, db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)
				build, err := build.FindOneId(t.Context(), testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(t.Context(), testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionFailed)
			})
			Convey("if the task is activated by stepback user, the task should deactivate", func() {
				So(SetActiveState(ctx, evergreen.StepbackTaskActivator, true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.StepbackTaskActivator)
				So(SetActiveState(ctx, u, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
				dependentTask, err = task.FindOne(ctx, db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)

				build, err := build.FindOneId(t.Context(), testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(t.Context(), testTask.Version)
				So(err, ShouldBeNil)
				So(version.Status, ShouldEqual, evergreen.VersionFailed)
			})
			Convey("if the task is not activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(ctx, userName, true, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, userName)
				So(SetActiveState(ctx, u, false, *testTask), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
				dependentTask, err = task.FindOne(ctx, db.Query(task.ById(dependentTask.Id)))
				So(dependentTask.Activated, ShouldBeFalse)
				build, err := build.FindOneId(t.Context(), testTask.BuildId)
				So(err, ShouldBeNil)
				So(build.Status, ShouldEqual, evergreen.BuildFailed)
				version, err := VersionFindOneId(t.Context(), testTask.Version)
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
		So(v.Insert(t.Context()), ShouldBeNil)
		So(dep1.Insert(t.Context()), ShouldBeNil)
		So(dep2.Insert(t.Context()), ShouldBeNil)

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
		So(b.Insert(t.Context()), ShouldBeNil)
		So(testTask.Insert(t.Context()), ShouldBeNil)
		So(testTask.DistroId, ShouldNotEqual, "")

		Convey("activating the task should activate the tasks it depends on", func() {
			So(SetActiveState(ctx, userName, true, testTask), ShouldBeNil)
			depTask, err := task.FindOne(ctx, db.Query(task.ById(dep1.Id)))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			depTask, err = task.FindOne(ctx, db.Query(task.ById(dep2.Id)))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			Convey("deactivating the task should not deactivate the tasks it depends on", func() {
				So(SetActiveState(ctx, userName, false, testTask), ShouldBeNil)
				depTask, err = task.FindOne(ctx, db.Query(task.ById(depTask.Id)))
				So(err, ShouldBeNil)
				So(depTask.Activated, ShouldBeTrue)
			})

		})

		Convey("activating a task with override dependencies set should not activate the tasks it depends on", func() {
			So(testTask.SetOverrideDependencies(ctx, userName), ShouldBeNil)

			So(SetActiveState(ctx, userName, true, testTask), ShouldBeNil)
			depTask, err := task.FindOne(ctx, db.Query(task.ById(dep1.Id)))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeFalse)

			depTask, err = task.FindOne(ctx, db.Query(task.ById(dep2.Id)))
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
		So(b.Insert(t.Context()), ShouldBeNil)
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
		So(dt.Insert(t.Context()), ShouldBeNil)
		t1 := &task.Task{
			Id:        "execTask",
			Activated: false,
			BuildId:   b.Id,
			Status:    evergreen.TaskUndispatched,
			Version:   "version",
		}
		So(t1.Insert(t.Context()), ShouldBeNil)
		Convey("that should not restart", func() {
			So(SetActiveState(ctx, "test", true, *dt), ShouldBeNil)
			t1FromDb, err := task.FindOne(ctx, db.Query(task.ById(t1.Id)))
			So(err, ShouldBeNil)
			So(t1FromDb.Activated, ShouldBeTrue)
			dtFromDb, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
			So(err, ShouldBeNil)
			So(dtFromDb.Activated, ShouldBeTrue)
		})
		Convey("that should activate and deactivate", func() {
			dt.DispatchTime = time.Now()
			So(SetActiveState(ctx, "test", true, *dt), ShouldBeNil)
			t1FromDb, err := task.FindOne(ctx, db.Query(task.ById(t1.Id)))
			So(err, ShouldBeNil)
			So(t1FromDb.Activated, ShouldBeTrue)
			dtFromDb, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
			So(err, ShouldBeNil)
			So(dtFromDb.Activated, ShouldBeTrue)

			So(SetActiveState(ctx, "test", false, *t1FromDb), ShouldBeNil)
			t1FromDb, err = task.FindOne(ctx, db.Query(task.ById(t1.Id)))
			So(err, ShouldBeNil)
			So(t1FromDb.Activated, ShouldBeFalse)
			dtFromDb, err = task.FindOne(ctx, db.Query(task.ById(dt.Id)))
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
		So(b.Insert(t.Context()), ShouldBeNil)
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
		So(taskDef.Insert(t.Context()), ShouldBeNil)

		taskDef.Id = "task2"
		taskDef.Activated = false
		taskDef.Status = evergreen.TaskUndispatched
		taskDef.TaskGroupOrder = 2
		So(taskDef.Insert(t.Context()), ShouldBeNil) // should be scheduled

		taskDef.Id = "task4"
		taskDef.TaskGroupOrder = 4
		So(taskDef.Insert(t.Context()), ShouldBeNil) //should not be activated

		taskDef.Id = "task3"
		taskDef.TaskGroupOrder = 3
		So(taskDef.Insert(t.Context()), ShouldBeNil) // the task we're activating

		So(SetActiveState(ctx, "test", true, *taskDef), ShouldBeNil)

		taskGroup, err := task.FindTaskGroupFromBuild(ctx, b.Id, taskDef.TaskGroup)
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
		So(b.Insert(t.Context()), ShouldBeNil)
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
		So(taskDef.Insert(t.Context()), ShouldBeNil)

		taskDef.Id = "task2"
		taskDef.TaskGroupOrder = 2
		taskDef.Status = evergreen.TaskDispatched
		taskDef.DependsOn = append(taskDef.DependsOn, task.Dependency{TaskId: "task1", Status: evergreen.TaskSucceeded})
		So(taskDef.Insert(t.Context()), ShouldBeNil) // should not be unscheduled

		taskDef.Id = "task3"
		taskDef.TaskGroupOrder = 3
		taskDef.DependsOn = append(taskDef.DependsOn, task.Dependency{TaskId: "task2", Status: evergreen.TaskSucceeded})
		So(taskDef.Insert(t.Context()), ShouldBeNil) // task to deactivate

		taskDef.Id = "task4"
		taskDef.TaskGroupOrder = 4
		taskDef.DependsOn = append(taskDef.DependsOn, task.Dependency{TaskId: "task3", Status: evergreen.TaskSucceeded})
		So(taskDef.Insert(t.Context()), ShouldBeNil) // task should also be deactivated

		taskDef.Id = "task3"
		So(SetActiveState(ctx, "test", false, *taskDef), ShouldBeNil)

		taskGroup, err := task.FindTaskGroupFromBuild(ctx, b.Id, taskDef.TaskGroup)
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

		So(v.Insert(t.Context()), ShouldBeNil)
		So(b.Insert(t.Context()), ShouldBeNil)
		So(previousTask.Insert(t.Context()), ShouldBeNil)
		So(currentTask.Insert(t.Context()), ShouldBeNil)
		Convey("activating a previous task should set the previous task's active field to true", func() {
			So(activatePreviousTask(ctx, currentTask.Id, "", nil), ShouldBeNil)
			t, err := task.FindOne(ctx, db.Query(task.ById(previousTask.Id)))
			So(err, ShouldBeNil)
			So(t.Activated, ShouldBeTrue)
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
		So(b.Insert(t.Context()), ShouldBeNil)
		So(v.Insert(t.Context()), ShouldBeNil)
		So(previouserTask.Insert(t.Context()), ShouldBeNil)
		So(previousTask.Insert(t.Context()), ShouldBeNil)
		So(currentTask.Insert(t.Context()), ShouldBeNil)
		So(activeDependentTask.Insert(t.Context()), ShouldBeNil)
		So(inactiveDependentTask.Insert(t.Context()), ShouldBeNil)
		Convey("should deactivate previous task", func() {
			So(DeactivatePreviousTasks(ctx, currentTask, userName), ShouldBeNil)
			var err error
			// Deactivates this task even though it has a dependent task, because it's inactive.
			previousTask, err = task.FindOne(ctx, db.Query(task.ById(previousTask.Id)))
			So(err, ShouldBeNil)
			So(previousTask.Activated, ShouldBeFalse)

			// Shouldn't deactivate this task because it has an active dependent task.
			previouserTask, err = task.FindOne(ctx, db.Query(task.ById(previouserTask.Id)))
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
		So(b1.Insert(t.Context()), ShouldBeNil)
		So(b2.Insert(t.Context()), ShouldBeNil)
		So(b3.Insert(t.Context()), ShouldBeNil)
		So(v1.Insert(t.Context()), ShouldBeNil)
		So(v2.Insert(t.Context()), ShouldBeNil)
		So(v3.Insert(t.Context()), ShouldBeNil)
		So(dt1.Insert(t.Context()), ShouldBeNil)
		So(dt2.Insert(t.Context()), ShouldBeNil)
		So(dt3.Insert(t.Context()), ShouldBeNil)
		So(et1.Insert(t.Context()), ShouldBeNil)
		So(et2.Insert(t.Context()), ShouldBeNil)
		So(et3.Insert(t.Context()), ShouldBeNil)
		So(et4.Insert(t.Context()), ShouldBeNil)
		Convey("deactivating a display task should deactivate its child tasks", func() {
			So(DeactivatePreviousTasks(ctx, dt2, userName), ShouldBeNil)
			dbTask, err := task.FindOne(ctx, db.Query(task.ById(dt1.Id)))
			So(err, ShouldBeNil)
			So(dbTask.Activated, ShouldBeFalse)
			dbTask, err = task.FindOne(ctx, db.Query(task.ById(et1.Id)))
			So(err, ShouldBeNil)
			So(dbTask.Activated, ShouldBeFalse)
			Convey("but should not touch any tasks that have started", func() {
				dbTask, err = task.FindOne(ctx, db.Query(task.ById(dt3.Id)))
				So(err, ShouldBeNil)
				So(dbTask.Activated, ShouldBeTrue)
				dbTask, err = task.FindOne(ctx, db.Query(task.ById(et3.Id)))
				So(err, ShouldBeNil)
				So(dbTask.Activated, ShouldBeTrue)
				So(dbTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				dbTask, err = task.FindOne(ctx, db.Query(task.ById(et4.Id)))
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
			expectedPatchStatus:       evergreen.VersionSucceeded,
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
			expectedPatchStatus:       evergreen.VersionSucceeded,
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
				Version:   primitive.NewObjectID().Hex(),
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
				Version:   v.Id,
				Status:    evergreen.VersionCreated,
				Activated: true,
			}
			require.NoError(t, b.Insert(t.Context()))
			require.NoError(t, v.Insert(t.Context()))
			require.NoError(t, p.Insert(t.Context()))

			for i, tempTask := range test.tasks {
				tempTask.Id = strconv.Itoa(i)
				tempTask.BuildId = b.Id
				tempTask.Version = v.Id
				require.NoError(t, tempTask.Insert(t.Context()))
			}
			// Verify tasks are inserted and found correctly
			tasks, err := task.FindWithFields(ctx, task.ByBuildId(b.Id))
			assert.NoError(t, err)
			assert.Len(t, tasks, 2)

			assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &task.Task{Version: v.Id, BuildId: b.Id}))

			b, err = build.FindOneId(t.Context(), b.Id)
			require.NoError(t, err)
			assert.Equal(t, test.expectedBuildStatus, b.Status)
			assert.Equal(t, test.expectedBuildActivation, b.Activated)

			v, err = VersionFindOneId(t.Context(), v.Id)
			require.NoError(t, err)
			require.NotZero(t, v)
			assert.Equal(t, test.expectedVersionStatus, v.Status)
			assert.Equal(t, test.expectedVersionActivation, utility.FromBoolPtr(v.Activated))
			if evergreen.IsFinishedVersionStatus(test.expectedVersionStatus) {
				events, err := event.FindAllByResourceID(t.Context(), v.Id)
				require.NoError(t, err)
				var numVersionFinishedEvents int
				for _, e := range events {
					if e.ResourceType != event.ResourceTypeVersion {
						continue
					}
					if e.EventType != event.VersionStateChange {
						continue
					}
					data, ok := e.Data.(*event.VersionEventData)
					require.True(t, ok)
					assert.Equal(t, test.expectedVersionStatus, data.Status)
					numVersionFinishedEvents++
				}
				assert.Equal(t, 1, numVersionFinishedEvents, "expected to find exactly one version finished event")
			}

			p, err = patch.FindOneId(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			require.NotZero(t, p)
			assert.Equal(t, test.expectedPatchStatus, p.Status)
			assert.Equal(t, test.expectedPatchActivation, p.Activated)
			if evergreen.IsFinishedVersionStatus(test.expectedPatchStatus) {
				events, err := event.FindAllByResourceID(t.Context(), p.Id.Hex())
				require.NoError(t, err)
				var numPatchFinishedEvents int
				for _, e := range events {
					if e.ResourceType != event.ResourceTypePatch {
						continue
					}
					if e.EventType != event.PatchStateChange {
						continue
					}
					data, ok := e.Data.(*event.PatchEventData)
					require.True(t, ok)
					assert.Equal(t, test.expectedPatchStatus, data.Status)
					numPatchFinishedEvents++
				}
				assert.Equal(t, 1, numPatchFinishedEvents, "expected to find exactly one patch finished event")
			}
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

	assert.NoError(t, b.Insert(t.Context()))
	assert.NoError(t, p.Insert(t.Context()))
	assert.NoError(t, v.Insert(t.Context()))
	assert.NoError(t, testTask.Insert(t.Context()))
	assert.NoError(t, anotherTask.Insert(t.Context()))

	assert.NoError(t, UpdateVersionAndPatchStatusForBuilds(ctx, []string{b.Id}))
	dbBuild, err := build.FindOneId(t.Context(), b.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildStarted, dbBuild.Status)
	dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionStarted, dbPatch.Status)

	err = task.UpdateOne(
		ctx,
		bson.M{task.IdKey: testTask.Id},
		bson.M{"$set": bson.M{task.StatusKey: evergreen.TaskFailed}},
	)
	assert.NoError(t, err)
	assert.NoError(t, UpdateVersionAndPatchStatusForBuilds(ctx, []string{b.Id}))
	dbBuild, err = build.FindOneId(t.Context(), b.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildFailed, dbBuild.Status)
	dbPatch, err = patch.FindOneId(t.Context(), p.Id.Hex())
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

	assert.NoError(t, b.Insert(t.Context()))
	assert.NoError(t, v.Insert(t.Context()))
	assert.NoError(t, testTask.Insert(t.Context()))
	assert.NoError(t, anotherTask.Insert(t.Context()))

	assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &testTask))
	dbBuild, err := build.FindOneId(t.Context(), b.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildStarted, dbBuild.Status)
	dbVersion, err := VersionFindOneId(t.Context(), v.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionStarted, dbVersion.Status)
	events, err := event.FindAllByResourceID(t.Context(), v.Id)
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

	assert.NoError(t, b1.Insert(t.Context()))
	assert.NoError(t, b2.Insert(t.Context()))
	v1 := Version{
		Id:     "v1",
		Status: evergreen.VersionStarted,
	}
	assert.NoError(t, v1.Insert(t.Context()))
	versionStatus, statusChanged, err := updateVersionStatus(t.Context(), &v1)
	assert.NoError(t, err)
	assert.Equal(t, versionStatus, v1.Status) // version status hasn't changed
	assert.False(t, statusChanged)

	events, err := event.FindAllByResourceID(t.Context(), "v1")
	assert.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, event.VersionGithubCheckFinished, events[0].EventType)
}

func TestUpdateVersionStatus(t *testing.T) {
	colls := []string{task.Collection, build.Collection, VersionCollection, event.EventCollection}
	type testCase struct {
		builds []build.Build

		expectedVersionStatus        string
		expectedVersionAborted       bool
		expectedVersionActivation    bool
		expectedVersionStatusChanged bool
	}

	for name, test := range map[string]testCase{
		"VersionCreatedForAllUnactivatedBuilds": {
			builds: []build.Build{
				{Status: evergreen.BuildCreated},
				{Status: evergreen.BuildCreated},
			},
			expectedVersionStatus:        evergreen.VersionCreated,
			expectedVersionStatusChanged: false,
			expectedVersionAborted:       false,
			expectedVersionActivation:    false,
		},
		"VersionStartedForMixOfSucceededBuildAndBuildWithUnfinishedEssentialTasks": {
			builds: []build.Build{
				{Status: evergreen.BuildCreated, Activated: true, HasUnfinishedEssentialTask: true},
				{Status: evergreen.BuildSucceeded, Activated: true},
			},
			expectedVersionStatus:        evergreen.VersionStarted,
			expectedVersionStatusChanged: true,
			expectedVersionAborted:       false,
			expectedVersionActivation:    true,
		},
		"VersionStartedForMixOfFailedBuildAndBuildWithUnfinishedEssentialTasks": {
			builds: []build.Build{
				{Status: evergreen.BuildCreated, HasUnfinishedEssentialTask: true},
				{Status: evergreen.BuildFailed, Activated: true},
			},
			expectedVersionStatus:        evergreen.VersionFailed,
			expectedVersionStatusChanged: true,
			expectedVersionAborted:       false,
			expectedVersionActivation:    true,
		},
		"VersionStartedForMixOfFinishedAndUnfinishedBuilds": {
			builds: []build.Build{
				{Status: evergreen.BuildStarted, Activated: true},
				{Status: evergreen.BuildSucceeded, Activated: true},
			},
			expectedVersionStatus:        evergreen.VersionStarted,
			expectedVersionStatusChanged: true,
			expectedVersionAborted:       false,
			expectedVersionActivation:    true,
		},
		"VersionAbortedForAbortedBuild": {
			builds: []build.Build{
				{Status: evergreen.BuildFailed, Activated: true, Aborted: true},
				{Status: evergreen.BuildSucceeded, Activated: true},
			},
			expectedVersionStatus:        evergreen.VersionFailed,
			expectedVersionStatusChanged: true,
			expectedVersionAborted:       true,
			expectedVersionActivation:    true,
		},
		"VersionFinishedForAllFinishedBuilds": {
			builds: []build.Build{
				{Status: evergreen.BuildFailed, Activated: true},
				{Status: evergreen.BuildSucceeded, Activated: true},
			},
			expectedVersionStatus:        evergreen.VersionFailed,
			expectedVersionStatusChanged: true,
			expectedVersionAborted:       false,
			expectedVersionActivation:    true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(colls...))
			v := &Version{
				Id:        primitive.NewObjectID().Hex(),
				Status:    evergreen.VersionCreated,
				Activated: utility.TruePtr(),
			}
			require.NoError(t, v.Insert(t.Context()))
			for i, b := range test.builds {
				b.Id = strconv.Itoa(i)
				b.Version = v.Id
				require.NoError(t, b.Insert(t.Context()))
			}

			status, statusChanged, err := updateVersionStatus(t.Context(), v)
			require.NoError(t, err)
			assert.Equal(t, test.expectedVersionStatus, status)
			assert.Equal(t, test.expectedVersionStatusChanged, statusChanged)
			if statusChanged {
				events, err := event.FindAllByResourceID(t.Context(), v.Id)
				require.NoError(t, err)
				require.Len(t, events, 1)
				assert.Equal(t, event.VersionStateChange, events[0].EventType, "should log version event if the version status has changed")
			} else {
				events, err := event.FindAllByResourceID(t.Context(), v.Id)
				require.NoError(t, err)
				assert.Empty(t, events, "should not log any version events if the version status hasn't changed")
			}

			dbVersion, err := VersionFindOneId(t.Context(), v.Id)
			require.NoError(t, err)
			assert.Equal(t, test.expectedVersionStatus, dbVersion.Status)
			assert.Equal(t, test.expectedVersionAborted, dbVersion.Aborted)
			assert.Equal(t, test.expectedVersionActivation, utility.FromBoolPtr(dbVersion.Activated))
		})
	}

	t.Run("ReturnsStatusNotChangedForMarkingSuccessOnVersionAlreadyMarkedSuccessful", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(colls...))
		v := &Version{
			Id:        primitive.NewObjectID().Hex(),
			Status:    evergreen.VersionSucceeded,
			Activated: utility.TruePtr(),
		}
		require.NoError(t, v.Insert(t.Context()))

		b := build.Build{
			Id:      "b",
			Version: v.Id,
			Status:  evergreen.VersionSucceeded,
		}
		require.NoError(t, b.Insert(t.Context()))

		status, statusChanged, err := updateVersionStatus(t.Context(), v)
		require.NoError(t, err)
		assert.Equal(t, evergreen.VersionSucceeded, status)
		assert.False(t, statusChanged, "status was already success so it should not change the status")

		events, err := event.FindAllByResourceID(t.Context(), v.Id)
		require.NoError(t, err)
		assert.Empty(t, events, "should not log any version events if the version status hasn't changed")

		dbVersion, err := VersionFindOneId(t.Context(), v.Id)
		require.NoError(t, err)
		assert.Equal(t, evergreen.VersionSucceeded, dbVersion.Status)
	})
}

func TestUpdatePatchStatus(t *testing.T) {
	type eventTypeAndData struct {
		eventType string
		data      any
	}
	checkPatchEvents := func(t *testing.T, p *patch.Patch, expectedEvents []eventTypeAndData) {
		events, err := event.FindAllByResourceID(t.Context(), p.Id.Hex())
		require.NoError(t, err)
		assert.Len(t, events, len(expectedEvents))
		for _, e := range events {
			for _, expected := range expectedEvents {
				if e.EventType != expected.eventType {
					continue
				}
				assert.EqualValues(t, expected.data, e.Data)
			}
		}
	}
	for tName, tCase := range map[string]func(t *testing.T, p *patch.Patch){
		"UpdatesPatchToStarted": func(t *testing.T, p *patch.Patch) {
			require.NoError(t, p.Insert(t.Context()))

			const newStatus = evergreen.VersionStarted
			psu, err := updatePatchStatus(t.Context(), p, newStatus)
			require.NoError(t, err)

			assert.True(t, psu.patchStatusChanged)
			assert.False(t, psu.isPatchFamilyDone)
			assert.Zero(t, psu.parentPatch)
			assert.Empty(t, psu.patchFamilyFinishedCollectiveStatus)

			dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			require.NotZero(t, dbPatch)
			assert.Equal(t, newStatus, dbPatch.Status)
			assert.Zero(t, dbPatch.FinishTime)

			checkPatchEvents(t, p, []eventTypeAndData{
				{
					eventType: event.PatchStateChange,
					data: &event.PatchEventData{
						Status: newStatus,
					},
				},
			})
		},
		"UpdatesParentPatchToStarted": func(t *testing.T, p *patch.Patch) {
			childPatch := &patch.Patch{
				Id:     patch.NewId(primitive.NewObjectID().Hex()),
				Status: evergreen.VersionCreated,
				Triggers: patch.TriggerInfo{
					ParentPatch: p.Id.Hex(),
				},
			}
			require.NoError(t, childPatch.Insert(t.Context()))
			p.Triggers.ChildPatches = []string{childPatch.Id.Hex()}
			require.NoError(t, p.Insert(t.Context()))

			const newStatus = evergreen.VersionStarted
			psu, err := updatePatchStatus(t.Context(), p, newStatus)
			require.NoError(t, err)

			assert.True(t, psu.patchStatusChanged)
			assert.False(t, psu.isPatchFamilyDone)
			require.NotZero(t, psu.parentPatch)
			assert.Equal(t, p.Id.Hex(), psu.parentPatch.Id.Hex())
			assert.Empty(t, psu.patchFamilyFinishedCollectiveStatus)

			dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			require.NotZero(t, dbPatch)
			assert.Equal(t, newStatus, dbPatch.Status)
			assert.Zero(t, dbPatch.FinishTime)

			checkPatchEvents(t, p, []eventTypeAndData{
				{
					eventType: event.PatchStateChange,
					data: &event.PatchEventData{
						Status: newStatus,
					},
				},
			})
		},
		"UpdatesParentPatchToSuccessButChildFailed": func(t *testing.T, p *patch.Patch) {
			childPatch := &patch.Patch{
				Id:     patch.NewId(primitive.NewObjectID().Hex()),
				Status: evergreen.VersionFailed,
				Triggers: patch.TriggerInfo{
					ParentPatch: p.Id.Hex(),
				},
			}
			require.NoError(t, childPatch.Insert(t.Context()))
			p.Triggers.ChildPatches = []string{childPatch.Id.Hex()}
			require.NoError(t, p.Insert(t.Context()))

			const newStatus = evergreen.VersionSucceeded
			psu, err := updatePatchStatus(t.Context(), p, newStatus)
			require.NoError(t, err)

			assert.True(t, psu.patchStatusChanged)
			assert.True(t, psu.isPatchFamilyDone)
			require.NotZero(t, psu.parentPatch)
			assert.Equal(t, p.Id.Hex(), psu.parentPatch.Id.Hex())
			assert.Equal(t, evergreen.VersionFailed, psu.patchFamilyFinishedCollectiveStatus)

			dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			require.NotZero(t, dbPatch)
			assert.Equal(t, newStatus, dbPatch.Status)
			assert.NotZero(t, dbPatch.FinishTime)

			checkPatchEvents(t, p, []eventTypeAndData{
				{
					eventType: event.PatchStateChange,
					data: &event.PatchEventData{
						Status: newStatus,
					},
				},
				{
					eventType: event.PatchChildrenCompletion,
					data: &event.PatchEventData{
						Author: p.Author,
						Status: evergreen.VersionFailed,
					},
				},
			})
		},
		"UpdatesPatchToFinished": func(t *testing.T, p *patch.Patch) {
			require.NoError(t, p.Insert(t.Context()))

			const newStatus = evergreen.VersionSucceeded
			psu, err := updatePatchStatus(t.Context(), p, newStatus)
			require.NoError(t, err)

			assert.True(t, psu.patchStatusChanged)
			assert.True(t, psu.isPatchFamilyDone)
			assert.Zero(t, psu.parentPatch)
			assert.Equal(t, newStatus, psu.patchFamilyFinishedCollectiveStatus)

			dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			require.NotZero(t, dbPatch)
			assert.Equal(t, newStatus, dbPatch.Status)
			assert.NotZero(t, dbPatch.FinishTime)

			checkPatchEvents(t, p, []eventTypeAndData{
				{
					eventType: event.PatchStateChange,
					data: &event.PatchEventData{
						Status: newStatus,
					},
				},
				{
					eventType: event.PatchChildrenCompletion,
					data: &event.PatchEventData{
						Status: newStatus,
						Author: p.Author,
					},
				},
			})
		},
		"NoopsForUpdatingFinishedPatchToSameStatus": func(t *testing.T, p *patch.Patch) {
			p.Status = evergreen.VersionSucceeded
			p.FinishTime = time.Now()
			require.NoError(t, p.Insert(t.Context()))

			const newStatus = evergreen.VersionSucceeded
			psu, err := updatePatchStatus(t.Context(), p, newStatus)
			require.NoError(t, err)

			assert.False(t, psu.patchStatusChanged, "patch status should not change")

			dbPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			require.NotZero(t, dbPatch)
			assert.Equal(t, newStatus, dbPatch.Status)
			assert.NotZero(t, dbPatch.FinishTime)

			events, err := event.FindAllByResourceID(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			assert.Empty(t, events, "should not log new patch/vesion finished events when the patch is already finished")
		},
		"SetsChildrenCompletedTimeWhenParentPatchCompletes": func(t *testing.T, p *patch.Patch) {
			childPatch := &patch.Patch{
				Id:         patch.NewId(primitive.NewObjectID().Hex()),
				Status:     evergreen.VersionSucceeded,
				FinishTime: time.Now().Add(-time.Hour),
				Triggers: patch.TriggerInfo{
					ParentPatch: p.Id.Hex(),
				},
			}
			require.NoError(t, childPatch.Insert(t.Context()))
			p.Triggers.ChildPatches = []string{childPatch.Id.Hex()}
			require.NoError(t, p.Insert(t.Context()))

			psu, err := updatePatchStatus(t.Context(), p, evergreen.VersionSucceeded)
			require.NoError(t, err)

			assert.True(t, psu.patchStatusChanged)
			assert.True(t, psu.isPatchFamilyDone)

			dbParentPatch, err := patch.FindOneId(t.Context(), p.Id.Hex())
			require.NoError(t, err)
			require.NotZero(t, dbParentPatch)
			assert.NotZero(t, dbParentPatch.Triggers.ChildrenCompletedTime)
			assert.WithinDuration(t, dbParentPatch.FinishTime, dbParentPatch.Triggers.ChildrenCompletedTime, time.Second)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(patch.Collection, event.EventCollection))

			patchID := primitive.NewObjectID().Hex()
			p := &patch.Patch{
				Id:      patch.NewId(patchID),
				Status:  evergreen.VersionCreated,
				Version: patchID,
			}

			tCase(t, p)
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

	assert.NoError(t, b1.Insert(t.Context()))
	assert.NoError(t, b2.Insert(t.Context()))
	assert.NoError(t, v.Insert(t.Context()))
	assert.NoError(t, testTask.Insert(t.Context()))
	assert.NoError(t, anotherTask.Insert(t.Context()))

	assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &testTask))
	dbBuild1, err := build.FindOneId(t.Context(), b1.Id)
	assert.NoError(t, err)
	assert.False(t, dbBuild1.Aborted)
	dbBuild2, err := build.FindOneId(t.Context(), b2.Id)
	assert.NoError(t, err)
	assert.False(t, dbBuild2.Aborted)
	dbVersion, err := VersionFindOneId(t.Context(), v.Id)
	assert.NoError(t, err)
	assert.False(t, dbVersion.Aborted)

	// abort started task
	assert.NoError(t, testTask.SetAborted(ctx, task.AbortInfo{}))
	assert.NoError(t, testTask.MarkFailed(ctx))
	assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &testTask))
	dbBuild1, err = build.FindOneId(t.Context(), b1.Id)
	assert.NoError(t, err)
	assert.True(t, dbBuild1.Aborted)
	assert.Equal(t, evergreen.BuildFailed, dbBuild1.Status)
	dbBuild2, err = build.FindOneId(t.Context(), b2.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildSucceeded, dbBuild2.Status)
	assert.False(t, dbBuild2.Aborted)
	dbVersion, err = VersionFindOneId(t.Context(), v.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionFailed, dbVersion.Status)
	assert.True(t, dbVersion.Aborted)

	// restart aborted task
	assert.NoError(t, testTask.Archive(ctx))
	assert.NoError(t, testTask.MarkUnscheduled(ctx))
	assert.NoError(t, UpdateBuildAndVersionStatusForTask(ctx, &testTask))
	dbBuild1, err = build.FindOneId(t.Context(), b1.Id)
	assert.NoError(t, err)
	assert.False(t, dbBuild1.Aborted)
	assert.Equal(t, evergreen.BuildCreated, dbBuild1.Status)
	dbBuild2, err = build.FindOneId(t.Context(), b2.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildSucceeded, dbBuild2.Status)
	assert.False(t, dbBuild2.Aborted)
	dbVersion, err = VersionFindOneId(t.Context(), v.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.VersionStarted, dbVersion.Status)
	assert.False(t, dbVersion.Aborted)
}

func TestGetBuildStatus(t *testing.T) {
	// The build shouldn't start until a task starts running.
	buildTasks := []task.Task{
		{Status: evergreen.TaskUndispatched},
		{Status: evergreen.TaskUndispatched},
	}
	buildStatus := getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildCreated, buildStatus.status)
	assert.False(t, buildStatus.allTasksBlocked)

	// Any started tasks should start the build.
	buildTasks = []task.Task{
		{Status: evergreen.TaskUndispatched, Activated: true},
		{Status: evergreen.TaskStarted},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildStarted, buildStatus.status)
	assert.False(t, buildStatus.allTasksBlocked)

	// Unactivated tasks shouldn't prevent the build from completing.
	buildTasks = []task.Task{
		{Status: evergreen.TaskUndispatched, Activated: false},
		{Status: evergreen.TaskFailed},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildFailed, buildStatus.status)
	assert.False(t, buildStatus.allTasksBlocked)

	// Blocked tasks shouldn't prevent the build from completing.
	buildTasks = []task.Task{
		{Status: evergreen.TaskUndispatched,
			DependsOn: []task.Dependency{{Unattainable: true}}},
		{Status: evergreen.TaskSucceeded},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildSucceeded, buildStatus.status)
	assert.False(t, buildStatus.allTasksBlocked)

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
	assert.False(t, buildStatus.allTasksBlocked)

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
	assert.False(t, buildStatus.allTasksBlocked)

	// Builds with only blocked tasks should stay as created.
	buildTasks = []task.Task{
		{Status: evergreen.TaskUndispatched,
			DependsOn: []task.Dependency{{Unattainable: true}}},
		{Status: evergreen.TaskUndispatched,
			DependsOn: []task.Dependency{{Unattainable: true}}},
	}
	buildStatus = getBuildStatus(buildTasks)
	assert.Equal(t, evergreen.BuildCreated, buildStatus.status)
	assert.True(t, buildStatus.allTasksBlocked)

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
	require.NoError(t, v.Insert(t.Context()))

	builds := []build.Build{
		{IsGithubCheck: true, Status: evergreen.BuildSucceeded},
		{IsGithubCheck: false, Status: evergreen.BuildCreated},
	}

	assert.NoError(t, updateVersionGithubStatus(t.Context(), v, builds))

	e, err := event.FindUnprocessedEvents(t.Context(), -1)
	assert.NoError(t, err)
	require.Len(t, e, 1)
}

func TestUpdateBuildGithubStatus(t *testing.T) {
	require.NoError(t, db.ClearCollections(build.Collection, event.EventCollection))
	buildID := "b1"
	b := &build.Build{Id: buildID}
	require.NoError(t, b.Insert(t.Context()))

	tasks := []task.Task{
		{IsGithubCheck: true, Status: evergreen.TaskSucceeded},
		{IsGithubCheck: false, Status: evergreen.TaskUndispatched},
	}

	assert.NoError(t, updateBuildGithubStatus(t.Context(), b, tasks))

	b, err := build.FindOneId(t.Context(), buildID)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.BuildSucceeded, b.GithubCheckStatus)

	e, err := event.FindUnprocessedEvents(t.Context(), -1)
	assert.NoError(t, err)
	require.Len(t, e, 1)
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

	require.NoError(projRef.Insert(t.Context()))
	require.NoError(b.Insert(t.Context()))
	require.NoError(testTask.Insert(t.Context()))
	require.NoError(v.Insert(t.Context()))
	require.NoError(pp.Insert(t.Context()))
	require.NoError(dependentTask.Insert(t.Context()))
	require.NoError(taskHost.Insert(ctx))

	details := apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
	}
	settings := testutil.TestConfig()
	assert.NoError(MarkEnd(ctx, settings, &testTask, userName, time.Now(), &details))

	b, err := build.FindOneId(t.Context(), b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildFailed, b.Status)

	dbDependentTask, err := task.FindOneId(ctx, dependentTask.Id)
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
		So(b.Insert(t.Context()), ShouldBeNil)
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		So(v.Insert(t.Context()), ShouldBeNil)
		pp := &ParserProject{
			Id:         b.Version,
			Identifier: utility.ToStringPtr("sample"),
		}
		So(pp.Insert(t.Context()), ShouldBeNil)
		dt := &task.Task{
			Id:             "displayTask",
			Activated:      true,
			BuildId:        b.Id,
			Status:         evergreen.TaskStarted,
			DisplayOnly:    true,
			ExecutionTasks: []string{"execTask"},
			Version:        "version1",
		}
		So(dt.Insert(t.Context()), ShouldBeNil)
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
		So(t1.Insert(t.Context()), ShouldBeNil)
		So(t2.Insert(t.Context()), ShouldBeNil)

		detail := &apimodels.TaskEndDetail{
			Status: evergreen.TaskSucceeded,
		}
		endTime := time.Now().Round(time.Second)
		So(MarkEnd(ctx, settings, t1, "test", endTime, detail), ShouldBeNil)
		t1FromDb, err := task.FindOne(ctx, db.Query(task.ById(t1.Id)))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskSucceeded)
		dtFromDb, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskSucceeded)

		// Ensure that calling MarkEnd on a non-aborted finished task returns early
		// by checking that its finish_time hasn't changed
		So(MarkEnd(ctx, settings, t1, "test", time.Now().Add(time.Minute), detail), ShouldBeNil)
		t1FromDb, err = task.FindOne(ctx, db.Query(task.ById(t1.Id)))
		So(err, ShouldBeNil)
		So(t1FromDb.FinishTime, ShouldEqual, endTime)

		// Ensure that calling MarkEnd on an aborted finished task does not return early.
		endTime = time.Now().Round(time.Second)
		So(AbortTask(ctx, t2.Id, "testUser"), ShouldBeNil)
		t2FromDb, err := task.FindOne(ctx, db.Query(task.ById(t2.Id)))
		So(err, ShouldBeNil)
		So(t2FromDb.FinishTime, ShouldEqual, time.Time{})
		So(MarkEnd(ctx, settings, t2FromDb, "test", endTime, &t2FromDb.Details), ShouldBeNil)
		t2FromDb, err = task.FindOne(ctx, db.Query(task.ById(t2.Id)))
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
	settings := testutil.TestConfig()
	for name, test := range map[string]func(*testing.T){
		"NotResetWhenFinished": func(t *testing.T) {
			assert.NoError(t, MarkEnd(ctx, settings, runningTask, "test", time.Now(), detail))
			runningTaskDB, err := task.FindOneId(ctx, runningTask.Id)
			assert.NoError(t, err)
			assert.NotNil(t, runningTaskDB)
			assert.Equal(t, evergreen.TaskFailed, runningTaskDB.Status)
		},
		"ResetWhenFinished": func(t *testing.T) {
			assert.NoError(t, runningTask.SetResetWhenFinished(ctx, "test"))
			assert.NoError(t, MarkEnd(ctx, settings, runningTask, "test", time.Now(), detail))

			runningTaskDB, err := task.FindOneId(ctx, runningTask.Id)
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
			assert.NoError(runningTask.Insert(t.Context()))
			assert.NoError(otherTask.Insert(t.Context()))
			pRef := &ProjectRef{Id: "my_project"}
			assert.NoError(pRef.Insert(t.Context()))
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
			assert.NoError(pp.Insert(t.Context()))
			assert.NoError(b.Insert(t.Context()))
			assert.NoError(v.Insert(t.Context()))

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

func TestMarkEndIsAutomaticRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runningTask := &task.Task{
		ResetWhenFinished:  true,
		IsAutomaticRestart: true,
		Id:                 "say-hi-123",
		Status:             evergreen.TaskStarted,
		Activated:          true,
		ActivatedTime:      time.Now(),
		BuildId:            "b",
		TaskGroupOrder:     1,
		Project:            "my_project",
		DistroId:           "my_distro",
		Version:            "abc",
		BuildVariant:       "a_variant",
		HostId:             "h1",
	}
	displayTask := &task.Task{
		ResetWhenFinished:  true,
		IsAutomaticRestart: true,
		Id:                 "dt",
		Status:             evergreen.TaskStarted,
		Activated:          true,
		ActivatedTime:      time.Now(),
		BuildId:            "b",
		TaskGroupOrder:     1,
		Project:            "my_project",
		DistroId:           "my_distro",
		Version:            "abc",
		BuildVariant:       "a_variant",
		HostId:             "h1",
		ExecutionTasks:     []string{"execTask0", "execTask1"},
		DisplayOnly:        true,
	}
	execTask0 := &task.Task{
		Id:            "execTask0",
		BuildId:       "b",
		Status:        evergreen.TaskStarted,
		Activated:     true,
		ActivatedTime: time.Now(),
		Project:       "my_project",
		DistroId:      "my_distro",
		Version:       "abc",
		BuildVariant:  "a_variant",
		HostId:        "h1",
	}
	execTask1 := &task.Task{
		Id:            "execTask1",
		BuildId:       "b",
		Status:        evergreen.TaskStarted,
		Activated:     true,
		ActivatedTime: time.Now(),
		Project:       "my_project",
		DistroId:      "my_distro",
		Version:       "abc",
		BuildVariant:  "a_variant",
		HostId:        "h1",
	}
	tgTask1 := &task.Task{
		ResetWhenFinished:  true,
		IsAutomaticRestart: true,
		Id:                 "tg1",
		DisplayName:        "say-hi",
		Status:             evergreen.TaskStarted,
		Activated:          true,
		ActivatedTime:      time.Now(),
		BuildId:            "b",
		TaskGroup:          "my_task_group",
		TaskGroupMaxHosts:  1,
		TaskGroupOrder:     1,
		Project:            "my_project",
		DistroId:           "my_distro",
		Version:            "abc",
		BuildVariant:       "a_variant",
		HostId:             "h1",
	}
	tgTask2 := &task.Task{
		Id:                "tg2",
		DisplayName:       "say-hi",
		Status:            evergreen.TaskUndispatched,
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
		DependsOn: []task.Dependency{
			{
				TaskId: "tg1",
				Status: evergreen.TaskSucceeded,
			},
		},
	}
	detail := &apimodels.TaskEndDetail{
		Type:   evergreen.CommandTypeSystem,
		Status: evergreen.TaskFailed,
	}
	for name, test := range map[string]func(*testing.T){
		"ResetsSingleTask": func(t *testing.T) {
			assert.NoError(t, MarkEnd(ctx, &evergreen.Settings{}, runningTask, "test", time.Now(), detail))
			runningTaskDB, err := task.FindOneId(ctx, runningTask.Id)
			assert.NoError(t, err)
			assert.NotNil(t, runningTaskDB)
			assert.Equal(t, 1, runningTaskDB.Execution)
			assert.Equal(t, evergreen.TaskUndispatched, runningTaskDB.Status)

			// Check that trying to automatically reset again does not reset the task again.
			runningTaskDB.HostId = "h1"
			assert.NoError(t, MarkEnd(ctx, &evergreen.Settings{}, runningTaskDB, "test", time.Now(), detail))
			runningTaskDB, err = task.FindOneId(ctx, runningTask.Id)
			assert.NoError(t, err)
			assert.NotNil(t, runningTaskDB)
			assert.Equal(t, 1, runningTaskDB.Execution)
			assert.Equal(t, evergreen.TaskFailed, runningTaskDB.Status)
		},
		"ResetsDisplayTask": func(t *testing.T) {
			// Check that marking a single execution task as retryable does not yet reset the display task.
			assert.NoError(t, MarkEnd(ctx, &evergreen.Settings{}, execTask0, "test", time.Now(), detail))
			displayTaskDB, err := task.FindOneId(ctx, displayTask.Id)
			assert.NoError(t, err)
			assert.NotNil(t, displayTaskDB)
			assert.Equal(t, 0, displayTaskDB.Execution)
			assert.Equal(t, evergreen.TaskStarted, displayTaskDB.Status)

			// Check that the display task is reset when the second execution task completes.
			assert.NoError(t, MarkEnd(ctx, &evergreen.Settings{}, execTask1, "test", time.Now(), detail))
			displayTaskDB, err = task.FindOneId(ctx, displayTask.Id)
			assert.NoError(t, err)
			assert.NotNil(t, displayTaskDB)
			assert.Equal(t, 1, displayTaskDB.Execution)
			assert.Equal(t, evergreen.TaskUndispatched, displayTaskDB.Status)

		},
		"ResetsSingleHostTaskGroupWithFailure": func(t *testing.T) {
			assert.NoError(t, MarkEnd(ctx, &evergreen.Settings{}, tgTask1, "test", time.Now(), detail))
			tasks, err := task.FindTaskGroupFromBuild(ctx, tgTask1.BuildId, tgTask1.TaskGroup)
			assert.NoError(t, err)
			require.Len(t, tasks, 2)

			// The task group should reset immediately upon the first task failure.
			assert.Equal(t, 1, tasks[0].Execution)
			assert.Equal(t, evergreen.TaskUndispatched, tasks[0].Status)

			assert.Equal(t, 0, tasks[1].Execution)
			assert.Equal(t, evergreen.TaskUndispatched, tasks[1].Status)
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection,
				build.Collection, VersionCollection, ParserProjectCollection, ProjectRefCollection))
			assert := assert.New(t)
			runningTask.Status = evergreen.TaskStarted
			assert.NoError(runningTask.Insert(t.Context()))
			assert.NoError(tgTask1.Insert(t.Context()))
			assert.NoError(tgTask2.Insert(t.Context()))
			assert.NoError(displayTask.Insert(t.Context()))
			assert.NoError(execTask0.Insert(t.Context()))
			assert.NoError(execTask1.Insert(t.Context()))
			pRef := &ProjectRef{Id: "my_project"}
			assert.NoError(pRef.Insert(t.Context()))
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
			assert.NoError(pp.Insert(t.Context()))
			assert.NoError(b.Insert(t.Context()))
			assert.NoError(v.Insert(t.Context()))

			test(t)
		})
	}
}

func TestMarkEndWithDisplayTaskResetWhenFinished(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection, host.Collection))

	const (
		etID      = "execution_task_id"
		dtID      = "display_task_id"
		buildID   = "build_id"
		versionID = "version_id"
		hostID    = "host_id"
	)
	et := task.Task{
		Id:            etID,
		DisplayTaskId: utility.ToStringPtr(dtID),
		Status:        evergreen.TaskStarted,
		BuildId:       buildID,
		Version:       versionID,
		HostId:        hostID,
	}
	assert.NoError(t, et.Insert(t.Context()))
	dt := task.Task{
		Id:                dtID,
		DisplayOnly:       true,
		ExecutionTasks:    []string{etID},
		BuildId:           buildID,
		Version:           versionID,
		Status:            evergreen.TaskStarted,
		ResetWhenFinished: true,
	}
	assert.NoError(t, dt.Insert(t.Context()))
	b := build.Build{
		Id:     buildID,
		Status: evergreen.BuildStarted,
	}
	assert.NoError(t, b.Insert(t.Context()))
	v := Version{
		Id:     versionID,
		Status: evergreen.VersionStarted,
	}
	assert.NoError(t, v.Insert(t.Context()))
	h := host.Host{
		Id:     hostID,
		Status: evergreen.HostRunning,
	}
	assert.NoError(t, h.Insert(ctx))

	assert.NoError(t, MarkEnd(ctx, testutil.TestConfig(), &et, "", time.Now(), &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}))

	restartedDisplayTask, err := task.FindOneId(ctx, dtID)
	assert.NoError(t, err)
	require.NotZero(t, restartedDisplayTask)
	assert.Equal(t, evergreen.TaskUndispatched, restartedDisplayTask.Status, "display task should restart when execution task finishes")
	assert.Equal(t, 1, restartedDisplayTask.Execution, "execution number should have incremented")

	originalDisplayTask, err := task.FindOneOldByIdAndExecution(ctx, dtID, 0)
	assert.NoError(t, err)
	require.NotZero(t, originalDisplayTask)
	assert.Equal(t, evergreen.TaskSucceeded, originalDisplayTask.Status, "original display task should be successful")

	restartedExecTask, err := task.FindOneId(ctx, etID)
	assert.NoError(t, err)
	require.NotZero(t, restartedExecTask)
	assert.Equal(t, evergreen.TaskUndispatched, restartedExecTask.Status, "execution task should restart when it finishes")
	assert.Equal(t, 1, restartedExecTask.Execution, "execution number should have incremented")

	originalExecTask, err := task.FindOneOldByIdAndExecution(ctx, etID, 0)
	assert.NoError(t, err)
	require.NotZero(t, originalExecTask)
	assert.Equal(t, evergreen.TaskSucceeded, originalExecTask.Status, "original execution task should be successful")

}

func TestTryResetTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := testutil.TestConfig()
	Convey("With a task that does not exist", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection))
		So(TryResetTask(ctx, settings, "id", "username", "", nil), ShouldNotBeNil)
	})
	Convey("With a task, a build, version and a project", t, func() {
		Convey("resetting a task without a max number of executions", func() {
			require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection, event.EventCollection))

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

			So(b.Insert(t.Context()), ShouldBeNil)
			So(testTask.Insert(t.Context()), ShouldBeNil)
			So(otherTask.Insert(t.Context()), ShouldBeNil)
			So(dependentTask.Insert(t.Context()), ShouldBeNil)
			So(v.Insert(t.Context()), ShouldBeNil)
			Convey("should reset and add a task to the old tasks collection", func() {
				So(TryResetTask(ctx, settings, testTask.Id, userName, "", detail), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldResemble, apimodels.TaskEndDetail{})
				So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(testTask.FinishTime, ShouldResemble, utility.ZeroTime)
				So(testTask.Activated, ShouldBeTrue)
				oldTaskId := fmt.Sprintf("%v_%v", testTask.Id, 1)
				oldTask, err := task.FindOneOld(ctx, task.ById(oldTaskId))
				So(err, ShouldBeNil)
				So(oldTask, ShouldNotBeNil)
				So(oldTask.Execution, ShouldEqual, 1)
				So(oldTask.Details, ShouldResemble, *detail)
				So(oldTask.FinishTime, ShouldNotResemble, utility.ZeroTime)

				// should also reset the build status to "started"
				buildFromDb, err := build.FindOne(t.Context(), build.ById(b.Id))
				So(err, ShouldBeNil)
				So(buildFromDb.Status, ShouldEqual, evergreen.BuildStarted)

				// Task's dependency should be marked as unfinished.
				dbDependentTask, err := task.FindOneId(ctx, dependentTask.Id)
				So(err, ShouldBeNil)
				So(dbDependentTask, ShouldNotBeNil)
				So(len(dbDependentTask.DependsOn), ShouldEqual, 1)
				So(dbDependentTask.DependsOn[0].TaskId, ShouldEqual, testTask.Id)
				So(dbDependentTask.DependsOn[0].Finished, ShouldBeFalse)
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
				Execution:   settings.TaskLimits.MaxTaskExecution,
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
				Execution:   settings.TaskLimits.MaxTaskExecution,
				Project:     "sample",
				Status:      evergreen.TaskSucceeded,
				Version:     b.Version,
			}
			So(b.Insert(t.Context()), ShouldBeNil)
			So(testTask.Insert(t.Context()), ShouldBeNil)
			So(v.Insert(t.Context()), ShouldBeNil)
			So(anotherTask.Insert(t.Context()), ShouldBeNil)

			systemFailedTask := &task.Task{
				Id:          "system_failed_task",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Execution:   0,
				Project:     "sample",
				Status:      evergreen.TaskFailed,
				Version:     b.Version,
				Requester:   evergreen.GithubMergeRequester,
			}
			So(systemFailedTask.Insert(t.Context()), ShouldBeNil)

			anotherSystemFailedTask := &task.Task{
				Id:          "another_system_failed_task",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Execution:   1, // We won't auto-restart system failures after one execution.
				Project:     "sample",
				Status:      evergreen.TaskFailed,
				Version:     b.Version,
				Requester:   evergreen.GithubMergeRequester,
			}
			So(anotherSystemFailedTask.Insert(t.Context()), ShouldBeNil)

			var err error

			Convey("should reset if ui package tries to reset", func() {
				So(TryResetTask(ctx, settings, testTask.Id, userName, evergreen.UIPackage, detail), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
			})
			Convey("should not reset if an origin other than the ui package tries to reset", func() {
				So(TryResetTask(ctx, settings, testTask.Id, userName, "", detail), ShouldBeNil)
				testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldNotResemble, *detail)
				So(testTask.Status, ShouldNotEqual, detail.Status)
			})
			Convey("should reset and use detail information if the UI package passes in a detail ", func() {
				So(TryResetTask(ctx, settings, anotherTask.Id, userName, evergreen.UIPackage, detail), ShouldBeNil)
				a, err := task.FindOne(ctx, db.Query(task.ById(anotherTask.Id)))
				So(err, ShouldBeNil)
				So(a.Details, ShouldResemble, apimodels.TaskEndDetail{})
				So(a.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(a.FinishTime, ShouldResemble, utility.ZeroTime)
			})
			Convey("system failed tasks should not reset if admin setting disabled", func() {
				newSettings := &evergreen.Settings{
					ServiceFlags: evergreen.ServiceFlags{
						SystemFailedTaskRestartDisabled: true,
					},
					TaskLimits: evergreen.TaskLimitsConfig{
						MaxTaskExecution: 9,
					},
				}
				So(TryResetTask(ctx, newSettings, systemFailedTask.Id, userName, "", detail), ShouldBeNil)
				systemFailedTask, err = task.FindOne(ctx, db.Query(task.ById(systemFailedTask.Id)))
				So(err, ShouldBeNil)
				So(systemFailedTask.Details, ShouldNotResemble, *detail)
				So(systemFailedTask.Status, ShouldNotEqual, detail.Status)
				So(testTask.Status, ShouldNotEqual, evergreen.TaskUndispatched)
			})
			Convey("system failed tasks should reset if they haven't reached the admin setting limit", func() {
				detail.Type = evergreen.CommandTypeSystem
				So(TryResetTask(ctx, settings, systemFailedTask.Id, userName, "", detail), ShouldBeNil)
				systemFailedTask, err = task.FindOne(ctx, db.Query(task.ById(systemFailedTask.Id)))
				So(err, ShouldBeNil)
				So(systemFailedTask.Status, ShouldEqual, evergreen.TaskUndispatched)
			})
			Convey("system failed tasks should not reset if they've reached the admin setting limit", func() {
				detail.Type = evergreen.CommandTypeSystem
				So(TryResetTask(ctx, settings, anotherSystemFailedTask.Id, userName, "", detail), ShouldBeNil)
				anotherSystemFailedTask, err = task.FindOne(ctx, db.Query(task.ById(systemFailedTask.Id)))
				So(err, ShouldBeNil)
				So(systemFailedTask.Status, ShouldNotEqual, evergreen.TaskUndispatched)
			})
		})
	})

	Convey("with a display task", t, func() {
		b := &build.Build{
			Id:      "displayBuild",
			Project: "sample",
			Version: "version1",
		}
		So(b.Insert(t.Context()), ShouldBeNil)
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		So(v.Insert(t.Context()), ShouldBeNil)
		dt := &task.Task{
			Id:             "displayTask",
			Activated:      true,
			BuildId:        b.Id,
			Status:         evergreen.TaskSucceeded,
			DisplayOnly:    true,
			ExecutionTasks: []string{"execTask"},
			Version:        b.Version,
		}
		So(dt.Insert(t.Context()), ShouldBeNil)
		t1 := &task.Task{
			Id:        "execTask",
			Activated: true,
			BuildId:   b.Id,
			Status:    evergreen.TaskSucceeded,
			Version:   b.Version,
		}
		So(t1.Insert(t.Context()), ShouldBeNil)

		So(TryResetTask(ctx, settings, dt.Id, "user", "test", nil), ShouldBeNil)
		t1FromDb, err := task.FindOne(ctx, db.Query(task.ById(t1.Id)))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskUndispatched)
		t1Events, err := event.FindAllByResourceID(t.Context(), dt.Id)
		So(err, ShouldBeNil)
		So(len(t1Events), ShouldEqual, 1)
		So(t1Events[0].EventType, ShouldEqual, event.TaskRestarted)

		dtFromDb, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskUndispatched)

		dtEvents, err := event.FindAllByResourceID(t.Context(), dt.Id)
		So(err, ShouldBeNil)
		So(len(dtEvents), ShouldEqual, 1)
		So(dtEvents[0].EventType, ShouldEqual, event.TaskRestarted)
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
	assert.NoError(b.Insert(t.Context()))
	assert.NoError(v.Insert(t.Context()))
	d := &distro.Distro{
		Id: "my_distro",
		PlannerSettings: distro.PlannerSettings{
			Version: evergreen.PlannerVersionTunable,
		},
	}
	assert.NoError(d.Insert(ctx))

	settings := testutil.TestConfig()

	for name, test := range map[string]func(*testing.T, *task.Task, string){
		"NotFinished": func(t *testing.T, t1 *task.Task, t2Id string) {
			assert.NoError(TryResetTask(ctx, settings, t2Id, "user", "test", nil))
			err := TryResetTask(ctx, settings, t1.Id, "user", evergreen.UIPackage, nil)
			require.Error(err)
			assert.Contains(err.Error(), "cannot reset task in this status")
		},
		"CanResetTaskGroup": func(t *testing.T, t1 *task.Task, t2Id string) {
			assert.NoError(t1.MarkFailed(ctx))
			assert.NoError(TryResetTask(ctx, settings, t2Id, "user", "test", nil))

			var err error
			t1, err = task.FindOneId(ctx, t1.Id)
			assert.NoError(err)
			assert.NotNil(t1)
			assert.Equal(evergreen.TaskUndispatched, t1.Status)
			assert.Equal(evergreen.TaskWillRun, t1.DisplayStatusCache)
			t2, err := task.FindOneId(ctx, t2Id)
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
			assert.NoError(runningTask.Insert(t.Context()))
			assert.NoError(otherTask.Insert(t.Context()))
			assert.NoError(runningTask.MarkStart(ctx, time.Now()))
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
		So(b.Insert(t.Context()), ShouldBeNil)
		So(v.Insert(t.Context()), ShouldBeNil)
		So(testTask.Insert(t.Context()), ShouldBeNil)
		So(finishedTask.Insert(t.Context()), ShouldBeNil)
		var err error
		Convey("with a task that has started, aborting a task should work", func() {
			So(AbortTask(ctx, testTask.Id, userName), ShouldBeNil)
			testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
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
			So(dt.Insert(t.Context()), ShouldBeNil)
			et1 := task.Task{
				Id:      "et1",
				Status:  evergreen.TaskStarted,
				BuildId: b.Id,
				Version: v.Id,
			}
			So(et1.Insert(t.Context()), ShouldBeNil)
			et2 := task.Task{
				Id:      "et2",
				Status:  evergreen.TaskFailed,
				BuildId: b.Id,
				Version: v.Id,
			}
			So(et2.Insert(t.Context()), ShouldBeNil)

			So(AbortTask(ctx, dt.Id, userName), ShouldBeNil)
			dbTask, err := task.FindOneId(ctx, dt.Id)
			So(err, ShouldBeNil)
			So(dbTask.Aborted, ShouldBeTrue)
			dbTask, err = task.FindOneId(ctx, et1.Id)
			So(err, ShouldBeNil)
			So(dbTask.Aborted, ShouldBeTrue)
			dbTask, err = task.FindOneId(ctx, et2.Id)
			So(err, ShouldBeNil)
			So(dbTask.Aborted, ShouldBeFalse)
		})
	})

}

func TestMarkStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		So(b.Insert(t.Context()), ShouldBeNil)
		So(testTask.Insert(t.Context()), ShouldBeNil)
		So(v.Insert(t.Context()), ShouldBeNil)

		Convey("when calling MarkStart, the task, version and build should be updated", func() {
			updates := StatusChanges{}
			err := MarkStart(ctx, testTask, &updates)
			So(updates.BuildNewStatus, ShouldBeEmpty)
			So(updates.PatchNewStatus, ShouldBeEmpty)
			So(err, ShouldBeNil)
			testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskStarted)
			So(testTask.DisplayStatusCache, ShouldEqual, evergreen.TaskStarted)
			b, err = build.FindOne(t.Context(), build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			v, err = VersionFindOne(t.Context(), VersionById(v.Id))
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
		So(b.Insert(t.Context()), ShouldBeNil)
		v := &Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		So(v.Insert(t.Context()), ShouldBeNil)
		dt := &task.Task{
			Id:             "displayTask",
			Activated:      true,
			BuildId:        b.Id,
			Status:         evergreen.TaskUndispatched,
			Version:        v.Id,
			DisplayOnly:    true,
			ExecutionTasks: []string{"execTask"},
		}
		So(dt.Insert(t.Context()), ShouldBeNil)
		t1 := &task.Task{
			Id:        "execTask",
			Activated: true,
			BuildId:   b.Id,
			Version:   v.Id,
			Status:    evergreen.TaskUndispatched,
		}
		So(t1.Insert(t.Context()), ShouldBeNil)

		So(MarkStart(ctx, t1, &StatusChanges{}), ShouldBeNil)
		t1FromDb, err := task.FindOne(ctx, db.Query(task.ById(t1.Id)))
		So(err, ShouldBeNil)
		So(t1FromDb.Status, ShouldEqual, evergreen.TaskStarted)
		dtFromDb, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
		So(err, ShouldBeNil)
		So(dtFromDb.Status, ShouldEqual, evergreen.TaskStarted)
	})
}

func TestMarkDispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

		So(b.Insert(t.Context()), ShouldBeNil)
		So(testTask.Insert(t.Context()), ShouldBeNil)
		Convey("when calling MarkStart, the task, version and build should be updated", func() {
			sampleHost := &host.Host{
				Id: "testHost",
				Distro: distro.Distro{
					Id: "distroId",
				},
				AgentRevision: "testAgentVersion",
			}
			So(MarkHostTaskDispatched(ctx, testTask, sampleHost), ShouldBeNil)
			var err error
			testTask, err = task.FindOne(ctx, db.Query(task.ById(testTask.Id)))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskDispatched)
			So(testTask.HostId, ShouldEqual, "testHost")
			So(testTask.DistroId, ShouldEqual, "distroId")
			So(testTask.AgentVersion, ShouldEqual, "testAgentVersion")
		})
	})
}

func TestGetStepback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When the project has a stepback policy set to true", t, func() {
		require.NoError(t, db.ClearCollections(ProjectRefCollection, ParserProjectCollection, task.Collection, build.Collection, VersionCollection))

		config := `
stepback: true
tasks:
 - name: true
   stepback: true
 - name: false
   stepback: false
 - name: override_false
   stepback: false
buildvariants:
 - name: sbnil
 - name: sbtrue
   stepback: true
 - name: sbfalse
   stepback: false
   tasks:
     - name: override_false
       stepback: true
`
		pp := &ParserProject{}
		err := util.UnmarshalYAMLWithFallback([]byte(config), &pp)
		assert.NoError(t, err)
		pp.Id = "version_id"
		assert.NoError(t, pp.Insert(t.Context()))

		project, err := TranslateProject(pp)
		assert.NoError(t, err)
		ver := &Version{
			Id:         "version_id",
			Identifier: "p1",
		}
		So(ver.Insert(t.Context()), ShouldBeNil)
		projRef := &ProjectRef{
			Id: "p1",
		}
		So(projRef.Insert(t.Context()), ShouldBeNil)
		Convey("if the project ref overrides the settings", func() {
			testTask := &task.Task{Id: "t1", DisplayName: "nil", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(t.Context()), ShouldBeNil)
			projRef.StepbackDisabled = utility.TruePtr()
			So(projRef.Replace(t.Context()), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(ctx, testTask.Id, projRef, project)
				So(err, ShouldBeNil)
				So(val.shouldStepback, ShouldBeFalse)
			})
		})
		Convey("if the task does not override the setting", func() {
			testTask := &task.Task{Id: "t1", DisplayName: "nil", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(t.Context()), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(ctx, testTask.Id, projRef, project)
				So(err, ShouldBeNil)
				So(val.shouldStepback, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with true", func() {
			testTask := &task.Task{Id: "t2", DisplayName: "true", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(t.Context()), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(ctx, testTask.Id, projRef, project)
				So(err, ShouldBeNil)
				So(val.shouldStepback, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with false", func() {
			testTask := &task.Task{Id: "t3", DisplayName: "false", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(t.Context()), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(ctx, testTask.Id, projRef, project)
				So(err, ShouldBeNil)
				So(val.shouldStepback, ShouldBeFalse)
			})
		})

		Convey("if the buildvariant does not override the setting", func() {
			testTask := &task.Task{Id: "t4", DisplayName: "bvnil", BuildVariant: "sbnil", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(t.Context()), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(ctx, testTask.Id, projRef, project)
				So(err, ShouldBeNil)
				So(val.shouldStepback, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with true", func() {
			testTask := &task.Task{Id: "t5", DisplayName: "bvtrue", BuildVariant: "sbtrue", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(t.Context()), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(ctx, testTask.Id, projRef, project)
				So(err, ShouldBeNil)
				So(val.shouldStepback, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with false", func() {
			testTask := &task.Task{Id: "t6", DisplayName: "bvfalse", BuildVariant: "sbfalse", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(t.Context()), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(ctx, testTask.Id, projRef, project)
				So(err, ShouldBeNil)
				So(val.shouldStepback, ShouldBeFalse)
			})
		})

		Convey("if the bvtask overrides the setting with false", func() {
			testTask := &task.Task{Id: "override_false", DisplayName: "override_false", BuildVariant: "sbfalse", Project: projRef.Id, Version: ver.Id}
			So(testTask.Insert(t.Context()), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(ctx, "override_false", projRef, project)
				So(err, ShouldBeNil)
				So(val.shouldStepback, ShouldBeTrue)
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
	assert.NoError(b.Insert(t.Context()))
	assert.NoError(v.Insert(t.Context()))
	assert.NoError(systemFailTask.Insert(t.Context()))
	assert.NoError(successfulTask.Insert(t.Context()))
	assert.NoError(inLargerRangeTask.Insert(t.Context()))
	assert.NoError(setupFailTask.Insert(t.Context()))
	assert.NoError(ranInRangeTask.Insert(t.Context()))
	assert.NoError(startedOutOfRangeTask.Insert(t.Context()))

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
	assert.Len(results.ItemsRestarted, 3)
	restarted := []string{inLargerRangeTask.Id, ranInRangeTask.Id, startedOutOfRangeTask.Id}
	assert.EqualValues(restarted, results.ItemsRestarted)

	opts.IncludeTestFailed = true
	opts.IncludeSysFailed = true
	results, err = RestartFailedTasks(ctx, opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Len(results.ItemsRestarted, 4)
	restarted = []string{systemFailTask.Id, inLargerRangeTask.Id, ranInRangeTask.Id, startedOutOfRangeTask.Id}
	assert.EqualValues(restarted, results.ItemsRestarted)

	opts.IncludeTestFailed = false
	opts.IncludeSysFailed = false
	opts.IncludeSetupFailed = true
	results, err = RestartFailedTasks(ctx, opts)
	assert.NoError(err)
	assert.Nil(results.ItemsErrored)
	assert.Len(results.ItemsRestarted, 1)
	assert.Equal("setupFailed", results.ItemsRestarted[0])

	// Test restarting all tasks but with a smaller time range
	opts.StartTime = time.Date(2017, time.June, 12, 11, 0, 0, 0, time.Local)
	opts.DryRun = false
	opts.IncludeTestFailed = false
	opts.IncludeSysFailed = false
	opts.IncludeSetupFailed = false
	results, err = RestartFailedTasks(ctx, opts)
	assert.NoError(err)
	assert.Empty(results.ItemsErrored)
	assert.Len(results.ItemsRestarted, 4)
	restarted = []string{systemFailTask.Id, setupFailTask.Id, ranInRangeTask.Id, startedOutOfRangeTask.Id}
	assert.EqualValues(restarted, results.ItemsRestarted)
	dbTask, err := task.FindOne(ctx, db.Query(task.ById(systemFailTask.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	assert.Greater(dbTask.Execution, 1)
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(successfulTask.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskSucceeded, dbTask.Status)
	assert.Equal(1, dbTask.Execution)
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(inLargerRangeTask.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.Equal(1, dbTask.Execution)
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(setupFailTask.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
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
	assert.NoError(b.Insert(t.Context()))
	assert.NoError(v.Insert(t.Context()))
	assert.NoError(testTask1.Insert(t.Context()))
	assert.NoError(testTask2.Insert(t.Context()))
	assert.NoError(testTask3.Insert(t.Context()))
	assert.NoError(testTask4.Insert(t.Context()))
	assert.NoError(testTask5.Insert(t.Context()))

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
	assert.Len(results.ItemsRestarted, 2) // not all are included in items restarted
	// but all tasks are restarted
	dbTask, err := task.FindOne(ctx, db.Query(task.ById(testTask1.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(testTask2.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(testTask3.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(testTask4.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(testTask5.Id)))
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

	assert.NoError(b1.Insert(t.Context()))
	assert.NoError(b2.Insert(t.Context()))
	assert.NoError(b3.Insert(t.Context()))
	assert.NoError(t1.Insert(t.Context()))
	assert.NoError(t2.Insert(t.Context()))
	assert.NoError(t3.Insert(t.Context()))
	assert.NoError(et1.Insert(t.Context()))
	assert.NoError(et2.Insert(t.Context()))
	assert.NoError(et3.Insert(t.Context()))
	assert.NoError(dt1.Insert(t.Context()))
	assert.NoError(dt2.Insert(t.Context()))
	assert.NoError(dt3.Insert(t.Context()))
	assert.NoError(v1.Insert(t.Context()))
	assert.NoError(v2.Insert(t.Context()))
	assert.NoError(v3.Insert(t.Context()))
	// test stepping back a regular task
	assert.NoError(doLinearStepback(ctx, t3))
	dbTask, err := task.FindOne(ctx, db.Query(task.ById(t2.Id)))
	assert.NoError(err)
	assert.True(dbTask.Activated)

	// test stepping back a display task
	assert.NoError(doLinearStepback(ctx, dt3))
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(dt2.Id)))
	assert.NoError(err)
	assert.True(dbTask.Activated)
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(dt2.Id)))
	assert.NoError(err)
	assert.True(dbTask.Activated)
}

func TestLinearStepbackWithGenerators(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context, data map[string]*task.Task){
		"ExistingUndispatchedGeneratorTask": func(t *testing.T, ctx context.Context, data map[string]*task.Task) {
			// Generator should be inactive and have no generated tasks to activate.
			generator := requireTaskFromDB(ctx, t, "t-generator-1-0") // 1st version, 0th generator
			assert.False(t, generator.Activated)
			assert.Nil(t, generator.GeneratedTasksToActivate["bv"])

			// Doing stepback on "t-generated-0-1-0" should activate the existing undispatched generator task "t-generator-0-0"
			// and set the generated task to activate on it.
			require.NoError(t, doLinearStepback(ctx, data["t-generated-2-0-0"])) // 2nd version, 0th generator, 0th generated task
			generator = requireTaskFromDB(ctx, t, "t-generator-1-0")             // 1st version, 0th generator
			assert.True(t, generator.Activated)
			assert.Equal(t, []string{"generated-task-0-0"}, generator.GeneratedTasksToActivate["bv"]) // 0th generator, 0th generated task

			// Doing stepback on the 1st generated task should add it to the generator's list of generated tasks to activate.
			require.NoError(t, doLinearStepback(ctx, data["t-generated-2-0-1"])) // 2nd version, 0th generator, 1st generated task
			generator = requireTaskFromDB(ctx, t, "t-generator-1-0")             // 1st version, 0th generator
			assert.True(t, generator.Activated)
			assert.Equal(t, []string{"generated-task-0-0", "generated-task-0-1"}, generator.GeneratedTasksToActivate["bv"]) // 0th generator, 0th generated task and 0th generator, 1st generated task
		},
		"ExistingUndispatchedGeneratedTask": func(t *testing.T, ctx context.Context, data map[string]*task.Task) {
			generated := requireTaskFromDB(ctx, t, "t-generated-1-1-0") // 1st version, 1st generator, 0th generated task
			assert.False(t, generated.Activated)

			// Doing stepback on "t-generated-2-1-0" should activate the existing undispatched generated task "t-generated-1-1-0".
			require.NoError(t, doLinearStepback(ctx, data["t-generated-2-1-0"])) // 2nd version, 1st generator, 0th generated task

			generated = requireTaskFromDB(ctx, t, "t-generated-1-1-0") // 1st version, 1st generator, 0th generated task
			assert.True(t, generated.Activated)

			// The generator should not be activated/affected.
			generator := requireTaskFromDB(ctx, t, "t-generator-1-1") // 1st version, 1st generator
			assert.False(t, generator.Activated)
			assert.Nil(t, generator.GeneratedTasksToActivate["bv"])

			// Other generated tasks should be unaffected.
			generated = requireTaskFromDB(ctx, t, "t-generated-1-1-1") // 1st version, 1st generator, 1st generated task
			assert.False(t, generated.Activated)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// The test data is across three mainline versions.
			// All have some background successful tasks. All have two generator tasks.
			// v0 is just the all passing version so stepback can enable.

			// "t-generator-0-(orderNumber)" is undispatched on v1 and succeeded on v2.
			// It's generated tasks don't exist on v1 but failed for v2.

			// "t-generator-1-(orderNumber)" succeeded on v1 and v2.
			// It's generated tasks are undispatched on v1 and failed on v2.
			data := map[string]*task.Task{}

			project := "proj"
			v0 := &Version{Id: "v0"}
			require.NoError(t, v0.Insert(t.Context()))
			v1 := &Version{Id: "v1"}
			require.NoError(t, v1.Insert(t.Context()))
			v2 := &Version{Id: "v2"}
			require.NoError(t, v2.Insert(t.Context()))
			b1 := &build.Build{
				Id:          "build1",
				DisplayName: "bv",
				Status:      evergreen.BuildStarted,
				Requester:   evergreen.RepotrackerVersionRequester,
			}
			require.NoError(t, b1.Insert(t.Context()))
			for orderNumber, v := range []string{v0.Id, v1.Id, v2.Id} {
				// 3 Background tasks that succeeded and should not be restarted
				// across the two versions.
				for i := 0; i < 3; i++ {
					backgroundTask := &task.Task{
						Id:                  fmt.Sprintf("t-success-%d-%d", orderNumber, i),
						DisplayName:         fmt.Sprintf("background-task-%d", i),
						Version:             v,
						BuildId:             b1.Id,
						BuildVariant:        b1.DisplayName,
						Project:             project,
						Status:              evergreen.TaskSucceeded,
						Requester:           evergreen.RepotrackerVersionRequester,
						Activated:           false,
						RevisionOrderNumber: orderNumber + 1,
					}
					require.NoError(t, backgroundTask.Insert(t.Context()))
					data[backgroundTask.Id] = backgroundTask
				}

				// Two generators.
				// t-generator-(orderNumber)-0 is undispatched on v1 and succeeded on v2.
				// t-generator-(orderNumber)-1 succeeded on v1 and v2.
				for i := 0; i < 2; i++ {
					status := evergreen.TaskSucceeded
					if i == 0 && v == v1.Id {
						// The first generator is undispatched on v1.
						status = evergreen.TaskUndispatched
					}
					if v == v0.Id {
						// Everything passes on v0.
						status = evergreen.TaskSucceeded
					}
					generator := &task.Task{
						Id:                  fmt.Sprintf("t-generator-%d-%d", orderNumber, i),
						DisplayName:         fmt.Sprintf("generator-task-%d", i),
						Version:             v,
						BuildId:             b1.Id,
						BuildVariant:        b1.DisplayName,
						Project:             project,
						Status:              status,
						Requester:           evergreen.RepotrackerVersionRequester,
						Activated:           false,
						RevisionOrderNumber: orderNumber + 1,
						GenerateTask:        true,
					}
					require.NoError(t, generator.Insert(t.Context()))
					data[generator.Id] = generator
				}

				// 3 Generated tasks for generator "t-generator-(orderNumber)-0" that don't exist for version 1 but failed in version 2.
				for i := 0; i < 3; i++ {
					status := evergreen.TaskFailed
					if v == v0.Id {
						// Everything passes on v0.
						status = evergreen.TaskSucceeded
					}
					if v == v1.Id {
						// Does not exist on v1.
						continue
					}
					generatedTask := &task.Task{
						Id:                  fmt.Sprintf("t-generated-%d-0-%d", orderNumber, i),
						DisplayName:         fmt.Sprintf("generated-task-0-%d", i),
						Version:             v,
						BuildId:             b1.Id,
						BuildVariant:        b1.DisplayName,
						Project:             project,
						GeneratedBy:         fmt.Sprintf("t-generator-%d-0", orderNumber),
						Status:              status,
						Requester:           evergreen.RepotrackerVersionRequester,
						Activated:           false,
						RevisionOrderNumber: orderNumber + 1,
					}
					require.NoError(t, generatedTask.Insert(t.Context()))
					data[generatedTask.Id] = generatedTask
				}

				// 3 Generated tasks for generator "t-generator-(ordernumber)-1" that are undispatched in version 1 and failed in version 2.
				for i := 0; i < 3; i++ {
					status := evergreen.TaskUndispatched
					if v == v2.Id {
						// Failed on v2.
						status = evergreen.TaskFailed
					}
					if v == v0.Id {
						// Everything passes on v0.
						status = evergreen.TaskSucceeded
					}
					generatedTask := &task.Task{
						Id:                  fmt.Sprintf("t-generated-%d-1-%d", orderNumber, i),
						DisplayName:         fmt.Sprintf("generated-task-1-%d", i),
						Version:             v,
						BuildId:             b1.Id,
						BuildVariant:        b1.DisplayName,
						Project:             project,
						GeneratedBy:         fmt.Sprintf("t-generator-%d-1", orderNumber),
						Status:              status,
						Requester:           evergreen.RepotrackerVersionRequester,
						Activated:           false,
						RevisionOrderNumber: orderNumber + 1,
					}
					require.NoError(t, generatedTask.Insert(t.Context()))
					data[generatedTask.Id] = generatedTask
				}

				// An additional 2 for generator "t-generator-(orderNumber)-1" that passed in both versions that should not be restarted.
				for i := 3; i < 5; i++ {
					generatedTask := &task.Task{
						Id:                  fmt.Sprintf("t-generated-%d-1-%d", orderNumber, i),
						DisplayName:         fmt.Sprintf("generated-task-1-%d", i),
						Version:             v,
						BuildId:             b1.Id,
						BuildVariant:        b1.DisplayName,
						Project:             project,
						GeneratedBy:         fmt.Sprintf("t-generator-%d-1", orderNumber),
						Status:              evergreen.TaskSucceeded,
						Requester:           evergreen.RepotrackerVersionRequester,
						Activated:           false,
						RevisionOrderNumber: orderNumber + 1,
					}
					require.NoError(t, generatedTask.Insert(t.Context()))
					data[generatedTask.Id] = generatedTask
				}
			}

			tCase(t, ctx, data)

			t.Run("SuccessfulTasksAreUnmodified", func(t *testing.T) {
				for orderNumber := 0; orderNumber < 3; orderNumber++ {
					// Background tasks.
					for i := 0; i < 3; i++ {
						dbTask, err := task.FindOne(ctx, db.Query(task.ById(fmt.Sprintf("t-success-%d-%d", orderNumber, i))))
						require.NoError(t, err)
						require.NotNil(t, dbTask)
						assert.False(t, dbTask.Activated)
					}

					// 1st Generator "t-generator-1-(orderNumber)" generated tasks that passed.
					for i := 3; i < 5; i++ {
						dbTask, err := task.FindOne(ctx, db.Query(task.ById(fmt.Sprintf("t-generated-%d-1-%d", orderNumber, i))))
						require.NoError(t, err)
						require.NotNil(t, dbTask)
						assert.False(t, dbTask.Activated)
					}
				}
			})
		})
	}
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
	require.NoError(projRef.Insert(t.Context()))
	v := &Version{
		Id:         "sample_version",
		Identifier: "sample",
		Requester:  evergreen.RepotrackerVersionRequester,
		Status:     evergreen.VersionStarted,
	}
	require.NoError(v.Insert(t.Context()))

	pp := ParserProject{
		Id:         "sample_version",
		Identifier: utility.ToStringPtr("sample"),
	}
	require.NoError(pp.Insert(t.Context()))
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
	assert.NoError(testTask.Insert(t.Context()))
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
	assert.NoError(anotherTask.Insert(t.Context()))
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
	assert.NoError(displayTask.Insert(t.Context()))
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
	assert.True(exeTask0.IsPartOfDisplay(ctx))
	assert.NoError(exeTask0.Insert(t.Context()))
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
	assert.True(exeTask1.IsPartOfDisplay(ctx))
	assert.NoError(exeTask1.Insert(t.Context()))

	b := &build.Build{
		Id:        buildID,
		Status:    evergreen.BuildStarted,
		Activated: true,
		Version:   v.Id,
	}
	require.NoError(b.Insert(t.Context()))
	assert.False(b.IsFinished())

	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   evergreen.CommandTypeSystem,
	}

	settings := testutil.TestConfig()

	assert.NoError(MarkEnd(ctx, settings, testTask, "", time.Now(), details))
	var err error
	v, err = VersionFindOneId(t.Context(), v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, v.Status)

	b, err = build.FindOneId(t.Context(), b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildStarted, b.Status)

	assert.NoError(MarkEnd(ctx, settings, anotherTask, "", time.Now(), details))
	v, err = VersionFindOneId(t.Context(), v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, v.Status)

	b, err = build.FindOneId(t.Context(), b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildStarted, b.Status)

	assert.NoError(MarkEnd(ctx, settings, exeTask0, "", time.Now(), details))
	v, err = VersionFindOneId(t.Context(), v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, v.Status)

	b, err = build.FindOneId(t.Context(), b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildStarted, b.Status)

	exeTask1.DisplayTask = nil
	assert.NoError(err)
	assert.NoError(MarkEnd(ctx, settings, exeTask1, "", time.Now(), details))
	v, err = VersionFindOneId(t.Context(), v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionFailed, v.Status)

	b, err = build.FindOneId(t.Context(), b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildFailed, b.Status)

	e, err := event.FindUnprocessedEvents(t.Context(), -1)
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
	require.NoError(v.Insert(t.Context()))

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
	assert.NoError(testTask.Insert(t.Context()))
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
	require.NoError(anotherTask.Insert(t.Context()))

	b := &build.Build{
		Id:        buildID,
		Status:    evergreen.BuildStarted,
		Activated: true,
		Version:   v.Id,
	}
	require.NoError(b.Insert(t.Context()))

	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   "test",
	}

	settings := testutil.TestConfig()

	assert.NoError(MarkEnd(ctx, settings, &testTask, "", time.Now(), details))
	var err error
	v, err = VersionFindOneId(t.Context(), v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionFailed, v.Status)

	b, err = build.FindOneId(t.Context(), b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildFailed, b.Status)

	e, err := event.FindUnprocessedEvents(t.Context(), -1)
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
	require.NoError(v.Insert(t.Context()))

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
	assert.NoError(testTask.Insert(t.Context()))
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
	require.NoError(anotherTask.Insert(t.Context()))

	b := &build.Build{
		Id:        buildID,
		Status:    evergreen.BuildStarted,
		Activated: true,
		Version:   v.Id,
	}
	require.NoError(b.Insert(t.Context()))

	details := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   "test",
	}
	settings := testutil.TestConfig()
	assert.NoError(MarkEnd(ctx, settings, &testTask, "", time.Now(), details))

	var err error
	v, err = VersionFindOneId(t.Context(), v.Id)
	assert.NoError(err)
	assert.Equal(evergreen.VersionFailed, v.Status)

	b, err = build.FindOneId(t.Context(), b.Id)
	assert.NoError(err)
	assert.Equal(evergreen.BuildFailed, b.Status)

	e, err := event.FindUnprocessedEvents(t.Context(), -1)
	assert.NoError(err)
	assert.Len(e, 4)
}

func TestClearAndResetStrandedHostTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection, VersionCollection))
	assert := assert.New(t)

	settings := testutil.TestConfig()

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
			Execution:     settings.TaskLimits.MaxTaskExecution,
		},
	}
	for _, tsk := range tasks {
		require.NoError(t, tsk.Insert(t.Context()))
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
	assert.NoError(b.Insert(t.Context()))
	v := Version{
		Id: b.Version,
	}
	assert.NoError(v.Insert(t.Context()))

	b2 := build.Build{
		Id:      "b2",
		Version: "version2",
	}
	assert.NoError(b2.Insert(t.Context()))
	v2 := Version{
		Id: b2.Version,
	}
	assert.NoError(v2.Insert(t.Context()))

	assert.NoError(ClearAndResetStrandedHostTask(ctx, settings, h))

	runningTask, err := task.FindOne(ctx, db.Query(task.ById("t")))
	require.NoError(t, err)
	assert.Equal(evergreen.TaskUndispatched, runningTask.Status)

	foundBuild, err := build.FindOneId(t.Context(), "b")
	require.NoError(t, err)
	assert.Equal(evergreen.BuildCreated, foundBuild.Status)

	foundVersion, err := VersionFindOneId(t.Context(), b.Version)
	require.NoError(t, err)
	assert.Equal(evergreen.VersionCreated, foundVersion.Status)

	h.RunningTask = "unschedulableTask"
	assert.NoError(ClearAndResetStrandedHostTask(ctx, settings, h))

	unschedulableTask, err := task.FindOne(ctx, db.Query(task.ById("unschedulableTask")))
	require.NoError(t, err)
	assert.Equal(evergreen.TaskFailed, unschedulableTask.Status)

	dependencyTask, err := task.FindOneId(ctx, "dependencyTask")
	require.NotNil(t, dependencyTask)
	require.NoError(t, err)
	assert.True(dependencyTask.DependsOn[0].Unattainable)
	assert.True(dependencyTask.DependsOn[0].Finished)

	dt, err := task.FindOne(ctx, db.Query(task.ById("displayTask")))
	require.NoError(t, err)
	assert.Equal(evergreen.TaskFailed, dt.Status)
	assert.Equal(dt.Details, task.GetSystemFailureDetails(evergreen.TaskDescriptionStranded))

	foundBuild, err = build.FindOneId(t.Context(), "b2")
	require.NoError(t, err)
	assert.Equal(evergreen.BuildFailed, foundBuild.Status)

	foundVersion, err = VersionFindOneId(t.Context(), b2.Version)
	require.NoError(t, err)
	assert.Equal(evergreen.VersionFailed, foundVersion.Status)

	h.RunningTask = "t2"
	assert.NoError(resetTask(ctx, "t2", ""))
	assert.NoError(ClearAndResetStrandedHostTask(ctx, settings, h))
	foundTask, err := task.FindOne(ctx, db.Query(task.ById("t2")))
	require.NoError(t, err)
	// The task should not have been reset twice.
	assert.Equal(1, foundTask.Execution)
}

func TestClearAndResetStaleStrandedHostTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(host.Collection, VersionCollection, patch.Collection, ParserProjectCollection, ProjectRefCollection, task.Collection, task.OldCollection, build.Collection))
	assert := assert.New(t)

	projectRef := ProjectRef{
		Identifier: "project-ref",
	}
	require.NoError(t, projectRef.Insert(t.Context()))
	version := Version{
		Id:         "version",
		Identifier: "mci",
	}
	require.NoError(t, version.Insert(t.Context()))
	build := build.Build{
		Id: version.Id,
	}
	require.NoError(t, build.Insert(t.Context()))
	parserProject := ParserProject{
		Id: version.Id,
	}
	require.NoError(t, parserProject.Insert(t.Context()))
	patch := patch.Patch{
		Id:      mgobson.NewObjectId(),
		Version: version.Id,
	}
	require.NoError(t, patch.Insert(t.Context()))
	host := &host.Host{
		Id:          "h1",
		RunningTask: "t",
	}
	assert.NoError(host.Insert(ctx))
	runningTask := &task.Task{
		Id:            "t",
		Status:        evergreen.TaskStarted,
		Activated:     true,
		ActivatedTime: utility.ZeroTime,
		Version:       version.Id,
		BuildId:       build.Id,
		Project:       projectRef.Identifier,
		HostId:        host.Id,
	}
	assert.NoError(runningTask.Insert(t.Context()))

	settings := testutil.TestConfig()
	assert.NoError(ClearAndResetStrandedHostTask(ctx, settings, host))
	runningTask, err := task.FindOne(ctx, db.Query(task.ById("t")))
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, runningTask.Status)
	assert.Equal("system", runningTask.Details.Type)
}

func TestClearAndResetStrandedHostTaskFailedOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(host.Collection, task.Collection, task.OldCollection, build.Collection, VersionCollection, event.EventCollection))

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
	assert.NoError(t, dispTask.Insert(t.Context()))
	assert.NoError(t, execTask1.Insert(t.Context()))
	assert.NoError(t, execTask2.Insert(t.Context()))

	h := &host.Host{
		Id:          "h1",
		RunningTask: "et1",
	}
	assert.NoError(t, h.Insert(ctx))

	b := build.Build{
		Id:      "b",
		Version: "version",
	}
	assert.NoError(t, b.Insert(t.Context()))
	v := Version{
		Id: "version",
	}
	assert.NoError(t, v.Insert(t.Context()))
	settings := testutil.TestConfig()
	assert.NoError(t, ClearAndResetStrandedHostTask(ctx, settings, h))

	checkTaskRestartEvent := func(t *testing.T, taskID string) {
		events, err := event.FindAllByResourceID(t.Context(), taskID)
		require.NoError(t, err)
		require.NotEmpty(t, events)
		var foundTaskRestartEvent bool
		for _, e := range events {
			if e.EventType == event.TaskRestarted {
				foundTaskRestartEvent = true
				break
			}
		}
		assert.True(t, foundTaskRestartEvent, "should have a task restart event for task '%s'", taskID)
	}

	restartedDisplayTask, err := task.FindOne(ctx, db.Query(task.ById("dt")))
	assert.NoError(t, err)
	require.NotZero(t, restartedDisplayTask)
	assert.Equal(t, evergreen.TaskUndispatched, restartedDisplayTask.Status)
	assert.Equal(t, 1, restartedDisplayTask.Execution)
	checkTaskRestartEvent(t, restartedDisplayTask.Id)

	restartedExecutionTask, err := task.FindOne(ctx, db.Query(task.ById("et1")))
	assert.NoError(t, err)
	require.NotZero(t, restartedExecutionTask)
	assert.Equal(t, 1, restartedExecutionTask.Execution)
	assert.Equal(t, 1, restartedExecutionTask.LatestParentExecution)
	assert.Equal(t, evergreen.TaskUndispatched, restartedExecutionTask.Status)
	restartedExecutionTaskEvents, err := event.FindAllByResourceID(t.Context(), restartedExecutionTask.Id)
	require.NoError(t, err)
	require.NotEmpty(t, restartedExecutionTaskEvents)
	checkTaskRestartEvent(t, restartedExecutionTask.Id)

	nonRestartedExecutionTask, err := task.FindOne(ctx, db.Query(task.ById("et2")))
	assert.NoError(t, err)
	require.NotZero(t, nonRestartedExecutionTask)
	assert.Equal(t, evergreen.TaskSucceeded, nonRestartedExecutionTask.Status)
	assert.Equal(t, 0, nonRestartedExecutionTask.Execution)
	assert.Equal(t, 1, restartedExecutionTask.LatestParentExecution)
	events, err := event.FindAllByResourceID(t.Context(), nonRestartedExecutionTask.Id)
	require.NoError(t, err)
	assert.Empty(t, events, "should not have any new events for a non-restarted task")

	oldRestartedExecutionTask, err := task.FindOneOld(ctx, task.ById(fmt.Sprintf("%v_%v", execTask1.Id, 0)))
	assert.NoError(t, err)
	assert.NotNil(t, oldRestartedExecutionTask)
	assert.Equal(t, evergreen.TaskFailed, oldRestartedExecutionTask.Status)
	assert.Equal(t, 0, oldRestartedExecutionTask.Execution)
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
	assert.NoError(t, dispTask.Insert(t.Context()))
	assert.NoError(t, execTask.Insert(t.Context()))

	h := &host.Host{
		Id:          "h1",
		RunningTask: "et",
	}
	assert.NoError(t, h.Insert(ctx))

	b := build.Build{
		Id:      "b",
		Version: "version",
	}
	assert.NoError(t, b.Insert(t.Context()))
	v := Version{
		Id: "version",
	}
	assert.NoError(t, v.Insert(t.Context()))

	settings := testutil.TestConfig()
	assert.NoError(t, ClearAndResetStrandedHostTask(ctx, settings, h))
	restartedDisplayTask, err := task.FindOne(ctx, db.Query(task.ById("dt")))
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskUndispatched, restartedDisplayTask.Status)
	restartedExecutionTask, err := task.FindOne(ctx, db.Query(task.ById("et")))
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskUndispatched, restartedExecutionTask.Status)
}

func TestResetStaleTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := testutil.TestConfig()
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection, VersionCollection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, tsk task.Task){
		"SuccessfullyRestartsStaleTask": func(t *testing.T, tsk task.Task) {
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbArchivedTask, err := task.FindOneOldByIdAndExecution(ctx, tsk.Id, 0)
			require.NoError(t, err)
			require.NotZero(t, dbArchivedTask, "should have archived the old task execution")
			assert.Equal(t, evergreen.TaskFailed, dbArchivedTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbArchivedTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionHeartbeat, dbArchivedTask.Details.Description)
			assert.True(t, dbArchivedTask.Details.TimedOut)
			assert.False(t, utility.IsZeroTime(dbArchivedTask.FinishTime))

			dbTask, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask, "should have created a new task execution")
			assert.Equal(t, evergreen.TaskUndispatched, dbTask.Status)
			assert.True(t, dbTask.Activated)

			dbBuild, err := build.FindOneId(t.Context(), tsk.BuildId)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.Equal(t, evergreen.BuildCreated, dbBuild.Status, "build status should be updated for restarted task")

			dbVersion, err := VersionFindOneId(t.Context(), tsk.Version)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.VersionCreated, dbVersion.Status, "version status should be updated for restarted task")

			dependencyTask, err := task.FindOneId(ctx, "dependencyTask")
			require.NotNil(t, dependencyTask)
			require.NoError(t, err)
			assert.False(t, dependencyTask.DependsOn[0].Unattainable)
			assert.False(t, dependencyTask.DependsOn[0].Finished)
		},
		"SuccessfullySystemFailsAbortedTask": func(t *testing.T, tsk task.Task) {
			tsk.Aborted = true
			require.NoError(t, tsk.Insert(t.Context()))
			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbArchivedTask, err := task.FindOneOldByIdAndExecution(ctx, tsk.Id, 0)
			require.NoError(t, err)
			require.Zero(t, dbArchivedTask, "should not have archived the aborted task")

			dbTask, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionAborted, dbTask.Details.Description)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))

			dependencyTask, err := task.FindOneId(ctx, "dependencyTask")
			require.NotNil(t, dependencyTask)
			require.NoError(t, err)
			assert.True(t, dependencyTask.DependsOn[0].Unattainable)
			assert.True(t, dependencyTask.DependsOn[0].Finished)
		},
		"ResetsParentDisplayTaskForStaleExecutionTask": func(t *testing.T, tsk task.Task) {
			otherExecTask := task.Task{
				Id:        "execution_task_id",
				Status:    evergreen.TaskStarted,
				Activated: true,
			}
			require.NoError(t, otherExecTask.Insert(t.Context()))
			dt := task.Task{
				Id:             "display_task_id",
				DisplayOnly:    true,
				ExecutionTasks: []string{tsk.Id, otherExecTask.Id},
				Status:         evergreen.TaskStarted,
				BuildId:        tsk.BuildId,
				Version:        tsk.Version,
			}
			require.NoError(t, dt.Insert(t.Context()))
			tsk.DisplayTaskId = utility.ToStringPtr(dt.Id)
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbDisplayTask, err := task.FindOneId(ctx, dt.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask)
			assert.True(t, dbDisplayTask.ResetFailedWhenFinished, "display task should reset failed when other exec task finishes running")

			dbArchivedTask, err := task.FindOneOldByIdAndExecution(ctx, tsk.Id, 0)
			assert.NoError(t, err)
			assert.Zero(t, dbArchivedTask, "execution task should not be archived until display task can reset")

			dbTask, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, 0, dbTask.Execution, "current task execution should still be the stranded one")
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionHeartbeat, dbTask.Details.Description)
			assert.True(t, dbTask.Details.TimedOut)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))

			dbOtherExecTask, err := task.FindOneId(ctx, otherExecTask.Id)
			require.NoError(t, err)
			require.NotZero(t, dbOtherExecTask)
			assert.Equal(t, evergreen.TaskStarted, dbOtherExecTask.Status, "other execution task should still be running")
		},
		"FailsStaleTaskThatHitsUnschedulableThresholdWithoutRestartingIt": func(t *testing.T, tsk task.Task) {
			tsk.ActivatedTime = time.Now().Add(-10 * task.UnschedulableThreshold)
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbTask, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			assert.Equal(t, 0, dbTask.Execution, "current task execution should still be the stranded one")
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionHeartbeat, dbTask.Details.Description)
			assert.True(t, dbTask.Details.TimedOut)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))
		},
		"FailsStaleTaskThatHitsMaxExecutionRestartsWithoutRestartingIt": func(t *testing.T, tsk task.Task) {
			execNum := settings.TaskLimits.MaxTaskExecution + 1
			tsk.Execution = execNum
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, FixStaleTask(ctx, settings, &tsk))

			dbTask, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			assert.Equal(t, execNum, dbTask.Execution, "current task execution should still be the stranded one")
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.Equal(t, evergreen.CommandTypeSystem, dbTask.Details.Type)
			assert.Equal(t, evergreen.TaskDescriptionHeartbeat, dbTask.Details.Description)
			assert.True(t, dbTask.Details.TimedOut)
			assert.False(t, utility.IsZeroTime(dbTask.FinishTime))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, VersionCollection, patch.Collection, ParserProjectCollection, ProjectRefCollection, task.Collection, task.OldCollection, build.Collection))
			projectRef := ProjectRef{
				Identifier: "project-ref",
			}
			require.NoError(t, projectRef.Insert(t.Context()))
			version := Version{
				Id:         "version",
				Identifier: "mci",
			}
			require.NoError(t, version.Insert(t.Context()))
			build := build.Build{
				Id:      version.Id,
				Version: version.Id,
			}
			require.NoError(t, build.Insert(t.Context()))
			parserProject := ParserProject{
				Id: version.Id,
			}
			require.NoError(t, parserProject.Insert(t.Context()))
			patch := patch.Patch{
				Id:      mgobson.NewObjectId(),
				Version: version.Id,
			}
			require.NoError(t, patch.Insert(t.Context()))
			host := &host.Host{
				Id:          "h1",
				RunningTask: "t",
			}
			require.NoError(t, host.Insert(ctx))
			tsk := task.Task{
				Id:                "task_id",
				Execution:         0,
				ExecutionPlatform: task.ExecutionPlatformHost,
				Status:            evergreen.TaskStarted,
				Activated:         true,
				ActivatedTime:     time.Now(),
				LastHeartbeat:     time.Now().Add(-30 * time.Hour),
				BuildId:           build.Id,
				Version:           version.Id,
				Project:           projectRef.Identifier,
				HostId:            host.Id,
			}
			depTask := task.Task{
				Id:            "dependencyTask",
				Status:        evergreen.TaskUndispatched,
				Activated:     true,
				ActivatedTime: time.Now(),
				BuildId:       build.Id,
				Version:       version.Id,
				Project:       projectRef.Identifier,
				DependsOn: []task.Dependency{
					{
						TaskId: "task_id",
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			require.NoError(t, depTask.Insert(t.Context()))
			tCase(t, tsk)
		})
	}
}

func TestDisplayTaskUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	assert.NoError(dt.Insert(t.Context()))
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
	assert.NoError(dt2.Insert(t.Context()))
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
	assert.NoError(dt3.Insert(t.Context()))
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
	assert.NoError(blockedDt.Insert(t.Context()))
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
	assert.NoError(task1.Insert(t.Context()))
	task2 := task.Task{
		Id:         "task2",
		Status:     evergreen.TaskSucceeded,
		TimeTaken:  2 * time.Minute,
		StartTime:  time.Date(2000, 0, 0, 0, 30, 0, 0, time.Local), // this should end up as the start time for dt1
		FinishTime: time.Date(2000, 0, 0, 1, 0, 5, 0, time.Local),
	}
	assert.NoError(task2.Insert(t.Context()))
	task3 := task.Task{
		Id:         "task3",
		Activated:  true,
		Status:     evergreen.TaskSystemUnresponse,
		TimeTaken:  5 * time.Minute,
		StartTime:  time.Date(2000, 0, 0, 0, 44, 0, 0, time.Local),
		FinishTime: time.Date(2000, 0, 0, 1, 0, 1, 0, time.Local),
	}
	assert.NoError(task3.Insert(t.Context()))
	task4 := task.Task{
		Id:         "task4",
		Activated:  true,
		Status:     evergreen.TaskSystemUnresponse,
		TimeTaken:  1 * time.Minute,
		StartTime:  time.Date(2000, 0, 0, 1, 0, 20, 0, time.Local),
		FinishTime: time.Date(2000, 0, 0, 1, 22, 0, 0, time.Local), // this should end up as the end time for dt1
	}
	assert.NoError(task4.Insert(t.Context()))
	task5 := task.Task{
		Id:        "task5",
		Activated: true,
		Status:    evergreen.TaskDispatched,
	}
	assert.NoError(task5.Insert(t.Context()))
	task6 := task.Task{
		Id:        "task6",
		Activated: true,
		Status:    evergreen.TaskSucceeded,
	}
	assert.NoError(task6.Insert(t.Context()))
	task7 := task.Task{
		Id:        "task7",
		Activated: true,
		Status:    evergreen.TaskSucceeded,
	}
	assert.NoError(task7.Insert(t.Context()))
	task8 := task.Task{
		Id:        "task8",
		Activated: true,
		Status:    evergreen.TaskUndispatched,
		DependsOn: []task.Dependency{{TaskId: "task9", Unattainable: true}},
	}
	assert.NoError(task8.Insert(t.Context()))
	task9 := task.Task{
		Id:        "task9",
		Activated: true,
		Status:    evergreen.TaskFailed,
	}
	assert.NoError(task9.Insert(t.Context()))
	task10 := task.Task{
		Id:        "task10",
		Activated: true,
		Status:    evergreen.TaskUndispatched,
	}
	assert.NoError(task10.Insert(t.Context()))
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
	assert.NoError(task11.Insert(t.Context()))
	assert.NoError(task12.Insert(t.Context()))

	// test that updating the status + activated from execution tasks works
	assert.NoError(UpdateDisplayTaskForTask(ctx, &task1))
	dbTask, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.True(dbTask.Details.TimedOut)
	assert.True(dbTask.Activated)
	assert.Equal(11*time.Minute, dbTask.TimeTaken)
	assert.Equal(task2.StartTime, dbTask.StartTime)
	assert.Equal(task4.FinishTime, dbTask.FinishTime)

	// test that you can't update a display task
	assert.Error(UpdateDisplayTaskForTask(ctx, &dt))

	// test that a display task with a finished + unfinished task is "started"
	assert.NoError(UpdateDisplayTaskForTask(ctx, &task5))
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(dt2.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)
	assert.Zero(dbTask.FinishTime)

	// check that the updates above logged an event for the first one
	events, err := event.Find(t.Context(), event.TaskEventsForId(dt.Id))
	assert.NoError(err)
	assert.Len(events, 1)
	events, err = event.Find(t.Context(), event.TaskEventsForId(dt2.Id))
	assert.NoError(err)
	assert.Empty(events)

	// a blocked execution task + unblocked unfinshed tasks should still be "started"
	assert.NoError(UpdateDisplayTaskForTask(ctx, &task7))
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(blockedDt.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	// a blocked execution task should not contribute to the status
	assert.NoError(task10.MarkFailed(ctx))
	assert.NoError(UpdateDisplayTaskForTask(ctx, &task8))
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(blockedDt.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.Equal(evergreen.TaskFailed, dbTask.DisplayStatusCache)

	// a display task should not set its start time to any exec tasks that have zero start time
	assert.NoError(UpdateDisplayTaskForTask(ctx, &task11))
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(dt3.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(task12.StartTime, dbTask.StartTime)
}

func TestDisplayTaskUpdateNoUndispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	assert.NoError(dt.Insert(t.Context()))
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
	assert.NoError(task1.Insert(t.Context()))
	task2 := task.Task{
		Id:        "task2",
		Status:    evergreen.TaskStarted,
		StartTime: time.Date(2000, 0, 0, 0, 30, 0, 0, time.Local), // this should end up as the start time for dt1
	}
	assert.NoError(task2.Insert(t.Context()))

	// test that updating the status + activated from execution tasks shows started
	assert.NoError(UpdateDisplayTaskForTask(ctx, &task1))
	dbTask, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	events, err := event.Find(t.Context(), event.TaskEventsForId(dt.Id))
	assert.NoError(err)
	assert.Empty(events)
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
	assert.NoError(dt.Insert(t.Context()))
	task1 := task.Task{
		Id:      "task1",
		BuildId: "b",
		Version: "version",
		Status:  evergreen.TaskSucceeded,
	}
	assert.NoError(task1.Insert(t.Context()))
	task2 := task.Task{
		Id:      "task2",
		BuildId: "b",
		Version: "version",
		Status:  evergreen.TaskSucceeded,
	}
	assert.NoError(task2.Insert(t.Context()))
	b := build.Build{
		Id:      "b",
		Version: "version",
	}
	assert.NoError(b.Insert(t.Context()))
	v := Version{
		Id: "version",
	}
	assert.NoError(v.Insert(t.Context()))

	settings := testutil.TestConfig()

	// request that the task restarts when it's done
	assert.NoError(dt.SetResetWhenFinished(ctx, "caller"))
	dbTask, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.True(dbTask.ResetWhenFinished)
	assert.Equal(evergreen.TaskStarted, dbTask.Status)

	// end the final task so that it restarts
	assert.NoError(checkResetDisplayTask(ctx, settings, "", "", &dt))
	dbTask, err = task.FindOne(ctx, db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	dbTask2, err := task.FindOne(ctx, db.Query(task.ById(task2.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskUndispatched, dbTask2.Status)

	oldTask, err := task.FindOneOld(ctx, task.ById("dt_0"))
	assert.NoError(err)
	assert.NotNil(oldTask)
}

func TestDisplayTaskUpdatesAreConcurrencySafe(t *testing.T) {
	// This test is intentionally testing concurrent/conflicting updates to the
	// same display task. If UpdateDisplayTaskForTask is working properly, this
	// test should never be flaky. If it is, that's a sign that it's not
	// concurrency safe.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, event.EventCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, event.EventCollection))
	}()

	const displayTaskID = "display_task_id"
	et0 := task.Task{
		Id:            "execution_task0",
		DisplayTaskId: utility.ToStringPtr(displayTaskID),
		Activated:     true,
		ActivatedTime: time.Now(),
		Status:        evergreen.TaskSucceeded,
		StartTime:     time.Now().Add(-time.Hour),
		FinishTime:    time.Now(),
	}
	et1 := task.Task{
		Id:            "execution_task1",
		DisplayTaskId: utility.ToStringPtr(displayTaskID),
		Activated:     true,
		ActivatedTime: time.Now(),
		StartTime:     time.Now().Add(-time.Hour),
		Status:        evergreen.TaskStarted,
	}
	dt := task.Task{
		Id:             displayTaskID,
		DisplayOnly:    true,
		ExecutionTasks: []string{et0.Id, et1.Id},
		Status:         evergreen.TaskUndispatched,
		Activated:      false,
	}
	require.NoError(t, et0.Insert(t.Context()))
	require.NoError(t, et1.Insert(t.Context()))
	require.NoError(t, dt.Insert(t.Context()))

	const numConcurrentUpdates = 3
	errs := make(chan error, 1+numConcurrentUpdates)
	var updatesDone sync.WaitGroup
	for i := 0; i < numConcurrentUpdates; i++ {
		updatesDone.Add(1)
		go func() {
			defer updatesDone.Done()
			// This goroutine will potentially see one execution task is not
			// finished, so it may try either update the display task status to
			// starting or success.
			// The task has to be copied into the goroutine to avoid concurrent
			// modifications of the in-memory display task.
			et0Copy := et0
			errs <- UpdateDisplayTaskForTask(ctx, &et0Copy)
		}()
	}

	updatesDone.Add(1)
	go func() {
		defer updatesDone.Done()

		// Simulate a condition where some goroutines see the execution task as
		// still running, while others see it as succeeded.
		if err := et1.MarkEnd(ctx, time.Now(), &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}); err != nil {
			errs <- err
			return
		}

		if err := et1.MarkStart(ctx, time.Now()); err != nil {
			errs <- err
			return
		}

		if err := et1.MarkEnd(ctx, time.Now(), &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}); err != nil {
			errs <- err
			return
		}

		// The last goroutine initially sees that all execution tasks are
		// finished, so it should try to update the final status to success.
		// The task has to be copied into the goroutine to avoid concurrent
		// modifications of the in-memory display task.
		et0Copy := et0
		errs <- UpdateDisplayTaskForTask(ctx, &et0Copy)
	}()

	updatesDone.Wait()
	close(errs)

	for err := range errs {
		assert.NoError(t, err)
	}

	// The final display task status must be success after all the concurrent
	// updates are done because all the execution tasks finished with success.
	dbDisplayTask, err := task.FindOneId(ctx, dt.Id)
	require.NoError(t, err)
	require.NotZero(t, dbDisplayTask)

	assert.Equal(t, evergreen.TaskSucceeded, dbDisplayTask.Status, "final display task status must be success after all concurrent updates finish")

	latestEvents, err := event.Find(t.Context(), event.MostRecentTaskEvents(dt.Id, 1))
	require.NoError(t, err)
	require.Len(t, latestEvents, 1)
	assert.Equal(t, event.TaskFinished, latestEvents[0].EventType, "should have logged event for display task finished")
}

func TestAbortedTaskDelayedRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(ParserProjectCollection, task.Collection, task.OldCollection, host.Collection, build.Collection, VersionCollection))
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
	assert.NoError(t, task1.Insert(t.Context()))
	taskHost := host.Host{
		Id: "hostId",
	}
	assert.NoError(t, taskHost.Insert(ctx))
	b := build.Build{
		Id:      "b",
		Version: "version",
	}
	assert.NoError(t, b.Insert(t.Context()))
	v := Version{
		Id: "version",
	}
	assert.NoError(t, v.Insert(t.Context()))
	pp := ParserProject{
		Id: v.Id,
	}
	require.NoError(t, pp.Insert(t.Context()))
	detail := &apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
	}
	settings := testutil.TestConfig()
	assert.NoError(t, MarkEnd(ctx, settings, &task1, "test", time.Now(), detail))
	newTask, err := task.FindOneId(ctx, task1.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.TaskUndispatched, newTask.Status)
	assert.Equal(t, 1, newTask.Execution)
	oldTask, err := task.FindOneOld(ctx, task.ById("task1_0"))
	assert.NoError(t, err)
	assert.True(t, oldTask.Aborted)
}

func TestDisplayTaskFailedExecTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))
	dt := task.Task{
		Id:             "task",
		DisplayOnly:    true,
		Status:         evergreen.TaskUndispatched,
		ExecutionTasks: []string{"exec0", "exec1"},
	}
	assert.NoError(dt.Insert(t.Context()))
	execTask0 := task.Task{
		Id:        "exec0",
		Activated: true,
		Status:    evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Status: evergreen.TaskFailed,
			Type:   evergreen.CommandTypeSystem,
		}}
	assert.NoError(execTask0.Insert(t.Context()))

	execTask1 := task.Task{Id: "exec1", Status: evergreen.TaskUndispatched}
	assert.NoError(execTask1.Insert(t.Context()))

	assert.NoError(UpdateDisplayTaskForTask(ctx, &execTask0))
	dbTask, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.Equal(evergreen.CommandTypeSystem, dbTask.Details.Type)
	assert.True(dbTask.Activated)
}

func TestDisplayTaskFailedAndSucceededExecTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))
	dt := task.Task{
		Id:             "task",
		DisplayOnly:    true,
		Status:         evergreen.TaskUndispatched,
		ExecutionTasks: []string{"exec0", "exec1"},
	}
	assert.NoError(dt.Insert(t.Context()))
	execTask0 := task.Task{
		Id:        "exec0",
		Activated: true,
		Status:    evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Status: evergreen.TaskFailed,
			Type:   evergreen.CommandTypeSetup,
		},
	}
	assert.NoError(execTask0.Insert(t.Context()))

	execTask1 := task.Task{Id: "exec1", Activated: true, Status: evergreen.TaskSucceeded}
	assert.NoError(execTask1.Insert(t.Context()))

	assert.NoError(UpdateDisplayTaskForTask(ctx, &execTask0))
	dbTask, err := task.FindOne(ctx, db.Query(task.ById(dt.Id)))
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, dbTask.Status)
	assert.Equal(evergreen.CommandTypeSetup, dbTask.Details.Type)
	assert.True(dbTask.Activated)
}

func TestMarkEndDeactivatesPrevious(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, ProjectRefCollection, distro.Collection, build.Collection, VersionCollection, ParserProjectCollection, host.Collection))

	taskHost1 := &host.Host{
		Id:          "myHost1",
		RunningTask: "t2",
	}
	require.NoError(t, taskHost1.Insert(ctx))
	taskHost2 := &host.Host{
		Id:          "myHost2",
		RunningTask: "t3",
	}
	require.NoError(t, taskHost2.Insert(ctx))
	proj := ProjectRef{
		Id:                 "proj",
		DeactivatePrevious: utility.TruePtr(),
	}
	require.NoError(t, proj.Insert(t.Context()))
	d := distro.Distro{
		Id: "distro",
	}
	require.NoError(t, d.Insert(ctx))
	v := Version{
		Id:        "sample_version",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	require.NoError(t, v.Insert(t.Context()))
	pp := ParserProject{
		Id: v.Id,
	}
	require.NoError(t, pp.Insert(t.Context()))
	stepbackTask := task.Task{
		Id:                  "t2",
		BuildId:             "b2",
		Status:              evergreen.TaskUndispatched,
		BuildVariant:        "bv",
		DisplayName:         "task",
		Project:             "proj",
		HostId:              "myHost1",
		Activated:           true,
		RevisionOrderNumber: 2,
		DispatchTime:        utility.ZeroTime,
		Requester:           evergreen.RepotrackerVersionRequester,
		Version:             v.Id,
	}
	assert.NoError(stepbackTask.Insert(t.Context()))
	b2 := build.Build{
		Id:           "b2",
		BuildVariant: "bv",
	}
	assert.NoError(b2.Insert(t.Context()))
	finishedTask := &task.Task{
		Id:                  "t3",
		BuildId:             "b3",
		Status:              evergreen.TaskUndispatched,
		BuildVariant:        "bv",
		DisplayName:         "task",
		Project:             "proj",
		HostId:              "myHost2",
		Activated:           true,
		RevisionOrderNumber: 3,
		Requester:           evergreen.TriggerRequester,
		Version:             v.Id,
	}
	require.NoError(t, finishedTask.Insert(t.Context()))
	b3 := build.Build{
		Id:           "b3",
		BuildVariant: "bv",
	}
	assert.NoError(b3.Insert(t.Context()))

	// Should not unschedule previous tasks if the requester is not repotracker.

	settings := testutil.TestConfig()
	detail := &apimodels.TaskEndDetail{
		Status: evergreen.TaskSucceeded,
	}
	assert.NoError(MarkEnd(ctx, settings, finishedTask, "test", time.Now().Add(time.Minute), detail))
	checkTask, err := task.FindOneId(ctx, stepbackTask.Id)
	assert.NoError(err)
	assert.True(checkTask.Activated)

	require.NoError(t, task.UpdateOne(ctx, bson.M{"_id": finishedTask.Id},
		bson.M{"$set": bson.M{"status": evergreen.TaskUndispatched}}))
	finishedTask.Requester = evergreen.RepotrackerVersionRequester
	finishedTask.Status = evergreen.TaskUndispatched
	assert.NoError(MarkEnd(ctx, settings, finishedTask, "test", time.Now().Add(time.Minute), detail))
	checkTask, err = task.FindOneId(ctx, stepbackTask.Id)
	assert.NoError(err)
	assert.False(checkTask.Activated)
}

func TestGetDeactivatePrevious(t *testing.T) {
	config := `
tasks:
 - name: my_task
buildvariants:
 - name: deactivate_nil
 - name: deactivate_true
   deactivate_previous: true 
 - name: deactivate_false
   deactivate_previous: false
`
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(config), &pp)
	assert.NoError(t, err)

	project, err := TranslateProject(pp)
	assert.NoError(t, err)
	for tName, testCase := range map[string]func(t *testing.T){
		"TrueVariantDeactivatesPrevious": func(t *testing.T) {
			deactivateTask := &task.Task{
				Id:           "t1",
				DisplayName:  "my_task",
				BuildVariant: "deactivate_true",
			}
			pRef := &ProjectRef{
				Id:                 "proj",
				DeactivatePrevious: nil,
			}
			assert.True(t, getDeactivatePrevious(deactivateTask, pRef, project))
		},
		"TrueVariantOverriddenByFalseProject": func(t *testing.T) {
			deactivateTask := &task.Task{
				Id:           "t1",
				DisplayName:  "my_task",
				BuildVariant: "deactivate_true",
			}
			pRef := &ProjectRef{
				Id:                 "proj",
				DeactivatePrevious: utility.FalsePtr(),
			}
			assert.False(t, getDeactivatePrevious(deactivateTask, pRef, project))
		},
		"FalseVariantOverridesTrueProject": func(t *testing.T) {
			deactivateTask := &task.Task{
				Id:           "t1",
				DisplayName:  "my_task",
				BuildVariant: "deactivate_false",
			}
			pRef := &ProjectRef{
				Id:                 "proj",
				DeactivatePrevious: utility.TruePtr(),
			}
			assert.False(t, getDeactivatePrevious(deactivateTask, pRef, project))
		},
		"NilVariantDefaultsToProject": func(t *testing.T) {
			deactivateTask := &task.Task{
				Id:           "t1",
				DisplayName:  "my_task",
				BuildVariant: "deactivate_nil",
			}
			pRef := &ProjectRef{
				Id:                 "proj",
				DeactivatePrevious: utility.TruePtr(),
			}
			assert.True(t, getDeactivatePrevious(deactivateTask, pRef, project))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			testCase(t)
		})
	}

}

func TestEvalBisectStepback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert := assert.New(t)
	require := require.New(t)

	// Task data for tests is:
	// ('-' is failed, '?' is undispatched, '+' is succeeded).
	// t1  t2  t3  t4  t5  t6  t7  t8  t9  t10
	// -   ?   ?   ?   ?   ?   ?   ?   ?   +
	for tName, tCase := range map[string]func(t *testing.T, t10 task.Task, pRef *ProjectRef, project *Project){
		"NoPreviousSuccessfulTask": func(t *testing.T, t10 task.Task, pRef *ProjectRef, project *Project) {
			// Set the first task to failed status.
			require.NoError(task.UpdateOne(ctx, bson.M{"_id": "t1"},
				bson.M{"$set": bson.M{"status": evergreen.TaskFailed}}))
			require.NoError(evalStepback(ctx, &t10, evergreen.TaskFailed, pRef, project))
			midTask, err := task.ByBeforeMidwayTaskFromIds(ctx, "t10", "t1")
			require.NoError(err)
			assert.False(midTask.Activated)
		},
		"MissingTask": func(t *testing.T, t10 task.Task, pRef *ProjectRef, project *Project) {
			// If t5 is missing, t4 should be used.
			require.NoError(task.Remove(ctx, "t5"))
			require.NoError(evalStepback(ctx, &t10, evergreen.TaskFailed, pRef, project))
			midTask, err := task.ByBeforeMidwayTaskFromIds(ctx, "t10", "t1")
			require.NoError(err)
			assert.True(midTask.Activated)
			assert.Equal("t4", midTask.Id)
		},
		"ManyMissingTasks": func(t *testing.T, t10 task.Task, pRef *ProjectRef, project *Project) {
			// If t5, t4, t3 are missing, t2 should be used.
			require.NoError(task.Remove(ctx, "t5"))
			require.NoError(task.Remove(ctx, "t4"))
			require.NoError(task.Remove(ctx, "t3"))
			require.NoError(evalStepback(ctx, &t10, evergreen.TaskFailed, pRef, project))
			midTask, err := task.ByBeforeMidwayTaskFromIds(ctx, "t10", "t1")
			require.NoError(err)
			assert.True(midTask.Activated)
			assert.Equal("t2", midTask.Id)
		},
		"AllMissingTasks": func(t *testing.T, t10 task.Task, pRef *ProjectRef, project *Project) {
			// If t5, t4, t3, t2 are missing then stepback has
			// no task to step back to and it should
			// no-op (and not re-activate t1).
			require.NoError(task.Remove(ctx, "t5"))
			require.NoError(task.Remove(ctx, "t4"))
			require.NoError(task.Remove(ctx, "t3"))
			require.NoError(task.Remove(ctx, "t2"))
			require.NoError(evalStepback(ctx, &t10, evergreen.TaskFailed, pRef, project))
			midTask, err := task.ByBeforeMidwayTaskFromIds(ctx, "t10", "t1")
			require.NoError(err)
			assert.False(midTask.Activated)
			assert.Equal("t1", midTask.Id)
		},
		"FailedTaskInStepback": func(t *testing.T, t10 task.Task, pRef *ProjectRef, project *Project) {
			require.NoError(evalStepback(ctx, &t10, evergreen.TaskFailed, pRef, project))
			midTask, err := task.ByBeforeMidwayTaskFromIds(ctx, "t10", "t1")
			prevTask := *midTask
			require.NoError(err)
			assert.True(midTask.Activated)
			// Check mid task stepback info.
			require.NotNil(midTask.StepbackInfo)
			assert.Equal("t10", midTask.StepbackInfo.LastFailingStepbackTaskId)
			assert.Equal("t1", midTask.StepbackInfo.LastPassingStepbackTaskId)
			assert.Empty(midTask.StepbackInfo.NextStepbackTaskId)
			assert.Equal("t10", midTask.StepbackInfo.PreviousStepbackTaskId)
			// Check last failing stepback info.
			lastFailing, err := task.FindOneId(ctx, midTask.StepbackInfo.LastFailingStepbackTaskId)
			require.NoError(err)
			require.NotNil(lastFailing.StepbackInfo)
			assert.Empty(lastFailing.StepbackInfo.LastFailingStepbackTaskId)
			assert.Empty(lastFailing.StepbackInfo.LastPassingStepbackTaskId)
			assert.Equal(midTask.Id, lastFailing.StepbackInfo.NextStepbackTaskId)
			assert.Empty(lastFailing.StepbackInfo.PreviousStepbackTaskId)
			// Check last passing stepback info. It should be blank as the chain of stepbacks
			// only relates to the first failing, not the first passing.
			lastPassing, err := task.FindOneId(ctx, midTask.StepbackInfo.LastPassingStepbackTaskId)
			require.NoError(err)
			require.Nil(lastPassing.StepbackInfo)

			// 2nd Iteration. Task failed, moving last failing stepback to midtask.
			prevTask.Status = evergreen.TaskFailed
			require.NoError(task.UpdateOne(ctx, bson.M{"_id": midTask.Id},
				bson.M{"$set": bson.M{"status": evergreen.TaskFailed}}))
			// Activate next stepback
			require.NoError(evalStepback(ctx, &prevTask, evergreen.TaskFailed, pRef, project))
			midTask, err = task.ByBeforeMidwayTaskFromIds(ctx, prevTask.Id, "t1")
			require.NoError(err)
			assert.True(midTask.Activated)
			// Check mid task stepback info.
			require.NotNil(midTask.StepbackInfo)
			assert.Equal(prevTask.Id, midTask.StepbackInfo.LastFailingStepbackTaskId)
			assert.Equal("t1", midTask.StepbackInfo.LastPassingStepbackTaskId)
			assert.Empty(midTask.StepbackInfo.NextStepbackTaskId)
			assert.Equal(prevTask.Id, midTask.StepbackInfo.PreviousStepbackTaskId)
			// Check last failing stepback info.
			lastFailing, err = task.FindOneId(ctx, midTask.StepbackInfo.LastFailingStepbackTaskId)
			require.NoError(err)
			require.NotNil(lastFailing.StepbackInfo)
			assert.Equal("t10", lastFailing.StepbackInfo.LastFailingStepbackTaskId)
			assert.Equal("t1", lastFailing.StepbackInfo.LastPassingStepbackTaskId)
			assert.Equal(midTask.Id, lastFailing.StepbackInfo.NextStepbackTaskId)
			assert.Equal("t10", lastFailing.StepbackInfo.PreviousStepbackTaskId)
			// Check last passing stepback info.
			lastPassing, err = task.FindOneId(ctx, midTask.StepbackInfo.LastPassingStepbackTaskId)
			require.NoError(err)
			require.Nil(lastPassing.StepbackInfo)
		},
		"PassedTaskInStepback": func(t *testing.T, t10 task.Task, pRef *ProjectRef, project *Project) {
			require.NoError(evalStepback(ctx, &t10, evergreen.TaskFailed, pRef, project))
			midTask, err := task.ByBeforeMidwayTaskFromIds(ctx, "t10", "t1")
			require.NoError(err)
			assert.True(midTask.Activated)
			// Check mid task stepback info.
			require.NotNil(midTask.StepbackInfo)
			assert.Equal("t10", midTask.StepbackInfo.LastFailingStepbackTaskId)
			assert.Equal("t1", midTask.StepbackInfo.LastPassingStepbackTaskId)
			assert.Empty(midTask.StepbackInfo.NextStepbackTaskId)
			assert.Equal("t10", midTask.StepbackInfo.PreviousStepbackTaskId)
			// Check last failing stepback info.
			lastFailing, err := task.FindOneId(ctx, midTask.StepbackInfo.LastFailingStepbackTaskId)
			require.NoError(err)
			require.NotNil(lastFailing.StepbackInfo)
			assert.Empty(lastFailing.StepbackInfo.LastFailingStepbackTaskId)
			assert.Empty(lastFailing.StepbackInfo.LastPassingStepbackTaskId)
			assert.Equal(midTask.Id, lastFailing.StepbackInfo.NextStepbackTaskId)
			assert.Empty(lastFailing.StepbackInfo.PreviousStepbackTaskId)
			// Check last passing stepback info. It should be blank as the chain of stepbacks
			// only relates to the first failing, not the first passing.
			lastPassing, err := task.FindOneId(ctx, midTask.StepbackInfo.LastPassingStepbackTaskId)
			require.NoError(err)
			require.Nil(lastPassing.StepbackInfo)

			// 2nd Iteration. Task passed, moving last passing stepback to midtask.
			midTask.Status = evergreen.TaskSucceeded
			prevTask := *midTask
			require.NoError(task.UpdateOne(ctx, bson.M{"_id": midTask.Id},
				bson.M{"$set": bson.M{"status": evergreen.TaskSucceeded}}))
			// Activate next stepback
			require.NoError(evalStepback(ctx, midTask, evergreen.TaskSucceeded, pRef, project))
			midTask, err = task.ByBeforeMidwayTaskFromIds(ctx, "t10", prevTask.Id)
			require.NoError(err)
			assert.True(midTask.Activated)
			// Check mid task stepback info.
			require.NotNil(midTask.StepbackInfo)
			assert.Equal("t10", midTask.StepbackInfo.LastFailingStepbackTaskId)
			assert.Equal(prevTask.Id, midTask.StepbackInfo.LastPassingStepbackTaskId)
			assert.Empty(midTask.StepbackInfo.NextStepbackTaskId)
			assert.Equal(prevTask.Id, midTask.StepbackInfo.PreviousStepbackTaskId)
			// Check last failing stepback info.
			lastFailing, err = task.FindOneId(ctx, midTask.StepbackInfo.LastFailingStepbackTaskId)
			require.NoError(err)
			require.NotNil(lastFailing.StepbackInfo)
			assert.Empty(lastFailing.StepbackInfo.LastFailingStepbackTaskId)
			assert.Empty(lastFailing.StepbackInfo.LastPassingStepbackTaskId)
			assert.Equal(prevTask.Id, lastFailing.StepbackInfo.NextStepbackTaskId)
			assert.Empty(lastFailing.StepbackInfo.PreviousStepbackTaskId)
			// Check last passing stepback info.
			lastPassing, err = task.FindOneId(ctx, midTask.StepbackInfo.LastPassingStepbackTaskId)
			require.NoError(err)
			require.NotNil(lastPassing.StepbackInfo)
			assert.Equal("t10", lastPassing.StepbackInfo.LastFailingStepbackTaskId)
			assert.Equal("t1", lastPassing.StepbackInfo.LastPassingStepbackTaskId)
			assert.Equal(midTask.Id, lastPassing.StepbackInfo.NextStepbackTaskId)
			assert.Equal("t10", lastPassing.StepbackInfo.PreviousStepbackTaskId)
		},
		"GeneratedTasksStepbackGenerator": func(t *testing.T, t10 task.Task, pRef *ProjectRef, project *Project) {
			// Make all generator tasks pass.
			for i := 1; i <= 10; i++ {
				require.NoError(task.UpdateOne(ctx, bson.M{"_id": fmt.Sprintf("t%d", i)},
					bson.M{"$set": bson.M{"status": evergreen.TaskSucceeded}}))
			}
			generated1Tasks := []task.Task{}
			generated2Tasks := []task.Task{}
			for i := 1; i <= 10; i++ {
				generated1 := task.Task{
					Id:                  fmt.Sprintf("g1-%d", i),
					BuildId:             fmt.Sprintf("b%d", i),
					GeneratedBy:         fmt.Sprintf("t%d", i),
					Status:              evergreen.TaskUndispatched,
					BuildVariant:        "bv",
					DisplayName:         "generated1",
					Project:             "proj",
					Activated:           false,
					RevisionOrderNumber: i,
					Requester:           evergreen.RepotrackerVersionRequester,
					Version:             fmt.Sprintf("v%d", i),
				}
				assert.NoError(generated1.Insert(t.Context()))
				generated1Tasks = append(generated1Tasks, generated1)

				generated2 := task.Task{
					Id:                  fmt.Sprintf("g2-%d", i),
					BuildId:             fmt.Sprintf("b%d", i),
					GeneratedBy:         fmt.Sprintf("t%d", i),
					Status:              evergreen.TaskUndispatched,
					BuildVariant:        "bv",
					DisplayName:         "generated2",
					Project:             "proj",
					Activated:           false,
					RevisionOrderNumber: i,
					Requester:           evergreen.RepotrackerVersionRequester,
					Version:             fmt.Sprintf("v%d", i),
				}
				assert.NoError(generated2.Insert(t.Context()))
				generated2Tasks = append(generated2Tasks, generated2)
			}
			// Make the first generated tasks fail and the last pass.
			generated1Tasks[0].Status = evergreen.TaskSucceeded
			require.NoError(task.UpdateOne(ctx, bson.M{"_id": generated1Tasks[0].Id},
				bson.M{"$set": bson.M{"status": generated1Tasks[0].Status}}))
			generated2Tasks[0].Status = evergreen.TaskSucceeded
			require.NoError(task.UpdateOne(ctx, bson.M{"_id": generated2Tasks[0].Id},
				bson.M{"$set": bson.M{"status": generated2Tasks[0].Status}}))
			generated1Tasks[9].Status = evergreen.TaskFailed
			require.NoError(task.UpdateOne(ctx, bson.M{"_id": generated1Tasks[9].Id},
				bson.M{"$set": bson.M{"status": generated1Tasks[9].Status}}))
			generated2Tasks[9].Status = evergreen.TaskFailed
			require.NoError(task.UpdateOne(ctx, bson.M{"_id": generated2Tasks[9].Id},
				bson.M{"$set": bson.M{"status": generated2Tasks[9].Status}}))
			require.NoError(evalStepback(ctx, &generated1Tasks[9], evergreen.TaskFailed, pRef, project))
			require.NoError(evalStepback(ctx, &generated2Tasks[9], evergreen.TaskFailed, pRef, project))
			midTask, err := task.ByBeforeMidwayTaskFromIds(ctx, "t10", "t1")
			require.NoError(err)
			assert.True(midTask.Activated)
			require.NotNil(midTask.StepbackInfo)
			// The task itself should have no stepback info, only for its generated tasks.
			require.Empty(midTask.StepbackInfo.LastFailingStepbackTaskId)
			require.Empty(midTask.StepbackInfo.LastPassingStepbackTaskId)
			require.Empty(midTask.StepbackInfo.NextStepbackTaskId)
			require.Empty(midTask.StepbackInfo.PreviousStepbackTaskId)
			// For generated task 1.
			g1Info := midTask.StepbackInfo.GetStepbackInfoForGeneratedTask("generated1", "bv")
			require.NotNil(g1Info)
			assert.Equal("t10", g1Info.LastFailingStepbackTaskId)
			assert.Equal("t1", g1Info.LastPassingStepbackTaskId)
			assert.Equal("t5", g1Info.NextStepbackTaskId)
			assert.Equal("t10", g1Info.PreviousStepbackTaskId)
			// For generated task 2.
			g2Info := midTask.StepbackInfo.GetStepbackInfoForGeneratedTask("generated2", "bv")
			require.NotNil(g2Info)
			assert.Equal("t10", g2Info.LastFailingStepbackTaskId)
			assert.Equal("t1", g2Info.LastPassingStepbackTaskId)
			assert.Equal("t5", g2Info.NextStepbackTaskId)
			assert.Equal("t10", g2Info.PreviousStepbackTaskId)
			// For last failing.
			lastFailing, err := task.FindOneId(ctx, "t10")
			require.NoError(err)
			require.NotNil(lastFailing.StepbackInfo)
			// For generator's generated task info 1.
			g1Info = lastFailing.StepbackInfo.GetStepbackInfoForGeneratedTask("generated1", "bv")
			require.NotNil(g1Info)
			assert.Equal("t10", g1Info.LastFailingStepbackTaskId)
			assert.Equal("t1", g1Info.LastPassingStepbackTaskId)
			assert.Equal(midTask.Id, g1Info.NextStepbackTaskId)
			assert.Equal("t10", g1Info.PreviousStepbackTaskId)
			assert.Equal(generated1Tasks[0].DisplayName, g1Info.DisplayName)
			assert.Equal(generated1Tasks[0].BuildVariant, g1Info.BuildVariant)
			// For generator's generated task info 2.
			g2Info = lastFailing.StepbackInfo.GetStepbackInfoForGeneratedTask("generated2", "bv")
			require.NotNil(g2Info)
			assert.Equal("t10", g2Info.LastFailingStepbackTaskId)
			assert.Equal("t1", g2Info.LastPassingStepbackTaskId)
			assert.Equal(midTask.Id, g2Info.NextStepbackTaskId)
			assert.Equal("t10", g2Info.PreviousStepbackTaskId)
			assert.Equal(generated2Tasks[0].DisplayName, g2Info.DisplayName)
			assert.Equal(generated2Tasks[0].BuildVariant, g2Info.BuildVariant)
			// Last passing should be empty.
			lastPassing, err := task.FindOneId(ctx, "t1")
			require.NoError(err)
			require.Nil(lastPassing.StepbackInfo)

			// 2nd Iteration. Generated Task 1 passed, moving last passing stepback to midtask.
			// Generated Task 2 failed, moving last failing to midtask.
			midTaskG1, err := task.FindOneId(ctx, "g1-5")
			require.NoError(err)
			midTaskG1.Status = evergreen.TaskSucceeded
			require.NoError(task.UpdateOne(ctx, bson.M{"_id": midTaskG1.Id},
				bson.M{"$set": bson.M{"status": midTaskG1.Status}}))
			midTaskG2, err := task.FindOneId(ctx, "g2-5")
			require.NoError(err)
			midTaskG2.Status = evergreen.TaskFailed
			require.NoError(task.UpdateOne(ctx, bson.M{"_id": midTaskG2.Id},
				bson.M{"$set": bson.M{"status": midTaskG2.Status}}))

			prevTask := *midTask
			// Activate g1 next stepback
			require.NoError(evalStepback(ctx, midTaskG1, evergreen.TaskSucceeded, pRef, project))
			require.NoError(evalStepback(ctx, midTaskG2, evergreen.TaskFailed, pRef, project))

			// Check mid task stepback info relating to generated task 1.
			midTask, err = task.ByBeforeMidwayTaskFromIds(ctx, "t10", prevTask.Id)
			require.NoError(err)
			assert.True(midTask.Activated)
			require.NotNil(midTask.StepbackInfo)
			// The mid task is the generator task and should have no stepback info.
			// (It should only be in the generated task's stepback info.)
			require.Empty(midTask.StepbackInfo.LastFailingStepbackTaskId)
			require.Empty(midTask.StepbackInfo.LastPassingStepbackTaskId)
			require.Empty(midTask.StepbackInfo.NextStepbackTaskId)
			require.Empty(midTask.StepbackInfo.PreviousStepbackTaskId)
			// For generated task 1.
			g1Info = midTask.StepbackInfo.GetStepbackInfoForGeneratedTask("generated1", "bv")
			require.NotNil(g1Info)
			assert.Equal("t10", g1Info.LastFailingStepbackTaskId)
			assert.Equal(prevTask.Id, g1Info.LastPassingStepbackTaskId)
			assert.Equal("t7", g1Info.NextStepbackTaskId)
			assert.Equal(prevTask.Id, g1Info.PreviousStepbackTaskId)
			// For generated task 2 (this mid task should not have data on it).
			g2Info = midTask.StepbackInfo.GetStepbackInfoForGeneratedTask("generated2", "bv")
			require.Nil(g2Info)

			// Check mid task stepback info relating to generated task 2.
			midTask, err = task.ByBeforeMidwayTaskFromIds(ctx, prevTask.Id, "t1")
			require.NoError(err)
			assert.True(midTask.Activated)
			require.NotNil(midTask.StepbackInfo)
			// The mid task is the generator task and should have no stepback info.
			// (It should only be in the generated task's stepback info.)
			require.Empty(midTask.StepbackInfo.LastFailingStepbackTaskId)
			require.Empty(midTask.StepbackInfo.LastPassingStepbackTaskId)
			require.Empty(midTask.StepbackInfo.NextStepbackTaskId)
			require.Empty(midTask.StepbackInfo.PreviousStepbackTaskId)
			// For generated task 1 (this mid task should not have data on it).
			g1Info = midTask.StepbackInfo.GetStepbackInfoForGeneratedTask("generated1", "bv")
			require.Nil(g1Info)
			// For generated task 2.
			g2Info = midTask.StepbackInfo.GetStepbackInfoForGeneratedTask("generated2", "bv")
			require.NotNil(g2Info)
			assert.Equal(prevTask.Id, g2Info.LastFailingStepbackTaskId)
			assert.Equal("t1", g2Info.LastPassingStepbackTaskId)
			assert.Equal("t3", g2Info.NextStepbackTaskId)
			assert.Equal(prevTask.Id, g2Info.PreviousStepbackTaskId)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(db.ClearCollections(task.Collection, ProjectRefCollection, ParserProjectCollection, distro.Collection, build.Collection, VersionCollection))
			pRef := &ProjectRef{
				Id:             "proj",
				StepbackBisect: utility.TruePtr(),
			}
			require.NoError(pRef.Insert(t.Context()))
			d := distro.Distro{
				Id: "distro",
			}
			require.NoError(d.Insert(ctx))
			// Tasks 2-9
			for i := 2; i <= 9; i++ {
				v := Version{
					Id:        fmt.Sprintf("v%d", i),
					Requester: evergreen.RepotrackerVersionRequester,
				}
				require.NoError(v.Insert(t.Context()))
				pp := &ParserProject{
					Id:       v.Id,
					Stepback: utility.TruePtr(),
				}
				require.NoError(pp.Insert(t.Context()))
				t1 := task.Task{
					Id:                  fmt.Sprintf("t%d", i),
					BuildId:             fmt.Sprintf("b%d", i),
					Status:              evergreen.TaskUndispatched,
					BuildVariant:        "bv",
					DisplayName:         "task",
					Project:             "proj",
					Activated:           false,
					RevisionOrderNumber: i,
					Requester:           evergreen.RepotrackerVersionRequester,
					Version:             v.Id,
				}
				assert.NoError(t1.Insert(t.Context()))
				b := build.Build{
					Id:           fmt.Sprintf("b%d", i),
					BuildVariant: "bv",
				}
				assert.NoError(b.Insert(t.Context()))
			}
			v := Version{
				Id:        "v1",
				Requester: evergreen.RepotrackerVersionRequester,
			}
			require.NoError(v.Insert(t.Context()))
			pp := &ParserProject{
				Id:       v.Id,
				Stepback: utility.TruePtr(),
			}
			assert.NoError(pp.Insert(t.Context()))
			project, err := TranslateProject(pp)
			assert.NoError(err)
			// First task (which has passed).
			t1 := task.Task{
				Id:                  "t1",
				BuildId:             "b1",
				Status:              evergreen.TaskSucceeded,
				BuildVariant:        "bv",
				DisplayName:         "task",
				Project:             "proj",
				Activated:           false,
				RevisionOrderNumber: 1,
				Requester:           evergreen.RepotrackerVersionRequester,
				Version:             v.Id,
			}
			assert.NoError(t1.Insert(t.Context()))
			b1 := build.Build{
				Id:           "b1",
				BuildVariant: "bv",
			}
			assert.NoError(b1.Insert(t.Context()))
			// Latest task (which has failed).
			v = Version{
				Id:        "v10",
				Requester: evergreen.RepotrackerVersionRequester,
			}
			require.NoError(v.Insert(t.Context()))
			pp = &ParserProject{
				Id:       v.Id,
				Stepback: utility.TruePtr(),
			}
			assert.NoError(pp.Insert(t.Context()))
			t10 := task.Task{
				Id:                  "t10",
				BuildId:             "b10",
				Status:              evergreen.TaskFailed,
				BuildVariant:        "bv",
				DisplayName:         "task",
				Project:             "proj",
				Activated:           false,
				RevisionOrderNumber: 10,
				Requester:           evergreen.RepotrackerVersionRequester,
				Version:             v.Id,
			}
			assert.NoError(t10.Insert(t.Context()))
			b10 := build.Build{
				Id:           "b10",
				BuildVariant: "bv",
			}
			assert.NoError(b10.Insert(t.Context()))
			tCase(t, t10, pRef, project)
		})
	}
}

func TestEvalLinearStepback(t *testing.T) {
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
	pRef := &ProjectRef{
		Id: "proj",
	}
	require.NoError(t, pRef.Insert(t.Context()))
	d := distro.Distro{
		Id: "distro",
	}
	require.NoError(t, d.Insert(ctx))
	v := Version{
		Id:        "sample_version",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	require.NoError(t, v.Insert(t.Context()))
	pp := &ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(yml), &pp)
	assert.NoError(err)
	//pp.Id = v.Id
	//assert.NoError(pp.Insert(t.Context()))
	project, err := TranslateProject(pp)
	assert.NoError(err)
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
	assert.NoError(stepbackTask.Insert(t.Context()))
	b2 := build.Build{
		Id:           "b2",
		BuildVariant: "bv",
	}
	assert.NoError(b2.Insert(t.Context()))
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
	assert.NoError(finishedTask.Insert(t.Context()))
	b3 := build.Build{
		Id:           "b3",
		BuildVariant: "bv",
	}
	assert.NoError(b3.Insert(t.Context()))

	// should not step back if there was never a successful task
	assert.NoError(evalStepback(ctx, &finishedTask, evergreen.TaskFailed, pRef, project))
	checkTask, err := task.FindOneId(ctx, stepbackTask.Id)
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
	assert.NoError(prevComplete.Insert(t.Context()))
	b1 := build.Build{
		Id:           "b1",
		BuildVariant: "bv",
	}
	assert.NoError(b1.Insert(t.Context()))
	assert.NoError(evalStepback(ctx, &finishedTask, evergreen.TaskFailed, pRef, project))
	checkTask, err = task.FindOneId(ctx, stepbackTask.Id)
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
	assert.NoError(prevComplete.Insert(t.Context()))
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
	assert.NoError(stepbackTask.Insert(t.Context()))
	b4 := build.Build{
		Id:           "b4",
		BuildVariant: "bv",
	}
	assert.NoError(b4.Insert(t.Context()))
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
	assert.NoError(generator.Insert(t.Context()))
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
	assert.NoError(generated.Insert(t.Context()))
	b5 := build.Build{
		Id:           "b5",
		BuildVariant: "bv",
	}
	assert.NoError(b5.Insert(t.Context()))
	// Ensure system failure doesn't cause a stepback unless we're already stepping back.
	assert.NoError(evalStepback(ctx, &generated, evergreen.TaskSystemFailed, pRef, project))
	checkTask, err = task.FindOneId(ctx, stepbackTask.Id)
	assert.NoError(err)
	assert.False(checkTask.Activated)

	// System failure steps back since activated by stepback (and steps back generator).
	generated.ActivatedBy = evergreen.StepbackTaskActivator
	assert.NoError(evalStepback(ctx, &generated, evergreen.TaskSystemFailed, pRef, project))
	checkTask, err = task.FindOneId(ctx, stepbackTask.Id)
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
	require.NoError(t, db.InsertMany(t.Context(), VersionCollection, v1, v2, v3))
	pp := ParserProject{
		Stepback: utility.TruePtr(),
	}
	project, err := TranslateProject(&pp)
	require.NoError(t, err)

	pRef := &ProjectRef{
		Id: "p1",
	}
	require.NoError(t, pRef.Insert(t.Context()))
	b1 := build.Build{
		Id: "prev_b1",
	}
	b2 := build.Build{
		Id: "prev_b2",
	}
	require.NoError(t, db.InsertMany(t.Context(), build.Collection, b1, b2))
	t1 := task.Task{
		Id:                  "t1",
		Project:             pRef.Id,
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
		Project:             pRef.Id,
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
		Project:             pRef.Id,
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
		Project:             pRef.Id,
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
		Project:             pRef.Id,
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
		Project:             pRef.Id,
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
		Project:             pRef.Id,
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
		Project:             pRef.Id,
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
		Project:             pRef.Id,
		TaskGroup:           "my_group",
		BuildVariant:        "bv",
		DisplayName:         "third",
		Requester:           evergreen.RepotrackerVersionRequester,
		Status:              evergreen.TaskSucceeded,
		Activated:           true,
		RevisionOrderNumber: 1,
	}
	assert.NoError(t, db.InsertMany(t.Context(), task.Collection, t1, t2, t3, prevT1, prevT2, prevT3, prevSuccessT1, prevSuccessT2, prevSuccessT3))
	assert.NoError(t, evalStepback(ctx, &t2, evergreen.TaskFailed, pRef, project))

	// verify only the previous t1 and t2 are stepped back
	prevT1FromDb, err := task.FindOneId(ctx, prevT1.Id)
	assert.NoError(t, err)
	assert.True(t, prevT1FromDb.Activated)
	prevT2FromDb, err := task.FindOneId(ctx, prevT2.Id)
	assert.NoError(t, err)
	assert.True(t, prevT2FromDb.Activated)
	prevT3FromDb, err := task.FindOneId(ctx, prevT3.Id)
	assert.NoError(t, err)
	assert.False(t, prevT3FromDb.Activated)

	// stepping back t3 should now also stepback t3 and not error on earlier activated tasks
	assert.NoError(t, evalStepback(ctx, &t3, evergreen.TaskFailed, pRef, project))
	prevT3FromDb, err = task.FindOneId(ctx, prevT3.Id)
	assert.NoError(t, err)
	assert.True(t, prevT3FromDb.Activated)
}

func TestUpdateBlockedDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(VersionCollection, task.Collection, build.Collection, event.EventCollection))

	v := Version{Id: "version0"}
	require.NoError(v.Insert(t.Context()))
	b0 := build.Build{
		Id:      "build0",
		Version: v.Id,
		Status:  evergreen.BuildStarted,
	}
	require.NoError(b0.Insert(t.Context()))
	b1 := build.Build{
		Id:      "build1",
		Version: v.Id,
		Status:  evergreen.BuildCreated,
	}
	require.NoError(b1.Insert(t.Context()))
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
	for _, task := range tasks {
		assert.NoError(task.Insert(t.Context()))
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
	assert.NoError(execTask.Insert(t.Context()))

	assert.NoError(UpdateBlockedDependencies(ctx, []task.Task{tasks[0]}, false))

	dbTask1, err := task.FindOneId(ctx, tasks[1].Id)
	assert.NoError(err)
	assert.Len(dbTask1.DependsOn, 2)
	assert.True(dbTask1.DependsOn[0].Unattainable)
	assert.True(dbTask1.DependsOn[1].Unattainable) // this task has duplicates which are also marked

	dbTask2, err := task.FindOneId(ctx, tasks[2].Id)
	assert.NoError(err)
	assert.True(dbTask2.DependsOn[0].Unattainable)

	dbTask3, err := task.FindOneId(ctx, tasks[3].Id)
	assert.NoError(err)
	assert.True(dbTask3.DependsOn[0].Unattainable)

	// We don't traverse past t3 which was already unattainable == true
	dbTask4, err := task.FindOneId(ctx, tasks[4].Id)
	assert.NoError(err)
	assert.False(dbTask4.DependsOn[0].Unattainable)

	// update more than one dependency (t1 and t5)
	dbTask5, err := task.FindOneId(ctx, tasks[5].Id)
	assert.NoError(err)
	assert.True(dbTask5.DependsOn[0].Unattainable)

	dbExecTask, err := task.FindOneId(ctx, execTask.Id)
	assert.NoError(err)
	assert.True(dbExecTask.DependsOn[0].Unattainable)

	dbBuild0, err := build.FindOneId(t.Context(), b0.Id)
	require.NoError(err)
	require.NotZero(dbBuild0)
	assert.Equal(evergreen.BuildFailed, dbBuild0.Status, "build status with failed and blocked tasks should be updated")

	dbBuild1, err := build.FindOneId(t.Context(), b1.Id)
	require.NoError(err)
	require.NotZero(dbBuild1)
	assert.Equal(evergreen.BuildCreated, dbBuild1.Status, "build status should not need to be updated")

	dbVersion, err := VersionFindOneId(t.Context(), v.Id)
	require.NoError(err)
	require.NotZero(dbVersion)
	assert.Equal(evergreen.VersionFailed, dbVersion.Status, "version status with all finished or blocked tasks should be updated")

	// one event inserted for every updated task, one for the updated build, and
	// one for the updated version.
	events, err := event.Find(t.Context(), db.Q{})
	assert.NoError(err)
	assert.Len(events, 6)
}

func TestUpdateUnblockedDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	for _, task := range tasks {
		assert.NoError(task.Insert(t.Context()))
	}

	assert.NoError(v.Insert(t.Context()))
	assert.NoError(b.Insert(t.Context()))
	assert.NoError(b2.Insert(t.Context()))

	assert.NoError(UpdateUnblockedDependencies(ctx, []task.Task{tasks[0]}))

	// this task should still be marked blocked because t1 is unattainable
	dbTask2, err := task.FindOneId(ctx, tasks[2].Id)
	assert.NoError(err)
	assert.False(dbTask2.DependsOn[0].Unattainable)
	assert.True(dbTask2.DependsOn[1].Unattainable)

	dbTask3, err := task.FindOneId(ctx, tasks[3].Id)
	assert.NoError(err)
	assert.False(dbTask3.DependsOn[0].Unattainable)

	dbTask4, err := task.FindOneId(ctx, tasks[4].Id)
	assert.NoError(err)
	assert.False(dbTask4.DependsOn[0].Unattainable)

	// We don't traverse past the t4 which was already unattainable == false
	dbTask5, err := task.FindOneId(ctx, tasks[5].Id)
	assert.NoError(err)
	assert.True(dbTask5.DependsOn[0].Unattainable)

	// Unblocking a dependent task should unblock its build
	dbTask6, err := task.FindOneId(ctx, tasks[6].Id)
	assert.NoError(err)
	assert.False(dbTask6.DependsOn[0].Unattainable)
	dbBuild2, err := build.FindOneId(t.Context(), b2.Id)
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
	s.NoError(taskToAbort.Insert(s.T().Context()))
	s.NoError((&build.Build{Id: "b1"}).Insert(s.T().Context()))
	s.NoError((&Version{Id: "v1"}).Insert(s.T().Context()))
	u := user.DBUser{
		Id: "user1",
	}
	s.NoError(u.Insert(s.T().Context()))
	err := AbortTask(ctx, "task1", "user1")
	s.NoError(err)
	foundTask, err := task.FindOneId(ctx, "task1")
	s.NoError(err)
	s.Equal("user1", foundTask.AbortInfo.User)
	s.True(foundTask.Aborted)
}

func (s *TaskConnectorAbortTaskSuite) TestAbortFail() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(db.ClearCollections(task.Collection, user.Collection))
	taskToAbort := task.Task{Id: "task1", Status: evergreen.TaskStarted}
	s.NoError(taskToAbort.Insert(s.T().Context()))
	u := user.DBUser{
		Id: "user1",
	}
	s.NoError(u.Insert(s.T().Context()))
	err := AbortTask(ctx, "task1", "user1")
	s.Error(err)
}

func TestHandleEndTaskForGithubMergeQueueTask(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, VersionCollection))
	defer func() {
		require.NoError(t, db.ClearCollections(task.Collection, VersionCollection))
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	v1 := Version{
		Id:        "version1",
		Requester: evergreen.GithubMergeRequester,
	}
	require.NoError(t, v1.Insert(t.Context()))
	t1 := &task.Task{
		Id:        "task1",
		Version:   "version1",
		Requester: evergreen.GithubMergeRequester,
		Status:    evergreen.TaskSucceeded,
	}
	require.NoError(t, t1.Insert(t.Context()))
	t2 := &task.Task{
		Id:        "task2",
		Version:   "version1",
		Requester: evergreen.GithubMergeRequester,
		Status:    evergreen.TaskStarted,
		Aborted:   true,
	}
	require.NoError(t, t2.Insert(t.Context()))
	t3 := &task.Task{
		Id:        "task3",
		Version:   "version1",
		Requester: evergreen.GithubMergeRequester,
		Status:    evergreen.TaskStarted,
	}
	require.NoError(t, t3.Insert(t.Context()))
	t4 := &task.Task{
		Id:        "task4",
		Version:   "version1",
		Requester: evergreen.GithubMergeRequester,
		Status:    evergreen.TaskStarted,
	}
	require.NoError(t, t4.Insert(t.Context()))
	t5 := &task.Task{
		Id:        "task5",
		Version:   "version1",
		Requester: evergreen.GithubMergeRequester,
		Status:    evergreen.TaskStarted,
	}
	require.NoError(t, t5.Insert(t.Context()))

	// Neither of these should abort any tasks.
	assert.NoError(t, HandleEndTaskForGithubMergeQueueTask(ctx, t1, evergreen.TaskSucceeded))
	assert.NoError(t, HandleEndTaskForGithubMergeQueueTask(ctx, t2, evergreen.TaskFailed))
	tasks, err := task.Find(ctx, task.ByVersion("version1"))
	assert.NoError(t, err)
	for _, task := range tasks {
		// only t2 should be aborted, since it already was
		if task.Id == "task2" {
			assert.True(t, task.Aborted, task.Id)
		} else {
			assert.False(t, task.Aborted, task.Id)
		}
	}

	// This should abort all tasks.
	assert.NoError(t, HandleEndTaskForGithubMergeQueueTask(ctx, t3, evergreen.TaskFailed))
	tasks, err = task.Find(ctx, task.ByVersion("version1"))
	assert.NoError(t, err)
	for _, task := range tasks {
		// all but t1, which already succeeded, and t3, the caller, should be aborted
		if task.Id == "task1" || task.Id == "task3" {
			assert.False(t, task.Aborted, task.Id)
		} else {
			assert.True(t, task.Aborted, task.Id)
		}
	}
}
