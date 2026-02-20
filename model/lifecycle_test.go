package model

import (
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func taskIdInSlice(tasks []task.Task, id string) bool {
	for _, task := range tasks {
		if task.Id == id {
			return true
		}
	}
	return false
}

func TestTaskSetPriority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a task", t, func() {

		require.NoError(t, db.ClearCollections(task.Collection, build.Collection, VersionCollection))

		v := &Version{Id: "abcdef"}
		require.NoError(t, v.Insert(t.Context()))

		tasks := []task.Task{
			{
				Id:             "one",
				DependsOn:      []task.Dependency{{TaskId: "two", Status: ""}, {TaskId: "three", Status: ""}, {TaskId: "four", Status: ""}},
				Activated:      true,
				BuildId:        "b0",
				Version:        v.Id,
				DisplayOnly:    true,
				ExecutionTasks: []string{"six"},
			},
			{
				Id:        "two",
				Priority:  5,
				Activated: true,
				BuildId:   "b0",
				Version:   v.Id,
			},
			{
				Id:        "three",
				DependsOn: []task.Dependency{{TaskId: "five", Status: ""}},
				Activated: true,
				BuildId:   "b0",
				Version:   v.Id,
			},
			{
				Id:        "four",
				DependsOn: []task.Dependency{{TaskId: "five", Status: ""}},
				Activated: true,
				BuildId:   "b0",
				Version:   v.Id,
			},
			{
				Id:        "five",
				Activated: true,
				BuildId:   "b0",
				Version:   v.Id,
			},
			{
				Id:        "six",
				Activated: true,
				BuildId:   "b0",
				Version:   v.Id,
			},
		}

		for _, task := range tasks {
			So(task.Insert(t.Context()), ShouldBeNil)
		}

		b0 := build.Build{Id: "b0"}
		require.NoError(t, b0.Insert(t.Context()))

		Convey("setting its priority should update it and all dependencies in the database", func() {

			So(SetTaskPriority(ctx, tasks[0], 1, "user"), ShouldBeNil)

			t, err := task.FindOne(ctx, db.Query(task.ById("one")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 1)

			t, err = task.FindOne(ctx, db.Query(task.ById("two")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 5)

			t, err = task.FindOne(ctx, db.Query(task.ById("three")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 1)

			t, err = task.FindOne(ctx, db.Query(task.ById("four")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "four")
			So(t.Priority, ShouldEqual, 1)

			t, err = task.FindOne(ctx, db.Query(task.ById("five")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "five")
			So(t.Priority, ShouldEqual, 1)

			t, err = task.FindOne(ctx, db.Query(task.ById("six")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "six")
			So(t.Priority, ShouldEqual, 1)

		})

		Convey("decreasing priority should update the task and its execution tasks but not its dependencies", func() {
			So(SetTaskPriority(ctx, tasks[0], 1, "user"), ShouldBeNil)
			So(tasks[0].Activated, ShouldEqual, true)
			So(SetTaskPriority(ctx, tasks[0], -1, "user"), ShouldBeNil)

			t, err := task.FindOne(ctx, db.Query(task.ById("one")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, -1)
			So(t.Activated, ShouldEqual, false)

			t, err = task.FindOne(ctx, db.Query(task.ById("two")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 5)
			So(t.Activated, ShouldEqual, true)

			t, err = task.FindOne(ctx, db.Query(task.ById("three")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 1)
			So(t.Activated, ShouldEqual, true)

			t, err = task.FindOne(ctx, db.Query(task.ById("four")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "four")
			So(t.Priority, ShouldEqual, 1)
			So(t.Activated, ShouldEqual, true)

			t, err = task.FindOne(ctx, db.Query(task.ById("five")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "five")
			So(t.Priority, ShouldEqual, 1)
			So(t.Activated, ShouldEqual, true)

			t, err = task.FindOne(ctx, db.Query(task.ById("six")))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "six")
			So(t.Priority, ShouldEqual, -1)
			So(t.Activated, ShouldEqual, false)
		})
	})
}

func TestBuildSetPriority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a build", t, func() {

		require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

		b := &build.Build{Id: "build"}
		So(b.Insert(t.Context()), ShouldBeNil)

		v := &Version{Id: "abcdef"}
		require.NoError(t, v.Insert(t.Context()))

		taskOne := &task.Task{Id: "taskOne", BuildId: b.Id, Version: v.Id}
		So(taskOne.Insert(t.Context()), ShouldBeNil)

		taskTwo := &task.Task{Id: "taskTwo", BuildId: b.Id, Version: v.Id}
		So(taskTwo.Insert(t.Context()), ShouldBeNil)

		taskThree := &task.Task{Id: "taskThree", BuildId: b.Id, Version: v.Id}
		So(taskThree.Insert(t.Context()), ShouldBeNil)

		Convey("setting its priority should update the priority"+
			" of all its tasks in the database", func() {

			So(SetBuildPriority(ctx, b.Id, 42, ""), ShouldBeNil)

			tasks, err := task.Find(ctx, task.ByBuildId(b.Id))
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 3)
			So(tasks[0].Priority, ShouldEqual, 42)
			So(tasks[1].Priority, ShouldEqual, 42)
			So(tasks[2].Priority, ShouldEqual, 42)
		})

	})

}

func TestBuildRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, VersionCollection, build.Collection))
	}()

	// Running a multi-document transaction requires the collections to exist
	// first before any documents can be inserted.
	require.NoError(t, db.CreateCollections(task.Collection, task.OldCollection, VersionCollection, build.Collection))
	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, build.Collection))
	v := &Version{Id: "version"}
	require.NoError(t, v.Insert(t.Context()))
	b := &build.Build{Id: "build", Version: "version"}
	require.NoError(t, b.Insert(t.Context()))
	Convey("Restarting a build", t, func() {
		Convey("with task abort should update the status of"+
			" non in-progress tasks and abort in-progress ones and mark them to be reset", func() {

			taskOne := &task.Task{
				Id:            "task1",
				DisplayName:   "task1",
				BuildId:       b.Id,
				DisplayTaskId: utility.ToStringPtr(""),
				Status:        evergreen.TaskSucceeded,
				Activated:     true,
			}
			So(taskOne.Insert(t.Context()), ShouldBeNil)

			taskTwo := &task.Task{
				Id:            "task2",
				DisplayName:   "task2",
				BuildId:       b.Id,
				DisplayTaskId: utility.ToStringPtr(""),
				Status:        evergreen.TaskDispatched,
				Activated:     true,
			}
			So(taskTwo.Insert(t.Context()), ShouldBeNil)

			So(RestartBuild(ctx, b, []string{"task1", "task2"}, true, ""), ShouldBeNil)
			var err error
			b, err = build.FindOne(ctx, build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			So(b.Activated, ShouldEqual, true)
			taskOne, err = task.FindOne(ctx, db.Query(task.ById("task1")))
			So(err, ShouldBeNil)
			So(taskOne.Status, ShouldEqual, evergreen.TaskUndispatched)
			So(taskOne.Activated, ShouldEqual, true)
			So(taskOne.DisplayStatusCache, ShouldEqual, evergreen.TaskWillRun)
			taskTwo, err = task.FindOne(ctx, db.Query(task.ById("task2")))
			So(err, ShouldBeNil)
			So(taskTwo.Aborted, ShouldEqual, true)
			So(taskTwo.DisplayStatusCache, ShouldEqual, evergreen.TaskAborted)
			So(taskTwo.ResetWhenFinished, ShouldBeTrue)
		})

		Convey("without task abort should update the status"+
			" of only those build tasks not in-progress", func() {
			taskThree := &task.Task{
				Id:                 "task3",
				DisplayName:        "task3",
				BuildId:            b.Id,
				DisplayTaskId:      utility.ToStringPtr(""),
				Status:             evergreen.TaskSucceeded,
				DisplayStatusCache: evergreen.TaskSucceeded,
				Activated:          true,
			}
			So(taskThree.Insert(t.Context()), ShouldBeNil)

			taskFour := &task.Task{
				Id:                 "task4",
				DisplayName:        "task4",
				BuildId:            b.Id,
				DisplayTaskId:      utility.ToStringPtr(""),
				Status:             evergreen.TaskDispatched,
				DisplayStatusCache: evergreen.TaskDispatched,
				Activated:          true,
			}
			So(taskFour.Insert(t.Context()), ShouldBeNil)

			So(RestartBuild(ctx, b, []string{"task3", "task4"}, false, ""), ShouldBeNil)
			var err error
			b, err = build.FindOne(ctx, build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			taskThree, err = task.FindOne(ctx, db.Query(task.ById("task3")))
			So(err, ShouldBeNil)
			So(taskThree.Status, ShouldEqual, evergreen.TaskUndispatched)
			So(taskThree.DisplayStatusCache, ShouldEqual, evergreen.TaskWillRun)
			taskFour, err = task.FindOne(ctx, db.Query(task.ById("task4")))
			So(err, ShouldBeNil)
			So(taskFour.Aborted, ShouldEqual, false)
			So(taskFour.Status, ShouldEqual, evergreen.TaskDispatched)
			So(taskFour.DisplayStatusCache, ShouldEqual, evergreen.TaskDispatched)
		})

		Convey("single host task group tasks be omitted from the immediate restart logic", func() {

			taskFive := &task.Task{
				Id:                "task5",
				DisplayName:       "task5",
				BuildId:           b.Id,
				DisplayTaskId:     utility.ToStringPtr(""),
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				TaskGroup:         "tg",
				TaskGroupMaxHosts: 1,
			}
			So(taskFive.Insert(t.Context()), ShouldBeNil)

			taskSix := &task.Task{
				Id:                "task6",
				DisplayName:       "task6",
				BuildId:           b.Id,
				DisplayTaskId:     utility.ToStringPtr(""),
				Status:            evergreen.TaskDispatched,
				Activated:         true,
				TaskGroup:         "tg",
				TaskGroupMaxHosts: 1,
			}
			So(taskSix.Insert(t.Context()), ShouldBeNil)

			taskSeven := &task.Task{
				Id:            "task7",
				DisplayName:   "task7",
				BuildId:       b.Id,
				DisplayTaskId: utility.ToStringPtr(""),
				Status:        evergreen.TaskSucceeded,
				Activated:     true,
			}
			So(taskSeven.Insert(t.Context()), ShouldBeNil)

			So(RestartBuild(ctx, b, []string{"task5", "task6", "task7"}, false, ""), ShouldBeNil)
			var err error
			b, err = build.FindOne(ctx, build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			taskFive, err = task.FindOne(ctx, db.Query(task.ById("task5")))
			So(err, ShouldBeNil)
			So(taskFive.Status, ShouldEqual, evergreen.TaskSucceeded)
			So(taskFive.ResetWhenFinished, ShouldBeTrue)
			taskSix, err = task.FindOne(ctx, db.Query(task.ById("task6")))
			So(err, ShouldBeNil)
			taskSeven, err = task.FindOne(ctx, db.Query(task.ById("task7")))
			So(err, ShouldBeNil)
			So(taskSeven.Status, ShouldEqual, evergreen.TaskUndispatched)
		})

		Convey("a fully completed single host task group should get reset", func() {
			taskEight := &task.Task{
				Id:                "task8",
				DisplayName:       "task8",
				BuildId:           b.Id,
				Version:           v.Id,
				DisplayTaskId:     utility.ToStringPtr(""),
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				TaskGroup:         "tg2",
				TaskGroupMaxHosts: 1,
			}
			So(taskEight.Insert(t.Context()), ShouldBeNil)

			taskNine := &task.Task{
				Id:                "task9",
				DisplayName:       "task9",
				BuildId:           b.Id,
				Version:           v.Id,
				DisplayTaskId:     utility.ToStringPtr(""),
				Status:            evergreen.TaskSucceeded,
				Activated:         true,
				TaskGroup:         "tg2",
				TaskGroupMaxHosts: 1,
			}
			So(taskNine.Insert(t.Context()), ShouldBeNil)

			So(RestartBuild(ctx, b, []string{"task8", "task9"}, false, ""), ShouldBeNil)
			var err error
			b, err = build.FindOne(ctx, build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			taskEight, err = task.FindOne(ctx, db.Query(task.ById("task8")))
			So(err, ShouldBeNil)
			So(taskEight.Status, ShouldEqual, evergreen.TaskUndispatched)
			So(taskEight.DisplayStatusCache, ShouldEqual, evergreen.TaskWillRun)
			taskNine, err = task.FindOne(ctx, db.Query(task.ById("task9")))
			So(err, ShouldBeNil)
			So(taskNine.Status, ShouldEqual, evergreen.TaskUndispatched)
			So(taskNine.DisplayStatusCache, ShouldEqual, evergreen.TaskWillRun)
		})

	})
}

func TestBuildMarkAborted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a build", t, func() {

		require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

		v := &Version{
			Id: "v",
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant:     "bv",
					ActivationStatus: ActivationStatus{Activated: true},
				},
			},
		}

		So(v.Insert(t.Context()), ShouldBeNil)

		b := &build.Build{
			Id:           "build",
			Activated:    true,
			BuildVariant: "bv",
			Version:      "v",
		}
		So(b.Insert(t.Context()), ShouldBeNil)

		Convey("when marking it as aborted", func() {

			Convey("it should be deactivated", func() {
				var err error
				So(AbortBuild(ctx, b.Id, ""), ShouldBeNil)
				b, err = build.FindOne(ctx, build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeFalse)
			})

			Convey("all abortable tasks for it should be aborted", func() {

				// insert two abortable tasks and one non-abortable task
				// for the correct build, and one abortable task for
				// a different build

				abortableOne := &task.Task{
					Id:      "abortableOne",
					BuildId: b.Id,
					Status:  evergreen.TaskStarted,
				}
				So(abortableOne.Insert(t.Context()), ShouldBeNil)

				abortableTwo := &task.Task{
					Id:      "abortableTwo",
					BuildId: b.Id,
					Status:  evergreen.TaskDispatched,
				}
				So(abortableTwo.Insert(t.Context()), ShouldBeNil)

				notAbortable := &task.Task{
					Id:      "notAbortable",
					BuildId: b.Id,
					Status:  evergreen.TaskSucceeded,
				}
				So(notAbortable.Insert(t.Context()), ShouldBeNil)

				wrongBuildId := &task.Task{
					Id:      "wrongBuildId",
					BuildId: "blech",
					Status:  evergreen.TaskStarted,
				}
				So(wrongBuildId.Insert(t.Context()), ShouldBeNil)

				// aborting the build should mark only the two abortable tasks
				// with the correct build id as aborted

				So(AbortBuild(ctx, b.Id, ""), ShouldBeNil)

				abortedTasks, err := task.Find(ctx, task.ByAborted(true))
				So(err, ShouldBeNil)
				So(len(abortedTasks), ShouldEqual, 2)
				So(taskIdInSlice(abortedTasks, abortableOne.Id), ShouldBeTrue)
				So(taskIdInSlice(abortedTasks, abortableTwo.Id), ShouldBeTrue)
			})
		})
	})
}

func TestModifyVersionDoesntSucceedVersionOnAbort(t *testing.T) {
	require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

	vID := "abcdef"
	v := &Version{Id: vID, BuildIds: []string{"b0"}, Status: evergreen.VersionStarted}
	require.NoError(t, v.Insert(t.Context()))

	b0 := build.Build{
		Id: "b0", Version: vID, Activated: true, Tasks: []build.TaskCache{{Id: "t0"}, {Id: "t1"}, {Id: "t2"}}, Status: evergreen.BuildStarted,
	}
	b1 := build.Build{
		Id: "b1", Version: vID, Activated: false, Tasks: []build.TaskCache{{Id: "t3"}}, Status: evergreen.BuildCreated,
	}
	require.NoError(t, b0.Insert(t.Context()))
	require.NoError(t, b1.Insert(t.Context()))

	tasks := []task.Task{
		{Id: "t0", BuildId: "b0", Version: vID, Activated: true, Status: evergreen.TaskStarted},
		{Id: "t1", BuildId: "b0", Version: vID, Activated: true, Status: evergreen.TaskFailed},
		{Id: "t2", BuildId: "b0", Version: vID, Activated: true, Status: evergreen.TaskUndispatched},
		{Id: "t3", BuildId: "b1", Version: vID, Activated: false, Status: evergreen.TaskUndispatched},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	modification := VersionModification{
		Action: evergreen.SetActiveAction,
		Abort:  true,
		Active: false,
	}
	_, err := ModifyVersion(t.Context(), *v, user.DBUser{Id: "testuser"}, modification)
	assert.NoError(t, err)

	dbVersion, err := VersionFindOneId(t.Context(), v.Id)
	require.NoError(t, err)
	require.NotZero(t, dbVersion)
	assert.Equal(t, evergreen.VersionStarted, dbVersion.Status)
}

func TestModifyVersionScheduling(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

	t.Run("PreventMergeQueueActivation", func(t *testing.T) {
		vID := "merge_queue_version"
		v := &Version{
			Id:        vID,
			BuildIds:  []string{"b0"},
			Status:    evergreen.VersionStarted,
			Requester: evergreen.GithubMergeRequester,
		}
		require.NoError(t, v.Insert(ctx))

		b0 := build.Build{
			Id:        "b0",
			Version:   vID,
			Activated: false,
			Tasks:     []build.TaskCache{{Id: "t0"}},
			Status:    evergreen.BuildCreated,
		}
		require.NoError(t, b0.Insert(ctx))

		t0 := task.Task{
			Id:        "t0",
			BuildId:   "b0",
			Version:   vID,
			Activated: false,
			Status:    evergreen.TaskUndispatched,
		}
		require.NoError(t, t0.Insert(ctx))

		modification := VersionModification{
			Action: evergreen.SetActiveAction,
			Active: true,
		}

		status, err := ModifyVersion(ctx, *v, user.DBUser{Id: "testuser"}, modification)
		assert.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, err.Error(), "merge queue patches cannot be manually scheduled")
	})

	t.Run("PreventMergeQueueDeactivation", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

		vID := "merge_queue_version_2"
		v := &Version{
			Id:        vID,
			BuildIds:  []string{"b1"},
			Status:    evergreen.VersionStarted,
			Requester: evergreen.GithubMergeRequester,
		}
		require.NoError(t, v.Insert(ctx))

		b1 := build.Build{
			Id:        "b1",
			Version:   vID,
			Activated: true,
			Tasks:     []build.TaskCache{{Id: "t1"}},
			Status:    evergreen.BuildStarted,
		}
		require.NoError(t, b1.Insert(ctx))

		t1 := task.Task{
			Id:        "t1",
			BuildId:   "b1",
			Version:   vID,
			Activated: true,
			Status:    evergreen.TaskStarted,
		}
		require.NoError(t, t1.Insert(ctx))

		modification := VersionModification{
			Action: evergreen.SetActiveAction,
			Active: false,
		}

		status, err := ModifyVersion(ctx, *v, user.DBUser{Id: "testuser"}, modification)
		assert.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, err.Error(), "merge queue patches cannot be manually scheduled")
	})

	t.Run("AllowNonMQScheduling", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

		vID := "regular_version"
		v := &Version{
			Id:        vID,
			BuildIds:  []string{"b2"},
			Status:    evergreen.VersionStarted,
			Requester: evergreen.RepotrackerVersionRequester,
		}
		require.NoError(t, v.Insert(ctx))

		b2 := build.Build{
			Id:        "b2",
			Version:   vID,
			Activated: false,
			Tasks:     []build.TaskCache{{Id: "t2"}},
			Status:    evergreen.BuildCreated,
		}
		require.NoError(t, b2.Insert(ctx))

		t2 := task.Task{
			Id:        "t2",
			BuildId:   "b2",
			Version:   vID,
			Activated: false,
			Status:    evergreen.TaskUndispatched,
		}
		require.NoError(t, t2.Insert(ctx))

		modification := VersionModification{
			Action: evergreen.SetActiveAction,
			Active: true,
		}

		status, err := ModifyVersion(ctx, *v, user.DBUser{Id: "testuser"}, modification)
		assert.NoError(t, err)
		assert.Equal(t, 0, status)

		modification.Active = false
		status, err = ModifyVersion(ctx, *v, user.DBUser{Id: "testuser"}, modification)
		assert.NoError(t, err)
		assert.Equal(t, 0, status)
	})
}

func TestSetVersionActivation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

	vID := "abcdef"
	v := &Version{Id: vID, BuildIds: []string{"b0", "b1"}}
	require.NoError(t, v.Insert(t.Context()))

	builds := []build.Build{
		{Id: "b0", Version: vID, Activated: true, Tasks: []build.TaskCache{{Id: "t0"}}, Status: evergreen.BuildCreated},
		{Id: "b1", Version: vID, Activated: true, Tasks: []build.TaskCache{{Id: "t1"}}, Status: evergreen.BuildSucceeded},
	}
	for _, build := range builds {
		require.NoError(t, build.Insert(t.Context()))
	}

	tasks := []task.Task{
		{Id: "t0", BuildId: "b0", Version: vID, Activated: true, Status: evergreen.TaskUndispatched},
		{Id: "t1", BuildId: "b1", Version: vID, Activated: true, Status: evergreen.TaskSucceeded},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	assert.NoError(t, SetVersionActivation(context.Background(), vID, false, "user"))
	builds, err := build.FindBuildsByVersions(t.Context(), []string{vID})
	require.NoError(t, err)
	require.Len(t, builds, 2)
	for _, b := range builds {
		assert.False(t, b.Activated)
	}

	t0, err := task.FindOneId(ctx, tasks[0].Id)
	require.NoError(t, err)
	assert.False(t, t0.Activated)

	t1, err := task.FindOneId(ctx, tasks[1].Id)
	require.NoError(t, err)
	assert.True(t, t1.Activated)
}

func TestBuildSetActivated(t *testing.T) {
	Convey("With a build", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

		Convey("when changing the activated status of the build to true", func() {
			Convey("the activated status of the build and all undispatched"+
				" tasks that are part of it should be set", func() {

				user := "differentUser"
				vID := "abcdef"
				v := &Version{Id: vID}
				require.NoError(t, v.Insert(t.Context()))

				b := &build.Build{
					Id:           "build",
					Activated:    true,
					BuildVariant: "bv",
					Version:      vID,
					Status:       evergreen.BuildStarted,
					ActivatedBy:  evergreen.GenerateTasksActivator,
				}
				So(b.Insert(t.Context()), ShouldBeNil)

				// insert three tasks, with only one of them undispatched and
				// belonging to the correct build

				wrongBuildId := &task.Task{
					Id:          "wrongBuildId",
					BuildId:     "blech",
					Status:      evergreen.TaskUndispatched,
					Activated:   true,
					ActivatedBy: evergreen.GenerateTasksActivator,
				}
				So(wrongBuildId.Insert(t.Context()), ShouldBeNil)

				wrongStatus := &task.Task{
					Id:          "wrongStatus",
					BuildId:     b.Id,
					Status:      evergreen.TaskDispatched,
					Activated:   true,
					ActivatedBy: evergreen.GenerateTasksActivator,
				}
				So(wrongStatus.Insert(t.Context()), ShouldBeNil)

				matching := &task.Task{
					Id:        "matching",
					BuildId:   b.Id,
					Status:    evergreen.TaskUndispatched,
					Activated: true,
					DependsOn: []task.Dependency{
						{
							TaskId: "dependency",
							Status: evergreen.TaskSucceeded,
						},
					},
					ActivatedBy: evergreen.GenerateTasksActivator,
				}

				So(matching.Insert(t.Context()), ShouldBeNil)

				differentUser := &task.Task{
					Id:          "differentUser",
					BuildId:     b.Id,
					Status:      evergreen.TaskUndispatched,
					Activated:   true,
					ActivatedBy: user,
				}
				So(differentUser.Insert(t.Context()), ShouldBeNil)

				dependency := &task.Task{
					Id:           "dependency",
					BuildId:      "dependent_build",
					Status:       evergreen.TaskUndispatched,
					Activated:    false,
					DispatchTime: utility.ZeroTime,
					ActivatedBy:  evergreen.GenerateTasksActivator,
				}
				So(dependency.Insert(t.Context()), ShouldBeNil)

				canary := &task.Task{
					Id:           "canary",
					BuildId:      "dependent_build",
					Status:       evergreen.TaskUndispatched,
					Activated:    false,
					DispatchTime: utility.ZeroTime,
					ActivatedBy:  evergreen.GenerateTasksActivator,
				}
				So(canary.Insert(t.Context()), ShouldBeNil)

				So(ActivateBuildsAndTasks(ctx, []string{b.Id}, false, evergreen.GenerateTasksActivator), ShouldBeNil)
				// the build should have been updated in the db
				b, err := build.FindOne(ctx, build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeFalse)
				So(b.ActivatedBy, ShouldEqual, evergreen.GenerateTasksActivator)

				// only the matching task should have been updated that has not been set by a user
				deactivatedTasks, err := task.Find(ctx, task.ByActivation(false))
				So(err, ShouldBeNil)
				So(len(deactivatedTasks), ShouldEqual, 3)
				So(deactivatedTasks[0].Id, ShouldEqual, matching.Id)

				// task with the different user activating should be activated with that user
				differentUserTask, err := task.FindOne(ctx, db.Query(task.ById(differentUser.Id)))
				So(err, ShouldBeNil)
				So(differentUserTask.Activated, ShouldBeTrue)
				So(differentUserTask.ActivatedBy, ShouldEqual, user)

				So(ActivateBuildsAndTasks(ctx, []string{b.Id}, true, ""), ShouldBeNil)
				activatedTasks, err := task.Find(ctx, task.ByActivation(true))
				So(err, ShouldBeNil)
				So(len(activatedTasks), ShouldEqual, 5)
			})

			Convey("if a build is activated by a user it should not be able to be deactivated by evergreen", func() {
				user := "differentUser"
				vID := "abcdef"
				v := &Version{
					Id: vID,
					BuildVariants: []VersionBuildStatus{
						{
							BuildVariant:     "bv",
							ActivationStatus: ActivationStatus{Activated: true},
						},
					},
				}
				require.NoError(t, v.Insert(t.Context()))

				b := &build.Build{
					Id:           "anotherBuild",
					Activated:    true,
					BuildVariant: "bv",
					Version:      vID,
					ActivatedBy:  evergreen.BuildActivator,
				}

				So(b.Insert(t.Context()), ShouldBeNil)

				matching := &task.Task{
					Id:          "matching",
					BuildId:     b.Id,
					Status:      evergreen.TaskUndispatched,
					Activated:   false,
					ActivatedBy: evergreen.BuildActivator,
				}
				So(matching.Insert(t.Context()), ShouldBeNil)

				matching2 := &task.Task{
					Id:          "matching2",
					BuildId:     b.Id,
					Status:      evergreen.TaskUndispatched,
					Activated:   false,
					ActivatedBy: evergreen.BuildActivator,
				}
				So(matching2.Insert(t.Context()), ShouldBeNil)

				// have a user set the build activation to true
				So(ActivateBuildsAndTasks(ctx, []string{b.Id}, true, user), ShouldBeNil)

				// task with the different user activating should be activated with that user
				task1, err := task.FindOne(ctx, db.Query(task.ById(matching.Id)))
				So(err, ShouldBeNil)
				So(task1.Activated, ShouldBeTrue)
				So(task1.ActivatedBy, ShouldEqual, user)

				// task with the different user activating should be activated with that user
				task2, err := task.FindOne(ctx, db.Query(task.ById(matching2.Id)))
				So(err, ShouldBeNil)
				So(task2.Activated, ShouldBeTrue)
				So(task2.ActivatedBy, ShouldEqual, user)

				// refresh from the database and check again
				b, err = build.FindOne(ctx, build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeTrue)
				So(b.ActivatedBy, ShouldEqual, user)

				// deactivate the task from evergreen and nothing should be deactivated.
				So(ActivateBuildsAndTasks(ctx, []string{b.Id}, false, evergreen.BuildActivator), ShouldBeNil)

				// refresh from the database and check again
				b, err = build.FindOne(ctx, build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeTrue)
				So(b.ActivatedBy, ShouldEqual, user)

				// task with the different user activating should be activated with that user
				task1, err = task.FindOne(ctx, db.Query(task.ById(matching.Id)))
				So(err, ShouldBeNil)
				So(task1.Activated, ShouldBeTrue)
				So(task1.ActivatedBy, ShouldEqual, user)

				// task with the different user activating should be activated with that user
				task2, err = task.FindOne(ctx, db.Query(task.ById(matching2.Id)))
				So(err, ShouldBeNil)
				So(task2.Activated, ShouldBeTrue)
				So(task2.ActivatedBy, ShouldEqual, user)

			})
		})

	})
}

func TestBuildMarkStarted(t *testing.T) {
	Convey("With a build", t, func() {

		require.NoError(t, db.Clear(build.Collection))

		b := &build.Build{
			Id:     "build",
			Status: evergreen.BuildCreated,
		}
		So(b.Insert(t.Context()), ShouldBeNil)

		Convey("marking it as started should update the status and"+
			" start time, both in memory and in the database", func() {

			startTime := time.Now()
			So(build.TryMarkStarted(t.Context(), b.Id, startTime), ShouldBeNil)

			// refresh from db and check again
			var err error
			b, err = build.FindOne(t.Context(), build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			So(b.StartTime.Round(time.Second).Equal(
				startTime.Round(time.Second)), ShouldBeTrue)
		})
	})
}

func TestBuildMarkFinished(t *testing.T) {

	Convey("With a build", t, func() {

		require.NoError(t, db.Clear(build.Collection))

		startTime := time.Now()
		b := &build.Build{
			Id:            "build",
			StartTime:     startTime,
			ActivatedTime: startTime,
		}
		So(b.Insert(t.Context()), ShouldBeNil)

		Convey("marking it as finished should update the status,"+
			" finish time, and duration, both in memory and in the"+
			" database", func() {

			finishTime := time.Now()
			So(b.MarkFinished(t.Context(), evergreen.BuildSucceeded, finishTime), ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildSucceeded)
			So(b.FinishTime.Equal(finishTime), ShouldBeTrue)
			So(b.TimeTaken, ShouldEqual, finishTime.Sub(startTime))

			var err error
			// refresh from db and check again
			b, err = build.FindOne(t.Context(), build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildSucceeded)
			So(b.FinishTime.Round(time.Second).Equal(
				finishTime.Round(time.Second)), ShouldBeTrue)
			So(b.TimeTaken, ShouldEqual, finishTime.Sub(startTime))
		})
	})
}

func TestCreateBuildFromVersion(t *testing.T) {
	ctx := context.TODO()

	Convey("When creating a build from a version", t, func() {

		require.NoError(t, db.ClearCollections(ProjectRefCollection, VersionCollection, build.Collection, task.Collection, ProjectAliasCollection))

		// the mock build variant we'll be using. runs all three tasks
		buildVar1 := parserBV{
			Name:        "buildVar",
			DisplayName: "Build Variant",
			RunOn:       []string{"arch"},
			Tasks: parserBVTaskUnits{
				{Name: "taskA"}, {Name: "taskB"}, {Name: "taskC"}, {Name: "taskD"},
			},
			DisplayTasks: []displayTask{
				{
					Name: "bv1DisplayTask1",
					ExecutionTasks: []string{
						"taskA",
						"taskB",
					},
				},
				{
					Name: "bv1DisplayTask2",
					ExecutionTasks: []string{
						"taskC",
						"taskD",
					},
				},
			},
		}
		buildVar2 := parserBV{
			Name:        "buildVar2",
			DisplayName: "Build Variant 2",
			RunOn:       []string{"arch"},
			Tasks: parserBVTaskUnits{
				{Name: "taskA"}, {Name: "taskB"}, {Name: "taskC"}, {Name: "taskE"},
			},
		}
		buildVar3 := parserBV{
			Name:        "buildVar3",
			DisplayName: "Build Variant 3",
			RunOn:       []string{"arch"},
			Tasks: parserBVTaskUnits{
				{
					// wait for the first BV's taskA to complete
					Name: "taskA",
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: "taskA",
							Variant: &variantSelector{StringSelector: "buildVar"}},
						},
					},
				},
			},
		}
		buildVar4 := parserBV{
			Name:        "buildVar4",
			DisplayName: "Build Variant 4",
			RunOn:       []string{"container1"},
			Tasks: parserBVTaskUnits{
				{Name: "taskA"}, {Name: "taskB"}, {Name: "taskC"}, {
					Name:  "taskE",
					RunOn: parserStringSlice{"container2"},
				},
			},
		}
		buildVar5 := parserBV{
			Name:        "buildVar5",
			DisplayName: "Build Variant 5",
			RunOn:       []string{"arch"},
			Tasks: parserBVTaskUnits{
				{Name: "singleHostTaskGroup"},
			},
		}

		pref := &ProjectRef{
			Id:         "projectId",
			Identifier: "projectName",
		}
		So(pref.Insert(t.Context()), ShouldBeNil)

		alias := ProjectAlias{ProjectID: pref.Id, TaskTags: []string{"pull-requests"}, Alias: evergreen.GithubPRAlias,
			Variant: ".*"}
		So(alias.Upsert(t.Context()), ShouldBeNil)
		mustHaveResults := true
		parserProject := &ParserProject{
			Identifier: utility.ToStringPtr("projectId"),
			TaskGroups: []parserTaskGroup{
				{
					Name:     "singleHostTaskGroup",
					MaxHosts: 1,
					Tasks:    []string{"singleHostTaskGroup1", "singleHostTaskGroup2", "singleHostTaskGroup3"},
				},
			},
			Tasks: []parserTask{
				{
					Name:      "taskA",
					Priority:  5,
					Tags:      []string{"tag1", "tag2"},
					DependsOn: nil,
				},
				{
					Name: "taskB",
					Tags: []string{"tag1", "tag2"},
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: "taskA",
							Variant: &variantSelector{StringSelector: "buildVar"}},
						},
					},
				},
				{
					Name: "taskC",
					Tags: []string{"tag1", "tag2", "pull-requests"},
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: "taskA"}},
						{TaskSelector: taskSelector{Name: "taskB"}},
					},
				},
				{
					Name: "taskD",
					Tags: []string{"tag1", "tag2"},
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: AllDependencies}},
					},
					MustHaveResults: &mustHaveResults,
				},
				{
					Name: "taskE",
					Tags: []string{"tag1", "tag2"},
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: AllDependencies,
							Variant: &variantSelector{StringSelector: AllVariants}},
						},
					},
				},
				{
					Name: "singleHostTaskGroup1",
				},
				{
					Name: "singleHostTaskGroup2",
				},
				{
					Name: "singleHostTaskGroup3",
				},
			},
			BuildVariants: []parserBV{buildVar1, buildVar2, buildVar3, buildVar4, buildVar5},
		}

		// the mock version we'll be using
		v := &Version{
			Id:                  "versionId",
			CreateTime:          time.Now(),
			Revision:            "foobar",
			RevisionOrderNumber: 500,
			Requester:           evergreen.RepotrackerVersionRequester,
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant:     buildVar1.Name,
					ActivationStatus: ActivationStatus{Activated: true},
				},
				{
					BuildVariant:     buildVar2.Name,
					ActivationStatus: ActivationStatus{Activated: true},
				},
				{
					BuildVariant:     buildVar3.Name,
					ActivationStatus: ActivationStatus{Activated: true},
				},
				{
					BuildVariant:     buildVar4.Name,
					ActivationStatus: ActivationStatus{Activated: true},
				},
				{
					BuildVariant:     buildVar5.Name,
					ActivationStatus: ActivationStatus{Activated: true},
				},
			},
		}
		So(v.Insert(t.Context()), ShouldBeNil)

		project, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(project, ShouldNotBeNil)
		table := NewTaskIdConfigForRepotrackerVersion(t.Context(), project, v, TVPairSet{}, "", "")
		tt := table.ExecutionTasks
		dt := table.DisplayTasks

		Convey("the task id table should be well-formed", func() {
			So(tt.GetId("buildVar", "taskA"), ShouldNotEqual, "")
			So(tt.GetId("buildVar", "taskB"), ShouldNotEqual, "")
			So(tt.GetId("buildVar", "taskC"), ShouldNotEqual, "")
			So(tt.GetId("buildVar", "taskD"), ShouldNotEqual, "")
			So(tt.GetId("buildVar2", "taskA"), ShouldNotEqual, "")
			So(tt.GetId("buildVar2", "taskB"), ShouldNotEqual, "")
			So(tt.GetId("buildVar2", "taskC"), ShouldNotEqual, "")
			So(tt.GetId("buildVar2", "taskE"), ShouldNotEqual, "")
			So(tt.GetId("buildVar3", "taskA"), ShouldNotEqual, "")
			So(dt.GetId("buildVar", "bv1DisplayTask1"), ShouldNotEqual, "")
			So(dt.GetId("buildVar", "bv1DisplayTask2"), ShouldNotEqual, "")

			Convey(`and incorrect GetId() calls should return ""`, func() {
				So(tt.GetId("buildVar", "taskF"), ShouldEqual, "")
				So(tt.GetId("buildVar2", "taskD"), ShouldEqual, "")
				So(tt.GetId("buildVar7", "taskA"), ShouldEqual, "")
				So(dt.GetId("buildVar7", "displayTask"), ShouldEqual, "")
			})
		})

		Convey("if a non-existent build variant is passed in, an error should be returned", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: "blecch",
				ActivateBuild:    false,
				TaskNames:        []string{},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldNotBeNil)
			So(build, ShouldBeNil)
			So(tasks, ShouldBeNil)
		})

		Convey("if no task names are passed in to be used, all of the default"+
			" tasks for the build variant should be created", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar1.Name,
				ActivateBuild:    false,
				TaskNames:        []string{},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build, ShouldNotBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 6)

			creationInfo.BuildVariantName = buildVar2.Name
			build, tasks, err = CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build, ShouldNotBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 4)
			So(len(build.Tasks), ShouldEqual, 4)
			So(len(tasks[0].Tags), ShouldEqual, 2)
		})

		Convey("if a non-empty list of task names is passed in, only the"+
			" specified tasks should be created", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar1.Name,
				ActivateBuild:    true,
				TaskNames:        []string{"taskA", "taskB"},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 2)
			for _, t := range tasks {
				So(t.Activated, ShouldBeTrue)
			}
		})

		Convey("if a non-empty list of TasksWithBatchTime is passed in, only the specified tasks should be activated", func() {
			batchTimeTasks := []string{"taskA", "taskB"}
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar1.Name,
				ActivateBuild:    true,
				TaskNames:        []string{"taskA", "taskB", "taskC", "taskD"}, // excluding display tasks
				ActivationInfo: specificActivationInfo{activationTasks: map[string][]string{
					buildVar1.Name: batchTimeTasks},
				},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 4)
			for _, t := range tasks {
				if utility.StringSliceContains(batchTimeTasks, t.DisplayName) {
					So(t.Activated, ShouldBeFalse)
				} else {
					So(t.Activated, ShouldBeTrue)
				}
			}
		})

		Convey("if an alias is passed in, dependencies are also created", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar2.Name,
				ActivateBuild:    false,
				Aliases:          []ProjectAlias{alias},
				TaskNames:        []string{"taskA", "taskB", "taskC"},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 3)
		})

		Convey("ensure distro is populated to tasks", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar1.Name,
				ActivateBuild:    false,
				TaskNames:        []string{"taskA", "taskB"},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			for _, t := range tasks {
				So(t.DistroId, ShouldEqual, "arch")
			}

		})

		Convey("host execution mode should be populated for execution tasks running on a distro", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar1.Name,
				ActivateBuild:    false,
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			for _, t := range tasks {
				if t.DisplayOnly {
					So(t.Execution, ShouldBeZeroValue)
				} else {
					So(t.ExecutionPlatform, ShouldEqual, task.ExecutionPlatformHost)
				}
			}
		})

		Convey("execution platform should be set to containers and container options should be populated when run_on contains a container name", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar4.Name,
				ActivateBuild:    false,
				TaskNames:        []string{},
			}
			build, _, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(build.Tasks), ShouldEqual, 4)
		})

		Convey("the build should contain task caches that correspond exactly"+
			" to the tasks created", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar2.Name,
				ActivateBuild:    false,
				TaskNames:        []string{},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 4)
			So(len(build.Tasks), ShouldEqual, 4)

			// make sure the task caches are correct.  they should also appear
			// in the same order that they appear in the project file
			So(build.Tasks[0].Id, ShouldContainSubstring, "taskA")
			So(build.Tasks[1].Id, ShouldContainSubstring, "taskB")
			So(build.Tasks[2].Id, ShouldContainSubstring, "taskC")
			So(build.Tasks[3].Id, ShouldContainSubstring, "taskE")
		})

		Convey("a task cache should not contain execution tasks that are part of a display task", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar1.Name,
				ActivateBuild:    false,
				TaskNames:        []string{},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(build.Tasks), ShouldEqual, 2)

			// make sure the task caches are correct
			So(build.Tasks[0].Id, ShouldContainSubstring, buildVar1.DisplayTasks[0].Name)
			So(build.Tasks[1].Id, ShouldContainSubstring, buildVar1.DisplayTasks[1].Name)

			// check the display tasks too
			So(len(tasks), ShouldEqual, 6)
			So(tasks[0].DisplayName, ShouldEqual, buildVar1.DisplayTasks[0].Name)
			So(tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[0].DisplayStatusCache, ShouldEqual, evergreen.TaskUnscheduled)
			So(tasks[0].DisplayOnly, ShouldBeTrue)
			So(len(tasks[0].ExecutionTasks), ShouldEqual, 2)
			So(tasks[1].DisplayName, ShouldEqual, buildVar1.DisplayTasks[1].Name)
			So(tasks[1].DisplayOnly, ShouldBeTrue)
			So(tasks[1].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[1].DisplayStatusCache, ShouldEqual, evergreen.TaskUnscheduled)
		})
		Convey("all of the tasks created should have the dependencies"+
			"and priorities specified in the project", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar1.Name,
				ActivateBuild:    false,
				TaskNames:        []string{},
			}
			build, tasks1, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")

			creationInfo.BuildVariantName = buildVar2.Name
			build, tasks2, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			creationInfo.BuildVariantName = buildVar3.Name
			build, tasks3, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			So(tasks1.InsertUnordered(context.Background()), ShouldBeNil)
			So(tasks2.InsertUnordered(context.Background()), ShouldBeNil)
			So(tasks3.InsertUnordered(context.Background()), ShouldBeNil)
			dbTasks, err := task.FindWithSort(ctx, bson.M{}, []string{task.DisplayNameKey, task.BuildVariantKey})
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 9)

			// taskA
			So(len(dbTasks[0].DependsOn), ShouldEqual, 0)
			So(len(dbTasks[1].DependsOn), ShouldEqual, 0)
			So(len(dbTasks[2].DependsOn), ShouldEqual, 1)
			So(dbTasks[0].Priority, ShouldEqual, 5)
			So(dbTasks[1].Priority, ShouldEqual, 5)
			So(dbTasks[2].DependsOn, ShouldResemble,
				[]task.Dependency{{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded}})

			// taskB
			So(dbTasks[3].DependsOn, ShouldResemble,
				[]task.Dependency{{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded}})
			So(dbTasks[4].DependsOn, ShouldResemble,
				[]task.Dependency{{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded}}) //cross-variant
			So(dbTasks[3].Priority, ShouldEqual, 0)
			So(dbTasks[4].Priority, ShouldEqual, 0) //default priority

			// taskC
			So(dbTasks[5].DependsOn, ShouldHaveLength, 2)
			So(dbTasks[5].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded})
			So(dbTasks[5].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[3].Id, Status: evergreen.TaskSucceeded})

			So(dbTasks[6].DependsOn, ShouldHaveLength, 2)
			So(dbTasks[6].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[1].Id, Status: evergreen.TaskSucceeded})
			So(dbTasks[6].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[4].Id, Status: evergreen.TaskSucceeded})

			So(dbTasks[7].DependsOn, ShouldHaveLength, 3)
			So(dbTasks[7].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded})
			So(dbTasks[7].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[3].Id, Status: evergreen.TaskSucceeded})
			So(dbTasks[7].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[5].Id, Status: evergreen.TaskSucceeded})

			So(dbTasks[8].DisplayName, ShouldEqual, "taskE")
			So(len(dbTasks[8].DependsOn), ShouldEqual, 15)
		})

		Convey("all of the build's essential fields should be set correctly", func() {
			creationInfo := TaskCreationInfo{
				Project:                             project,
				ProjectRef:                          pref,
				Version:                             v,
				TaskIDs:                             table,
				BuildVariantName:                    buildVar1.Name,
				ActivateBuild:                       false,
				TaskNames:                           []string{},
				ActivatedTasksAreEssentialToSucceed: true,
			}
			build, _, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")

			// verify all the fields are set appropriately
			So(len(build.Tasks), ShouldEqual, 2)
			So(build.CreateTime.Truncate(time.Second), ShouldResemble,
				v.CreateTime.Truncate(time.Second))
			So(build.Activated, ShouldBeFalse)
			So(build.ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(build.Project, ShouldEqual, project.Identifier)
			So(build.Revision, ShouldEqual, v.Revision)
			So(build.Status, ShouldEqual, evergreen.BuildCreated)
			So(build.BuildVariant, ShouldEqual, buildVar1.Name)
			So(build.Version, ShouldEqual, v.Id)
			So(build.DisplayName, ShouldEqual, buildVar1.DisplayName)
			So(build.RevisionOrderNumber, ShouldEqual, v.RevisionOrderNumber)
			So(build.Requester, ShouldEqual, v.Requester)
			So(build.HasUnfinishedEssentialTask, ShouldBeFalse)
		})

		Convey("all of the tasks' essential fields should be set correctly", func() {
			creationInfo := TaskCreationInfo{
				Project:                             project,
				ProjectRef:                          pref,
				Version:                             v,
				TaskIDs:                             table,
				BuildVariantName:                    buildVar1.Name,
				ActivateBuild:                       false,
				TaskNames:                           []string{},
				TaskCreateTime:                      time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
				ActivatedTasksAreEssentialToSucceed: true,
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(build.HasUnfinishedEssentialTask, ShouldBeFalse)

			So(len(tasks), ShouldEqual, 6)
			for _, t := range tasks {
				// Tasks with specific activation conditions are not essential
				// to succeed.
				So(t.IsEssentialToSucceed, ShouldBeFalse)
			}
			So(tasks[2].Id, ShouldNotEqual, "")
			So(tasks[2].Secret, ShouldNotEqual, "")
			So(tasks[2].DisplayName, ShouldEqual, "taskA")
			So(tasks[2].BuildId, ShouldEqual, build.Id)
			So(tasks[2].DistroId, ShouldEqual, "arch")
			So(tasks[2].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[2].CreateTime.Equal(creationInfo.TaskCreateTime), ShouldBeTrue)
			So(tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[2].DisplayStatusCache, ShouldEqual, evergreen.TaskUnscheduled)
			So(tasks[2].Activated, ShouldBeFalse)
			So(tasks[2].ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(tasks[2].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
			So(tasks[2].Requester, ShouldEqual, build.Requester)
			So(tasks[2].Version, ShouldEqual, v.Id)
			So(tasks[2].Revision, ShouldEqual, v.Revision)
			So(tasks[2].Project, ShouldEqual, project.Identifier)

			So(tasks[3].Id, ShouldNotEqual, "")
			So(tasks[3].Secret, ShouldNotEqual, "")
			So(tasks[3].DisplayName, ShouldEqual, "taskB")
			So(tasks[3].BuildId, ShouldEqual, build.Id)
			So(tasks[3].DistroId, ShouldEqual, "arch")
			So(tasks[3].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[3].CreateTime.Equal(creationInfo.TaskCreateTime), ShouldBeTrue)
			So(tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[3].Activated, ShouldBeFalse)
			So(tasks[3].ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(tasks[3].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
			So(tasks[3].Requester, ShouldEqual, build.Requester)
			So(tasks[3].Version, ShouldEqual, v.Id)
			So(tasks[3].Revision, ShouldEqual, v.Revision)
			So(tasks[3].Project, ShouldEqual, project.Identifier)

			So(tasks[4].Id, ShouldNotEqual, "")
			So(tasks[4].Secret, ShouldNotEqual, "")
			So(tasks[4].DisplayName, ShouldEqual, "taskC")
			So(tasks[4].BuildId, ShouldEqual, build.Id)
			So(tasks[4].DistroId, ShouldEqual, "arch")
			So(tasks[4].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[4].CreateTime.Equal(creationInfo.TaskCreateTime), ShouldBeTrue)
			So(tasks[4].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[4].Activated, ShouldBeFalse)
			So(tasks[4].ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(tasks[4].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
			So(tasks[4].Requester, ShouldEqual, build.Requester)
			So(tasks[4].Version, ShouldEqual, v.Id)
			So(tasks[4].Revision, ShouldEqual, v.Revision)
			So(tasks[4].Project, ShouldEqual, project.Identifier)

			So(tasks[5].Id, ShouldNotEqual, "")
			So(tasks[5].Secret, ShouldNotEqual, "")
			So(tasks[5].DisplayName, ShouldEqual, "taskD")
			So(tasks[5].BuildId, ShouldEqual, build.Id)
			So(tasks[5].DistroId, ShouldEqual, "arch")
			So(tasks[5].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[5].CreateTime.Equal(creationInfo.TaskCreateTime), ShouldBeTrue)
			So(tasks[5].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[5].Activated, ShouldBeFalse)
			So(tasks[5].ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(tasks[5].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
			So(tasks[5].Requester, ShouldEqual, build.Requester)
			So(tasks[5].Version, ShouldEqual, v.Id)
			So(tasks[5].Revision, ShouldEqual, v.Revision)
			So(tasks[5].Project, ShouldEqual, project.Identifier)
		})

		Convey("if the activated flag is set, the build and all its tasks should be activated",
			func() {
				creationInfo := TaskCreationInfo{
					Project:                             project,
					ProjectRef:                          pref,
					Version:                             v,
					TaskIDs:                             table,
					BuildVariantName:                    buildVar1.Name,
					ActivateBuild:                       true,
					TaskNames:                           []string{},
					TaskCreateTime:                      time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
					ActivatedTasksAreEssentialToSucceed: true,
				}
				build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
				So(err, ShouldBeNil)
				So(build.Id, ShouldNotEqual, "")
				So(build.Activated, ShouldBeTrue)
				So(build.ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)
				So(build.HasUnfinishedEssentialTask, ShouldBeTrue)

				for _, t := range tasks {
					if !t.DisplayOnly {
						// Activated execution tasks are essential to succeed.
						So(t.IsEssentialToSucceed, ShouldBeTrue)
					}
				}

				So(len(tasks), ShouldEqual, 6)
				So(tasks[2].Id, ShouldNotEqual, "")
				So(tasks[2].Secret, ShouldNotEqual, "")
				So(tasks[2].DisplayName, ShouldEqual, "taskA")
				So(tasks[2].BuildId, ShouldEqual, build.Id)
				So(tasks[2].DistroId, ShouldEqual, "arch")
				So(tasks[2].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[2].CreateTime.Equal(creationInfo.TaskCreateTime), ShouldBeTrue)
				So(tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[2].Activated, ShouldBeTrue)
				So(tasks[2].ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)
				So(tasks[2].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[2].Requester, ShouldEqual, build.Requester)
				So(tasks[2].Version, ShouldEqual, v.Id)
				So(tasks[2].Revision, ShouldEqual, v.Revision)
				So(tasks[2].Project, ShouldEqual, project.Identifier)

				So(tasks[3].Id, ShouldNotEqual, "")
				So(tasks[3].Secret, ShouldNotEqual, "")
				So(tasks[3].DisplayName, ShouldEqual, "taskB")
				So(tasks[3].BuildId, ShouldEqual, build.Id)
				So(tasks[3].DistroId, ShouldEqual, "arch")
				So(tasks[3].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[3].CreateTime.Equal(creationInfo.TaskCreateTime), ShouldBeTrue)
				So(tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[3].Activated, ShouldBeTrue)
				So(tasks[3].ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)
				So(tasks[3].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[3].Requester, ShouldEqual, build.Requester)
				So(tasks[3].Version, ShouldEqual, v.Id)
				So(tasks[3].Revision, ShouldEqual, v.Revision)
				So(tasks[3].Project, ShouldEqual, project.Identifier)

				So(tasks[4].Id, ShouldNotEqual, "")
				So(tasks[4].Secret, ShouldNotEqual, "")
				So(tasks[4].DisplayName, ShouldEqual, "taskC")
				So(tasks[4].BuildId, ShouldEqual, build.Id)
				So(tasks[4].DistroId, ShouldEqual, "arch")
				So(tasks[4].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[4].CreateTime.Equal(creationInfo.TaskCreateTime), ShouldBeTrue)
				So(tasks[4].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[4].Activated, ShouldBeTrue)
				So(tasks[4].ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)
				So(tasks[4].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[4].Requester, ShouldEqual, build.Requester)
				So(tasks[4].Version, ShouldEqual, v.Id)
				So(tasks[4].Revision, ShouldEqual, v.Revision)
				So(tasks[4].Project, ShouldEqual, project.Identifier)

				So(tasks[5].Id, ShouldNotEqual, "")
				So(tasks[5].Secret, ShouldNotEqual, "")
				So(tasks[5].DisplayName, ShouldEqual, "taskD")
				So(tasks[5].BuildId, ShouldEqual, build.Id)
				So(tasks[5].DistroId, ShouldEqual, "arch")
				So(tasks[5].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[5].CreateTime.Equal(creationInfo.TaskCreateTime), ShouldBeTrue)
				So(tasks[5].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[5].Activated, ShouldBeTrue)
				So(tasks[5].ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)
				So(tasks[5].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[5].Requester, ShouldEqual, build.Requester)
				So(tasks[5].Version, ShouldEqual, v.Id)
				So(tasks[5].Revision, ShouldEqual, v.Revision)
				So(tasks[5].Project, ShouldEqual, project.Identifier)
			})

		Convey("the 'must have test results' flag should be set", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar1.Name,
				ActivateBuild:    true,
				TaskNames:        []string{},
			}
			_, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			for _, t := range tasks {
				if t.DisplayName == "taskD" {
					So(t.MustHaveResults, ShouldBeTrue)
				}
			}
		})

		Convey("single host task group tasks should be assigned child dependencies upon creation", func() {
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar5.Name,
				TaskNames:        []string{},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")

			So(len(tasks), ShouldEqual, 3)
			for _, singleHostTgTask := range tasks {
				switch singleHostTgTask.DisplayName {
				case "singleHostTaskGroup1":
					So(singleHostTgTask.DependsOn, ShouldHaveLength, 0)
				case "singleHostTaskGroup2":
					So(singleHostTgTask.DependsOn, ShouldHaveLength, 1)
					So(singleHostTgTask.DependsOn[0].TaskId, ShouldEqual, table.ExecutionTasks.GetId("buildVar5", "singleHostTaskGroup1"))
				case "singleHostTaskGroup3":
					So(singleHostTgTask.DependsOn, ShouldHaveLength, 1)
					So(singleHostTgTask.DependsOn[0].TaskId, ShouldEqual, table.ExecutionTasks.GetId("buildVar5", "singleHostTaskGroup2"))
				}
			}
		})

		Convey("single host task group dependencies should still work if some tasks are missing", func() {
			// remove singleHostTaskGroup2 from the table
			table.ExecutionTasks[TVPair{Variant: "buildVar5", TaskName: "singleHostTaskGroup2"}] = ""
			creationInfo := TaskCreationInfo{
				Project:          project,
				ProjectRef:       pref,
				Version:          v,
				TaskIDs:          table,
				BuildVariantName: buildVar5.Name,
				TaskNames:        []string{"singleHostTaskGroup1", "singleHostTaskGroup3"},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 2)
			for _, singleHostTgTask := range tasks {
				switch singleHostTgTask.DisplayName {
				case "singleHostTaskGroup1":
					So(singleHostTgTask.DependsOn, ShouldHaveLength, 0)
				case "singleHostTaskGroup3":
					So(singleHostTgTask.DependsOn, ShouldHaveLength, 1)
					So(singleHostTgTask.DependsOn[0].TaskId, ShouldEqual, table.ExecutionTasks.GetId("buildVar5", "singleHostTaskGroup1"))
				}
			}
		})

	})
}

func TestCreateTaskGroup(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(build.Collection, task.Collection))
	projYml := `
  tasks:
  - name: example_task_1
  - name: example_task_2
    depends_on:
      - name: "example_task_1"
  - name: example_task_3
    depends_on:
      - name: "example_task_2"
  task_groups:
  - name: example_task_group
    max_hosts: 2
    priority: 50
    setup_group:
    - command: shell.exec
      params:
        script: "echo setup_group"
    teardown_group:
    - command: shell.exec
      params:
        script: "echo teardown_group"
    setup_task:
    - command: shell.exec
      params:
        script: "echo setup_group"
    teardown_task:
    - command: shell.exec
      params:
        script: "echo setup_group"
    tasks:
    - example_task_1
    - example_task_2
  buildvariants:
  - name: "bv"
    run_on:
    - "arch"
    tasks:
    - name: example_task_group
    - name: example_task_3
  `
	proj := &Project{}
	ctx := context.Background()
	const projectIdentifier = "test"
	_, err := LoadProjectInto(ctx, []byte(projYml), nil, projectIdentifier, proj)
	assert.NotNil(proj)
	assert.NoError(err)
	v := &Version{
		Id:                  "versionId",
		CreateTime:          time.Now(),
		Revision:            "foobar",
		RevisionOrderNumber: 500,
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant:     "bv",
				ActivationStatus: ActivationStatus{Activated: false},
			},
		},
	}
	pRef := ProjectRef{
		Id:         "projectId",
		Identifier: projectIdentifier,
	}
	table := NewTaskIdConfigForRepotrackerVersion(t.Context(), proj, v, TVPairSet{}, "", "")

	creationInfo := TaskCreationInfo{
		Project:          proj,
		ProjectRef:       &pRef,
		Version:          v,
		TaskIDs:          table,
		BuildVariantName: "bv",
		ActivateBuild:    true,
	}
	build, tasks, err := CreateBuildFromVersionNoInsert(ctx, creationInfo)
	assert.NoError(err)
	assert.Len(build.Tasks, 3)
	assert.Len(tasks, 3)
	assert.Equal("example_task_1", tasks[0].DisplayName)
	assert.Equal("example_task_group", tasks[0].TaskGroup)

	assert.Equal("example_task_2", tasks[1].DisplayName)
	assert.Contains(tasks[1].DependsOn[0].TaskId, "example_task_1")
	assert.Equal("example_task_group", tasks[1].TaskGroup)

	assert.Equal("example_task_3", tasks[2].DisplayName)
	assert.Empty(tasks[2].TaskGroup)
	assert.NotContains(tasks[2].TaskGroup, "example_task_group")
	assert.Contains(tasks[2].DependsOn[0].TaskId, "example_task_2")
}

func TestGetTaskIdTable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(task.Collection))

	v := &Version{
		Id:         "v0",
		Revision:   "abcde",
		CreateTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
	}

	p := &Project{
		Identifier: "p0_id",
		BuildVariants: []BuildVariant{
			{
				Name: "bv0",
				Tasks: []BuildVariantTaskUnit{
					{
						Name:    "t0",
						Variant: "bv0",
					},
					{
						Name:    "t1",
						Variant: "bv0",
					},
				},
			},
		},
	}

	pref := &ProjectRef{
		Identifier: "p0",
	}

	newPairs := TaskVariantPairs{
		ExecTasks: TVPairSet{
			// imagine t1 is a patch_optional task not included in newPairs
			{Variant: "bv0", TaskName: "t0"},
		},
	}
	creationInfo := TaskCreationInfo{
		Project:    p,
		ProjectRef: pref,
		Pairs:      newPairs,
		Version:    v,
	}
	existingTask := task.Task{Id: "t2", DisplayName: "existing_task", BuildVariant: "bv0", Version: v.Id}
	require.NoError(t, existingTask.Insert(t.Context()))

	tables, err := getTaskIdConfig(ctx, creationInfo)
	assert.NoError(t, err)
	assert.Len(t, tables.ExecutionTasks, 2)
	assert.Equal(t, "p0_bv0_t0_abcde_09_11_10_23_00_00", tables.ExecutionTasks.GetId("bv0", "t0"))
	assert.Equal(t, "t2", tables.ExecutionTasks.GetId("bv0", "existing_task"))
}

func TestMakeDeps(t *testing.T) {
	table := TaskIdTable{
		TVPair{TaskName: "t0", Variant: "bv0"}: "bv0_t0",
		TVPair{TaskName: "t1", Variant: "bv0"}: "bv0_t1",
		TVPair{TaskName: "t0", Variant: "bv1"}: "bv1_t0",
		TVPair{TaskName: "t1", Variant: "bv1"}: "bv1_t1",
	}
	thisTask := &task.Task{
		Id:           "bv1_t1",
		BuildVariant: "bv1",
		DisplayName:  "t1",
	}
	tSpec := BuildVariantTaskUnit{}

	t.Run("All tasks in all variants", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: AllDependencies, Variant: AllVariants},
		}

		deps := makeDeps(tSpec.DependsOn, thisTask, table)
		assert.Len(t, deps, 3)
		expectedIDs := []string{"bv0_t0", "bv0_t1", "bv1_t0"}
		for _, dep := range deps {
			assert.Contains(t, expectedIDs, dep.TaskId)
			assert.Equal(t, evergreen.TaskSucceeded, dep.Status)
		}
	})

	t.Run("All tasks in bv0", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: AllDependencies, Variant: "bv0"},
		}

		deps := makeDeps(tSpec.DependsOn, thisTask, table)
		assert.Len(t, deps, 2)
		expectedIDs := []string{"bv0_t0", "bv0_t1"}
		for _, dep := range deps {
			assert.Contains(t, expectedIDs, dep.TaskId)
			assert.Equal(t, evergreen.TaskSucceeded, dep.Status)
		}
	})

	t.Run("specific task", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: "t0", Variant: "bv0"},
		}

		deps := makeDeps(tSpec.DependsOn, thisTask, table)
		assert.Len(t, deps, 1)
		assert.Equal(t, "bv0_t0", deps[0].TaskId)
		assert.Equal(t, evergreen.TaskSucceeded, deps[0].Status)
	})

	t.Run("no duplicates", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: AllDependencies, Variant: AllVariants},
			{Name: "t0", Variant: "bv0"},
		}

		deps := makeDeps(tSpec.DependsOn, thisTask, table)
		assert.Len(t, deps, 3)
	})

	t.Run("non-default status", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: "t0", Variant: "bv0", Status: evergreen.TaskFailed},
		}

		deps := makeDeps(tSpec.DependsOn, thisTask, table)
		assert.Len(t, deps, 1)
		assert.Equal(t, "bv0_t0", deps[0].TaskId)
		assert.Equal(t, evergreen.TaskFailed, deps[0].Status)
	})

	t.Run("unspecified variant", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: AllDependencies},
		}

		deps := makeDeps(tSpec.DependsOn, thisTask, table)
		assert.Len(t, deps, 1)
		assert.Equal(t, "bv1_t0", deps[0].TaskId)
		assert.Equal(t, evergreen.TaskSucceeded, deps[0].Status)
	})

	t.Run("unspecified name", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Variant: AllVariants},
		}

		deps := makeDeps(tSpec.DependsOn, thisTask, table)
		assert.Len(t, deps, 1)
		assert.Equal(t, "bv0_t1", deps[0].TaskId)
		assert.Equal(t, evergreen.TaskSucceeded, deps[0].Status)
	})
}

func TestDeletingBuild(t *testing.T) {

	Convey("With a build", t, func() {

		require.NoError(t, db.Clear(build.Collection))

		b := &build.Build{
			Id: "build",
		}
		So(b.Insert(t.Context()), ShouldBeNil)

		Convey("deleting it should remove it and all its associated"+
			" tasks from the database", func() {

			require.NoError(t, db.ClearCollections(task.Collection))

			// insert two tasks that are part of the build, and one that isn't
			matchingTaskOne := &task.Task{
				Id:      "matchingOne",
				BuildId: b.Id,
			}
			So(matchingTaskOne.Insert(t.Context()), ShouldBeNil)

			matchingTaskTwo := &task.Task{
				Id:      "matchingTwo",
				BuildId: b.Id,
			}
			So(matchingTaskTwo.Insert(t.Context()), ShouldBeNil)

			nonMatchingTask := &task.Task{
				Id:      "nonMatching",
				BuildId: "blech",
			}
			So(nonMatchingTask.Insert(t.Context()), ShouldBeNil)
		})
	})
}

func TestSetNumDependents(t *testing.T) {
	Convey("SetNumDependents correctly sets NumDependents for each task", t, func() {
		tasks := []*task.Task{
			{Id: "task1"},
			{
				Id:        "task2",
				DependsOn: []task.Dependency{{TaskId: "task1"}},
			},
			{
				Id:        "task3",
				DependsOn: []task.Dependency{{TaskId: "task1"}},
			},
			{
				Id:        "task4",
				DependsOn: []task.Dependency{{TaskId: "task2"}, {TaskId: "task3"}, {TaskId: "not_here"}},
			},
		}
		SetNumDependents(tasks)
		So(len(tasks), ShouldEqual, 4)
		So(tasks[0].NumDependents, ShouldEqual, 3)
		So(tasks[1].NumDependents, ShouldEqual, 1)
		So(tasks[2].NumDependents, ShouldEqual, 1)
		So(tasks[3].NumDependents, ShouldEqual, 0)
	})
}

func TestSortTasks(t *testing.T) {
	Convey("sortTasks topologically sorts tasks by dependency", t, func() {
		Convey("for tasks with single dependencies", func() {
			tasks := []task.Task{
				{
					Id:          "idA",
					DisplayName: "A",
					DependsOn: []task.Dependency{
						{TaskId: "idB"},
					},
				},
				{
					Id:          "idB",
					DisplayName: "B",
					DependsOn: []task.Dependency{
						{TaskId: "idC"},
					},
				},
				{
					Id:          "idC",
					DisplayName: "C",
				},
			}

			sortedTasks := sortTasks(tasks)
			So(len(sortedTasks), ShouldEqual, 3)
			So(sortedTasks[0].DisplayName, ShouldEqual, "C")
			So(sortedTasks[1].DisplayName, ShouldEqual, "B")
			So(sortedTasks[2].DisplayName, ShouldEqual, "A")
		})
		Convey("for tasks with multiplie dependencies", func() {
			tasks := []task.Task{
				{
					Id:          "idA",
					DisplayName: "A",
					DependsOn: []task.Dependency{
						{TaskId: "idB"},
						{TaskId: "idC"},
					},
				},
				{
					Id:          "idB",
					DisplayName: "B",
					DependsOn: []task.Dependency{
						{TaskId: "idC"},
					},
				},
				{
					Id:          "idC",
					DisplayName: "C",
				},
			}

			sortedTasks := sortTasks(tasks)
			So(len(sortedTasks), ShouldEqual, 3)
			So(sortedTasks[0].DisplayName, ShouldEqual, "C")
			So(sortedTasks[1].DisplayName, ShouldEqual, "B")
			So(sortedTasks[2].DisplayName, ShouldEqual, "A")
		})
	})

	Convey("grouping tasks by common dependencies and sorting alphabetically within groups", t, func() {
		tasks := []task.Task{
			{
				Id:          "idA",
				DisplayName: "A",
				DependsOn: []task.Dependency{
					{TaskId: "idE"},
				},
			},
			{
				Id:          "idB",
				DisplayName: "B",
				DependsOn: []task.Dependency{
					{TaskId: "idD"},
				},
			},
			{
				Id:          "idC",
				DisplayName: "C",
				DependsOn: []task.Dependency{
					{TaskId: "idD"},
				},
			},
			{
				Id:          "idD",
				DisplayName: "D",
			},
			{
				Id:          "idE",
				DisplayName: "E",
			},
		}

		sortedTasks := sortTasks(tasks)
		So(len(sortedTasks), ShouldEqual, 5)
		So(sortedTasks[0].DisplayName, ShouldEqual, "D")
		So(sortedTasks[1].DisplayName, ShouldEqual, "E")
		So(sortedTasks[2].DisplayName, ShouldEqual, "B")
		So(sortedTasks[3].DisplayName, ShouldEqual, "C")
		So(sortedTasks[4].DisplayName, ShouldEqual, "A")
	})

	Convey("special-casing tasks with cross-variant dependencies to the far right", t, func() {
		tasks := []task.Task{
			{
				Id:          "idA",
				DisplayName: "A",
				DependsOn: []task.Dependency{
					{TaskId: "idB"},
					{TaskId: "idC"},
				},
			},
			{
				Id:          "idB",
				DisplayName: "B",
				DependsOn: []task.Dependency{
					{TaskId: "idC"},
				},
			},
			{
				Id:          "idC",
				DisplayName: "C",
				DependsOn: []task.Dependency{
					{TaskId: "cross-variant"},
				},
			},
			{
				Id:          "idD",
				DisplayName: "D",
			},
		}

		sortedTasks := sortTasks(tasks)
		So(len(sortedTasks), ShouldEqual, 4)
		So(sortedTasks[0].DisplayName, ShouldEqual, "D")
		So(sortedTasks[1].DisplayName, ShouldEqual, "C")
		So(sortedTasks[2].DisplayName, ShouldEqual, "B")
		So(sortedTasks[3].DisplayName, ShouldEqual, "A")

		Convey("when there are cross-variant dependencies on different tasks", func() {

			tasks = append(tasks,
				task.Task{
					Id:          "idE",
					DisplayName: "E",
					DependsOn: []task.Dependency{
						{TaskId: "cross-variant2"},
					}},
				task.Task{
					Id:          "idF",
					DisplayName: "F",
					DependsOn: []task.Dependency{
						{TaskId: "idE"},
					}})
			sortedTasks = sortTasks(tasks)
			So(len(sortedTasks), ShouldEqual, 6)
			So(sortedTasks[0].DisplayName, ShouldEqual, "D")
			So(sortedTasks[1].DisplayName, ShouldEqual, "C")
			So(sortedTasks[2].DisplayName, ShouldEqual, "E")
			So(sortedTasks[3].DisplayName, ShouldEqual, "B")
			So(sortedTasks[4].DisplayName, ShouldEqual, "F")
			So(sortedTasks[5].DisplayName, ShouldEqual, "A")
		})
	})
}

func TestVersionRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(resetTaskData())

	// test that restarting a version restarts its tasks
	taskIds := []string{"task1", "task3", "task4"}
	buildIds := []string{"build1", "build2"}
	assert.NoError(RestartVersion(ctx, "version", taskIds, false, "test"))
	tasks, err := task.Find(ctx, task.ByIds(taskIds))
	assert.NoError(err)
	assert.NotEmpty(tasks)
	builds, err := build.Find(t.Context(), build.ByIds(buildIds))
	assert.NoError(err)
	assert.NotEmpty(builds)
	for _, t := range tasks {
		assert.Equal(evergreen.TaskUndispatched, t.Status)
		assert.True(t.Activated)

		if t.Id == "task3" {
			require.Len(t.DependsOn, 1)
			assert.Equal("task1", t.DependsOn[0].TaskId)
			assert.False(t.DependsOn[0].Finished, "restarting task1 should have marked dependency as unfinished")
			assert.Equal(evergreen.TaskWillRun, t.DisplayStatusCache)
		}
	}
	for _, b := range builds {
		assert.False(b.StartTime.IsZero())
	}
	dbTask5, err := task.FindOneId(ctx, "task5")
	require.NoError(err)
	require.NotZero(dbTask5)
	require.Len(dbTask5.DependsOn, 1)
	assert.Equal("task1", dbTask5.DependsOn[0].TaskId)
	assert.False(dbTask5.DependsOn[0].Finished, "restarting task1 should have marked dependency in execution task as unfinished")

	dbVersion, err := VersionFindOneId(t.Context(), "version")
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, dbVersion.Status)

	// test that aborting in-progress tasks works correctly
	assert.NoError(resetTaskData())
	taskIds = []string{"task2"}
	assert.NoError(RestartVersion(ctx, "version", taskIds, true, "test"))
	dbTask, err := task.FindOne(ctx, db.Query(task.ById("task2")))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.True(dbTask.Aborted)
	assert.Equal("test", dbTask.AbortInfo.User)
	assert.Equal(evergreen.TaskDispatched, dbTask.Status)
	assert.True(dbTask.ResetWhenFinished)
	dbVersion, err = VersionFindOneId(t.Context(), "version")
	assert.NoError(err)
	// Version status should not update if only aborting tasks
	assert.Equal("", dbVersion.Status)

	// test that not aborting in-progress tasks does not reset them
	assert.NoError(resetTaskData())
	taskIds = []string{"task2"}
	assert.NoError(RestartVersion(ctx, "version", taskIds, false, "test"))
	dbTask, err = task.FindOne(ctx, db.Query(task.ById("task2")))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.False(dbTask.Aborted)
	assert.Equal(evergreen.TaskDispatched, dbTask.Status)
	dbVersion, err = VersionFindOneId(t.Context(), "version")
	assert.NoError(err)
	// Version status should not update if no tasks are being reset.
	assert.Equal("", dbVersion.Status)
}

func TestDisplayTaskRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	displayTasks := []string{"displayTask1"}
	allTasks := []string{"displayTask1", "task5", "task6"}

	// test restarting a version
	assert.NoError(resetTaskData())
	assert.NoError(RestartVersion(ctx, "version", displayTasks, false, "test"))
	tasks, err := task.FindAll(ctx, db.Query(task.ByIds(allTasks)))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
		assert.Equal("test", dbTask.ActivatedBy)
	}

	// test restarting a build
	assert.NoError(resetTaskData())
	assert.NoError(RestartBuild(ctx, &build.Build{Id: "build3", Version: "version"}, displayTasks, false, "test"))
	tasks, err = task.FindAll(ctx, db.Query(task.ByIds(allTasks)))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
		assert.Equal("test", dbTask.ActivatedBy)
	}

	// test that restarting a task correctly resets the task and archives it
	assert.NoError(resetTaskData())
	assert.NoError(resetTask(ctx, "displayTask1", "caller"))
	archivedTasks, err := task.FindOldWithDisplayTasks(ctx, nil)
	assert.NoError(err)
	assert.Len(archivedTasks, 3)
	foundDisplayTask := false
	for _, ot := range archivedTasks {
		if ot.OldTaskId == "displayTask1" {
			foundDisplayTask = true
		}
	}
	assert.True(foundDisplayTask)
	tasks, err = task.FindAll(ctx, db.Query(task.ByIds(allTasks)))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
		assert.Equal("caller", dbTask.ActivatedBy)

		dbEvents, err := event.FindAllByResourceID(t.Context(), dbTask.Id)
		require.NoError(t, err)
		require.Len(t, dbEvents, 1)
		assert.Equal(event.TaskRestarted, dbEvents[0].EventType)
	}

	// Test that restarting a display task with restartFailed correctly resets failed tasks.
	assert.NoError(resetTaskData())
	dt, err := task.FindOneId(ctx, "displayTask1")
	assert.NoError(err)
	assert.NoError(dt.SetResetFailedWhenFinished(ctx, "caller"))

	// Confirm that marking a display task to reset when finished increments the user's scheduling limit
	dbUser, err := user.FindOneById(t.Context(), "caller")
	assert.NoError(err)
	require.NotNil(t, dbUser)
	assert.Equal(2, dbUser.NumScheduledPatchTasks)

	assert.NoError(resetTask(ctx, dt.Id, "caller"))
	tasks, err = task.FindAll(ctx, db.Query(task.ByIds(allTasks)))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		if dbTask.Id == "task5" {
			assert.Equal(evergreen.TaskSucceeded, dbTask.Status, dbTask.Id)
		} else {
			assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		}
	}
	// Confirm that resetting a display task does not affect the user's scheduling limit
	dbUser, err = user.FindOneById(t.Context(), "caller")
	assert.NoError(err)
	require.NotNil(t, dbUser)
	assert.Equal(2, dbUser.NumScheduledPatchTasks)

	// test that execution tasks cannot be restarted
	assert.NoError(resetTaskData())
	settings := testutil.TestConfig()
	assert.Error(TryResetTask(ctx, settings, "task5", "", "", nil))

	// trying to restart execution tasks should restart the entire display task, if it's done
	assert.NoError(resetTaskData())
	assert.NoError(RestartVersion(ctx, "version", allTasks, false, "test"))
	tasks, err = task.FindAll(ctx, db.Query(task.ByIds(allTasks)))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
	}
}

func TestResetTaskOrDisplayTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, settings *evergreen.Settings){
		"ResettingExecutionTaskResetsDisplayTask": func(ctx context.Context, t *testing.T, settings *evergreen.Settings) {
			et, err := task.FindOneId(ctx, "task5")
			assert.NoError(t, err)
			require.NotNil(t, et)

			// restarting execution tasks should restart display task
			assert.NoError(t, ResetTaskOrDisplayTask(ctx, settings, et, "caller", evergreen.StepbackTaskActivator, false, nil))
			dt, err := task.FindOneId(ctx, "displayTask1")
			assert.NoError(t, err)
			require.NotNil(t, dt)
			assert.Equal(t, evergreen.TaskUndispatched, dt.Status)
			assert.Equal(t, 1, dt.Execution)
			assert.False(t, dt.ResetWhenFinished)

			dbUser, err := user.FindOneById(t.Context(), "caller")
			assert.NoError(t, err)
			require.NotNil(t, dbUser)
			assert.Equal(t, len(dt.ExecutionTasks), dbUser.NumScheduledPatchTasks)
		},
		"ResettingFailedTasksInFailedDisplayTaskResetsExecutionTasks": func(ctx context.Context, t *testing.T, settings *evergreen.Settings) {
			dt, err := task.FindOneId(ctx, "displayTask1")
			assert.NoError(t, err)
			require.NotNil(t, dt)

			assert.NoError(t, ResetTaskOrDisplayTask(ctx, settings, dt, "caller", evergreen.StepbackTaskActivator, true, nil))
			dt, err = task.FindOneId(ctx, "displayTask1")
			assert.NoError(t, err)
			require.NotNil(t, dt)
			assert.Equal(t, evergreen.TaskUndispatched, dt.Status)
			assert.Equal(t, 1, dt.Execution)
			assert.False(t, dt.ResetFailedWhenFinished, "should not mark to reset failed tasks when display task was already finished")

			failedExecTask, err := task.FindOneId(ctx, "task6")
			assert.NoError(t, err)
			require.NotNil(t, failedExecTask)
			assert.Equal(t, evergreen.TaskUndispatched, failedExecTask.Status, "failed execution task should be reset")

			successfulExecTask, err := task.FindOneId(ctx, "task5")
			assert.NoError(t, err)
			require.NotNil(t, successfulExecTask)
			assert.Equal(t, evergreen.TaskSucceeded, successfulExecTask.Status, "successful execution task should not be reset")

			dbUser, err := user.FindOneById(t.Context(), "caller")
			assert.NoError(t, err)
			require.NotNil(t, dbUser)
			assert.Equal(t, len(dt.ExecutionTasks), dbUser.NumScheduledPatchTasks)

			assert.NoError(t, ResetTaskOrDisplayTask(ctx, settings, dt, "caller", evergreen.StepbackTaskActivator, true, nil))
			dt, err = task.FindOneId(ctx, "displayTask1")
			assert.NoError(t, err)
			require.NotNil(t, dt)
			assert.Equal(t, evergreen.TaskUndispatched, dt.Status)
			assert.Equal(t, 1, dt.Execution)
			assert.True(t, dt.ResetFailedWhenFinished, "should mark to reset failed tasks when display task is unfinished")
		},
		"ResettingTasksInSuccessfulDisplayTaskResetsExecutionTasks": func(ctx context.Context, t *testing.T, settings *evergreen.Settings) {
			dt, err := task.FindOneId(ctx, "displayTask2")
			assert.NoError(t, err)
			require.NotNil(t, dt)

			assert.NoError(t, ResetTaskOrDisplayTask(ctx, settings, dt, "caller", evergreen.StepbackTaskActivator, false, nil))
			dt, err = task.FindOneId(ctx, "displayTask2")
			assert.NoError(t, err)
			require.NotNil(t, dt)
			assert.Equal(t, evergreen.TaskUndispatched, dt.Status, "display task should reset")
			assert.Equal(t, 1, dt.Execution, "should reset to new execution")

			et, err := task.FindOneId(ctx, "task7")
			assert.NoError(t, err)
			require.NotNil(t, et)
			assert.Equal(t, evergreen.TaskUndispatched, et.Status, "execution task should reset")
			assert.Equal(t, 1, et.Execution, "should reset to new execution")
		},
		"ResettingOnlyFailedTasksInSuccessfulDisplayTaskShouldNotResetButShouldAllowLaterReset": func(ctx context.Context, t *testing.T, settings *evergreen.Settings) {
			dt, err := task.FindOneId(ctx, "displayTask2")
			assert.NoError(t, err)
			require.NotNil(t, dt)

			assert.NoError(t, ResetTaskOrDisplayTask(ctx, settings, dt, "caller", evergreen.StepbackTaskActivator, true, nil))
			dt, err = task.FindOneId(ctx, "displayTask2")
			assert.NoError(t, err)
			require.NotNil(t, dt)
			assert.Equal(t, evergreen.TaskSucceeded, dt.Status, "display task should not reset because all execution tasks succeeded")
			assert.Equal(t, 0, dt.Execution, "should not reset to new execution because all execution tasks succeeded")

			et, err := task.FindOneId(ctx, "task7")
			assert.NoError(t, err)
			require.NotNil(t, et)
			assert.Equal(t, evergreen.TaskSucceeded, et.Status, "successful execution task should not be reset")

			// After trying to only reset failed execution tasks and no-oping,
			// try resetting the display task unconditionally. The display task
			// and execution tasks should restart.
			assert.NoError(t, ResetTaskOrDisplayTask(ctx, settings, dt, "caller", evergreen.StepbackTaskActivator, false, nil))
			dt, err = task.FindOneId(ctx, "displayTask2")
			assert.NoError(t, err)
			require.NotNil(t, dt)
			assert.Equal(t, evergreen.TaskUndispatched, dt.Status, "display task should reset")
			assert.Equal(t, 1, dt.Execution, "should reset to new execution")

			et, err = task.FindOneId(ctx, "task7")
			assert.NoError(t, err)
			require.NotNil(t, et)
			assert.Equal(t, evergreen.TaskUndispatched, et.Status, "execution task should reset")
			assert.Equal(t, 1, et.Execution, "should reset to new execution")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			settings := testutil.TestConfig()
			assert.NoError(t, resetTaskData())

			tCase(ctx, t, settings)
		})
	}
}

func resetTaskData() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := db.ClearCollections(build.Collection, task.Collection, VersionCollection, task.OldCollection, user.Collection, event.EventCollection); err != nil {
		return err
	}
	v := &Version{
		Id: "version",
	}
	if err := v.Insert(ctx); err != nil {
		return err
	}
	u := user.DBUser{
		Id:                     "caller",
		NumScheduledPatchTasks: 50,
	}
	if err := u.Insert(ctx); err != nil {
		return err
	}
	build1 := &build.Build{
		Id:      "build1",
		Version: v.Id,
	}
	build2 := &build.Build{
		Id:      "build2",
		Version: v.Id,
	}
	build3 := &build.Build{
		Id:      "build3",
		Version: v.Id,
	}
	if err := build1.Insert(ctx); err != nil {
		return err
	}
	if err := build2.Insert(ctx); err != nil {
		return err
	}
	if err := build3.Insert(ctx); err != nil {
		return err
	}
	task1 := &task.Task{
		Id:            "task1",
		DisplayName:   "task1",
		BuildId:       build1.Id,
		Version:       v.Id,
		DisplayTaskId: utility.ToStringPtr(""),
		Status:        evergreen.TaskSucceeded,
		Activated:     true,
	}
	if err := task1.Insert(ctx); err != nil {
		return err
	}
	task2 := &task.Task{
		Id:            "task2",
		DisplayName:   "task2",
		BuildId:       build1.Id,
		Version:       v.Id,
		DisplayTaskId: utility.ToStringPtr(""),
		Status:        evergreen.TaskDispatched,
		Activated:     true,
	}
	if err := task2.Insert(ctx); err != nil {
		return err
	}
	task3 := &task.Task{
		Id:            "task3",
		DisplayName:   "task3",
		BuildId:       build2.Id,
		Version:       v.Id,
		DisplayTaskId: utility.ToStringPtr(""),
		Status:        evergreen.TaskSucceeded,
		Activated:     true,
		DependsOn: []task.Dependency{
			{
				TaskId:   task1.Id,
				Finished: true,
			},
		},
	}
	if err := task3.Insert(ctx); err != nil {
		return err
	}
	task4 := &task.Task{
		Id:            "task4",
		DisplayName:   "task4",
		BuildId:       build2.Id,
		Version:       v.Id,
		DisplayTaskId: utility.ToStringPtr(""),
		Status:        evergreen.TaskFailed,
		Activated:     true,
	}
	if err := task4.Insert(ctx); err != nil {
		return err
	}
	task5 := &task.Task{
		Id:            "task5",
		DisplayName:   "task5",
		BuildId:       build3.Id,
		Version:       v.Id,
		DisplayTaskId: utility.ToStringPtr("displayTask1"),
		Status:        evergreen.TaskSucceeded,
		Activated:     true,
		DispatchTime:  time.Now(),
		DependsOn: []task.Dependency{
			{
				TaskId:   task1.Id,
				Finished: true,
			},
		},
	}
	if err := task5.Insert(ctx); err != nil {
		return err
	}
	task6 := &task.Task{
		Id:            "task6",
		DisplayName:   "task6",
		BuildId:       build3.Id,
		Version:       v.Id,
		DisplayTaskId: utility.ToStringPtr("displayTask1"),
		Status:        evergreen.TaskFailed,
		Activated:     true,
		DispatchTime:  time.Now(),
	}
	if err := task6.Insert(ctx); err != nil {
		return err
	}
	task7 := &task.Task{
		Id:            "task7",
		DisplayName:   "task7",
		BuildId:       build3.Id,
		Version:       v.Id,
		DisplayTaskId: utility.ToStringPtr("displayTask2"),
		Status:        evergreen.TaskSucceeded,
		Activated:     true,
		DispatchTime:  time.Now(),
	}
	if err := task7.Insert(ctx); err != nil {
		return err
	}
	displayTask1 := &task.Task{
		Id:             "displayTask1",
		DisplayName:    "displayTask1",
		Execution:      0,
		Requester:      evergreen.PatchVersionRequester,
		BuildId:        build3.Id,
		Version:        v.Id,
		DisplayTaskId:  utility.ToStringPtr(""),
		DisplayOnly:    true,
		ExecutionTasks: []string{task5.Id, task6.Id},
		Status:         evergreen.TaskFailed,
		Activated:      true,
		DispatchTime:   time.Now(),
	}
	if err := displayTask1.Insert(ctx); err != nil {
		return err
	}
	if err := UpdateDisplayTaskForTask(ctx, task5); err != nil {
		return err
	}
	displayTask2 := &task.Task{
		Id:             "displayTask2",
		DisplayName:    "displayTask2",
		Execution:      0,
		Requester:      evergreen.PatchVersionRequester,
		BuildId:        build3.Id,
		Version:        v.Id,
		DisplayTaskId:  utility.ToStringPtr(""),
		DisplayOnly:    true,
		ExecutionTasks: []string{task7.Id},
		Status:         evergreen.TaskSucceeded,
		Activated:      true,
		DispatchTime:   time.Now(),
	}
	if err := displayTask2.Insert(ctx); err != nil {
		return err
	}
	return nil
}

func TestCreateTasksFromGroup(t *testing.T) {
	assert := assert.New(t)
	const tgName = "name"
	const bvName = "first_build_variant"
	in := BuildVariantTaskUnit{
		Name:      tgName,
		IsGroup:   true,
		Variant:   bvName,
		Priority:  0,
		DependsOn: []TaskUnitDependency{{Name: "new_dependency"}},
		RunOn:     []string{},
	}
	p := &Project{
		BuildVariants: []BuildVariant{
			{
				Name:  "first_build_variant",
				Tasks: []BuildVariantTaskUnit{in},
			},
		},
		Tasks: []ProjectTask{
			{
				Name:      "first_task",
				DependsOn: []TaskUnitDependency{{Name: "dependency"}},
			},
			{
				Name: "second_task",
			},
			{
				Name:      "third_task",
				Patchable: utility.FalsePtr(),
			},
		},
		TaskGroups: []TaskGroup{
			{
				Name:  tgName,
				Tasks: []string{"first_task", "second_task", "third_task"},
			},
		},
	}
	bvts := CreateTasksFromGroup(in, p, evergreen.PatchVersionRequester)
	require.Len(t, bvts, 2)
	for _, bvtu := range bvts {
		require.Len(t, bvtu.DependsOn, 1)
		assert.Equal("new_dependency", bvtu.DependsOn[0].Name)
		assert.False(bvtu.IsGroup)
		assert.True(bvtu.IsPartOfGroup)
		assert.Equal(tgName, bvtu.GroupName)
	}
}

func TestMarkAsHostDispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		taskId       string
		hostId       string
		agentVersion string
		buildId      string
		distroId     string
		taskDoc      *task.Task
		b            *build.Build
	)

	Convey("With a task", t, func() {

		taskId = "t1"
		hostId = "h1"
		agentVersion = "a1"
		buildId = "b1"
		distroId = "d1"

		taskDoc = &task.Task{
			Id:      taskId,
			BuildId: buildId,
		}

		b = &build.Build{Id: buildId}

		require.NoError(t, db.ClearCollections(task.Collection, build.Collection))

		So(taskDoc.Insert(t.Context()), ShouldBeNil)
		So(b.Insert(t.Context()), ShouldBeNil)

		Convey("when marking the task as dispatched, the fields for"+
			" the task, the host it is on, and the build it is a part of"+
			" should be set to reflect this", func() {

			So(taskDoc.MarkAsHostDispatched(ctx, hostId, distroId, agentVersion, time.Now()), ShouldBeNil)

			// make sure the task's fields were updated, both in ©memory and
			// in the db
			So(taskDoc.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(taskDoc.Status, ShouldEqual, evergreen.TaskDispatched)
			So(taskDoc.HostId, ShouldEqual, hostId)
			So(taskDoc.AgentVersion, ShouldEqual, agentVersion)
			So(taskDoc.LastHeartbeat, ShouldResemble, taskDoc.DispatchTime)
			taskDoc, err := task.FindOne(ctx, db.Query(task.ById(taskId)))
			So(err, ShouldBeNil)
			So(taskDoc.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(taskDoc.Status, ShouldEqual, evergreen.TaskDispatched)
			So(taskDoc.HostId, ShouldEqual, hostId)
			So(taskDoc.AgentVersion, ShouldEqual, agentVersion)
			So(taskDoc.LastHeartbeat, ShouldResemble, taskDoc.DispatchTime)

		})

	})

}

func TestSetTaskActivationForBuildsActivated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

	vId := "v"
	v := &Version{Id: vId}
	require.NoError(t, v.Insert(t.Context()))

	build := build.Build{Id: "b0", Version: vId}
	require.NoError(t, build.Insert(t.Context()))

	tasks := []task.Task{
		{Id: "t0", BuildId: "b0", Status: evergreen.TaskUndispatched},
		{Id: "t1", BuildId: "b1", Status: evergreen.TaskUndispatched},
		{Id: "t2", BuildId: "b0", DependsOn: []task.Dependency{{TaskId: "t1"}}, Status: evergreen.TaskUndispatched},
		{Id: "t3", BuildId: "b0", DependsOn: []task.Dependency{{TaskId: "t0"}}, Status: evergreen.TaskUndispatched},
		// Execution tasks for display tasks
		{Id: "exec1", BuildId: "b0", Status: evergreen.TaskUndispatched, DisplayTaskId: utility.ToStringPtr("dt1")},
		{Id: "exec2", BuildId: "b0", Status: evergreen.TaskUndispatched, DisplayTaskId: utility.ToStringPtr("dt1")},
		{Id: "exec3", BuildId: "b0", Status: evergreen.TaskUndispatched, DisplayTaskId: utility.ToStringPtr("dt2")},
		{Id: "exec4", BuildId: "b0", Status: evergreen.TaskUndispatched, DisplayTaskId: utility.ToStringPtr("dt2")},
		{Id: "exec5", BuildId: "b0", Status: evergreen.TaskUndispatched, DisplayTaskId: utility.ToStringPtr("dt3")},
		{Id: "exec6", BuildId: "b0", Status: evergreen.TaskUndispatched, DisplayTaskId: utility.ToStringPtr("dt3")},
		// Display dbTask with all execution tasks in ignore list
		{Id: "dt1", BuildId: "b0", Status: evergreen.TaskUndispatched, DisplayOnly: true, ExecutionTasks: []string{"exec1", "exec2"}},
		// Display dbTask with some execution tasks in ignore list
		{Id: "dt2", BuildId: "b0", Status: evergreen.TaskUndispatched, DisplayOnly: true, ExecutionTasks: []string{"exec3", "exec4"}},
		// Display dbTask with no execution tasks in ignore list
		{Id: "dt3", BuildId: "b0", Status: evergreen.TaskUndispatched, DisplayOnly: true, ExecutionTasks: []string{"exec5", "exec6"}},
	}

	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	// t0 should still be activated because it's a dependency of a dbTask that is being activated
	// dt1 should NOT be activated because all its execution tasks (exec1, exec2) are in the ignore list
	// dt2 should be activated because only some of its execution tasks (exec3) are in the ignore list
	// dt3 should be activated because none of its execution tasks are in the ignore list
	assert.NoError(t, setTaskActivationForBuilds(context.Background(), []string{"b0"}, true, true, []string{"t0", "exec1", "exec2", "exec3"}, ""))

	dbTasks, err := task.FindAll(ctx, task.All)
	require.NoError(t, err)
	require.Len(t, dbTasks, 13)
	for _, dbTask := range dbTasks {
		switch dbTask.Id {
		case "exec1", "exec2", "exec3":
			// Execution tasks in ignore list should NOT be activated
			assert.False(t, dbTask.Activated, "%s should not be activated because it's in the ignore list", dbTask.Id)
		case "dt1":
			// Display dbTask with all execution tasks ignored should NOT be activated
			assert.False(t, dbTask.Activated, "dt1 should not be activated because all its execution tasks are in the ignore list")
		case "dt2", "dt3":
			// Display dbTask with some or no execution tasks ignored should be activated
			assert.True(t, dbTask.Activated, "%s should be activated because not all its execution tasks are in the ignore list", dbTask.Id)
		case "t0":
			// t0 is in ignore list but should be activated because it's a dependency
			assert.True(t, dbTask.Activated, "t0 should be activated because it's a dependency")
		default:
			// All other tasks should be activated
			assert.True(t, dbTask.Activated, "%s should be activated", dbTask.Id)
		}
	}
}

func TestSetTaskActivationForBuildsWithIgnoreTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

	vId := "v"
	v := &Version{Id: vId}
	require.NoError(t, v.Insert(t.Context()))

	build := build.Build{Id: "b0", Version: vId}
	require.NoError(t, build.Insert(t.Context()))

	tasks := []task.Task{
		{Id: "t0", BuildId: "b0", Status: evergreen.TaskUndispatched},
		{Id: "t1", BuildId: "b1", Status: evergreen.TaskUndispatched},
		{Id: "t2", BuildId: "b0", DependsOn: []task.Dependency{{TaskId: "t1"}}, Status: evergreen.TaskUndispatched},
		{Id: "t3", BuildId: "b0", DependsOn: []task.Dependency{{TaskId: "t0"}}, Status: evergreen.TaskUndispatched},
	}

	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	assert.NoError(t, setTaskActivationForBuilds(context.Background(), []string{"b0"}, true, true, []string{"t3"}, ""))

	dbTasks, err := task.FindAll(ctx, task.All)
	require.NoError(t, err)
	require.Len(t, dbTasks, 4)
	for _, dbTask := range dbTasks {
		if dbTask.Id == "t3" {
			assert.False(t, dbTask.Activated)
			continue
		}
		assert.True(t, dbTask.Activated)
	}
}

func TestSetTaskActivationForBuildsDeactivated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection))

	vId := "v"
	v := &Version{Id: vId}
	require.NoError(t, v.Insert(t.Context()))

	build := build.Build{Id: "b0", Version: vId}
	require.NoError(t, build.Insert(t.Context()))

	tasks := []task.Task{
		{Id: "t0", Activated: true, BuildId: "b0", Status: evergreen.TaskUndispatched},
		{Id: "t1", Activated: true, BuildId: "b1", DependsOn: []task.Dependency{{TaskId: "t2"}}, Status: evergreen.TaskUndispatched},
		{Id: "t2", Activated: true, BuildId: "b0", Status: evergreen.TaskUndispatched},
	}

	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	// ignore tasks is ignored for deactivating
	assert.NoError(t, setTaskActivationForBuilds(context.Background(), []string{"b0"}, false, true, []string{"t0", "t1", "t2"}, ""))

	dbTasks, err := task.FindAll(ctx, task.All)
	require.NoError(t, err)
	require.Len(t, dbTasks, 3)
	for _, task := range dbTasks {
		assert.False(t, task.Activated)
	}
}

func TestAddNewTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection))
	}()

	require.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection))
	b := build.Build{
		Id:           "b0",
		BuildVariant: "bv0",
		Activated:    false,
	}

	v := &Version{
		Id:       "v0",
		BuildIds: []string{"b0"},
	}
	assert.NoError(t, v.Insert(t.Context()))

	tasksToAdd := TaskVariantPairs{
		ExecTasks: []TVPair{
			{
				Variant:  "bv0",
				TaskName: "t1",
			},
		},
	}

	project := Project{
		BuildVariants: []BuildVariant{
			{
				Name: "bv0",
				Tasks: []BuildVariantTaskUnit{
					{Name: "t0"},
					{
						Name:      "t1",
						Variant:   "bv0",
						DependsOn: []TaskUnitDependency{{Name: "t0"}},
						RunOn:     []string{"d0"},
					},
				},
			},
		},
		Tasks: []ProjectTask{
			{Name: "t0"},
			{Name: "t1"},
		},
	}

	for name, testCase := range map[string]struct {
		activationInfo specificActivationInfo
		activatedTasks []string
		existingTask   task.Task
		bvActive       bool
	}{
		"ActivatedNewTask": {
			activationInfo: specificActivationInfo{},
			activatedTasks: []string{"t0", "t1"},
			existingTask: task.Task{
				Id:           "t0",
				DisplayName:  "t0",
				BuildId:      "b0",
				BuildVariant: "bv0",
				Version:      "v0",
				Activated:    true,
			},
			bvActive: true,
		},
		"DeactivatedNewTask": {
			activationInfo: specificActivationInfo{activationTasks: map[string][]string{
				b.BuildVariant: {"t1"},
			}},
			activatedTasks: []string{},
			existingTask:   task.Task{},
			bvActive:       false,
		},
		"OnlyDeactivatedTasks": {
			activationInfo: specificActivationInfo{activationTasks: map[string][]string{
				b.BuildVariant: {"t1"},
			}},
			activatedTasks: []string{},
			existingTask: task.Task{
				Id:           "t0",
				DisplayName:  "t0",
				BuildId:      "b0",
				BuildVariant: "bv0",
				Version:      "v0",
				Activated:    false,
			},
			bvActive: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, build.Collection))
			assert.NoError(t, testCase.existingTask.Insert(t.Context()))
			assert.NoError(t, b.Insert(t.Context()))
			creationInfo := TaskCreationInfo{
				Project:        &project,
				ProjectRef:     &ProjectRef{},
				Version:        v,
				Pairs:          tasksToAdd,
				ActivationInfo: testCase.activationInfo,
				GeneratedBy:    "",
			}
			_, _, err := addNewTasksToExistingBuilds(context.Background(), creationInfo, []build.Build{b}, "")
			assert.NoError(t, err)
			buildTasks, err := task.FindAll(ctx, db.Query(bson.M{task.BuildIdKey: "b0"}))
			assert.NoError(t, err)
			activatedTasks, err := task.FindAll(ctx, db.Query(bson.M{task.ActivatedKey: true}))
			assert.NoError(t, err)
			build, err := build.FindOneId(t.Context(), "b0")
			assert.NoError(t, err)
			assert.NotNil(t, build)
			assert.Equal(t, len(testCase.activatedTasks), len(activatedTasks))
			assert.Len(t, build.Tasks, len(buildTasks))
			for _, task := range activatedTasks {
				assert.Contains(t, testCase.activatedTasks, task.DisplayName)
			}
			assert.Equal(t, testCase.bvActive, build.Activated)
		})
	}
}

func TestRecomputeNumDependents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.Clear(task.Collection))
	t1 := task.Task{
		Id: "1",
		DependsOn: []task.Dependency{
			{TaskId: "2"},
		},
		Version: "v1",
	}
	assert.NoError(t, t1.Insert(t.Context()))
	t2 := task.Task{
		Id: "2",
		DependsOn: []task.Dependency{
			{TaskId: "3"},
		},
		Version: "v1",
	}
	assert.NoError(t, t2.Insert(t.Context()))
	t3 := task.Task{
		Id: "3",
		DependsOn: []task.Dependency{
			{TaskId: "4"},
		},
		Version: "v1",
	}
	assert.NoError(t, t3.Insert(t.Context()))
	t4 := task.Task{
		Id: "4",
		DependsOn: []task.Dependency{
			{TaskId: "5"},
		},
		Version: "v1",
	}
	assert.NoError(t, t4.Insert(t.Context()))
	t5 := task.Task{
		Id:      "5",
		Version: "v1",
	}
	assert.NoError(t, t5.Insert(t.Context()))

	assert.NoError(t, RecomputeNumDependents(ctx, t3))
	tasks, err := task.Find(ctx, task.ByVersion(t1.Version))
	assert.NoError(t, err)
	for i, dbTask := range tasks {
		assert.Equal(t, i, dbTask.NumDependents)
	}

	assert.NoError(t, RecomputeNumDependents(ctx, t5))
	tasks, err = task.Find(ctx, task.ByVersion(t1.Version))
	assert.NoError(t, err)
	for i, dbTask := range tasks {
		assert.Equal(t, i, dbTask.NumDependents)
	}

	t6 := task.Task{
		Id: "6",
		DependsOn: []task.Dependency{
			{TaskId: "8"},
		},
		Version: "v2",
	}
	assert.NoError(t, t6.Insert(t.Context()))
	t7 := task.Task{
		Id: "7",
		DependsOn: []task.Dependency{
			{TaskId: "8"},
		},
		Version: "v2",
	}
	assert.NoError(t, t7.Insert(t.Context()))
	t8 := task.Task{
		Id: "8",
		DependsOn: []task.Dependency{
			{TaskId: "9"},
		},
		Version: "v2",
	}
	assert.NoError(t, t8.Insert(t.Context()))
	t9 := task.Task{
		Id:      "9",
		Version: "v2",
	}
	assert.NoError(t, t9.Insert(t.Context()))

	assert.NoError(t, RecomputeNumDependents(ctx, t8))
	tasks, err = task.Find(ctx, task.ByVersion(t6.Version))
	assert.NoError(t, err)
	expected := map[string]int{
		"6": 0,
		"7": 0,
		"8": 2,
		"9": 3,
	}
	for _, dbTask := range tasks {
		assert.Equal(t, expected[dbTask.Id], dbTask.NumDependents)
	}
}

func TestCanBuildVariantEnableTestSelection(t *testing.T) {
	t.Run("ReturnsTrueIfBVIncludedInTestSelection", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
			TestSelectionParams: TestSelectionParams{
				IncludeBuildVariants: []*regexp.Regexp{
					regexp.MustCompile("bv1"),
					regexp.MustCompile("bv2"),
				},
			},
		}
		assert.True(t, canBuildVariantEnableTestSelection("bv2", creationInfo))
	})
	t.Run("ReturnsTrueIfBVAndTasksIncludedInTestSelection", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
			TestSelectionParams: TestSelectionParams{
				IncludeBuildVariants: []*regexp.Regexp{
					regexp.MustCompile("bv1"),
					regexp.MustCompile("bv2"),
				},
				IncludeTasks: []*regexp.Regexp{regexp.MustCompile("t1")},
			},
		}
		assert.True(t, canBuildVariantEnableTestSelection("bv2", creationInfo))
	})
	t.Run("ReturnsTrueIfBVIncludedInTestSelectionAndNotExcluded", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
			TestSelectionParams: TestSelectionParams{
				IncludeBuildVariants: []*regexp.Regexp{
					regexp.MustCompile("bv1"),
					regexp.MustCompile("bv2"),
				},
				ExcludeBuildVariants: []*regexp.Regexp{regexp.MustCompile("bv1")},
			},
		}
		assert.True(t, canBuildVariantEnableTestSelection("bv2", creationInfo))
	})
	t.Run("ReturnsFalseIfBVExcludedFromTestSelectionEvenIfDefaultEnabled", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed:        utility.TruePtr(),
					DefaultEnabled: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
			TestSelectionParams: TestSelectionParams{
				ExcludeBuildVariants: []*regexp.Regexp{regexp.MustCompile("bv2")},
			},
		}
		assert.False(t, canBuildVariantEnableTestSelection("bv2", creationInfo))
	})
	t.Run("ReturnsFalseIfBVExcludedFromTestSelection", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
			TestSelectionParams: TestSelectionParams{
				IncludeBuildVariants: []*regexp.Regexp{
					regexp.MustCompile("bv1"),
					regexp.MustCompile("bv2"),
				},
				ExcludeBuildVariants: []*regexp.Regexp{regexp.MustCompile("bv2")},
			},
		}
		assert.False(t, canBuildVariantEnableTestSelection("bv2", creationInfo))
	})
	t.Run("ReturnsTrueIfTasksWithinAllBVsIncludedInTestSelection", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
			TestSelectionParams: TestSelectionParams{
				IncludeTasks: []*regexp.Regexp{regexp.MustCompile("t1")},
			},
		}
		// This includes tasks that match the pattern t1 across all build
		// variants, so the build variant can enable test selection if such a
		// task exists within it.
		assert.True(t, canBuildVariantEnableTestSelection("bv3", creationInfo))
	})
	t.Run("ReturnsFalseIfBVNotIncludedInTestSelection", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
			TestSelectionParams: TestSelectionParams{
				IncludeBuildVariants: []*regexp.Regexp{regexp.MustCompile("bv1")},
			},
		}
		assert.False(t, canBuildVariantEnableTestSelection("bv3", creationInfo))
	})
	t.Run("ReturnsFalseIfBVNotIncludedInTestSelectionEvenWithDefaultEnabled", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed:        utility.TruePtr(),
					DefaultEnabled: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
			TestSelectionParams: TestSelectionParams{
				IncludeBuildVariants: []*regexp.Regexp{regexp.MustCompile("bv1")},
			},
		}
		assert.False(t, canBuildVariantEnableTestSelection("bv2", creationInfo))
	})
	t.Run("ReturnsFalseIfNoBVsIncluded", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
		}
		assert.False(t, canBuildVariantEnableTestSelection("bv1", creationInfo))
	})
	t.Run("ReturnsFalseForNonPatchVersion", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed:        utility.TruePtr(),
					DefaultEnabled: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.RepotrackerVersionRequester,
			},
			TestSelectionParams: TestSelectionParams{
				IncludeBuildVariants: []*regexp.Regexp{regexp.MustCompile("bv1")},
			},
		}
		assert.False(t, canBuildVariantEnableTestSelection("bv1", creationInfo))
	})
	t.Run("ReturnsTrueIfTestSelectionDefaultEnabled", func(t *testing.T) {
		creationInfo := TaskCreationInfo{
			ProjectRef: &ProjectRef{
				TestSelection: TestSelectionSettings{
					Allowed:        utility.TruePtr(),
					DefaultEnabled: utility.TruePtr(),
				},
			},
			Version: &Version{
				Requester: evergreen.PatchVersionRequester,
			},
		}
		assert.True(t, canBuildVariantEnableTestSelection("bv1", creationInfo))
	})
}

func TestIsTestSelectionEnabledForTask(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo){
		"ReturnsTrueIfTaskIncludedInTestSelection": func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo) {
			creationInfo.TestSelectionParams.IncludeTasks = []*regexp.Regexp{regexp.MustCompile("t1")}
			isTestSelectionEnabled, err := isTestSelectionEnabledForTask(tsk, displayTasks, creationInfo)
			assert.NoError(t, err)
			assert.True(t, isTestSelectionEnabled)
		},
		"ReturnsTrueIfParentDisplayTaskIncludedInTestSelection": func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo) {
			tsk.DisplayTaskId = utility.ToStringPtr("display_task_id1")
			creationInfo.TestSelectionParams.IncludeTasks = []*regexp.Regexp{regexp.MustCompile("dt1")}
			isTestSelectionEnabled, err := isTestSelectionEnabledForTask(tsk, displayTasks, creationInfo)
			assert.NoError(t, err)
			assert.True(t, isTestSelectionEnabled)
		},
		"ReturnsTrueIfTaskGroupIsIncludedInTestSelection": func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo) {
			tsk.TaskGroup = "tg1"
			creationInfo.TestSelectionParams.IncludeTasks = []*regexp.Regexp{regexp.MustCompile("tg1")}
			isTestSelectionEnabled, err := isTestSelectionEnabledForTask(tsk, displayTasks, creationInfo)
			assert.NoError(t, err)
			assert.True(t, isTestSelectionEnabled)
		},
		"ReturnsFalseIfTaskIsNotIncludedInTestSelection": func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo) {
			creationInfo.TestSelectionParams.IncludeTasks = []*regexp.Regexp{regexp.MustCompile("t2")}
			isTestSelectionEnabled, err := isTestSelectionEnabledForTask(tsk, displayTasks, creationInfo)
			assert.NoError(t, err)
			assert.False(t, isTestSelectionEnabled)
		},
		"ReturnsFalseIfTaskIsIncludedByNameButDisplayTaskIsExcluded": func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo) {
			tsk.DisplayTaskId = utility.ToStringPtr("display_task_id1")
			creationInfo.TestSelectionParams.IncludeTasks = []*regexp.Regexp{regexp.MustCompile("t1")}
			creationInfo.TestSelectionParams.ExcludeTasks = []*regexp.Regexp{regexp.MustCompile("dt1")}
			isTestSelectionEnabled, err := isTestSelectionEnabledForTask(tsk, displayTasks, creationInfo)
			assert.NoError(t, err)
			assert.False(t, isTestSelectionEnabled)
		},
		"ReturnsFalseIfTaskIsIncludedByNameButTaskGroupIsExcluded": func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo) {
			tsk.TaskGroup = "tg1"
			creationInfo.TestSelectionParams.IncludeTasks = []*regexp.Regexp{regexp.MustCompile("t1")}
			creationInfo.TestSelectionParams.ExcludeTasks = []*regexp.Regexp{regexp.MustCompile("tg1")}
			isTestSelectionEnabled, err := isTestSelectionEnabledForTask(tsk, displayTasks, creationInfo)
			assert.NoError(t, err)
			assert.False(t, isTestSelectionEnabled)
		},
		"ReturnsFalseIfTaskIsExcluded": func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo) {
			creationInfo.TestSelectionParams.ExcludeTasks = []*regexp.Regexp{regexp.MustCompile("t1")}
			isTestSelectionEnabled, err := isTestSelectionEnabledForTask(tsk, displayTasks, creationInfo)
			assert.NoError(t, err)
			assert.False(t, isTestSelectionEnabled)
		},
		"ReturnsTrueIfVariantCanEnableTestSelectionAndTaskIsNotExcluded": func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo) {
			creationInfo.TestSelectionParams.ExcludeTasks = []*regexp.Regexp{regexp.MustCompile("t2")}
			isTestSelectionEnabled, err := isTestSelectionEnabledForTask(tsk, displayTasks, creationInfo)
			assert.NoError(t, err)
			assert.True(t, isTestSelectionEnabled)
		},
		"ReturnsFalseIfBVCannotEnableTestSelection": func(t *testing.T, tsk *task.Task, displayTasks map[string]string, creationInfo TaskCreationInfo) {
			creationInfo.TestSelectionParams.CanBuildVariantEnableTestSelection = false
			creationInfo.TestSelectionParams.IncludeTasks = []*regexp.Regexp{regexp.MustCompile("t1")}
			isTestSelectionEnabled, err := isTestSelectionEnabledForTask(tsk, displayTasks, creationInfo)
			assert.NoError(t, err)
			assert.False(t, isTestSelectionEnabled)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tsk := &task.Task{
				Id:          "task_id1",
				DisplayName: "t1",
			}
			displayTasks := map[string]string{"display_task_id1": "dt1"}
			creationInfo := TaskCreationInfo{
				TestSelectionParams: TestSelectionParams{
					CanBuildVariantEnableTestSelection: true,
				},
			}
			tCase(t, tsk, displayTasks, creationInfo)
		})
	}
}
