package task

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
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
	err := db.Aggregate(Collection, aggregation, &dbTasks)
	assert.NoError(t, err)
	assert.Equal(t, expected, dbTasks[0].DisplayStatus)
	assert.Equal(t, expected, toCheck.GetDisplayStatus())
}

func TestFindMergeTaskForVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := &Task{
		Id:               "t1",
		Version:          "abcdef123456",
		CommitQueueMerge: false,
	}
	assert.NoError(t, t1.Insert())

	_, err := FindMergeTaskForVersion("abcdef123456")
	assert.Error(t, err)
	assert.True(t, adb.ResultsNotFound(err))

	t2 := &Task{
		Id:               "t2",
		Version:          "abcdef123456",
		CommitQueueMerge: true,
	}
	assert.NoError(t, t2.Insert())
	t2Db, err := FindMergeTaskForVersion("abcdef123456")
	assert.NoError(t, err)
	assert.Equal(t, "t2", t2Db.Id)
}

func TestFindTasksByIds(t *testing.T) {
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
				So(task.Insert(), ShouldBeNil)
			}

			dbTasks, err := Find(ByIds([]string{"one", "two"}))
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 2)
			So(dbTasks[0].Id, ShouldNotEqual, "three")
			So(dbTasks[1].Id, ShouldNotEqual, "three")
		})
	})
}
func TestDisplayTasksByVersion(t *testing.T) {
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
				So(task.Insert(), ShouldBeNil)
			}

			dbTasks, err := FindAll(db.Query(DisplayTasksByVersion("v1")))
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
	assert.NoError(t, db.InsertMany(Collection, displayTask, regularTask, wrongVersionTask, execTask, legacyTask))

	tasks, err := Find(NonExecutionTasksByVersions([]string{"v1", "v2"}))
	assert.NoError(t, err)
	assert.Len(t, tasks, 3) // doesn't include wrong version or execution task with DisplayTaskId cached
	for _, task := range tasks {
		assert.NotEqual(t, task.Id, "exec_task")
		assert.NotEqual(t, task.Version, "lame_version")
	}
}

func TestFailedTasksByVersion(t *testing.T) {
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
				So(task.Insert(), ShouldBeNil)
			}

			dbTasks, err := Find(FailedTasksByVersion("v1"))
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 2)
			So(dbTasks[0].Id, ShouldNotEqual, "three")
			So(dbTasks[1].Id, ShouldNotEqual, "three")
		})
	})
}

func TestFindTasksByBuildIdAndGithubChecks(t *testing.T) {
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
		assert.NoError(t, task.Insert())
	}
	dbTasks, err := FindAll(db.Query(ByBuildIdAndGithubChecks("b1")))
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 1)
	dbTasks, err = FindAll(db.Query(ByBuildIdAndGithubChecks("b2")))
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 2)
	dbTasks, err = FindAll(db.Query(ByBuildIdAndGithubChecks("b3")))
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 0)
}

func TestFindOneIdAndExecutionWithDisplayStatus(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))
	taskDoc := Task{
		Id:        "task",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
	}
	assert.NoError(taskDoc.Insert())
	task, err := FindOneIdAndExecutionWithDisplayStatus(taskDoc.Id, utility.ToIntPtr(0))
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(task.DisplayStatus, evergreen.TaskWillRun)

	// Should fetch tasks from the old collection
	assert.NoError(taskDoc.Archive())
	task, err = FindOneOldByIdAndExecution(taskDoc.Id, 0)
	assert.NoError(err)
	assert.NotNil(task)
	task, err = FindOneIdAndExecutionWithDisplayStatus(taskDoc.Id, utility.ToIntPtr(0))
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(task.OldTaskId, taskDoc.Id)

	// Should fetch recent executions by default
	task, err = FindOneIdAndExecutionWithDisplayStatus(taskDoc.Id, nil)
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(task.Execution, 1)
	assert.Equal(task.DisplayStatus, evergreen.TaskWillRun)

	taskDoc = Task{
		Id:        "task2",
		Status:    evergreen.TaskUndispatched,
		Activated: false,
	}
	assert.NoError(taskDoc.Insert())
	task, err = FindOneIdAndExecutionWithDisplayStatus(taskDoc.Id, utility.ToIntPtr(0))
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(task.DisplayStatus, evergreen.TaskUnscheduled)
}

func TestFindOldTasksByID(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))

	taskDoc := Task{
		Id: "task",
	}
	assert.NoError(taskDoc.Insert())
	assert.NoError(taskDoc.Archive())
	taskDoc.Execution += 1
	assert.NoError(taskDoc.Archive())
	taskDoc.Execution += 1

	tasks, err := FindOld(ByOldTaskID("task"))
	assert.NoError(err)
	assert.Len(tasks, 2)
	assert.Equal(0, tasks[0].Execution)
	assert.Equal("task_0", tasks[0].Id)
	assert.Equal("task", tasks[0].OldTaskId)
	assert.Equal(1, tasks[1].Execution)
	assert.Equal("task_1", tasks[1].Id)
	assert.Equal("task", tasks[1].OldTaskId)
}

func TestFindAllFirstExecution(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, OldCollection))
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1", Execution: 1},
		{Id: "t2", DisplayOnly: true},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}
	oldTask := Task{Id: MakeOldID("t1", 0)}
	require.NoError(t, db.Insert(OldCollection, &oldTask))

	foundTasks, err := FindAllFirstExecution(All)
	assert.NoError(t, err)
	assert.Len(t, foundTasks, 3)
	expectedIDs := []string{"t0", MakeOldID("t1", 0), "t2"}
	for _, task := range foundTasks {
		assert.Contains(t, expectedIDs, task.Id)
		assert.Equal(t, 0, task.Execution)
	}
}

func TestWithinTimePeriodProjectFilter(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))

	taskDocs := []Task{
		{
			Id:      "task1",
			Project: "proj",
		},
		{
			Id:      "task2",
			Project: "other",
		},
	}

	for _, taskDoc := range taskDocs {
		assert.NoError(taskDoc.Insert())
	}

	tasks, err := Find(WithinTimePeriod(time.Time{}, time.Time{}, "proj", []string{}))
	assert.NoError(err)
	assert.Len(tasks, 1)
	assert.Equal(tasks[0].Id, "task1")
}

func TestWithinTimePeriodDatesFilter(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))

	taskDocs := []Task{
		{
			Id:         "task1",
			FinishTime: time.Now().AddDate(0, 0, -2), // Should match
			StartTime:  time.Now().AddDate(0, 0, -5), // Shouldn't match
		},
		{
			Id:         "task2",
			FinishTime: time.Now().AddDate(0, 0, -2), // Should match
			StartTime:  time.Now().AddDate(0, 0, -3), // Should match
		},
		{
			Id:         "task3",
			FinishTime: time.Now(),                   // Shouldn't match
			StartTime:  time.Now().AddDate(0, 0, -3), // Should match
		},
	}

	for _, taskDoc := range taskDocs {
		assert.NoError(taskDoc.Insert())
	}

	tasks, err := Find(WithinTimePeriod(
		time.Now().AddDate(0, 0, -4), time.Now().AddDate(0, 0, -1), "", []string{}))
	assert.NoError(err)
	assert.Len(tasks, 1)
	assert.Equal(tasks[0].Id, "task2")
}

func TestWithinTimePeriodStatusesFilter(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))

	taskDocs := []Task{
		{
			Id:     "task1",
			Status: "A",
		},
		{
			Id:     "task2",
			Status: "B",
		},
		{
			Id:     "task3",
			Status: "C",
		},
	}

	for _, taskDoc := range taskDocs {
		assert.NoError(taskDoc.Insert())
	}

	statuses := []string{"A", "B"}

	tasks, err := Find(WithinTimePeriod(time.Time{}, time.Time{}, "", statuses))
	assert.NoError(err)
	assert.Len(tasks, 2)
	assert.Subset([]string{tasks[0].Status, tasks[1].Status}, statuses)
}

func TestFindOneIdOldOrNew(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(Collection, OldCollection))

	taskDoc := Task{
		Id:     "task",
		Status: evergreen.TaskSucceeded,
	}
	require.NoError(taskDoc.Insert())
	require.NoError(taskDoc.Archive())
	result0 := testresult.TestResult{
		ID:        mgobson.NewObjectId(),
		TaskID:    "task",
		Execution: 0,
	}
	result1 := testresult.TestResult{
		ID:        mgobson.NewObjectId(),
		TaskID:    "task",
		Execution: 1,
	}
	require.NoError(result0.Insert())
	require.NoError(result1.Insert())

	task00, err := FindOneIdOldOrNew("task", 0)
	assert.NoError(err)
	require.NotNil(task00)
	assert.Equal("task_0", task00.Id)
	assert.Equal(0, task00.Execution)

	task01, err := FindOneIdOldOrNew("task", 1)
	assert.NoError(err)
	require.NotNil(task01)
	assert.Equal("task", task01.Id)
	assert.Equal(1, task01.Execution)
}

func TestAddHostCreateDetails(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	task := Task{Id: "t1", Execution: 0}
	assert.NoError(t, task.Insert())
	errToSave := errors.Wrapf(errors.New("InsufficientCapacityError"), "error trying to start host")
	assert.NoError(t, AddHostCreateDetails(task.Id, "h1", 0, errToSave))
	dbTask, err := FindOneId(task.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dbTask)
	require.Len(t, dbTask.HostCreateDetails, 1)
	assert.Equal(t, dbTask.HostCreateDetails[0].HostId, "h1")
	assert.Contains(t, dbTask.HostCreateDetails[0].Error, "InsufficientCapacityError")

	assert.NoError(t, AddHostCreateDetails(task.Id, "h2", 0, errToSave))
	dbTask, err = FindOneId(task.Id)
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
	assert.NoError(t, t1.Insert())
	checkStatuses(t, evergreen.TaskSucceeded, t1)
	t2 := Task{
		Id:        "t2",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
	}
	assert.NoError(t, t2.Insert())
	checkStatuses(t, evergreen.TaskWillRun, t2)
	t3 := Task{
		Id:        "t3",
		Status:    evergreen.TaskFailed,
		Activated: true,
	}
	assert.NoError(t, t3.Insert())
	checkStatuses(t, evergreen.TaskFailed, t3)
	t4 := Task{
		Id:     "t4",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type: evergreen.CommandTypeSetup,
		},
	}
	assert.NoError(t, t4.Insert())
	checkStatuses(t, evergreen.TaskSetupFailed, t4)
	t5 := Task{
		Id:     "t5",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type: evergreen.CommandTypeSystem,
		},
	}
	assert.NoError(t, t5.Insert())
	checkStatuses(t, evergreen.TaskSystemFailed, t5)
	t6 := Task{
		Id:     "t6",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type:     evergreen.CommandTypeSystem,
			TimedOut: true,
		},
	}
	assert.NoError(t, t6.Insert())
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
	assert.NoError(t, t7.Insert())
	checkStatuses(t, evergreen.TaskSystemUnresponse, t7)
	t8 := Task{
		Id:        "t8",
		Status:    evergreen.TaskStarted,
		Activated: true,
	}
	assert.NoError(t, t8.Insert())
	checkStatuses(t, evergreen.TaskStarted, t8)
	t9 := Task{
		Id:        "t9",
		Status:    evergreen.TaskUndispatched,
		Activated: false,
	}
	assert.NoError(t, t9.Insert())
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
	assert.NoError(t, t10.Insert())
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
	assert.NoError(t, t11.Insert())
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
	assert.NoError(t, t12.Insert())
	checkStatuses(t, evergreen.TaskWillRun, t11)
	t13 := Task{
		Id:        "t13",
		Status:    evergreen.TaskContainerUnallocated,
		Activated: true,
	}
	assert.NoError(t, t13.Insert())
	// No CheckStatuses for t12 to avoid paradox
	t14 := Task{
		Id:        "t14",
		Status:    evergreen.TaskContainerAllocated,
		Activated: true,
	}
	assert.NoError(t, t14.Insert())
	checkStatuses(t, evergreen.TaskContainerUnallocated, t13)
	checkStatuses(t, evergreen.TaskContainerAllocated, t14)
}

func TestFindTaskNamesByBuildVariant(t *testing.T) {
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
		assert.NoError(t, t1.Insert())
		t2 := Task{
			Id:                  "t2",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-agent",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t2.Insert())
		t3 := Task{
			Id:                  "t3",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-graphql",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t3.Insert())
		t4 := Task{
			Id:                  "t4",
			Status:              evergreen.TaskFailed,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-graphql",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t4.Insert())
		buildVariantTask, err := FindTaskNamesByBuildVariant("evergreen", "ubuntu1604", 1)
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
		assert.NoError(t, t1.Insert())
		t2 := Task{
			Id:                  "t2",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-graphql",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t2.Insert())
		t3 := Task{
			Id:                  "t3",
			Status:              evergreen.TaskSucceeded,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "dist",
			Project:             "evergreen",
			Requester:           evergreen.PatchVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t3.Insert())
		t4 := Task{
			Id:                  "t4",
			Status:              evergreen.TaskFailed,
			BuildVariant:        "ubuntu1604",
			DisplayName:         "test-something",
			Project:             "evergreen",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 1,
		}
		assert.NoError(t, t4.Insert())
		buildVariantTasks, err := FindTaskNamesByBuildVariant("evergreen", "ubuntu1604", 1)
		assert.NoError(t, err)
		assert.Equal(t, []string{"test-graphql", "test-something"}, buildVariantTasks)
	})

}

func TestFindNeedsContainerAllocation(t *testing.T) {
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	getTaskThatNeedsContainerAllocation := func() Task {
		return Task{
			Id:                utility.RandomString(),
			Activated:         true,
			ActivatedTime:     time.Now(),
			Status:            evergreen.TaskContainerUnallocated,
			ExecutionPlatform: ExecutionPlatformContainer,
		}
	}
	for tName, tCase := range map[string]func(t *testing.T){
		"IncludesOneContainerTaskWaitingForAllocation": func(t *testing.T) {
			needsAllocation := getTaskThatNeedsContainerAllocation()
			require.NoError(t, needsAllocation.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, needsAllocation.Id, found[0].Id)
		},
		"IncludesAllContainerTasksWaitingForAllocation": func(t *testing.T) {
			needsAllocation0 := getTaskThatNeedsContainerAllocation()
			require.NoError(t, needsAllocation0.Insert())
			needsAllocation1 := getTaskThatNeedsContainerAllocation()
			needsAllocation1.ActivatedTime = time.Now().Add(-time.Hour)
			require.NoError(t, needsAllocation1.Insert())
			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.Status = evergreen.TaskContainerAllocated
			require.NoError(t, doesNotNeedAllocation.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			require.Len(t, found, 2)
			assert.Equal(t, needsAllocation1.Id, found[0].Id, "tasks should be sorted by activation time, so task with earlier activation time should be first")
			assert.Equal(t, needsAllocation0.Id, found[1].Id, "tasks should be sorted by activation time, so task with later activation time should be second")
		},
		"IncludesTasksWithAllDependenciesMet": func(t *testing.T) {
			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.DependsOn = []Dependency{
				{
					TaskId:   "dependency0",
					Finished: true,
				},
				{
					TaskId:   "dependency1",
					Status:   evergreen.TaskFailed,
					Finished: true,
				},
			}
			require.NoError(t, needsAllocation.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, needsAllocation.Id, found[0].Id)
		},
		"IncludesTasksWithOverriddenDependencies": func(t *testing.T) {
			overriddenDependencies := getTaskThatNeedsContainerAllocation()
			overriddenDependencies.DependsOn = []Dependency{
				{
					TaskId: "dependency0",
				},
			}
			overriddenDependencies.OverrideDependencies = true
			require.NoError(t, overriddenDependencies.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, overriddenDependencies.Id, found[0].Id)
		},
		"IgnoresTasksWithUnmetDependencies": func(t *testing.T) {
			unmetDependencies := getTaskThatNeedsContainerAllocation()
			unmetDependencies.DependsOn = []Dependency{
				{
					TaskId: "dependency0",
				},
			}
			require.NoError(t, unmetDependencies.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			assert.Empty(t, found)
		},
		"IgnoresTasksWithoutExecutionPlatform": func(t *testing.T) {
			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.ExecutionPlatform = ""
			require.NoError(t, doesNotNeedAllocation.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			assert.Empty(t, found)
		},
		"IgnoresHostTasks": func(t *testing.T) {
			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.ExecutionPlatform = ExecutionPlatformHost
			require.NoError(t, doesNotNeedAllocation.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			assert.Empty(t, found)
		},
		"IgnoresDeactivatedTasks": func(t *testing.T) {
			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.Activated = false
			doesNotNeedAllocation.ActivatedTime = utility.ZeroTime
			require.NoError(t, doesNotNeedAllocation.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			assert.Empty(t, found)
		},
		"IgnoresTasksWithStatusesOtherThanContainerUnallocated": func(t *testing.T) {
			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.Status = evergreen.TaskContainerAllocated
			require.NoError(t, doesNotNeedAllocation.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			assert.Empty(t, found)
		},
		"IgnoresDisabledTasks": func(t *testing.T) {
			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.Priority = evergreen.DisabledTaskPriority
			require.NoError(t, doesNotNeedAllocation.Insert())

			found, err := FindNeedsContainerAllocation()
			require.NoError(t, err)
			assert.Empty(t, found)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			tCase(t)
		})
	}
}
