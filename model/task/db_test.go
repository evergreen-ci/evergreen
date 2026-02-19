package task

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func checkStatuses(t *testing.T, expected string, toCheck Task) {
	var dbTasks []Task
	aggregation := []bson.M{
		{"$match": bson.M{
			IdKey: toCheck.Id,
		}},
		addDisplayStatus,
	}
	err := db.Aggregate(t.Context(), Collection, aggregation, &dbTasks)
	assert.NoError(t, err)
	assert.Equal(t, expected, dbTasks[0].DisplayStatus)
	assert.Equal(t, expected, toCheck.GetDisplayStatus())
}

func TestFindTasksByIds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When calling FindTasksByIds...", t, func() {
		So(db.Clear(Collection), ShouldBeNil)
		Convey("only tasks with the specified ids should be returned", func() {

			tasks := []Task{
				{
					Id: "one",
				},
				{
					Id: "two",
				},
				{
					Id: "three",
				},
			}

			for _, task := range tasks {
				So(task.Insert(t.Context()), ShouldBeNil)
			}

			dbTasks, err := Find(ctx, ByIds([]string{"one", "two"}))
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 2)
			So(dbTasks[0].Id, ShouldNotEqual, "three")
			So(dbTasks[1].Id, ShouldNotEqual, "three")
		})
	})
}
func TestDisplayTasksByVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When calling DisplayTasksByVersion...", t, func() {
		So(db.Clear(Collection), ShouldBeNil)
		Convey("only tasks that are display tasks should be returned", func() {
			tasks := []Task{
				{
					Id:      "one",
					Version: "v1",
				},
				{
					Id:          "two",
					Version:     "v1",
					DisplayOnly: true,
				},
				{
					Id:            "three",
					Version:       "v1",
					DisplayTaskId: utility.ToStringPtr(""),
				},
				{
					Id:             "four",
					Version:        "v1",
					ExecutionTasks: []string{"execution_task_one, execution_task_two"},
				},
				{
					Id:            "five",
					Version:       "v1",
					ActivatedTime: utility.ZeroTime,
				},
				{
					Id:            "execution_task_one",
					Version:       "v1",
					DisplayTaskId: utility.ToStringPtr("four"),
				},
				{
					Id:            "execution_task_two",
					Version:       "v1",
					DisplayTaskId: utility.ToStringPtr("four"),
				},
			}

			for _, task := range tasks {
				So(task.Insert(t.Context()), ShouldBeNil)
			}

			dbTasks, err := FindAll(ctx, db.Query(DisplayTasksByVersion("v1", false)))
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 4)
			So(dbTasks[0].Id, ShouldNotEqual, "execution_task_one")
			So(dbTasks[1].Id, ShouldNotEqual, "execution_task_one")
			So(dbTasks[2].Id, ShouldNotEqual, "execution_task_one")
			So(dbTasks[3].Id, ShouldNotEqual, "execution_task_one")

			So(dbTasks[0].Id, ShouldNotEqual, "execution_task_two")
			So(dbTasks[1].Id, ShouldNotEqual, "execution_task_two")
			So(dbTasks[2].Id, ShouldNotEqual, "execution_task_two")
			So(dbTasks[3].Id, ShouldNotEqual, "execution_task_two")

			So(dbTasks[0].Id, ShouldNotEqual, "five")
			So(dbTasks[1].Id, ShouldNotEqual, "five")
			So(dbTasks[2].Id, ShouldNotEqual, "five")
			So(dbTasks[3].Id, ShouldNotEqual, "five")

		})
	})
}

func TestNonExecutionTasksByVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.Clear(Collection))
	displayTask := Task{
		Id:             "dt",
		Version:        "v1",
		DisplayTaskId:  nil, // legacy, not populated
		ExecutionTasks: []string{"exec_task", "legacy_task"},
	}
	regularTask := Task{
		Id:            "t1",
		Version:       "v1",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	wrongVersionTask := Task{
		Id:            "lame_task",
		Version:       "lame_version",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	execTask := Task{
		Id:            "exec_task",
		Version:       "v1",
		DisplayTaskId: utility.ToStringPtr("dt"),
	}
	legacyTask := Task{
		Id:            "legacy_task",
		Version:       "v2",
		DisplayTaskId: nil, // legacy, not populated
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, displayTask, regularTask, wrongVersionTask, execTask, legacyTask))

	tasks, err := Find(ctx, NonExecutionTasksByVersions([]string{"v1", "v2"}))
	assert.NoError(t, err)
	assert.Len(t, tasks, 3) // doesn't include wrong version or execution task with DisplayTaskId cached
	for _, task := range tasks {
		assert.NotEqual(t, "exec_task", task.Id)
		assert.NotEqual(t, "lame_version", task.Version)
	}
}

func TestFailedTasksByVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When calling FailedTasksByVersion...", t, func() {
		So(db.Clear(Collection), ShouldBeNil)
		Convey("only tasks with the failed statuses should be returned", func() {

			tasks := []Task{
				{
					Id:      "one",
					Version: "v1",
					Status:  evergreen.TaskFailed,
				},
				{
					Id:      "two",
					Version: "v1",
					Status:  evergreen.TaskSetupFailed,
				},
				{
					Id:      "three",
					Version: "v1",
					Status:  evergreen.TaskSucceeded,
				},
			}

			for _, task := range tasks {
				So(task.Insert(t.Context()), ShouldBeNil)
			}

			dbTasks, err := Find(ctx, FailedTasksByVersion("v1"))
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 2)
			So(dbTasks[0].Id, ShouldNotEqual, "three")
			So(dbTasks[1].Id, ShouldNotEqual, "three")
		})
	})
}

func TestPotentiallyBlockedTasksByIds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.Clear(Collection))
	tasks := []Task{
		{ // Can't be blocked (override dependencies)
			Id:                   "t1",
			OverrideDependencies: true,
		},
		{ // Can't be blocked (no dependences)
			Id:                   "t2",
			OverrideDependencies: false,
		},
		{ // Can be blocked
			Id:                   "t3",
			OverrideDependencies: false,
			DependsOn: []Dependency{
				{
					TaskId: "t1",
				},
			},
		},
		{ // Can't be blocked (no dependencies)
			Id:                   "t4",
			OverrideDependencies: false,
			DependsOn:            []Dependency{},
		},
		{ // Can't be blocked (override dependencies)
			Id:                   "t5",
			OverrideDependencies: true,
			DependsOn: []Dependency{
				{
					TaskId: "t1",
				},
			},
		},
		{ // Can be blocked
			Id:                   "t6",
			OverrideDependencies: false,
			DependsOn: []Dependency{
				{
					TaskId: "t1",
				},
			},
			DependenciesMetTime: utility.ZeroTime,
		},
		{ // Can't be blocked (dependencies met)
			Id:                   "t7",
			OverrideDependencies: false,
			DependsOn: []Dependency{
				{
					TaskId: "t1",
				},
			},
			DependenciesMetTime: time.Now(),
		},
		{ // Can be blocked
			Id: "t8",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
				},
			},
		},
	}
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
		ids = append(ids, task.Id)
	}

	dbTasks, err := Find(ctx, PotentiallyBlockedTasksByIds(ids))
	require.NoError(t, err)
	require.Len(t, dbTasks, 3)
	assert.Contains(t, []string{"t3", "t6", "t8"}, dbTasks[0].Id)
	assert.Contains(t, []string{"t3", "t6", "t8"}, dbTasks[1].Id)
	assert.Contains(t, []string{"t3", "t6", "t8"}, dbTasks[2].Id)
}

func TestFindTasksByVersionWithChildTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(Collection))
	mainVersion := "main_version"
	mainVersionTaskIds := []string{"t1", "t3"}
	tasks := []Task{
		{
			Id:      "t1",
			Version: mainVersion,
		},
		{
			Id:      "t2",
			Version: "different_version",
		},
		{
			Id:            "t3",
			Version:       "different_version",
			ParentPatchID: mainVersion,
		},
		{
			Id:            "t4",
			Version:       "different_version",
			ParentPatchID: "different_parent",
		},
	}
	for _, task := range tasks {
		assert.NoError(t, task.Insert(t.Context()))
	}

	dbTasks, err := Find(ctx, ByVersionWithChildTasks(mainVersion))
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 2)
	for _, dbTask := range dbTasks {
		assert.Contains(t, mainVersionTaskIds, dbTask.Id)
	}
}
func TestFindTasksByBuildIdAndGithubChecks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(Collection))
	tasks := []Task{
		{
			Id:            "t1",
			BuildId:       "b1",
			IsGithubCheck: true,
		},
		{
			Id:      "t2",
			BuildId: "b1",
		},
		{
			Id:            "t3",
			BuildId:       "b2",
			IsGithubCheck: true,
		},
		{
			Id:            "t4",
			BuildId:       "b2",
			IsGithubCheck: true,
		},
	}

	for _, task := range tasks {
		assert.NoError(t, task.Insert(t.Context()))
	}
	dbTasks, err := FindAll(ctx, db.Query(ByBuildIdAndGithubChecks("b1")))
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 1)
	dbTasks, err = FindAll(ctx, db.Query(ByBuildIdAndGithubChecks("b2")))
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 2)
	dbTasks, err = FindAll(ctx, db.Query(ByBuildIdAndGithubChecks("b3")))
	assert.NoError(t, err)
	assert.Empty(t, dbTasks)
}

func TestFindOneIdAndExecutionWithDisplayStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))
	taskDoc := Task{
		Id:        "task",
		Status:    evergreen.TaskSucceeded,
		Activated: true,
	}
	assert.NoError(taskDoc.Insert(t.Context()))
	task, err := FindOneIdAndExecutionWithDisplayStatus(ctx, taskDoc.Id, utility.ToIntPtr(0))
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(evergreen.TaskSucceeded, task.DisplayStatus)

	// Should fetch tasks from the old collection
	assert.NoError(taskDoc.Archive(ctx))
	task, err = FindOneOldByIdAndExecution(ctx, taskDoc.Id, 0)
	assert.NoError(err)
	assert.NotNil(task)
	task, err = FindOneIdAndExecutionWithDisplayStatus(ctx, taskDoc.Id, utility.ToIntPtr(0))
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(task.OldTaskId, taskDoc.Id)

	// Should fetch recent executions by default
	task, err = FindOneIdAndExecutionWithDisplayStatus(ctx, taskDoc.Id, nil)
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(1, task.Execution)
	assert.Equal(evergreen.TaskSucceeded, task.DisplayStatus)

	taskDoc = Task{
		Id:        "task2",
		Status:    evergreen.TaskUndispatched,
		Activated: false,
	}
	assert.NoError(taskDoc.Insert(t.Context()))
	task, err = FindOneIdAndExecutionWithDisplayStatus(ctx, taskDoc.Id, utility.ToIntPtr(0))
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(evergreen.TaskUnscheduled, task.DisplayStatus)
}

func TestFindAllFirstExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, OldCollection))
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1", Execution: 1},
		{Id: "t2", DisplayOnly: true},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}
	oldTask := Task{Id: MakeOldID("t1", 0)}
	require.NoError(t, db.Insert(t.Context(), OldCollection, &oldTask))

	foundTasks, err := FindAllFirstExecution(ctx, All)
	assert.NoError(t, err)
	assert.Len(t, foundTasks, 3)
	expectedIDs := []string{"t0", MakeOldID("t1", 0), "t2"}
	for _, task := range foundTasks {
		assert.Contains(t, expectedIDs, task.Id)
		assert.Equal(t, 0, task.Execution)
	}
}

func TestFindOneIdOldOrNew(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(Collection, OldCollection))

	taskDoc := Task{
		Id:     "task",
		Status: evergreen.TaskSucceeded,
	}
	require.NoError(taskDoc.Insert(t.Context()))
	require.NoError(taskDoc.Archive(ctx))

	task00, err := FindOneIdOldOrNew(ctx, "task", 0)
	assert.NoError(err)
	require.NotNil(task00)
	assert.Equal(0, task00.Execution)

	task01, err := FindOneIdOldOrNew(ctx, "task", 1)
	assert.NoError(err)
	require.NotNil(task01)
	assert.Equal("task", task01.Id)
	assert.Equal(1, task01.Execution)
}

func TestAddHostCreateDetails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(Collection))
	task := Task{Id: "t1", Execution: 0}
	assert.NoError(t, task.Insert(t.Context()))
	errToSave := errors.Wrapf(errors.New("InsufficientCapacityError"), "error trying to start host")
	assert.NoError(t, AddHostCreateDetails(ctx, task.Id, "h1", 0, errToSave))
	dbTask, err := FindOneId(ctx, task.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dbTask)
	require.Len(t, dbTask.HostCreateDetails, 1)
	assert.Equal(t, "h1", dbTask.HostCreateDetails[0].HostId)
	assert.Contains(t, dbTask.HostCreateDetails[0].Error, "InsufficientCapacityError")

	assert.NoError(t, AddHostCreateDetails(ctx, task.Id, "h2", 0, errToSave))
	dbTask, err = FindOneId(ctx, task.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dbTask)
	assert.Len(t, dbTask.HostCreateDetails, 2)
}

func TestDisplayStatus(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:     "t1",
		Status: evergreen.TaskSucceeded,
	}
	assert.NoError(t, t1.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskSucceeded, t1)
	t2 := Task{
		Id:        "t2",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
	}
	assert.NoError(t, t2.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskWillRun, t2)
	t3 := Task{
		Id:        "t3",
		Status:    evergreen.TaskFailed,
		Activated: true,
	}
	assert.NoError(t, t3.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskFailed, t3)
	t4 := Task{
		Id:     "t4",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type: evergreen.CommandTypeSetup,
		},
	}
	assert.NoError(t, t4.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskSetupFailed, t4)
	t5 := Task{
		Id:     "t5",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type: evergreen.CommandTypeSystem,
		},
	}
	assert.NoError(t, t5.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskSystemFailed, t5)
	t6 := Task{
		Id:     "t6",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type:     evergreen.CommandTypeSystem,
			TimedOut: true,
		},
	}
	assert.NoError(t, t6.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskSystemTimedOut, t6)
	t7 := Task{
		Id:     "t7",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type:        evergreen.CommandTypeSystem,
			TimedOut:    true,
			Description: evergreen.TaskDescriptionHeartbeat,
		},
	}
	assert.NoError(t, t7.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskSystemUnresponse, t7)
	t8 := Task{
		Id:        "t8",
		Status:    evergreen.TaskStarted,
		Activated: true,
	}
	assert.NoError(t, t8.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskStarted, t8)
	t9 := Task{
		Id:        "t9",
		Status:    evergreen.TaskUndispatched,
		Activated: false,
	}
	assert.NoError(t, t9.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskUnscheduled, t9)
	t10 := Task{
		Id:        "t10",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		DependsOn: []Dependency{
			{
				TaskId:       "t9",
				Unattainable: true,
				Status:       "success",
			},
			{
				TaskId:       "t8",
				Unattainable: false,
				Status:       "success",
			},
		},
	}
	assert.NoError(t, t10.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskStatusBlocked, t10)
	t11 := Task{
		Id:        "t11",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		DependsOn: []Dependency{
			{
				TaskId:       "t8",
				Unattainable: false,
				Status:       "success",
			},
		},
	}
	assert.NoError(t, t11.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskWillRun, t11)
	t12 := Task{
		Id:                   "t12",
		Status:               evergreen.TaskUndispatched,
		Activated:            true,
		OverrideDependencies: true,
		DependsOn: []Dependency{
			{
				TaskId:       "t9",
				Unattainable: true,
				Status:       "success",
			},
		},
	}
	// No CheckStatuses for t12 to avoid paradox
	assert.NoError(t, t12.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskWillRun, t11)
	t13 := Task{
		Id:        "t13",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
	}
	require.NoError(t, t13.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskWillRun, t13)
	t14 := Task{
		Id:        "t14",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
	}
	require.NoError(t, t14.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskWillRun, t14)
	t15 := Task{
		Id:        "t15",
		Status:    evergreen.TaskUndispatched,
		Activated: false,
	}
	require.NoError(t, t15.Insert(t.Context()))
	checkStatuses(t, evergreen.TaskUnscheduled, t15)
}

func TestFindTaskNamesByBuildVariant(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("Should return unique task names for a given build variant", t, func() {
		assert.NoError(t, db.ClearCollections(Collection))
		t1 := Task{
			Id:                  "t1",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "dist",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t1.Insert(t.Context()))
		t2 := Task{
			Id:                  "t2",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-agent",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t2.Insert(t.Context()))
		t3 := Task{
			Id:                  "t3",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-graphql",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t3.Insert(t.Context()))
		t4 := Task{
			Id:                  "t4",
			Status:              evergreen.TaskFailed,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-graphql",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t4.Insert(t.Context()))
		buildVariantTask, err := FindTaskNamesByBuildVariant(ctx, "evergreen", "ubuntu1604", 1)
		assert.NoError(t, err)
		assert.Equal(t, []string{"dist", "test-agent", "test-graphql"}, buildVariantTask)

	})
	Convey("Should only include tasks that appear on mainline commits", t, func() {
		assert.NoError(t, db.ClearCollections(Collection))
		t1 := Task{
			Id:                  "t1",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-patch-only",
			Project:             "evergreen",
			Requester:           evergreen.PatchVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t1.Insert(t.Context()))
		t2 := Task{
			Id:                  "t2",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-graphql",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t2.Insert(t.Context()))
		t3 := Task{
			Id:                  "t3",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "dist",
			Project:             "evergreen",
			Requester:           evergreen.PatchVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t3.Insert(t.Context()))
		t4 := Task{
			Id:                  "t4",
			Status:              evergreen.TaskFailed,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-something",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t4.Insert(t.Context()))
		buildVariantTasks, err := FindTaskNamesByBuildVariant(ctx, "evergreen", "ubuntu1604", 1)
		assert.NoError(t, err)
		assert.Equal(t, []string{"test-graphql", "test-something"}, buildVariantTasks)
	})

}

func TestFindByStaleRunningTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsDispatchedStaleTask": func(t *testing.T) {
			tsk := Task{
				Id:            "task",
				Status:        evergreen.TaskDispatched,
				LastHeartbeat: time.Now().Add(-time.Hour),
			}
			require.NoError(t, tsk.Insert(t.Context()))

			found, err := Find(ctx, ByStaleRunningTask(30*time.Minute))
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, tsk.Id, found[0].Id)
		},
		"ReturnsRunningStaleTask": func(t *testing.T) {
			tsk := Task{
				Id:            "task",
				Status:        evergreen.TaskStarted,
				LastHeartbeat: time.Now().Add(-time.Hour),
			}
			require.NoError(t, tsk.Insert(t.Context()))

			found, err := Find(ctx, ByStaleRunningTask(30*time.Minute))
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, tsk.Id, found[0].Id)
		},
		"ReturnsMultipleStaleTasks": func(t *testing.T) {
			tasks := []Task{
				{
					Id:            "task0",
					Status:        evergreen.TaskDispatched,
					LastHeartbeat: time.Now().Add(-time.Hour),
				},
				{
					Id:            "task1",
					Status:        evergreen.TaskStarted,
					LastHeartbeat: time.Now().Add(-time.Minute),
				},
				{
					Id:            "task2",
					Status:        evergreen.TaskStarted,
					LastHeartbeat: time.Now().Add(-time.Hour),
				},
			}
			for _, tsk := range tasks {
				require.NoError(t, tsk.Insert(t.Context()))
			}

			found, err := Find(ctx, ByStaleRunningTask(30*time.Minute))
			require.NoError(t, err)
			require.Len(t, found, 2)
			for _, tsk := range found {
				assert.True(t, utility.StringSliceContains([]string{tasks[0].Id, tasks[2].Id}, tsk.Id))
			}
		},
		"IgnoresRunningTaskWithRecentHeartbeat": func(t *testing.T) {
			tsk := Task{
				Id:            "task",
				Status:        evergreen.TaskStarted,
				LastHeartbeat: time.Now().Add(-time.Minute),
			}
			require.NoError(t, tsk.Insert(t.Context()))

			found, err := Find(ctx, ByStaleRunningTask(30*time.Minute))
			require.NoError(t, err)
			assert.Empty(t, found)
		},
		"IgnoresDisplayTasksWithNoHeartbeat": func(t *testing.T) {
			tsk := Task{
				Id:          "id",
				DisplayOnly: true,
				Status:      evergreen.TaskDispatched,
			}
			require.NoError(t, tsk.Insert(t.Context()))

			found, err := Find(ctx, ByStaleRunningTask(0))
			require.NoError(t, err)
			assert.Empty(t, found)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			tCase(t)
		})
	}
}

func TestGetTasksByVersionExecTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))
	// test that we can handle the different kinds of tasks
	t1 := Task{
		Id:            "execWithDisplayId",
		Version:       "v1",
		DisplayTaskId: utility.ToStringPtr("displayTask"),
	}
	t2 := Task{
		Id:            "notAnExec",
		Version:       "v1",
		DisplayTaskId: utility.ToStringPtr(""),
	}

	dt := Task{
		Id:             "displayTask",
		Version:        "v1",
		DisplayOnly:    true,
		ExecutionTasks: []string{"execWithDisplayId"},
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, dt))

	ctx := context.TODO()
	// execution tasks have been filtered outs
	opts := GetTasksByVersionOptions{
		Sorts: []TasksSortOrder{
			{Key: IdKey, Order: 1},
		},
	}
	tasks, count, err := GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
	// alphabetical order
	assert.Equal(t, dt.Id, tasks[0].Id)
	assert.Equal(t, t2.Id, tasks[1].Id)
}

func TestGetTasksByVersionIncludeNeverActivatedTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	inactiveTask := Task{
		Id:            "inactiveTask",
		Version:       "v1",
		ActivatedTime: utility.ZeroTime,
		DisplayTaskId: utility.ToStringPtr(""),
	}

	assert.NoError(t, inactiveTask.Insert(t.Context()))

	ctx := context.TODO()

	// inactive tasks should be included
	opts := GetTasksByVersionOptions{IncludeNeverActivatedTasks: true}
	_, count, err := GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	// inactive tasks should be excluded
	opts = GetTasksByVersionOptions{IncludeNeverActivatedTasks: false}
	_, count, err = GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestGetTasksByVersionAnnotations(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, annotations.Collection))
	t1 := Task{
		Id:            "t1",
		Version:       "v1",
		Execution:     2,
		Status:        evergreen.TaskSucceeded,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t2 := Task{
		Id:             "t2",
		Version:        "v1",
		Execution:      3,
		Status:         evergreen.TaskFailed,
		DisplayTaskId:  utility.ToStringPtr(""),
		HasAnnotations: true,
	}
	t3 := Task{
		Id:            "t3",
		Version:       "v1",
		Execution:     1,
		Status:        evergreen.TaskFailed,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3))

	ctx := context.TODO()

	opts := GetTasksByVersionOptions{}
	tasks, count, err := GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.Equal(t, "t1", tasks[0].Id)
	assert.Equal(t, evergreen.TaskSucceeded, tasks[0].DisplayStatus)
	assert.Equal(t, "t2", tasks[1].Id)
	assert.Equal(t, evergreen.TaskKnownIssue, tasks[1].DisplayStatus)
	assert.Equal(t, "t3", tasks[2].Id)
	assert.Equal(t, evergreen.TaskFailed, tasks[2].DisplayStatus)
}

func TestGetTasksByVersionBaseTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	t1 := Task{
		Id:                  "t1",
		Version:             "v1",
		BuildVariant:        "bv",
		DisplayName:         "displayName",
		Execution:           0,
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
		Revision:            "abc123",
		DisplayTaskId:       utility.ToStringPtr(""),
	}
	t2 := Task{
		Id:            "t2",
		Version:       "v2",
		BuildVariant:  "bv",
		DisplayName:   "displayName",
		Execution:     0,
		Status:        evergreen.TaskFailed,
		Requester:     evergreen.GithubPRRequester,
		Revision:      "abc123",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t3 := Task{
		Id:                  "t3",
		Version:             "v3",
		BuildVariant:        "bv",
		DisplayName:         "displayName",
		Execution:           0,
		Status:              evergreen.TaskFailed,
		RevisionOrderNumber: 2,
		Requester:           evergreen.RepotrackerVersionRequester,
		Revision:            "abc125",
		DisplayTaskId:       utility.ToStringPtr(""),
	}
	t4 := Task{
		Id:            "t4",
		Version:       "v4",
		BuildVariant:  "bv",
		DisplayName:   "displayName",
		Execution:     0,
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Requester:     evergreen.GithubPRRequester,
		Revision:      "def123",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3, t4))

	ctx := context.TODO()

	// Normal Patch builds
	opts := GetTasksByVersionOptions{
		BaseVersionID: "v1",
	}
	tasks, count, err := GetTasksByVersion(ctx, "v2", opts)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "t2", tasks[0].Id)
	assert.Equal(t, evergreen.TaskFailed, tasks[0].DisplayStatus)
	assert.NotNil(t, tasks[0].BaseTask)
	assert.Equal(t, "t1", tasks[0].BaseTask.Id)
	assert.Equal(t, t1.Status, tasks[0].BaseTask.Status)

	// Mainline builds
	opts = GetTasksByVersionOptions{
		BaseVersionID: "v1",
	}
	tasks, count, err = GetTasksByVersion(ctx, "v3", opts)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "t3", tasks[0].Id)
	assert.Equal(t, evergreen.TaskFailed, tasks[0].DisplayStatus)
	assert.NotNil(t, tasks[0].BaseTask)
	assert.Equal(t, "t1", tasks[0].BaseTask.Id)
	assert.Equal(t, t1.Status, tasks[0].BaseTask.Status)

	// With status & base status filters.
	opts = GetTasksByVersionOptions{
		BaseVersionID: "v1",
		BaseStatuses:  []string{evergreen.TaskSucceeded},
		Statuses:      []string{evergreen.TaskWillRun},
	}
	tasks, count, err = GetTasksByVersion(ctx, "v4", opts)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "t4", tasks[0].Id)
	assert.Equal(t, evergreen.TaskWillRun, tasks[0].DisplayStatus)
	assert.NotNil(t, tasks[0].BaseTask)
	assert.Equal(t, "t1", tasks[0].BaseTask.Id)
	assert.Equal(t, t1.Status, tasks[0].BaseTask.Status)

}

func TestGetTasksByVersionErrorHandling(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	// Generate 40000 tasks that average 5kb each in size. This will result in over 200MB of data.
	// This is a test to ensure we can handle the 100mb memory limitations present in the $facet stage of our query when querying for versions with a large amount of tasks.
	// See https://jira.mongodb.org/browse/SERVER-59837
	fiveKBString := strings.Repeat("a", 1024*5)
	task := Task{
		Version:             "v1",
		BuildVariant:        "bv",
		DisplayName:         "displayName",
		Execution:           0,
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
		Revision:            fiveKBString,
		DisplayTaskId:       utility.ToStringPtr(""),
	}

	for i := 0; i < 40; i++ {
		tasksToInsert := []any{}
		for j := 0; j < 1000; j++ {
			task.Id = fmt.Sprintf("t_%d_%d", i, j)
			task.BuildVariant = fmt.Sprintf("bv_%d", j)
			tasksToInsert = append(tasksToInsert, task)
		}
		assert.NoError(t, db.InsertMany(t.Context(), Collection, tasksToInsert...))
	}
	// Normal Patch builds
	opts := GetTasksByVersionOptions{}
	ctx := context.TODO()
	tasks, count, err := GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 40000, count)
	assert.Len(t, tasks, 40000)

	opts.Limit = 10
	tasks, count, err = GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 40000, count)
	assert.Len(t, tasks, 10)

}

func TestGetTaskStatusesByVersion(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:            "t1",
		Version:       "v1",
		BuildVariant:  "bv_foo",
		DisplayName:   "displayName_foo",
		Execution:     0,
		Status:        evergreen.TaskSucceeded,
		StartTime:     time.Date(2022, time.April, 7, 23, 0, 0, 0, time.UTC),
		TimeTaken:     time.Minute,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t2 := Task{
		Id:            "t2",
		Version:       "v1",
		BuildVariant:  "bv_bar",
		DisplayName:   "displayName_bar",
		Execution:     0,
		Status:        evergreen.TaskFailed,
		BaseTask:      BaseTaskInfo{Id: "t2_base", Status: evergreen.TaskFailed},
		StartTime:     time.Date(2022, time.April, 7, 23, 0, 0, 0, time.UTC),
		TimeTaken:     25 * time.Minute,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t3 := Task{
		Id:            "t3",
		Version:       "v1",
		BuildVariant:  "bv_qux",
		DisplayName:   "displayName_qux",
		Execution:     0,
		Status:        evergreen.TaskStarted,
		BaseTask:      BaseTaskInfo{Id: "t3_base", Status: evergreen.TaskSucceeded},
		StartTime:     time.Date(2021, time.November, 10, 23, 0, 0, 0, time.UTC),
		TimeTaken:     0,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t4 := Task{
		Id:            "t4",
		Version:       "v1",
		BuildVariant:  "bv_baz",
		DisplayName:   "displayName_baz",
		Execution:     0,
		Status:        evergreen.TaskSetupFailed,
		BaseTask:      BaseTaskInfo{Id: "t4_base", Status: evergreen.TaskSucceeded},
		StartTime:     time.Date(2022, time.April, 7, 23, 0, 0, 0, time.UTC),
		TimeTaken:     2 * time.Hour,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3, t4))
	ctx := context.TODO()
	tasks, err := GetTaskStatusesByVersion(ctx, "v1")
	assert.NoError(t, err)
	assert.Len(t, tasks, 4)
	assert.Equal(t, []string{evergreen.TaskFailed, evergreen.TaskSetupFailed, evergreen.TaskStarted, evergreen.TaskSucceeded}, tasks)

}

func TestGetTasksByVersionSorting(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	t1 := Task{
		Id:            "t1",
		Version:       "v1",
		BuildVariant:  "bv_foo",
		DisplayName:   "displayName_foo",
		Execution:     0,
		Status:        evergreen.TaskSucceeded,
		BaseTask:      BaseTaskInfo{Id: "t1_base", Status: evergreen.TaskSucceeded},
		StartTime:     time.Date(2022, time.April, 7, 23, 0, 0, 0, time.UTC),
		TimeTaken:     time.Minute,
		DisplayTaskId: utility.ToStringPtr(""),
	}

	t2 := Task{
		Id:            "t2",
		Version:       "v1",
		BuildVariant:  "bv_bar",
		DisplayName:   "displayName_bar",
		Execution:     0,
		Status:        evergreen.TaskFailed,
		BaseTask:      BaseTaskInfo{Id: "t2_base", Status: evergreen.TaskFailed},
		StartTime:     time.Date(2022, time.April, 7, 23, 0, 0, 0, time.UTC),
		TimeTaken:     25 * time.Minute,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t3 := Task{
		Id:            "t3",
		Version:       "v1",
		BuildVariant:  "bv_qux",
		DisplayName:   "displayName_qux",
		Execution:     0,
		Status:        evergreen.TaskStarted,
		BaseTask:      BaseTaskInfo{Id: "t3_base", Status: evergreen.TaskSucceeded},
		StartTime:     time.Date(2021, time.November, 10, 23, 0, 0, 0, time.UTC),
		TimeTaken:     0,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t4 := Task{
		Id:            "t4",
		Version:       "v1",
		BuildVariant:  "bv_baz",
		DisplayName:   "displayName_baz",
		Execution:     0,
		Status:        evergreen.TaskSetupFailed,
		BaseTask:      BaseTaskInfo{Id: "t4_base", Status: evergreen.TaskSucceeded},
		StartTime:     time.Date(2022, time.April, 7, 23, 0, 0, 0, time.UTC),
		TimeTaken:     2 * time.Hour,
		DisplayTaskId: utility.ToStringPtr(""),
	}

	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3, t4))

	ctx := context.TODO()

	// Sort by display name, asc
	opts := GetTasksByVersionOptions{
		Sorts: []TasksSortOrder{
			{Key: DisplayNameKey, Order: 1},
		},
	}
	tasks, count, err := GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 4, count)
	assert.Equal(t, "t2", tasks[0].Id)
	assert.Equal(t, "t4", tasks[1].Id)
	assert.Equal(t, "t1", tasks[2].Id)
	assert.Equal(t, "t3", tasks[3].Id)

	// Sort by build variant name, asc
	opts = GetTasksByVersionOptions{
		Sorts: []TasksSortOrder{
			{Key: BuildVariantKey, Order: 1},
		},
	}
	tasks, count, err = GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 4, count)
	assert.Equal(t, "t2", tasks[0].Id)
	assert.Equal(t, "t4", tasks[1].Id)
	assert.Equal(t, "t1", tasks[2].Id)
	assert.Equal(t, "t3", tasks[3].Id)

	// Sort by display status, asc
	opts = GetTasksByVersionOptions{
		Sorts: []TasksSortOrder{
			{Key: DisplayStatusKey, Order: 1},
		},
	}
	tasks, count, err = GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 4, count)
	assert.Equal(t, "t2", tasks[0].Id)
	assert.Equal(t, "t4", tasks[1].Id)
	assert.Equal(t, "t3", tasks[2].Id)
	assert.Equal(t, "t1", tasks[3].Id)

	// Sort by base task status, asc
	opts = GetTasksByVersionOptions{
		Sorts: []TasksSortOrder{
			{Key: BaseTaskStatusKey, Order: 1},
		},
	}
	tasks, count, err = GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 4, count)
	assert.Equal(t, "t2", tasks[0].Id)
	assert.Equal(t, "t1", tasks[1].Id)
	assert.Equal(t, "t3", tasks[2].Id)
	assert.Equal(t, "t4", tasks[3].Id)

	// Sort by duration, asc
	opts = GetTasksByVersionOptions{
		Sorts: []TasksSortOrder{
			{Key: TimeTakenKey, Order: 1},
		},
	}
	tasks, count, err = GetTasksByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 4, count)
	assert.Equal(t, "t1", tasks[0].Id)
	assert.Equal(t, "t2", tasks[1].Id)
	assert.Equal(t, "t4", tasks[2].Id)
	assert.Equal(t, "t3", tasks[3].Id)
}

func TestGetTaskStatsByVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:               "t1",
		Version:          "v1",
		Execution:        0,
		Status:           evergreen.TaskStarted,
		ExpectedDuration: time.Minute,
		StartTime:        time.Date(2009, time.November, 10, 12, 0, 0, 0, time.UTC),
		DisplayTaskId:    utility.ToStringPtr(""),
	}
	t2 := Task{
		Id:               "t2",
		Version:          "v1",
		Execution:        0,
		Status:           evergreen.TaskStarted,
		ExpectedDuration: 150 * time.Minute,
		StartTime:        time.Date(2009, time.November, 10, 12, 0, 0, 0, time.UTC),
		DisplayTaskId:    utility.ToStringPtr(""),
	}
	t3 := Task{
		Id:            "t3",
		Version:       "v1",
		Execution:     1,
		Status:        evergreen.TaskSucceeded,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t4 := Task{
		Id:            "t4",
		Version:       "v1",
		Execution:     1,
		Status:        evergreen.TaskFailed,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t5 := Task{
		Id:            "t5",
		Version:       "v1",
		Execution:     2,
		Status:        evergreen.TaskStatusPending,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t6 := Task{
		Id:            "t6",
		Version:       "v1",
		Execution:     2,
		Status:        evergreen.TaskFailed,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3, t4, t5, t6))
	ctx := context.TODO()
	opts := GetTasksByVersionOptions{}
	stats, err := GetTaskStatsByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Len(t, stats.Counts, 4)
	assert.True(t, stats.ETA.Equal(time.Date(2009, time.November, 10, 14, 30, 0, 0, time.UTC)))

	assert.NoError(t, db.ClearCollections(Collection))
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t3, t4, t5, t6))
	stats, err = GetTaskStatsByVersion(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.Nil(t, stats.ETA)
}

func TestGetGroupedTaskStatsByVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))

	t1 := Task{
		Id:                      "t1",
		Version:                 "v1",
		Execution:               0,
		Status:                  evergreen.TaskSucceeded,
		BuildVariant:            "bv1",
		BuildVariantDisplayName: "Build Variant 1",
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t2 := Task{
		Id:                      "t2",
		Version:                 "v1",
		Execution:               0,
		Status:                  evergreen.TaskFailed,
		BuildVariant:            "bv1",
		BuildVariantDisplayName: "Build Variant 1",
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t3 := Task{
		Id:                      "t3",
		Version:                 "v1",
		Execution:               1,
		Status:                  evergreen.TaskSucceeded,
		BuildVariant:            "bv1",
		BuildVariantDisplayName: "Build Variant 1",
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t4 := Task{
		Id:                      "t4",
		Version:                 "v1",
		Execution:               1,
		Status:                  evergreen.TaskFailed,
		BuildVariant:            "bv2",
		BuildVariantDisplayName: "Build Variant 2",
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t5 := Task{
		Id:                      "t5",
		Version:                 "v1",
		Execution:               2,
		Status:                  evergreen.TaskStatusPending,
		BuildVariant:            "bv2",
		BuildVariantDisplayName: "Build Variant 2",
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t6 := Task{
		Id:                      "t6",
		Version:                 "v1",
		Execution:               2,
		Status:                  evergreen.TaskFailed,
		BuildVariant:            "bv2",
		BuildVariantDisplayName: "Build Variant 2",
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3, t4, t5, t6))

	t.Run("Fetch GroupedTaskStats with no filters applied", func(t *testing.T) {

		ctx := context.TODO()
		opts := GetTasksByVersionOptions{}
		variants, err := GetGroupedTaskStatsByVersion(ctx, "v1", opts)
		assert.NoError(t, err)
		assert.Len(t, variants, 2)
		expectedValues := []*GroupedTaskStatusCount{
			{
				Variant:     "bv1",
				DisplayName: "Build Variant 1",
				StatusCounts: []*StatusCount{
					{
						Status: evergreen.TaskFailed,
						Count:  1,
					},
					{
						Status: evergreen.TaskSucceeded,
						Count:  2,
					},
				},
			},
			{
				Variant:     "bv2",
				DisplayName: "Build Variant 2",
				StatusCounts: []*StatusCount{
					{
						Status: evergreen.TaskFailed,
						Count:  2,
					},
					{
						Status: evergreen.TaskStatusPending,
						Count:  1,
					},
				},
			},
		}

		compareGroupedTaskStatusCounts(t, expectedValues, variants)
	})
	t.Run("Fetch GroupedTaskStats with filters applied", func(t *testing.T) {

		opts := GetTasksByVersionOptions{
			Variants: []string{"bv1"},
		}
		ctx := context.TODO()
		variants, err := GetGroupedTaskStatsByVersion(ctx, "v1", opts)
		assert.NoError(t, err)
		assert.Len(t, variants, 1)
		expectedValues := []*GroupedTaskStatusCount{
			{
				Variant:     "bv1",
				DisplayName: "Build Variant 1",
				StatusCounts: []*StatusCount{
					{
						Status: evergreen.TaskFailed,
						Count:  1,
					},
					{
						Status: evergreen.TaskSucceeded,
						Count:  2,
					},
				},
			},
		}
		compareGroupedTaskStatusCounts(t, expectedValues, variants)
	})

}

func compareGroupedTaskStatusCounts(t *testing.T, expected, actual []*GroupedTaskStatusCount) {
	// reflect.DeepEqual does not work here, it was failing because of the slice ptr values for StatusCounts.
	for i, e := range expected {
		a := actual[i]
		assert.Equal(t, e.Variant, a.Variant)
		assert.Equal(t, e.DisplayName, a.DisplayName)
		assert.Equal(t, len(e.StatusCounts), len(a.StatusCounts))
		for j, expectedCount := range e.StatusCounts {
			actualCount := a.StatusCounts[j]
			assert.Equal(t, expectedCount.Status, actualCount.Status)
			assert.Equal(t, expectedCount.Count, actualCount.Count)
		}
	}
}

func TestGetBaseStatusesForActivatedTasks(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:            "t1",
		Version:       "v1",
		Status:        evergreen.TaskStarted,
		ActivatedTime: time.Time{},
		DisplayName:   "task_1",
		BuildVariant:  "bv_1",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t2 := Task{
		Id:            "t2",
		Version:       "v1",
		Status:        evergreen.TaskSetupFailed,
		ActivatedTime: time.Time{},
		DisplayName:   "task_2",
		BuildVariant:  "bv_2",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t3 := Task{
		Id:            "t1_base",
		Version:       "v1_base",
		Status:        evergreen.TaskSucceeded,
		ActivatedTime: time.Time{},
		DisplayName:   "task_1",
		BuildVariant:  "bv_1",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t4 := Task{
		Id:            "t2_base",
		Version:       "v1_base",
		Status:        evergreen.TaskStarted,
		ActivatedTime: time.Time{},
		DisplayName:   "task_2",
		BuildVariant:  "bv_2",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t5 := Task{
		Id:            "only_on_base",
		Version:       "v1_base",
		Status:        evergreen.TaskFailed,
		ActivatedTime: time.Time{},
		DisplayName:   "only_on_base",
		BuildVariant:  "bv_2",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3, t4, t5))
	ctx := context.TODO()
	statuses, err := GetBaseStatusesForActivatedTasks(ctx, "v1", "v1_base")
	assert.NoError(t, err)
	assert.Len(t, statuses, 2)
	assert.Equal(t, evergreen.TaskStarted, statuses[0])
	assert.Equal(t, evergreen.TaskSucceeded, statuses[1])

	assert.NoError(t, db.ClearCollections(Collection))
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t5))
	statuses, err = GetBaseStatusesForActivatedTasks(ctx, "v1", "v1_base")
	assert.NoError(t, err)
	assert.Empty(t, statuses)
}

func TestHasMatchingTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:                      "t1",
		Version:                 "v1",
		BuildVariant:            "bv1",
		BuildVariantDisplayName: "Build Variant 1",
		Execution:               0,
		Status:                  evergreen.TaskSucceeded,
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t2 := Task{
		Id:                      "t2",
		Version:                 "v1",
		BuildVariant:            "bv1",
		BuildVariantDisplayName: "Build Variant 1",
		Execution:               0,
		Status:                  evergreen.TaskFailed,
		DisplayTaskId:           utility.ToStringPtr(""),
	}

	t3 := Task{
		Id:                      "t3",
		Version:                 "v1",
		BuildVariant:            "bv2",
		BuildVariantDisplayName: "Build Variant 2",
		Execution:               1,
		Status:                  evergreen.TaskSucceeded,
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t4 := Task{
		Id:                      "t4",
		Version:                 "v1",
		BuildVariant:            "bv2",
		BuildVariantDisplayName: "Build Variant 2",
		Execution:               1,
		Status:                  evergreen.TaskFailed,
		DisplayTaskId:           utility.ToStringPtr(""),
	}

	t5 := Task{
		Id:                      "t5",
		Version:                 "v1",
		BuildVariant:            "bv3",
		BuildVariantDisplayName: "Build Variant 3",
		Execution:               2,
		Status:                  evergreen.TaskStatusPending,
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	t6 := Task{
		Id:                      "t6",
		Version:                 "v1",
		BuildVariant:            "bv3",
		BuildVariantDisplayName: "Build Variant 3",
		Execution:               2,
		Status:                  evergreen.TaskFailed,
		DisplayTaskId:           utility.ToStringPtr(""),
	}

	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3, t4, t5, t6))
	opts := HasMatchingTasksOptions{
		Statuses: []string{evergreen.TaskFailed},
	}
	ctx := context.TODO()
	hasMatchingTasks, err := HasMatchingTasks(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.True(t, hasMatchingTasks)

	opts.Statuses = []string{evergreen.TaskWillRun}

	hasMatchingTasks, err = HasMatchingTasks(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.False(t, hasMatchingTasks)

	opts = HasMatchingTasksOptions{
		Variants: []string{"bv1"},
	}
	hasMatchingTasks, err = HasMatchingTasks(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.True(t, hasMatchingTasks)

	opts.Variants = []string{"Build Variant 2"}
	hasMatchingTasks, err = HasMatchingTasks(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.True(t, hasMatchingTasks)

	opts.Variants = []string{"DNE"}
	hasMatchingTasks, err = HasMatchingTasks(ctx, "v1", opts)
	assert.NoError(t, err)
	assert.False(t, hasMatchingTasks)
}

func TestFindAllUnmarkedDependenciesToBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection))

	t1 := &Task{
		Id:     "t1",
		Status: evergreen.TaskFailed,
	}

	tasks := []Task{
		{
			Id: "t2",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
		{
			Id: "t3",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
					Status: evergreen.TaskFailed,
				},
			},
		},
		{
			Id: "t4",
			DependsOn: []Dependency{
				{
					TaskId:       "t1",
					Status:       evergreen.TaskSucceeded,
					Unattainable: true,
				},
			},
		},
		{
			Id: "t5",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
					Status: evergreen.TaskFailed,
				},
				{
					TaskId: "t2",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
	}
	for _, task := range tasks {
		assert.NoError(task.Insert(t.Context()))
	}

	deps, err := FindAllDependencyTasksToModify(ctx, []Task{*t1}, true, false)
	assert.NoError(err)
	require.Len(t, deps, 1)
	assert.Equal("t2", deps[0].Id)
}

func TestFindAllUnattainableDependenciesToUnbock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection))

	t1 := &Task{Id: "t1"}
	tasks := []Task{
		{
			Id: "t2",
			DependsOn: []Dependency{
				{
					TaskId:       "t1",
					Unattainable: true,
				},
			},
		},
		{
			Id: "t3",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
				},
				{
					TaskId:       "t2",
					Unattainable: true,
				},
			},
		},
	}

	for _, task := range tasks {
		assert.NoError(task.Insert(t.Context()))
	}

	deps, err := FindAllDependencyTasksToModify(ctx, []Task{*t1}, false, false)
	assert.NoError(err)
	require.Len(t, deps, 1)
	assert.Equal("t2", deps[0].Id)
}

func TestCountNumExecutionsForInterval(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, OldCollection))

	now := time.Now()
	earlier := time.Now().Add(-time.Hour)
	reallyEarly := now.Add(-12 * time.Hour)
	tasks := []Task{
		{
			Id:           "notFinished",
			Project:      "myProject",
			Status:       evergreen.TaskStarted,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			Execution:    1,
		},
		{
			Id:           "finished",
			Project:      "myProject",
			Status:       evergreen.TaskFailed,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			FinishTime:   now,
			Execution:    1,
		},
		{
			Id:           "finishedEarlier",
			Project:      "myProject",
			Status:       evergreen.TaskFailed,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			FinishTime:   earlier,
			Execution:    1,
		},
		{
			Id:           "patch",
			Project:      "myProject",
			Status:       evergreen.TaskSucceeded,
			Requester:    evergreen.PatchVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			FinishTime:   now,
			Execution:    1,
		},
		{
			Id:           "tooEarly",
			Project:      "myProject",
			Status:       evergreen.TaskSucceeded,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task1",
			FinishTime:   reallyEarly,
			Execution:    1,
		},
		{
			Id:           "wrongTask",
			Project:      "myProject",
			Status:       evergreen.TaskFailed,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv1",
			DisplayName:  "task2",
			FinishTime:   now,
			Execution:    1,
		},
		{
			Id:           "wrongVariant",
			Project:      "myProject",
			Status:       evergreen.TaskFailed,
			Requester:    evergreen.RepotrackerVersionRequester,
			BuildVariant: "bv2",
			DisplayName:  "task1",
			FinishTime:   now,
			Execution:    1,
		},
	}
	for _, each := range tasks {
		assert.NoError(t, each.Insert(t.Context()))
		each.Execution = 0
		// Duplicate everything for the old task collection to ensure this is working.
		assert.NoError(t, db.Insert(t.Context(), OldCollection, each))
	}

	for testName, test := range map[string]func(*testing.T){
		"nothingInRange": func(t *testing.T) {
			input := NumExecutionsForIntervalInput{
				ProjectId:    "myProject",
				BuildVarName: "bv1",
				TaskName:     "task1",
				StartTime:    time.Now().Add(-20 * time.Hour),
				EndTime:      time.Now().Add(-18 * time.Hour),
			}
			numExecutions, err := CountNumExecutionsForInterval(t.Context(), input)
			assert.NoError(t, err)
			assert.Equal(t, 0, numExecutions)
		},
		"lotsInRange": func(t *testing.T) {
			input := NumExecutionsForIntervalInput{
				ProjectId:    "myProject",
				BuildVarName: "bv1",
				TaskName:     "task1",
				StartTime:    now.Add(-20 * time.Hour),
			}
			// Should include the finished tasks in both new and old.
			numExecutions, err := CountNumExecutionsForInterval(t.Context(), input)
			assert.NoError(t, err)
			assert.Equal(t, 6, numExecutions)
		},
		"lessInRange": func(t *testing.T) {
			input := NumExecutionsForIntervalInput{
				ProjectId:    "myProject",
				BuildVarName: "bv1",
				TaskName:     "task1",
				StartTime:    now.Add(-2 * time.Hour),
			}
			// Should include the finished tasks in both new and old except reallyEarly.
			numExecutions, err := CountNumExecutionsForInterval(t.Context(), input)
			assert.NoError(t, err)
			assert.Equal(t, 4, numExecutions)
		},
		"onlyPatches": func(t *testing.T) {
			input := NumExecutionsForIntervalInput{
				ProjectId:    "myProject",
				BuildVarName: "bv1",
				TaskName:     "task1",
				Requesters:   evergreen.PatchRequesters,
				StartTime:    now.Add(-2 * time.Hour),
			}
			// Should include the patch requester.
			numExecutions, err := CountNumExecutionsForInterval(t.Context(), input)
			assert.NoError(t, err)
			assert.Equal(t, 2, numExecutions)
		},
	} {
		t.Run(testName, test)
	}
}

func TestHasActivatedDependentTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.Clear(Collection))
	t1 := Task{
		Id:        "activeDependent",
		Activated: true,
		DependsOn: []Dependency{
			{TaskId: "current"},
		},
	}
	t2 := Task{
		Id: "inactiveDependent",
		DependsOn: []Dependency{
			{TaskId: "inactive"},
		},
	}
	t3 := Task{
		Id:        "manyDependencies",
		Activated: true,
		DependsOn: []Dependency{
			{TaskId: "current"},
			{TaskId: "secondTask"},
		},
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3))

	hasDependentTasks, err := HasActivatedDependentTasks(ctx, "current")
	assert.NoError(t, err)
	assert.True(t, hasDependentTasks)

	hasDependentTasks, err = HasActivatedDependentTasks(ctx, "secondTask")
	assert.NoError(t, err)
	assert.True(t, hasDependentTasks)

	// Tasks overriding dependencies don't count as dependent.
	assert.NoError(t, t3.SetOverrideDependencies(ctx, "me"))
	hasDependentTasks, err = HasActivatedDependentTasks(ctx, "secondTask")
	assert.NoError(t, err)
	assert.False(t, hasDependentTasks)
	assert.False(t, t3.DependenciesMetTime.IsZero())

	hasDependentTasks, err = HasActivatedDependentTasks(ctx, "inactive")
	assert.NoError(t, err)
	assert.False(t, hasDependentTasks)

}

func TestActivateTasksUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		require.NoError(t, db.Clear(Collection))
	}()

	caller := "me"
	activationTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	t.Run("NoDependencies", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		t0 := Task{
			Id: "t0",
		}

		require.NoError(t, t0.Insert(t.Context()))
		assert.NoError(t, activateTasks(ctx, []string{t0.Id}, caller, activationTime))
		dbTask, err := FindOneId(ctx, t0.Id)
		assert.NoError(t, err)
		assert.True(t, dbTask.Activated)
		assert.Equal(t, caller, dbTask.ActivatedBy)
		assert.True(t, activationTime.Equal(dbTask.ActivatedTime))
		assert.False(t, dbTask.UnattainableDependency)
	})
	t.Run("DisabledTask", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		require.NoError(t, db.ClearCollections(Collection, distro.Collection))

		t0 := Task{
			Id:                "t0",
			Status:            evergreen.TaskUndispatched,
			Priority:          evergreen.DisabledTaskPriority,
			ExecutionPlatform: ExecutionPlatformHost,
			DistroId:          "d",
		}
		d := distro.Distro{
			Id: "d",
		}

		require.NoError(t, d.Insert(ctx))
		require.NoError(t, t0.Insert(t.Context()))
		assert.NoError(t, activateTasks(ctx, []string{t0.Id}, caller, activationTime))

		tasks, err := FindHostSchedulable(ctx, "d")
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		assert.NoError(t, err)
		dbTask := tasks[0]
		assert.True(t, dbTask.Activated)
		assert.Equal(t, caller, dbTask.ActivatedBy)
		assert.True(t, activationTime.Equal(dbTask.ActivatedTime))
		assert.False(t, dbTask.UnattainableDependency)
		assert.EqualValues(t, 0, dbTask.Priority)
	})
}

func TestFindGeneratedTasksFromID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	checkGeneratedTaskInfo := func(t *testing.T, expected Task, actual GeneratedTaskInfo) {
		assert.Equal(t, expected.Id, actual.TaskID)
		assert.Equal(t, expected.DisplayName, actual.TaskName)
		assert.Equal(t, expected.BuildId, actual.BuildID)
		assert.Equal(t, expected.BuildVariant, actual.BuildVariant)
		assert.Equal(t, expected.BuildVariantDisplayName, actual.BuildVariantDisplayName)
	}
	for tName, tCase := range map[string]func(t *testing.T, generatorID string, generated []Task){
		"ReturnsSingleResult": func(t *testing.T, generatorID string, generated []Task) {
			require.NoError(t, generated[0].Insert(t.Context()))
			res, err := FindGeneratedTasksFromID(ctx, generatorID)
			require.NoError(t, err)
			require.Len(t, res, 1)
			checkGeneratedTaskInfo(t, generated[0], res[0])
		},
		"ReturnsMultipleResults": func(t *testing.T, generatorID string, generated []Task) {
			for _, tsk := range generated {
				require.NoError(t, tsk.Insert(t.Context()))
			}
			res, err := FindGeneratedTasksFromID(ctx, generatorID)
			require.NoError(t, err)
			require.Len(t, res, len(generated))
			for i := 0; i < len(generated); i++ {
				checkGeneratedTaskInfo(t, generated[i], res[i])
			}
		},
		"ReturnsNoResultsForNoMatch": func(t *testing.T, generatorID string, generated []Task) {
			for _, tsk := range generated {
				require.NoError(t, tsk.Insert(t.Context()))
			}
			res, err := FindGeneratedTasksFromID(ctx, "nonexistent")
			require.NoError(t, err)
			require.Empty(t, res)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			const generatorID = "generator"
			generated := []Task{
				{
					Id:                      "generated_task0",
					GeneratedBy:             generatorID,
					BuildId:                 "build_id0",
					BuildVariant:            "build-variant0",
					BuildVariantDisplayName: "first build variant",
				},
				{
					Id:                      "generated_task1",
					GeneratedBy:             generatorID,
					BuildId:                 "build_id1",
					BuildVariant:            "build-variant1",
					BuildVariantDisplayName: "second build variant",
				},
			}
			tCase(t, generatorID, generated)
		})
	}
}

func TestGetPendingGenerateTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	t1 := Task{
		Id:                         "should_sum1",
		Status:                     evergreen.TaskStarted,
		EstimatedNumGeneratedTasks: utility.ToIntPtr(1),
		GeneratedTasks:             false,
	}
	t2 := Task{
		Id:                         "should_sum2",
		Status:                     evergreen.TaskDispatched,
		EstimatedNumGeneratedTasks: utility.ToIntPtr(2),
	}
	t3 := Task{
		Id:                         "should_not_sum1",
		Status:                     evergreen.TaskStarted,
		EstimatedNumGeneratedTasks: utility.ToIntPtr(3),
		GeneratedTasks:             true,
	}
	t4 := Task{
		Id:                         "should_not_sum2",
		Status:                     evergreen.TaskFailed,
		EstimatedNumGeneratedTasks: utility.ToIntPtr(4),
		GeneratedTasks:             false,
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3, t4))

	ctx := context.TODO()
	numPending, err := GetPendingGenerateTasks(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, numPending)
}

func TestGetNoPendingGenerateTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	ctx := context.TODO()
	numPending, err := GetPendingGenerateTasks(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, numPending)
}

func TestGetLatestTaskFromImage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(Collection, distro.Collection))
	imageID := "distro"
	d1 := &distro.Distro{
		Id:      "distro-1",
		ImageID: imageID,
	}
	require.NoError(t, d1.Insert(ctx))
	d2 := &distro.Distro{
		Id:      "distro-2",
		ImageID: imageID,
	}
	require.NoError(t, d2.Insert(ctx))
	d3 := &distro.Distro{
		Id:      "distro-3",
		ImageID: imageID,
	}
	require.NoError(t, d3.Insert(ctx))
	tasks := []Task{
		{Id: "t0", FinishTime: time.Date(2023, time.February, 1, 10, 30, 15, 0, time.UTC), DistroId: d1.Id},
		{Id: "t1", FinishTime: time.Date(2023, time.January, 1, 10, 30, 15, 0, time.UTC), DistroId: d2.Id},
		{Id: "t2", FinishTime: time.Date(2023, time.March, 1, 10, 30, 15, 0, time.UTC), DistroId: d3.Id},
		{Id: "t3", FinishTime: time.Date(2024, time.January, 1, 10, 30, 15, 0, time.UTC), DistroId: "rando"},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}
	latestTask, err := GetLatestTaskFromImage(ctx, imageID)
	require.NoError(t, err)
	require.NotNil(t, latestTask)
	assert.Equal(t, "t2", latestTask.Id)
}
