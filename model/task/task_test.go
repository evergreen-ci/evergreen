package task

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/cost"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	oneMs = time.Millisecond
)

var depTaskIds = []Dependency{
	{TaskId: "td1", Status: evergreen.TaskSucceeded},
	{TaskId: "td2", Status: evergreen.TaskSucceeded},
	{TaskId: "td3", Status: ""}, // Default == "success"
	{TaskId: "td4", Status: evergreen.TaskFailed},
	{TaskId: "td5", Status: AllStatuses},
}

// update statuses of test tasks in the db
func updateTestDepTasks(ctx context.Context, t *testing.T) {
	// cases for success/default
	for _, depTaskId := range depTaskIds[:3] {
		require.NoError(t, UpdateOne(ctx, bson.M{"_id": depTaskId.TaskId}, bson.M{"$set": bson.M{"status": evergreen.TaskSucceeded}}))
	}
	// cases for * and failure
	for _, depTaskId := range depTaskIds[3:] {
		require.NoError(t, UpdateOne(ctx, bson.M{"_id": depTaskId.TaskId}, bson.M{"$set": bson.M{"status": evergreen.TaskFailed}}))
	}
}

func TestGetDisplayStatusAndColorSort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, annotations.Collection))
	t1 := Task{
		Id:             "t1",
		Version:        "v1",
		Execution:      3,
		Status:         evergreen.TaskFailed,
		DisplayTaskId:  utility.ToStringPtr(""),
		HasAnnotations: true,
	}
	t2 := Task{
		Id:            "t2",
		Version:       "v1",
		Aborted:       true,
		Execution:     1,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t3 := Task{
		Id:            "t3",
		Version:       "v1",
		Status:        evergreen.TaskSucceeded,
		Execution:     1,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t4 := Task{
		Id:      "t4",
		Version: "v1",
		Details: apimodels.TaskEndDetail{
			Type: evergreen.CommandTypeSetup,
		},
		Execution:     1,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t5 := Task{
		Id:      "t5",
		Version: "v1",
		Details: apimodels.TaskEndDetail{
			Type:        evergreen.CommandTypeSystem,
			Description: evergreen.TaskDescriptionHeartbeat,
			TimedOut:    true,
		},
		Execution:     1,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t6 := Task{
		Id:      "t6",
		Version: "v1",
		Details: apimodels.TaskEndDetail{
			Type:     evergreen.CommandTypeSystem,
			TimedOut: true,
		},
		Execution:     1,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t7 := Task{
		Id:      "t7",
		Version: "v1",
		Details: apimodels.TaskEndDetail{
			Type: evergreen.CommandTypeSystem,
		},
		Execution:     1,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t8 := Task{
		Id:      "t8",
		Version: "v1",
		Details: apimodels.TaskEndDetail{
			TimedOut: true,
		},
		Execution:     1,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t9 := Task{
		Id:            "t9",
		Version:       "v1",
		Status:        evergreen.TaskUndispatched,
		Activated:     false,
		Execution:     1,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t10 := Task{
		Id:            "t10",
		Version:       "v1",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t11 := Task{
		Id:        "t11",
		Version:   "v1",
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
		Execution:     1,
		DisplayTaskId: utility.ToStringPtr(""),
	}

	assert.NoError(t, db.InsertMany(t.Context(), Collection, t1, t2, t3, t4, t5, t6, t7, t8, t9, t10, t11))

	pipeline, err := getTasksByVersionPipeline("v1", GetTasksByVersionOptions{})
	require.NoError(t, err)
	sortFields := bson.D{bson.E{Key: "__" + DisplayStatusKey, Value: 1}, bson.E{Key: IdKey, Value: 1}}
	sortPipeline := []bson.M{addStatusColorSort(DisplayStatusKey), {"$sort": sortFields}}
	pipeline = append(pipeline, sortPipeline...)

	taskResults := []Task{}
	err = Aggregate(ctx, pipeline, &taskResults)
	require.NoError(t, err)

	assert.Len(t, taskResults, 11)
	// first, assert the correctness of displayStatusExpression and ensure the display
	// statuses are computed correctly and that it matches GetDisplayStatus
	for _, foundTask := range taskResults {
		switch foundTask.Id {
		case t1.Id:
			assert.Equal(t, evergreen.TaskKnownIssue, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskKnownIssue, t1.GetDisplayStatus())
		case t2.Id:
			assert.Equal(t, evergreen.TaskAborted, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskAborted, t2.GetDisplayStatus())
		case t3.Id:
			assert.Equal(t, evergreen.TaskSucceeded, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskSucceeded, t3.GetDisplayStatus())
		case t4.Id:
			assert.Equal(t, evergreen.TaskSetupFailed, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskSetupFailed, t4.GetDisplayStatus())
		case t5.Id:
			assert.Equal(t, evergreen.TaskSystemUnresponse, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskSystemUnresponse, t5.GetDisplayStatus())
		case t6.Id:
			assert.Equal(t, evergreen.TaskSystemTimedOut, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskSystemTimedOut, t6.GetDisplayStatus())
		case t7.Id:
			assert.Equal(t, evergreen.TaskSystemFailed, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskSystemFailed, t7.GetDisplayStatus())
		case t8.Id:
			assert.Equal(t, evergreen.TaskTimedOut, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskTimedOut, t8.GetDisplayStatus())
		case t9.Id:
			assert.Equal(t, evergreen.TaskUnscheduled, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskUnscheduled, t9.GetDisplayStatus())
		case t10.Id:
			assert.Equal(t, evergreen.TaskWillRun, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskWillRun, t10.GetDisplayStatus())
		case t11.Id:
			assert.Equal(t, evergreen.TaskStatusBlocked, foundTask.DisplayStatus)
			assert.Equal(t, evergreen.TaskStatusBlocked, t11.GetDisplayStatus())
		}
	}
	// check correctness of addStatusColorSort
	checkPriority(t, taskResults)
}

func checkPriority(t *testing.T, taskResults []Task) {
	assert.Equal(t, evergreen.TaskTimedOut, taskResults[0].DisplayStatus)
	assert.Equal(t, evergreen.TaskKnownIssue, taskResults[1].DisplayStatus)
	assert.Equal(t, evergreen.TaskSetupFailed, taskResults[2].DisplayStatus)
	assert.Equal(t, evergreen.TaskSystemUnresponse, taskResults[3].DisplayStatus)
	assert.Equal(t, evergreen.TaskSystemTimedOut, taskResults[4].DisplayStatus)
	assert.Equal(t, evergreen.TaskSystemFailed, taskResults[5].DisplayStatus)
	assert.Equal(t, evergreen.TaskWillRun, taskResults[6].DisplayStatus)
	assert.Equal(t, evergreen.TaskSucceeded, taskResults[7].DisplayStatus)
	assert.Equal(t, evergreen.TaskStatusBlocked, taskResults[8].DisplayStatus)
	assert.Equal(t, evergreen.TaskAborted, taskResults[9].DisplayStatus)
	assert.Equal(t, evergreen.TaskUnscheduled, taskResults[10].DisplayStatus)
}

func TestDependenciesMet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var taskId string
	var taskDoc *Task
	var depTasks []*Task

	Convey("With a task", t, func() {

		taskId = "t1"

		taskDoc = &Task{
			Id: taskId,
		}

		depTasks = []*Task{
			{Id: depTaskIds[0].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[1].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[2].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[3].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[4].TaskId, Status: evergreen.TaskUndispatched},
		}

		So(db.Clear(Collection), ShouldBeNil)
		for _, depTask := range depTasks {
			So(depTask.Insert(t.Context()), ShouldBeNil)
		}
		So(taskDoc.Insert(t.Context()), ShouldBeNil)

		Convey("sanity check the local version of the function in the nil case", func() {
			taskDoc.DependsOn = []Dependency{}
			met, err := taskDoc.AllDependenciesSatisfied(ctx, map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			taskDoc.DependenciesMetTime = utility.ZeroTime
		})

		Convey("if the task has no dependencies its dependencies should"+
			" be met by default", func() {
			taskDoc.DependsOn = []Dependency{}
			met, err := taskDoc.DependenciesMet(ctx, map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			So(utility.IsZeroTime(taskDoc.DependenciesMetTime), ShouldBeTrue)
			taskDoc.DependenciesMetTime = utility.ZeroTime
		})

		Convey("task with overridden dependencies should be met", func() {
			taskDoc.DependsOn = depTaskIds
			taskDoc.OverrideDependencies = true
			met, err := taskDoc.DependenciesMet(ctx, map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			So(utility.IsZeroTime(taskDoc.DependenciesMetTime), ShouldBeTrue)
			taskDoc.DependenciesMetTime = utility.ZeroTime
		})

		Convey("if only some of the tasks dependencies are finished"+
			" successfully, then it should not think its dependencies are met",
			func() {
				taskDoc.DependsOn = depTaskIds
				So(UpdateOne(
					ctx,
					bson.M{"_id": depTaskIds[0].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				met, err := taskDoc.DependenciesMet(ctx, map[string]Task{})
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)
				So(utility.IsZeroTime(taskDoc.DependenciesMetTime), ShouldBeTrue)
				taskDoc.DependenciesMetTime = utility.ZeroTime
			})

		Convey("if all of the tasks dependencies are finished properly, it"+
			" should correctly believe its dependencies are met", func() {
			taskDoc.DependsOn = depTaskIds
			updateTestDepTasks(ctx, t)
			met, err := taskDoc.DependenciesMet(ctx, map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			taskDoc.DependenciesMetTime = utility.ZeroTime
		})

		Convey("tasks not in the dependency cache should be pulled into the"+
			" cache during dependency checking", func() {
			dependencyCache := make(map[string]Task)
			taskDoc.DependsOn = depTaskIds
			updateTestDepTasks(ctx, t)
			met, err := taskDoc.DependenciesMet(ctx, dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			So(utility.IsZeroTime(taskDoc.DependenciesMetTime), ShouldBeFalse)
			taskDoc.DependenciesMetTime = utility.ZeroTime
			for _, depTaskId := range depTaskIds[:4] {
				So(dependencyCache[depTaskId.TaskId].Id, ShouldEqual, depTaskId.TaskId)
			}
			So(dependencyCache["td5"].Id, ShouldEqual, "td5")
		})

		Convey("cached dependencies should be used rather than fetching them"+
			" from the database", func() {
			updateTestDepTasks(ctx, t)
			dependencyCache := make(map[string]Task)
			taskDoc.DependsOn = depTaskIds
			met, err := taskDoc.DependenciesMet(ctx, dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			So(utility.IsZeroTime(taskDoc.DependenciesMetTime), ShouldBeFalse)
			taskDoc.DependenciesMetTime = utility.ZeroTime

			// alter the dependency cache so that it should seem as if the
			// dependencies are not met
			cachedTask := dependencyCache[depTaskIds[0].TaskId]
			So(cachedTask.Status, ShouldEqual, evergreen.TaskSucceeded)
			cachedTask.Status = evergreen.TaskFailed
			dependencyCache[depTaskIds[0].TaskId] = cachedTask
			met, err = taskDoc.DependenciesMet(ctx, dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeFalse)
			taskDoc.DependenciesMetTime = utility.ZeroTime

		})

		Convey("extraneous tasks in the dependency cache should be ignored",
			func() {
				So(UpdateOne(
					ctx,
					bson.M{"_id": depTaskIds[0].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				So(UpdateOne(
					ctx,
					bson.M{"_id": depTaskIds[1].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				So(UpdateOne(
					ctx,
					bson.M{"_id": depTaskIds[2].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskFailed,
						},
					},
				), ShouldBeNil)

				dependencyCache := make(map[string]Task)
				taskDoc.DependsOn = []Dependency{depTaskIds[0], depTaskIds[1],
					depTaskIds[2]}
				met, err := taskDoc.DependenciesMet(ctx, dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)
				So(utility.IsZeroTime(taskDoc.DependenciesMetTime), ShouldBeTrue)
				taskDoc.DependenciesMetTime = utility.ZeroTime

				met, err = taskDoc.AllDependenciesSatisfied(ctx, dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)

				// remove the failed task from the dependencies (but not from
				// the cache).  it should be ignored in the next pass
				taskDoc.DependsOn = []Dependency{depTaskIds[0], depTaskIds[1]}
				met, err = taskDoc.DependenciesMet(ctx, dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeTrue)
				So(utility.IsZeroTime(taskDoc.DependenciesMetTime), ShouldBeFalse)
				taskDoc.DependenciesMetTime = utility.ZeroTime

				met, err = taskDoc.AllDependenciesSatisfied(ctx, dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeTrue)
			})
	})
}

func TestGetFinishedBlockingDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskId := "t1"
	taskDoc := &Task{
		Id: taskId,
	}
	depTasks := []*Task{
		{Id: depTaskIds[0].TaskId, Status: evergreen.TaskUndispatched},
		{Id: depTaskIds[1].TaskId, Status: evergreen.TaskUndispatched},
		{Id: depTaskIds[2].TaskId, Status: evergreen.TaskFailed},
		{Id: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
		{Id: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
		{Id: "td6", Status: evergreen.TaskDispatched, DependsOn: []Dependency{{TaskId: "DNE", Unattainable: true}}},
	}
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	for name, test := range map[string]func(*testing.T){
		"NoDeps": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{}
			require.NoError(t, taskDoc.Insert(t.Context()))

			tasks, err := taskDoc.GetFinishedBlockingDependencies(ctx, map[string]Task{})
			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"Satisfied": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert(t.Context()))

			tasks, err := taskDoc.GetFinishedBlockingDependencies(ctx, map[string]Task{})
			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"SatisfiedWithCache": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: "cached-task", Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert(t.Context()))

			tasks, err := taskDoc.GetFinishedBlockingDependencies(ctx, map[string]Task{
				"cached-task": {Id: "cached-task", Status: evergreen.TaskSucceeded},
			})
			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"UnsatisfiedAndFinished": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: depTaskIds[2].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert(t.Context()))

			tasks, err := taskDoc.GetFinishedBlockingDependencies(ctx, map[string]Task{})
			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		},
		"UnsatisfiedAndFinishedWithCache": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: "cached-task", Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert(t.Context()))

			tasks, err := taskDoc.GetFinishedBlockingDependencies(ctx, map[string]Task{
				"cached-task": {Id: "cached-task", Status: evergreen.TaskFailed},
			})
			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		},
		"BlockedEarly": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded, Unattainable: true},
				{TaskId: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert(t.Context()))

			tasks, err := taskDoc.GetFinishedBlockingDependencies(ctx, map[string]Task{})
			assert.NoError(t, err)
			// already marked blocked
			assert.Empty(t, tasks)
		},
		"BlockedLater": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: "td6", Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert(t.Context()))

			tasks, err := taskDoc.GetFinishedBlockingDependencies(ctx, map[string]Task{})
			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		}} {
		require.NoError(t, db.Clear(Collection))
		for _, depTask := range depTasks {
			require.NoError(t, depTask.Insert(t.Context()))
		}
		t.Run(name, test)
	}
}

func TestGetDeactivatedBlockingDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskId := "t1"
	taskDoc := &Task{
		Id: taskId,
	}
	depTasks := []*Task{
		{Id: depTaskIds[0].TaskId, Status: evergreen.TaskUndispatched, Activated: true},
		{Id: depTaskIds[1].TaskId, Status: evergreen.TaskSucceeded, Activated: false},
		{Id: depTaskIds[2].TaskId, Status: evergreen.TaskUndispatched, Activated: false},
	}
	require.NoError(t, db.Clear(Collection))
	for _, depTask := range depTasks {
		require.NoError(t, depTask.Insert(t.Context()))
	}
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	t.Run("NotBlocked", func(t *testing.T) {
		taskDoc.DependsOn = []Dependency{
			{TaskId: depTaskIds[0].TaskId},
		}
		blockingTasks, err := taskDoc.GetDeactivatedBlockingDependencies(ctx, map[string]Task{})
		require.NoError(t, err)
		assert.Empty(t, blockingTasks)
	})
	t.Run("NotBlockedWithCache", func(t *testing.T) {
		taskDoc.DependsOn = []Dependency{
			{TaskId: depTaskIds[0].TaskId},
			{TaskId: "cached-task"},
		}
		blockingTasks, err := taskDoc.GetDeactivatedBlockingDependencies(ctx, map[string]Task{
			"cached-task": {Id: "cached-task", Status: evergreen.TaskSucceeded},
		})
		require.NoError(t, err)
		assert.Empty(t, blockingTasks)
	})
	t.Run("NoBlockedFinished", func(t *testing.T) {
		taskDoc.DependsOn = []Dependency{
			{TaskId: depTaskIds[1].TaskId},
		}
		blockingTasks, err := taskDoc.GetDeactivatedBlockingDependencies(ctx, map[string]Task{})
		require.NoError(t, err)
		assert.Empty(t, blockingTasks)
	})
	t.Run("Blocked", func(t *testing.T) {
		taskDoc.DependsOn = []Dependency{
			{TaskId: depTaskIds[2].TaskId},
		}
		blockingTasks, err := taskDoc.GetDeactivatedBlockingDependencies(ctx, map[string]Task{})
		require.NoError(t, err)
		assert.Len(t, blockingTasks, 1)
	})
	t.Run("BlockedWithCache", func(t *testing.T) {
		taskDoc.DependsOn = []Dependency{
			{TaskId: depTaskIds[2].TaskId},
			{TaskId: "cached-task"},
		}
		blockingTasks, err := taskDoc.GetDeactivatedBlockingDependencies(ctx, map[string]Task{
			"cached-task": {Id: "cached-task", Status: evergreen.TaskSucceeded},
		})
		require.NoError(t, err)
		assert.Len(t, blockingTasks, 1)
	})
}

func TestMarkDependenciesFinished(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T){
		"NoopsForNoDependencies": func(t *testing.T) {
			t0 := Task{
				Id:     "task0",
				Status: evergreen.TaskSucceeded,
			}
			t1 := Task{
				Id: "task1",
			}
			t2 := Task{
				Id: "task2",
				DependsOn: []Dependency{
					{TaskId: "task1"},
				},
			}
			require.NoError(t, t0.Insert(t.Context()))
			require.NoError(t, t1.Insert(t.Context()))
			require.NoError(t, t2.Insert(t.Context()))

			require.NoError(t, t0.MarkDependenciesFinished(ctx, true))

			dbTask2, err := FindOneId(ctx, t2.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask2)
			require.Len(t, dbTask2.DependsOn, 1)
			assert.False(t, dbTask2.DependsOn[0].Finished, "unconnected dependency edge should not be marked finished")
			assert.True(t, utility.IsZeroTime(dbTask2.DependsOn[0].FinishedAt), "unconnected dependency edge should not be marked finished")
		},
		"UpdatesDependencyWithMatchingStatus": func(t *testing.T) {
			t0 := Task{
				Id:     "task0",
				Status: evergreen.TaskFailed,
			}
			t1 := Task{
				Id: "task1",
				DependsOn: []Dependency{
					{
						TaskId: "task0",
						Status: evergreen.TaskFailed,
					},
				},
			}
			require.NoError(t, t0.Insert(t.Context()))
			require.NoError(t, t1.Insert(t.Context()))

			t0.FinishTime = time.Now()
			require.NoError(t, t0.MarkDependenciesFinished(ctx, true))

			dbTask1, err := FindOneId(ctx, t1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask1)
			require.Len(t, dbTask1.DependsOn, 1)
			assert.True(t, dbTask1.DependsOn[0].Finished)
			assert.False(t, utility.IsZeroTime(dbTask1.DependsOn[0].FinishedAt))
		},
		"UpdatesDependencyWithUnmatchingStatus": func(t *testing.T) {
			t0 := Task{
				Id:     "task0",
				Status: evergreen.TaskFailed,
			}
			t1 := Task{
				Id: "task1",
				DependsOn: []Dependency{
					{
						TaskId: "task0",
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			require.NoError(t, t0.Insert(t.Context()))
			require.NoError(t, t1.Insert(t.Context()))

			t0.FinishTime = time.Now()
			require.NoError(t, t0.MarkDependenciesFinished(ctx, true))
			assert.NoError(t, t0.MarkEnd(ctx, t0.FinishTime, &apimodels.TaskEndDetail{Status: evergreen.TaskFailed}))

			dbTask1, err := FindOneId(ctx, t1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask1)
			require.Len(t, dbTask1.DependsOn, 1)
			assert.True(t, dbTask1.DependsOn[0].Finished)
			assert.False(t, utility.IsZeroTime(dbTask1.DependsOn[0].FinishedAt))

			met, err := dbTask1.DependenciesMet(ctx, map[string]Task{})
			assert.NoError(t, err)
			assert.False(t, met)
			assert.Zero(t, dbTask1.DependenciesMetTime)
		},
		"UpdatesDependenciesMetTimeAccordingToFinishedAtWithSingleDependency": func(t *testing.T) {
			t0 := Task{
				Id:     "task0",
				Status: evergreen.TaskSucceeded,
			}
			t1 := Task{
				Id: "task1",
				DependsOn: []Dependency{
					{
						TaskId: "task0",
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			require.NoError(t, t0.Insert(t.Context()))
			require.NoError(t, t1.Insert(t.Context()))

			t0.FinishTime = time.Now().Round(time.Millisecond)
			require.NoError(t, t0.MarkDependenciesFinished(ctx, true))
			assert.NoError(t, t0.MarkEnd(ctx, t0.FinishTime, &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}))

			dbTask1, err := FindOneId(ctx, t1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask1)
			require.Len(t, dbTask1.DependsOn, 1)
			assert.True(t, dbTask1.DependsOn[0].Finished)
			assert.False(t, utility.IsZeroTime(dbTask1.DependsOn[0].FinishedAt))

			met, err := dbTask1.DependenciesMet(ctx, map[string]Task{})
			assert.NoError(t, err)
			assert.True(t, met)
			assert.Equal(t, dbTask1.DependenciesMetTime, t0.FinishTime)
		},
		"UpdatesDependenciesMetTimeAccordingToFinishedAt": func(t *testing.T) {
			t0 := Task{
				Id:     "task0",
				Status: evergreen.TaskSucceeded,
			}
			t1 := Task{
				Id:     "task1",
				Status: evergreen.TaskSucceeded,
			}
			t2 := Task{
				Id: "task2",
				DependsOn: []Dependency{
					{
						TaskId: "task0",
						Status: evergreen.TaskSucceeded,
					},
					{
						TaskId: "task1",
						Status: evergreen.TaskSucceeded,
					},
				},
			}
			require.NoError(t, t0.Insert(t.Context()))
			require.NoError(t, t1.Insert(t.Context()))
			require.NoError(t, t2.Insert(t.Context()))

			t0.FinishTime = time.Now()
			require.NoError(t, t0.MarkDependenciesFinished(ctx, true))
			assert.NoError(t, t0.MarkEnd(ctx, t0.FinishTime, &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}))

			t1.FinishTime = time.Now().Round(time.Millisecond)
			require.NoError(t, t1.MarkDependenciesFinished(ctx, true))
			assert.NoError(t, t1.MarkEnd(ctx, t1.FinishTime, &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}))

			dbTask2, err := FindOneId(ctx, t2.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask2)
			require.Len(t, dbTask2.DependsOn, 2)
			assert.True(t, dbTask2.DependsOn[0].Finished)
			assert.False(t, utility.IsZeroTime(dbTask2.DependsOn[0].FinishedAt))
			assert.True(t, dbTask2.DependsOn[1].Finished)
			assert.False(t, utility.IsZeroTime(dbTask2.DependsOn[1].FinishedAt))

			met, err := dbTask2.DependenciesMet(ctx, map[string]Task{})
			assert.NoError(t, err)
			assert.True(t, met)
			assert.Equal(t, dbTask2.DependenciesMetTime, t1.FinishTime)
			assert.True(t, dbTask2.DependenciesMetTime.After(t0.FinishTime))
		},
		"UpdatesSpecificDependency": func(t *testing.T) {
			t0 := Task{
				Id:     "task0",
				Status: evergreen.TaskFailed,
			}
			t1 := Task{
				Id: "task1",
				DependsOn: []Dependency{
					{TaskId: "task2"},
					{TaskId: "task3"},
					{
						TaskId: "task0",
						Status: evergreen.TaskSucceeded,
					},
					{TaskId: "task4"},
				},
			}
			require.NoError(t, t0.Insert(t.Context()))
			require.NoError(t, t1.Insert(t.Context()))

			t0.FinishTime = time.Now()
			require.NoError(t, t0.MarkDependenciesFinished(ctx, true))

			dbTask1, err := FindOneId(ctx, t1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask1)
			require.Len(t, dbTask1.DependsOn, 4)
			assert.False(t, dbTask1.DependsOn[0].Finished)
			assert.True(t, utility.IsZeroTime(dbTask1.DependsOn[0].FinishedAt))
			assert.False(t, dbTask1.DependsOn[1].Finished)
			assert.True(t, utility.IsZeroTime(dbTask1.DependsOn[1].FinishedAt))
			assert.True(t, dbTask1.DependsOn[2].Finished)
			assert.False(t, utility.IsZeroTime(dbTask1.DependsOn[2].FinishedAt))
			assert.False(t, dbTask1.DependsOn[3].Finished)
			assert.True(t, utility.IsZeroTime(dbTask1.DependsOn[3].FinishedAt))
		},
		"UpdatesDirectDependenciesOnly": func(t *testing.T) {
			t0 := Task{
				Id:     "task0",
				Status: evergreen.TaskSucceeded,
			}
			t1 := Task{
				Id: "task1",
				DependsOn: []Dependency{
					{TaskId: "task0"},
				},
			}
			t2 := Task{
				Id: "task2",
				DependsOn: []Dependency{
					{TaskId: "task1"},
				},
			}
			require.NoError(t, t0.Insert(t.Context()))
			require.NoError(t, t1.Insert(t.Context()))
			require.NoError(t, t2.Insert(t.Context()))

			t0.FinishTime = time.Now()
			require.NoError(t, t0.MarkDependenciesFinished(ctx, true))

			dbTask1, err := FindOneId(ctx, t1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask1)
			require.Len(t, dbTask1.DependsOn, 1)
			assert.True(t, dbTask1.DependsOn[0].Finished, "direct dependency should be marked finished")
			assert.False(t, utility.IsZeroTime(dbTask1.DependsOn[0].FinishedAt), "direct dependency should be marked finished")

			dbTask2, err := FindOneId(ctx, t2.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask2)
			require.Len(t, dbTask2.DependsOn, 1)
			assert.False(t, dbTask2.DependsOn[0].Finished, "indirect dependency edge should not be marked finished")
			assert.True(t, utility.IsZeroTime(dbTask2.DependsOn[0].FinishedAt), "indirect dependency edge should not be marked finished")
		},
		"UpdateDependencyToUnfinished": func(t *testing.T) {
			t0 := Task{
				Id:     "task0",
				Status: evergreen.TaskUndispatched,
			}
			t1 := Task{
				Id: "task1",
				DependsOn: []Dependency{
					{
						TaskId:   "task0",
						Finished: true,
					},
				},
			}
			require.NoError(t, t0.Insert(t.Context()))
			require.NoError(t, t1.Insert(t.Context()))

			require.NoError(t, t0.MarkDependenciesFinished(ctx, false))

			dbTask1, err := FindOneId(ctx, t1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask1)
			require.Len(t, dbTask1.DependsOn, 1)
			assert.False(t, dbTask1.DependsOn[0].Finished, "direct dependency should not be marked finished")
			assert.True(t, utility.IsZeroTime(dbTask1.DependsOn[0].FinishedAt), "direct dependency should not be marked finished")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			tCase(t)
		})
	}
}

func TestSetTasksScheduledTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With some tasks", t, func() {

		So(db.Clear(Collection), ShouldBeNil)

		tasks := []Task{
			{Id: "t0", ScheduledTime: utility.ZeroTime, DependenciesMetTime: utility.ZeroTime, ExecutionTasks: []string{"t1", "t2"}},
			{Id: "t1", ScheduledTime: utility.ZeroTime, DependenciesMetTime: utility.ZeroTime},
			{Id: "t2", ScheduledTime: utility.ZeroTime, DependenciesMetTime: utility.ZeroTime},
			{Id: "t3", ScheduledTime: utility.ZeroTime, DependenciesMetTime: utility.ZeroTime, DependsOn: []Dependency{{TaskId: "t2"}}},
		}
		for _, task := range tasks {
			So(task.Insert(t.Context()), ShouldBeNil)
		}
		Convey("when updating ScheduledTime for some of the tasks", func() {
			testTime := time.Unix(31337, 0)
			So(SetTasksScheduledAndDepsMetTime(ctx, tasks[2:], testTime), ShouldBeNil)

			Convey("the tasks should be updated in memory", func() {
				So(tasks[1].ScheduledTime, ShouldResemble, utility.ZeroTime)
				So(tasks[2].ScheduledTime, ShouldResemble, testTime)
				So(tasks[3].ScheduledTime, ShouldResemble, testTime)
				So(tasks[1].DependenciesMetTime, ShouldResemble, utility.ZeroTime)
				So(tasks[2].DependenciesMetTime, ShouldResemble, testTime)
				So(tasks[3].DependenciesMetTime, ShouldResemble, utility.ZeroTime)

				Convey("and in the db", func() {
					// Need to use a margin of error on time tests
					// because of minor differences between how mongo
					// and golang store dates. The date from the db
					// can be interpreted as being a few nanoseconds off
					t0, err := FindOne(ctx, db.Query(ById("t0")))
					So(err, ShouldBeNil)
					So(t0.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
					So(t0.DependenciesMetTime.Round(oneMs), ShouldResemble, testTime)
					t1, err := FindOne(ctx, db.Query(ById("t1")))
					So(err, ShouldBeNil)
					So(t1.ScheduledTime.Round(oneMs), ShouldResemble, utility.ZeroTime)
					So(t1.DependenciesMetTime.Round(oneMs), ShouldResemble, utility.ZeroTime)
					t2, err := FindOne(ctx, db.Query(ById("t2")))
					So(err, ShouldBeNil)
					So(t2.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
					So(t2.DependenciesMetTime.Round(oneMs), ShouldResemble, testTime)
					t3, err := FindOne(ctx, db.Query(ById("t3")))
					So(err, ShouldBeNil)
					So(t3.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
					So(t3.DependenciesMetTime.Round(oneMs), ShouldResemble, utility.ZeroTime)
				})

				Convey("if we update a second time", func() {
					newTime := time.Unix(99999999, 0)
					So(newTime, ShouldHappenAfter, testTime)
					So(SetTasksScheduledAndDepsMetTime(ctx, tasks, newTime), ShouldBeNil)

					Convey("only unset scheduled times should be updated", func() {
						t0, err := FindOne(ctx, db.Query(ById("t0")))
						So(err, ShouldBeNil)
						So(t0.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
						So(t0.DependenciesMetTime.Round(oneMs), ShouldResemble, testTime)
						t1, err := FindOne(ctx, db.Query(ById("t1")))
						So(err, ShouldBeNil)
						So(t1.ScheduledTime.Round(oneMs), ShouldResemble, newTime)
						So(t1.DependenciesMetTime.Round(oneMs), ShouldResemble, newTime)
						t2, err := FindOne(ctx, db.Query(ById("t2")))
						So(err, ShouldBeNil)
						So(t2.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
						So(t2.DependenciesMetTime.Round(oneMs), ShouldResemble, testTime)
						t3, err := FindOne(ctx, db.Query(ById("t3")))
						So(err, ShouldBeNil)
						So(t3.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
						So(t3.DependenciesMetTime.Round(oneMs), ShouldResemble, utility.ZeroTime)
					})
				})

				Convey("if we update a third time", func() {
					newTime := time.Unix(99999999, 0)
					So(newTime, ShouldHappenAfter, testTime)

					depsFinishedTime := time.Now()
					So(tasks[2].MarkDependenciesFinished(ctx, true), ShouldBeNil)
					So(tasks[2].MarkEnd(ctx, newTime, &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}), ShouldBeNil)
					t3FromDb, err := FindOneId(ctx, "t3")
					So(err, ShouldBeNil)
					depsMet, err := t3FromDb.DependenciesMet(ctx, map[string]Task{})
					So(err, ShouldBeNil)
					So(depsMet, ShouldBeTrue)
					So(SetTasksScheduledAndDepsMetTime(ctx, tasks, newTime), ShouldBeNil)

					Convey("nothing should have updated", func() {
						t0, err := FindOne(ctx, db.Query(ById("t0")))
						So(err, ShouldBeNil)
						So(t0.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
						So(t0.DependenciesMetTime.Round(oneMs), ShouldResemble, testTime)
						t1, err := FindOne(ctx, db.Query(ById("t1")))
						So(err, ShouldBeNil)
						So(t1.ScheduledTime.Round(oneMs), ShouldResemble, newTime)
						So(t1.DependenciesMetTime.Round(oneMs), ShouldResemble, newTime)
						t2, err := FindOne(ctx, db.Query(ById("t2")))
						So(err, ShouldBeNil)
						So(t2.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
						So(t2.DependenciesMetTime.Round(oneMs), ShouldResemble, testTime)
						t3, err := FindOne(ctx, db.Query(ById("t3")))
						So(err, ShouldBeNil)
						So(t3.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
						So(t3.DependenciesMetTime.After(depsFinishedTime), ShouldBeTrue)
					})
				})

			})

		})
	})
}

func TestEndingTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With tasks that are attempting to be marked as finished", t, func() {
		So(db.Clear(Collection), ShouldBeNil)
		Convey("a task that has a start time set", func() {
			now := time.Now()
			task := &Task{
				Id:        "taskId",
				Status:    evergreen.TaskStarted,
				StartTime: now.Add(-5 * time.Minute),
			}
			So(task.Insert(t.Context()), ShouldBeNil)
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}

			So(task.MarkEnd(ctx, now, details), ShouldBeNil)
			task, err := FindOne(ctx, db.Query(ById(task.Id)))
			So(err, ShouldBeNil)
			So(task.Status, ShouldEqual, evergreen.TaskFailed)
			So(task.FinishTime.Unix(), ShouldEqual, now.Unix())
			So(task.StartTime.Unix(), ShouldEqual, now.Add(-5*time.Minute).Unix())
		})
		Convey("a task with no start time set should have one added", func() {
			now := time.Now()
			Convey("a task with a create time < 2 hours should have the start time set to the ingest time", func() {
				task := &Task{
					Id:         "tid",
					Status:     evergreen.TaskDispatched,
					IngestTime: now.Add(-30 * time.Minute),
				}
				So(task.Insert(t.Context()), ShouldBeNil)
				details := &apimodels.TaskEndDetail{
					Status: evergreen.TaskFailed,
				}
				So(task.MarkEnd(ctx, now, details), ShouldBeNil)
				task, err := FindOne(ctx, db.Query(ById(task.Id)))
				So(err, ShouldBeNil)
				So(task.StartTime.Unix(), ShouldEqual, task.IngestTime.Unix())
				So(task.FinishTime.Unix(), ShouldEqual, now.Unix())
			})
			Convey("a task with a create time > 2 hours should have the start time set to two hours"+
				"before the finish time", func() {
				task := &Task{
					Id:         "tid",
					Status:     evergreen.TaskDispatched,
					CreateTime: now.Add(-3 * time.Hour),
				}
				So(task.Insert(t.Context()), ShouldBeNil)
				details := &apimodels.TaskEndDetail{
					Status: evergreen.TaskFailed,
				}
				So(task.MarkEnd(ctx, now, details), ShouldBeNil)
				task, err := FindOne(ctx, db.Query(ById(task.Id)))
				So(err, ShouldBeNil)
				startTime := now.Add(-2 * time.Hour)
				So(task.StartTime.Unix(), ShouldEqual, startTime.Unix())
				So(task.FinishTime.Unix(), ShouldEqual, now.Unix())
			})
		})
		Convey("a task that is allocated a container should be deallocated", func() {
			now := time.Now()
			task := &Task{
				Id:                     "taskId",
				Status:                 evergreen.TaskStarted,
				StartTime:              now.Add(-5 * time.Minute),
				ExecutionPlatform:      ExecutionPlatformContainer,
				ContainerAllocated:     true,
				ContainerAllocatedTime: time.Now(),
			}
			So(task.Insert(t.Context()), ShouldBeNil)
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}

			So(task.MarkEnd(ctx, now, details), ShouldBeNil)
			task, err := FindOne(ctx, db.Query(ById(task.Id)))
			So(err, ShouldBeNil)
			So(task.Status, ShouldEqual, evergreen.TaskFailed)
			So(task.ContainerAllocated, ShouldBeFalse)
			So(task.ContainerAllocatedTime, ShouldBeZeroValue)
		})
	})
}

func TestTaskResultOutcome(t *testing.T) {
	assert := assert.New(t)

	tasks := []Task{
		{Status: evergreen.TaskUndispatched, Activated: false}, // 0
		{Status: evergreen.TaskUndispatched, Activated: true},  // 1
		{Status: evergreen.TaskStarted},                        // 2
		{Status: evergreen.TaskSucceeded},                      // 3
		{Status: evergreen.TaskFailed},                         // 4
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem}},                                                                  // 5
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem, TimedOut: true}},                                                  // 6
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem, TimedOut: true, Description: evergreen.TaskDescriptionHeartbeat}}, // 7
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{TimedOut: true, Description: evergreen.TaskDescriptionHeartbeat}},                                    // 8
		{Status: evergreen.TaskSetupFailed, Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSetup}},                                                              // 5
	}

	out := GetResultCounts(tasks)
	assert.Equal(len(tasks), out.Total)
	assert.Equal(1, out.Inactive)
	assert.Equal(1, out.Unstarted)
	assert.Equal(1, out.Started)
	assert.Equal(1, out.Succeeded)
	assert.Equal(1, out.Failed)
	assert.Equal(1, out.SystemFailed)
	assert.Equal(1, out.SystemUnresponsive)
	assert.Equal(1, out.SystemTimedOut)
	assert.Equal(1, out.TestTimedOut)
	assert.Equal(1, out.SetupFailed)

	//

	assert.Equal(1, GetResultCounts([]Task{tasks[0]}).Inactive)
	assert.Equal(1, GetResultCounts([]Task{tasks[1]}).Unstarted)
	assert.Equal(1, GetResultCounts([]Task{tasks[2]}).Started)
	assert.Equal(1, GetResultCounts([]Task{tasks[3]}).Succeeded)
	assert.Equal(1, GetResultCounts([]Task{tasks[4]}).Failed)
	assert.Equal(1, GetResultCounts([]Task{tasks[5]}).SystemFailed)
	assert.Equal(1, GetResultCounts([]Task{tasks[6]}).SystemTimedOut)
	assert.Equal(1, GetResultCounts([]Task{tasks[7]}).SystemUnresponsive)
	assert.Equal(1, GetResultCounts([]Task{tasks[8]}).TestTimedOut)
	assert.Equal(1, GetResultCounts([]Task{tasks[9]}).SetupFailed)
}

func TestIsUnfinishedSystemUnresponsive(t *testing.T) {
	settings := testutil.TestConfig()
	var task Task

	task = Task{
		Status:  evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem, TimedOut: true, Description: evergreen.TaskDescriptionHeartbeat},
	}
	assert.True(t, task.IsUnfinishedSystemUnresponsive(), "current definition")

	task = Task{
		Status: evergreen.TaskSystemUnresponse,
	}
	assert.True(t, task.IsUnfinishedSystemUnresponsive(), "legacy definition")

	task = Task{
		Status:    evergreen.TaskFailed,
		Execution: settings.TaskLimits.MaxTaskExecution,
		Details:   apimodels.TaskEndDetail{TimedOut: true, Description: evergreen.TaskDescriptionHeartbeat}}
	assert.False(t, task.IsUnfinishedSystemUnresponsive(), "normal timeout")

	task = Task{
		Status: evergreen.TaskSucceeded,
	}
	assert.False(t, task.IsUnfinishedSystemUnresponsive(), "success")

	task = Task{
		Status:    evergreen.TaskSystemUnresponse,
		Execution: settings.TaskLimits.MaxTaskExecution,
	}
	assert.False(t, task.IsUnfinishedSystemUnresponsive(), "finished restarting")
}

func TestTaskStatusCount(t *testing.T) {
	assert := assert.New(t)
	counts := TaskStatusCount{}
	details := apimodels.TaskEndDetail{
		TimedOut:    true,
		Description: evergreen.TaskDescriptionHeartbeat,
	}
	counts.IncrementStatus(evergreen.TaskSetupFailed, details)
	counts.IncrementStatus(evergreen.TaskFailed, apimodels.TaskEndDetail{})
	counts.IncrementStatus(evergreen.TaskDispatched, details)
	counts.IncrementStatus(evergreen.TaskInactive, details)
	assert.Equal(1, counts.TimedOut)
	assert.Equal(1, counts.Failed)
	assert.Equal(1, counts.Started)
	assert.Equal(1, counts.Inactive)
}

func TestBlocked(t *testing.T) {
	for name, test := range map[string]func(*testing.T){
		"Blocked": func(*testing.T) {
			t1 := Task{
				Id: "t1",
				DependsOn: []Dependency{
					{Unattainable: false},
					{Unattainable: false},
					{Unattainable: true},
				},
			}
			assert.True(t, t1.Blocked())
		},
		"NotBlocked": func(*testing.T) {
			t1 := Task{
				Id: "t1",
				DependsOn: []Dependency{
					{Unattainable: false},
					{Unattainable: false},
					{Unattainable: false},
				},
			}
			assert.False(t, t1.Blocked())
		},
	} {
		require.NoError(t, db.ClearCollections(Collection))
		t.Run(name, test)
	}
}

func TestSiblingDependency(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:          "t1",
		DisplayName: "t1",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []Dependency{
			{TaskId: "t2", Status: evergreen.TaskSucceeded},
			{TaskId: "t3", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t1.Insert(t.Context()))
	t2 := Task{
		Id:          "t2",
		DisplayName: "t2",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []Dependency{
			{TaskId: "t4", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t2.Insert(t.Context()))
	t3 := Task{
		Id:          "t3",
		DisplayName: "t3",
		Activated:   true,
		Status:      evergreen.TaskStarted,
		DependsOn: []Dependency{
			{TaskId: "t4", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t3.Insert(t.Context()))
	t4 := Task{
		Id:          "t4",
		DisplayName: "t4",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
	}
	assert.NoError(t4.Insert(t.Context()))
}

func TestBulkInsert(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection))
	t1_a := Task{
		Id:      "t1",
		Version: "version",
	}
	t1_b := Task{
		Id:      "t1",
		Version: "version",
	}
	t2 := Task{
		Id:      "t2",
		Version: "version",
	}
	t3 := Task{
		Id:      "t3",
		Version: "version",
	}
	tasks := Tasks{&t1_a, &t1_b, &t2, &t3}
	assert.Error(tasks.InsertUnordered(context.Background()))
	dbTasks, err := Find(ctx, ByVersion("version"))
	assert.NoError(err)
	assert.Len(dbTasks, 3)
	for _, dbTask := range dbTasks {
		assert.Equal("version", dbTask.Version)
	}
}

func TestByBeforeMidwayTaskFromIds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	displayName := "cool-task-9000"
	buildVarient := "bv"
	requester := "r"
	project := "proj"
	tasks := []Task{}
	for i := 1; i <= 20; i++ {
		task := Task{
			Id:                  "t" + fmt.Sprint(i),
			DisplayName:         displayName,
			BuildVariant:        buildVarient,
			Requester:           requester,
			Project:             project,
			RevisionOrderNumber: i,
		}
		assert.NoError(task.Insert(t.Context()))
		tasks = append(tasks, task)
	}
	t10, err := ByBeforeMidwayTaskFromIds(ctx, tasks[0].Id, tasks[19].Id)
	assert.NoError(err)
	require.NotNil(t, t10)
	assert.Equal(10, t10.RevisionOrderNumber)

	t5, err := ByBeforeMidwayTaskFromIds(ctx, tasks[0].Id, tasks[9].Id)
	assert.NoError(err)
	require.NotNil(t, t5)
	assert.Equal(5, t5.RevisionOrderNumber, 5)

	t15, err := ByBeforeMidwayTaskFromIds(ctx, tasks[10].Id, tasks[19].Id)
	assert.NoError(err)
	require.NotNil(t, t15)
	assert.Equal(15, t15.RevisionOrderNumber)

	t19, err := ByBeforeMidwayTaskFromIds(ctx, tasks[17].Id, tasks[19].Id)
	assert.NoError(err)
	require.NotNil(t, t19)
	assert.Equal(19, t19.RevisionOrderNumber)

	t4, err := ByBeforeMidwayTaskFromIds(ctx, tasks[6].Id, tasks[0].Id)
	assert.NoError(err)
	require.NotNil(t, t4)
	assert.Equal(4, t4.RevisionOrderNumber)

	t12, err := ByBeforeMidwayTaskFromIds(ctx, tasks[11].Id, tasks[11].Id)
	assert.NoError(err)
	require.NotNil(t, t12)
	assert.Equal(12, t12.RevisionOrderNumber)

	t16, err := ByBeforeMidwayTaskFromIds(ctx, tasks[15].Id, tasks[16].Id)
	assert.NoError(err)
	require.NotNil(t, t16)
	assert.Equal(16, t16.RevisionOrderNumber)

	t.Run("IncompatibleTasks", func(t *testing.T) {
		otherDisplayName := Task{
			Id:           "otherTaskDisplayName",
			DisplayName:  "Other display name",
			BuildVariant: buildVarient,
			Requester:    requester,
			Project:      project,
		}
		assert.NoError(otherDisplayName.Insert(t.Context()))
		task, err := ByBeforeMidwayTaskFromIds(ctx, tasks[0].Id, otherDisplayName.Id)
		assert.Error(err)
		assert.Nil(task)

		otherBuildVariant := Task{
			Id:           "otherTaskBuildVariant",
			DisplayName:  displayName,
			BuildVariant: "Other Build Variant",
			Requester:    requester,
			Project:      project,
		}
		assert.NoError(otherBuildVariant.Insert(t.Context()))
		task, err = ByBeforeMidwayTaskFromIds(ctx, tasks[0].Id, otherBuildVariant.Id)
		assert.Error(err)
		assert.Nil(task)

		otherRequester := Task{
			Id:           "otherTaskRequester",
			DisplayName:  displayName,
			BuildVariant: buildVarient,
			Requester:    "Other Requester",
			Project:      project,
		}
		assert.NoError(otherRequester.Insert(t.Context()))
		task, err = ByBeforeMidwayTaskFromIds(ctx, tasks[0].Id, otherRequester.Id)
		assert.Error(err)
		assert.Nil(task)

		otherProject := Task{
			Id:           "otherTaskProject",
			DisplayName:  displayName,
			BuildVariant: buildVarient,
			Requester:    requester,
			Project:      "Other project",
		}
		assert.NoError(otherProject.Insert(t.Context()))
		task, err = ByBeforeMidwayTaskFromIds(ctx, tasks[0].Id, otherProject.Id)
		assert.Error(err)
		assert.Nil(task)
	})

	// Usually, the midway task will be found- but if what would be the midway
	// task is from a version (like periodic builds) that does not have the task
	// we should Get the task from earlier versions.
	t.Run("MissingTasks", func(t *testing.T) {
		assert.NoError(db.ClearCollections(Collection))
		// tasks 7-13 are missing.
		for i := 1; i <= 20; i++ {
			if i > 6 && i < 14 {
				continue
			}
			task := Task{
				Id:                  "t" + fmt.Sprint(i),
				DisplayName:         displayName,
				BuildVariant:        buildVarient,
				Requester:           requester,
				Project:             project,
				RevisionOrderNumber: i,
			}
			assert.NoError(task.Insert(t.Context()))
		}

		// The midway task would be t10, but since tasks 7-13 are missing
		// it gets the next task earliest task that is not missing, which is t6.
		t6, err := ByBeforeMidwayTaskFromIds(ctx, "t1", "t20")
		assert.NoError(err)
		require.NotNil(t, t6)
		assert.Equal(6, t6.RevisionOrderNumber)

		t6, err = ByBeforeMidwayTaskFromIds(ctx, "t5", "t20")
		assert.NoError(err)
		require.NotNil(t, t6)
		assert.Equal(6, t6.RevisionOrderNumber)

		t6, err = ByBeforeMidwayTaskFromIds(ctx, "t5", "t16")
		assert.NoError(err)
		require.NotNil(t, t6)
		assert.Equal(6, t6.RevisionOrderNumber)

		t6, err = ByBeforeMidwayTaskFromIds(ctx, "t6", "t14")
		assert.NoError(err)
		require.NotNil(t, t6)
		assert.Equal(6, t6.RevisionOrderNumber)

		// Using a task that doesn't exist should return an error.
		_, err = ByBeforeMidwayTaskFromIds(ctx, "t7", "t20")
		assert.Error(err)
	})
}

func TestUnscheduleStaleUnderwaterHostTasksNoDistro(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	require.NoError(t, db.EnsureIndex(Collection,
		mongo.IndexModel{Keys: ActivatedTasksByDistroIndex}))

	t1 := Task{
		Id:            "t1",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Time{},
	}
	assert.NoError(t1.Insert(t.Context()))

	t2 := Task{
		Id:            "t2",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Time{},
	}
	assert.NoError(t2.Insert(t.Context()))

	tasks, err := UnscheduleStaleUnderwaterHostTasks(ctx, "")
	assert.NoError(err)
	require.Len(t, tasks, 2)
	dbTask, err := FindOneId(ctx, "t1")
	assert.NoError(err)
	assert.False(dbTask.Activated)
	assert.Equal(evergreen.TaskUnscheduled, dbTask.DisplayStatusCache)
	assert.EqualValues(-1, dbTask.Priority)

	dbTask, err = FindOneId(ctx, "t2")
	assert.NoError(err)
	assert.False(dbTask.Activated)
	assert.Equal(evergreen.TaskUnscheduled, dbTask.DisplayStatusCache)
	assert.EqualValues(-1, dbTask.Priority)
}

func TestDeactivateStepbackTasksForProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, event.EventCollection))

	runningStepbackTask := Task{
		Id:           "running",
		Activated:    true,
		Status:       evergreen.TaskStarted, // should be aborted
		Project:      "p1",
		ActivatedBy:  evergreen.StepbackTaskActivator,
		BuildVariant: "myVariant",
		DisplayName:  "myTask",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	taskDependingOnStepbackTask := Task{
		Id:          "dependent_task",
		Activated:   true,
		Status:      evergreen.TaskUndispatched,
		Project:     "p1",
		ActivatedBy: "someone else", // Doesn't matter if dependencies were activated by stepback or not
		DependsOn: []Dependency{
			{
				TaskId: "running",
				Status: evergreen.TaskSucceeded,
			},
		},
		Requester: evergreen.RepotrackerVersionRequester,
	}
	wrongProjectTask := Task{
		Id:           "wrong_project",
		Activated:    true,
		Status:       evergreen.TaskUndispatched,
		Project:      "p2",
		ActivatedBy:  evergreen.StepbackTaskActivator,
		BuildVariant: "myVariant",
		DisplayName:  "myTask",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	wrongTaskNameTask := Task{
		Id:           "wrong_task",
		Activated:    true,
		ActivatedBy:  evergreen.StepbackTaskActivator,
		Status:       evergreen.TaskUndispatched,
		Project:      "p1",
		BuildVariant: "myVariant",
		DisplayName:  "wrongTaskNameTask",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	wrongVariantTask := Task{
		Id:           "wrong_variant",
		Activated:    true,
		ActivatedBy:  evergreen.StepbackTaskActivator,
		Status:       evergreen.TaskUndispatched,
		Project:      "p1",
		BuildVariant: "wrongVariantTask",
		DisplayName:  "myTask",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	notStepbackTask := Task{
		Id:           "not_stepback",
		Activated:    true,
		Status:       evergreen.TaskUndispatched,
		Project:      "p1",
		ActivatedBy:  "me",
		BuildVariant: "myVariant",
		DisplayName:  "myTask",
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, taskDependingOnStepbackTask, wrongProjectTask,
		wrongTaskNameTask, wrongVariantTask, runningStepbackTask, notStepbackTask))
	assert.NoError(t, DeactivateStepbackTask(ctx, "p1", "myVariant", "myTask", "me"))

	events, err := event.Find(t.Context(), db.Q{})
	assert.NoError(t, err)
	assert.Len(t, events, 3)
	var numDeactivated, numAborted int
	for _, e := range events {
		if e.EventType == event.TaskAbortRequest {
			numAborted++
		} else if e.EventType == event.TaskDeactivated {
			numDeactivated++
		}
	}
	assert.Equal(t, 2, numDeactivated)
	assert.Equal(t, 1, numAborted)

	tasks, err := FindAll(ctx, db.Q{})
	assert.NoError(t, err)
	for _, dbTask := range tasks {
		if dbTask.Id == taskDependingOnStepbackTask.Id {
			assert.False(t, dbTask.Activated)
			assert.False(t, dbTask.Aborted)
		} else if dbTask.Id == runningStepbackTask.Id {
			assert.False(t, dbTask.Activated)
			assert.True(t, dbTask.Aborted)
		} else {
			assert.True(t, dbTask.Activated)
			assert.False(t, dbTask.Aborted)
		}
	}
}

func TestUnscheduleStaleUnderwaterHostTasksWithDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, distro.Collection))
	require.NoError(t, db.EnsureIndex(Collection,
		mongo.IndexModel{Keys: ActivatedTasksByDistroIndex}))

	t1 := Task{
		Id:            "t1",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Time{},
		DistroId:      "d0",
	}
	require.NoError(t, t1.Insert(t.Context()))

	d := distro.Distro{
		Id: "d0",
	}
	require.NoError(t, d.Insert(ctx))

	tasks, err := UnscheduleStaleUnderwaterHostTasks(ctx, "d0")
	assert.NoError(t, err)
	require.Len(t, tasks, 1)
	dbTask, err := FindOneId(ctx, "t1")
	assert.NoError(t, err)
	assert.False(t, dbTask.Activated)
	assert.EqualValues(t, -1, dbTask.Priority)
}

func TestUnscheduleStaleUnderwaterHostTasksWithDistroAlias(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, distro.Collection))
	require.NoError(t, db.EnsureIndex(Collection,
		mongo.IndexModel{Keys: ActivatedTasksByDistroIndex}))

	t1 := Task{
		Id:            "t1",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Time{},
		DistroId:      "d0.0",
	}
	require.NoError(t, t1.Insert(t.Context()))

	d := distro.Distro{
		Id:      "d0",
		Aliases: []string{"d0.0", "d0.1"},
	}
	require.NoError(t, d.Insert(ctx))

	tasks, err := UnscheduleStaleUnderwaterHostTasks(ctx, "d0")
	assert.NoError(t, err)
	dbTask, err := FindOneId(ctx, "t1")
	assert.NoError(t, err)
	assert.False(t, dbTask.Activated)
	assert.EqualValues(t, -1, dbTask.Priority)
	require.Len(t, tasks, 1)
}

func TestGetRecentTaskStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection))
	tasks := []Task{
		{Id: "t1", Status: evergreen.TaskSucceeded, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t2", Status: evergreen.TaskSucceeded, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t3", Status: evergreen.TaskSucceeded, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t4", Status: evergreen.TaskSucceeded, DistroId: "d2", FinishTime: time.Now()},
		{Id: "t5", Status: evergreen.TaskFailed, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t6", Status: evergreen.TaskFailed, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t7", Status: evergreen.TaskFailed, DistroId: "d2", FinishTime: time.Now()},
	}
	for _, task := range tasks {
		assert.NoError(task.Insert(t.Context()))
	}

	list, err := GetRecentTaskStats(ctx, time.Minute, DistroIdKey)
	assert.NoError(err)

	// Two statuses
	assert.Len(list, 2)
	// Two distros to report status for
	assert.Len(list[0].Stats, 2)

	for _, status := range list {
		if status.Status == evergreen.TaskSucceeded {
			// Sorted order
			assert.Equal("d1", status.Stats[0].Name)
			assert.Equal(3, status.Stats[0].Count)
			assert.Equal("d2", status.Stats[1].Name)
			assert.Equal(1, status.Stats[1].Count)
		}
		if status.Status == evergreen.TaskFailed {
			// Sorted order
			assert.Equal("d1", status.Stats[0].Name)
			assert.Equal(2, status.Stats[0].Count)
			assert.Equal("d2", status.Stats[1].Name)
			assert.Equal(1, status.Stats[1].Count)
		}
	}
}

func TestGetResultCountList(t *testing.T) {
	assert := assert.New(t)
	statsList := []StatusItem{
		{Status: evergreen.TaskSucceeded, Stats: []Stat{{Name: "d1", Count: 2}, {Name: "d2", Count: 1}}},
		{Status: evergreen.TaskFailed, Stats: []Stat{{Name: "d1", Count: 3}, {Name: "d2", Count: 2}}},
	}

	list := GetResultCountList(statsList)
	_, ok := list[evergreen.TaskSucceeded]
	assert.True(ok)
	_, ok = list[evergreen.TaskFailed]
	assert.True(ok)

	assert.Len(list["totals"], 2)
	assert.Equal("d1", list["totals"][0].Name)
	assert.Equal(5, list["totals"][0].Count)
	assert.Equal("d2", list["totals"][1].Name)
	assert.Equal(3, list["totals"][1].Count)
}

func TestFindVariantsWithTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.Clear(Collection))
	tasks := Tasks{
		&Task{Id: "1", DisplayName: "match", Project: "p", Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 15, BuildVariant: "bv1"},
		&Task{Id: "2", DisplayName: "match", Project: "p", Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 12, BuildVariant: "bv2"},
		&Task{Id: "3", DisplayName: "nomatch", Project: "p", Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 14, BuildVariant: "bv1"},
		&Task{Id: "4", DisplayName: "match", Project: "p", Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 50, BuildVariant: "bv1"},
	}
	assert.NoError(tasks.Insert(t.Context()))

	bvs, err := FindVariantsWithTask(ctx, "match", "p", 10, 20)
	assert.NoError(err)
	require.Len(t, bvs, 2)
	assert.Contains(bvs, "bv1")
	assert.Contains(bvs, "bv2")
}

func TestAddDependency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, tsk *Task){
		"AddingDuplicateDependencyIsNoop": func(t *testing.T, tsk *Task) {
			assert.NoError(t, tsk.AddDependency(ctx, depTaskIds[0]))

			updated, err := FindOneId(ctx, tsk.Id)
			assert.NoError(t, err)
			require.NotZero(t, updated)
			assert.Equal(t, tsk.DependsOn, updated.DependsOn)
			assert.Len(t, updated.DependsOn, len(depTaskIds))
		},
		"UpdatesDuplicateDependencyForUnattainability": func(t *testing.T, tsk *Task) {
			assert.NoError(t, tsk.AddDependency(ctx, Dependency{
				TaskId:       depTaskIds[0].TaskId,
				Status:       evergreen.TaskSucceeded,
				Unattainable: true,
			}))

			updated, err := FindOneId(ctx, tsk.Id)
			assert.NoError(t, err)
			require.NotZero(t, updated)
			require.Len(t, updated.DependsOn, len(depTaskIds))
			assert.True(t, updated.DependsOn[0].Unattainable)
			assert.Equal(t, evergreen.TaskStatusBlocked, updated.DisplayStatusCache)
		},
		"AddsDependencyForSameTaskButDifferentStatus": func(t *testing.T, tsk *Task) {
			assert.NoError(t, tsk.AddDependency(ctx, Dependency{
				TaskId: depTaskIds[0].TaskId,
				Status: evergreen.TaskFailed,
			}))

			updated, err := FindOneId(ctx, tsk.Id)
			assert.NoError(t, err)
			require.NotZero(t, updated)
			assert.Len(t, updated.DependsOn, len(depTaskIds)+1)
		},
		"AddingSelfDependencyShouldNoop": func(t *testing.T, tsk *Task) {
			assert.NoError(t, tsk.AddDependency(ctx, Dependency{
				TaskId: tsk.Id,
			}))

			updated, err := FindOneId(ctx, tsk.Id)
			assert.NoError(t, err)
			require.NotZero(t, updated)
			assert.Len(t, updated.DependsOn, len(depTaskIds))
			for _, d := range updated.DependsOn {
				assert.NotEqual(t, d.TaskId, tsk.Id, "task should not add dependency on itself")
			}
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))

			tsk := &Task{Id: "t1", DependsOn: depTaskIds, Status: evergreen.TaskUndispatched, Activated: true}
			require.NoError(t, tsk.Insert(t.Context()))

			tCase(t, tsk)
		})
	}
}

func TestUnattainableSchedulableHostTasksQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection))
	tasks := []Task{
		{
			Id:        "t0",
			Activated: true,
			Status:    evergreen.TaskUndispatched,
			DependsOn: []Dependency{
				{
					TaskId:       "t10",
					Unattainable: false,
				},
				{
					TaskId:       "t11",
					Unattainable: true,
				},
			},
			UnattainableDependency: true,
			Priority:               0,
		},
		{
			Id:        "t1",
			Activated: true,
			Status:    evergreen.TaskUndispatched,
			DependsOn: []Dependency{
				{
					TaskId:       "t10",
					Unattainable: false,
				},
				{
					TaskId:       "t11",
					Unattainable: false,
				},
			},
			UnattainableDependency: false,
			Priority:               0,
		},
		{
			Id:        "t2",
			Activated: true,
			Status:    evergreen.TaskUndispatched,
			Priority:  0,
			DependsOn: []Dependency{
				{
					TaskId:       "t10",
					Unattainable: true,
				},
			},
			UnattainableDependency: true,
			OverrideDependencies:   true,
		},
	}
	for _, task := range tasks {
		assert.NoError(task.Insert(t.Context()))
	}

	q := db.Query(schedulableHostTasksQuery())
	schedulableTasks, err := FindAll(ctx, q)
	assert.NoError(err)
	assert.Len(schedulableTasks, 2)
}

func TestGetTimeSpent(t *testing.T) {
	assert := assert.New(t)
	referenceTime := time.Unix(1136239445, 0)
	tasks := []Task{
		{
			StartTime:  referenceTime,
			FinishTime: referenceTime.Add(time.Hour),
			TimeTaken:  time.Hour,
		},
		{
			StartTime:  referenceTime,
			FinishTime: referenceTime.Add(2 * time.Hour),
			TimeTaken:  2 * time.Hour,
		},
		{
			DisplayOnly: true,
			FinishTime:  referenceTime.Add(3 * time.Hour),
			TimeTaken:   2 * time.Hour,
		},
		{
			StartTime:  referenceTime,
			FinishTime: utility.ZeroTime,
			TimeTaken:  0,
		},
		{
			StartTime:  utility.ZeroTime,
			FinishTime: utility.ZeroTime,
			TimeTaken:  0,
		},
	}

	timeTaken, makespan := GetTimeSpent(tasks)
	assert.Equal(3*time.Hour, timeTaken)
	assert.Equal(2*time.Hour, makespan)

	timeTaken, makespan = GetTimeSpent(tasks[2:])
	assert.EqualValues(0, timeTaken)
	assert.EqualValues(0, makespan)
}

func TestGetFormattedTimeSpent(t *testing.T) {
	assert := assert.New(t)
	referenceTime := time.Unix(1136239445, 0)
	tasks := []Task{
		{
			StartTime:  referenceTime,
			FinishTime: referenceTime.Add(time.Hour),
			TimeTaken:  time.Hour,
		},
		{
			StartTime:  referenceTime,
			FinishTime: referenceTime.Add(2 * time.Hour),
			TimeTaken:  2 * time.Hour,
		},
		{
			DisplayOnly: true,
			FinishTime:  referenceTime.Add(3 * time.Hour),
			TimeTaken:   2 * time.Hour,
		},
		{
			StartTime:  referenceTime,
			FinishTime: utility.ZeroTime,
			TimeTaken:  0,
		},
		{
			StartTime:  utility.ZeroTime,
			FinishTime: utility.ZeroTime,
			TimeTaken:  0,
		},
	}

	timeTaken, makespan := GetFormattedTimeSpent(tasks)
	assert.Equal("2h 0m 0s", makespan)
	assert.Equal("3h 0m 0s", timeTaken)

	timeTaken, makespan = GetFormattedTimeSpent(tasks[2:])
	assert.Equal("0s", makespan)
	assert.Equal("0s", timeTaken)
}

func TestUpdateDependsOn(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(Collection))
	t1 := &Task{Id: "t1"}
	assert.NoError(t, t1.Insert(ctx))
	t2 := &Task{
		Id: "t2",
		DependsOn: []Dependency{
			{TaskId: "t1", Status: evergreen.TaskSucceeded},
			{TaskId: "t5", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t, t2.Insert(ctx))

	var err error
	assert.NoError(t, t1.UpdateDependsOn(ctx, evergreen.TaskFailed, []string{"t3", "t4"}))
	t2, err = FindOneId(ctx, "t2")
	assert.NoError(t, err)
	assert.Len(t, t2.DependsOn, 2)

	assert.NoError(t, t1.UpdateDependsOn(ctx, evergreen.TaskSucceeded, []string{"t3", "t4"}))
	t2, err = FindOneId(ctx, "t2")
	assert.NoError(t, err)
	assert.Len(t, t2.DependsOn, 4)

	t.Run("AddingSelfDependencyShouldNoop", func(t *testing.T) {
		assert.NoError(t, t1.UpdateDependsOn(ctx, evergreen.TaskSucceeded, []string{t1.Id}))
		dbTask1, err := FindOneId(ctx, t1.Id)
		assert.NoError(t, err)
		require.NotZero(t, dbTask1)
		for _, d := range dbTask1.DependsOn {
			assert.NotEqual(t, t1.Id, d.TaskId, "task should not add dependency on itself")
		}
	})
	t.Run("AddingSelfDependencyThroughExistingParentDependencyShouldNoop", func(t *testing.T) {
		assert.NoError(t, t2.UpdateDependsOn(ctx, evergreen.TaskSucceeded, []string{t1.Id}))
		dbTask2, err := FindOneId(ctx, t2.Id)
		require.NoError(t, err)
		require.NotZero(t, dbTask2)
		for _, d := range dbTask2.DependsOn {
			assert.NotEqual(t, t2.Id, d.TaskId, "task should not add a dependency on itself transitively through a parent dependency")
		}
	})
}

func TestMarkGeneratedTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	t1 := &Task{
		Id: "t1",
	}
	require.NoError(t, t1.Insert(t.Context()))

	mockError := errors.New("mock error")

	require.NoError(t, MarkGeneratedTasks(ctx, t1.Id))
	found, err := FindOneId(ctx, t1.Id)
	require.NoError(t, err)
	require.True(t, found.GeneratedTasks)
	require.Equal(t, "", found.GenerateTasksError)

	require.NoError(t, MarkGeneratedTasks(ctx, t1.Id))
	require.NoError(t, MarkGeneratedTasksErr(ctx, t1.Id, mockError))
	found, err = FindOneId(ctx, t1.Id)
	require.NoError(t, err)
	require.True(t, found.GeneratedTasks)
	require.Equal(t, "", found.GenerateTasksError, "calling after GeneratedTasks is set should not set an error")

	t3 := &Task{
		Id: "t3",
	}
	require.NoError(t, t3.Insert(t.Context()))
	require.NoError(t, MarkGeneratedTasksErr(ctx, t3.Id, mongo.ErrNoDocuments))
	found, err = FindOneId(ctx, t3.Id)
	require.NoError(t, err)
	require.False(t, found.GeneratedTasks, "document not found should not set generated tasks, since this was a race and did not generate.tasks")
	require.Equal(t, "", found.GenerateTasksError)

	t4 := &Task{
		Id: "t4",
	}
	dupError := errors.New("duplicate key error")
	require.NoError(t, t4.Insert(t.Context()))
	require.NoError(t, MarkGeneratedTasksErr(ctx, t4.Id, dupError))
	found, err = FindOneId(ctx, t4.Id)
	require.NoError(t, err)
	require.False(t, found.GeneratedTasks, "duplicate key error should not set generated tasks, since this was a race and did not generate.tasks")
	require.Equal(t, "", found.GenerateTasksError)
}

func TestGetAllDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	tasks := []Task{
		{
			Id:        "t0",
			DependsOn: []Dependency{{TaskId: "dependedOn0"}},
		},
		{
			Id:        "t1",
			DependsOn: []Dependency{{TaskId: "dependedOn1"}},
		},
	}
	// not in the map and not in the db
	dependencies, err := GetAllDependencies(ctx, []string{tasks[0].Id}, map[string]*Task{})
	assert.Error(t, err)
	assert.Nil(t, dependencies)

	// in the map
	dependencies, err = GetAllDependencies(ctx, []string{tasks[0].Id}, map[string]*Task{tasks[0].Id: &tasks[0]})
	assert.NoError(t, err)
	assert.Len(t, dependencies, 1)
	assert.Equal(t, "dependedOn0", dependencies[0].TaskId)

	// mix of map and db
	require.NoError(t, tasks[1].Insert(t.Context()))
	taskMap := map[string]*Task{tasks[0].Id: &tasks[0]}
	dependencies, err = GetAllDependencies(ctx, []string{tasks[0].Id, tasks[1].Id}, taskMap)
	assert.NoError(t, err)
	assert.Len(t, dependencies, 2)
	assert.Len(t, taskMap, 1)
}

func TestGetRecursiveDependenciesUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1"},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t1"}, {TaskId: "t0"}}},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t1"}}},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t2"}}},
		{Id: "t5", DependsOn: []Dependency{{TaskId: "t4"}}},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	taskDependsOn, err := GetRecursiveDependenciesUp(ctx, []Task{tasks[3], tasks[4]}, nil)
	assert.NoError(t, err)
	assert.Len(t, taskDependsOn, 3)
	expectedIDs := []string{"t2", "t1", "t0"}
	for _, task := range taskDependsOn {
		assert.Contains(t, expectedIDs, task.Id)
	}
}

func TestGetRecursiveDependenciesUpWithTaskGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	tasks := []Task{
		{Id: "t0", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 0},
		{Id: "t1", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 1},
		{Id: "t2", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 2},
		{Id: "t3", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 3},
		{Id: "t4", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 4},
	}

	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}
	taskDependsOn, err := GetRecursiveDependenciesUp(ctx, []Task{tasks[2], tasks[3]}, nil)
	assert.NoError(t, err)
	assert.Len(t, taskDependsOn, 2)
	expectedIDs := []string{"t0", "t1"}
	for _, task := range taskDependsOn {
		assert.Contains(t, expectedIDs, task.Id)
	}
}

func TestGetRecursiveDependenciesDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1"},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t1"}, {TaskId: "t0"}}},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t1"}}},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t2"}}},
		{Id: "t5", DependsOn: []Dependency{{TaskId: "t4"}}},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	dependingOnMe, err := getRecursiveDependenciesDown(ctx, []string{"t0"}, nil)
	assert.NoError(t, err)
	assert.Len(t, dependingOnMe, 3)
	expectedIDs := []string{"t2", "t4", "t5"}
	for _, task := range dependingOnMe {
		assert.Contains(t, expectedIDs, task.Id)
	}
}

func TestDeactivateDependencies(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(Collection, event.EventCollection))

	tasks := []Task{
		{Id: "t0", Status: evergreen.TaskUndispatched, DisplayStatusCache: evergreen.TaskWillRun},
		{Id: "t1", Status: evergreen.TaskUndispatched, DisplayStatusCache: evergreen.TaskWillRun},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t1"}, {TaskId: "t0"}}, Activated: false, Status: evergreen.TaskUndispatched, DisplayStatusCache: evergreen.TaskWillRun},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t1"}}, Status: evergreen.TaskUndispatched, DisplayStatusCache: evergreen.TaskWillRun},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t2"}}, Activated: true, Status: evergreen.TaskUndispatched, DisplayStatusCache: evergreen.TaskWillRun},
		{Id: "t5", DependsOn: []Dependency{{TaskId: "t4"}}, Activated: true, Status: evergreen.TaskUndispatched, DisplayStatusCache: evergreen.TaskWillRun},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	updatedIDs := []string{"t4", "t5"}
	err := DeactivateDependencies(ctx, []string{"t0"}, "")
	assert.NoError(t, err)

	dbTasks, err := FindAll(ctx, All)
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 6)

	for _, task := range dbTasks {
		if utility.StringSliceContains(updatedIDs, task.Id) {
			assert.False(t, task.Activated)
			assert.True(t, task.DeactivatedForDependency)
			assert.Equal(t, evergreen.TaskUnscheduled, task.DisplayStatusCache,
				"DisplayStatusCache should be updated to 'unscheduled' for deactivated task %s", task.Id)
		} else {
			for _, origTask := range tasks {
				if origTask.Id == task.Id {
					assert.Equal(t, origTask.Activated, task.Activated)
				}
			}
		}
	}
}

func TestActivateDeactivatedDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, event.EventCollection))

	tasks := []Task{
		{Id: "t0"},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t0"}}, Activated: false},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t1"}}, Activated: false, DeactivatedForDependency: true},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t0", Unattainable: true}}, Activated: false, DeactivatedForDependency: true},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t0", Unattainable: true}, {TaskId: "t3"}}, Activated: false, DeactivatedForDependency: true},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	updatedIDs := []string{"t3", "t4"}
	depTasksToUpdate, depTaskIDsToUpdate, err := getDependencyTaskIdsToActivate(ctx, []string{"t0"}, true)
	require.NoError(t, err)
	err = activateDeactivatedDependencies(ctx, depTasksToUpdate, depTaskIDsToUpdate, "")
	assert.NoError(t, err)

	dbTasks, err := FindAll(ctx, All)
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 5)

	for _, task := range dbTasks {
		if utility.StringSliceContains(updatedIDs, task.Id) {
			assert.True(t, task.Activated)
			assert.False(t, task.DeactivatedForDependency)
		} else {
			for _, origTask := range tasks {
				if origTask.Id == task.Id {
					assert.Equal(t, origTask.Activated, task.Activated)
				}
			}
		}
	}
}

func TestTopologicalSort(t *testing.T) {
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t2"}}},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t0"}}},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t1"}}},
	}

	sortedTasks, err := topologicalSort(tasks)
	assert.NoError(t, err)
	assert.Len(t, sortedTasks, 4)

	indexMap := make(map[string]int)
	for i, task := range sortedTasks {
		indexMap[task.Id] = i
	}

	assert.Less(t, indexMap["t0"], indexMap["t1"])
	assert.Less(t, indexMap["t0"], indexMap["t2"])
	assert.Less(t, indexMap["t0"], indexMap["t3"])
	assert.Less(t, indexMap["t2"], indexMap["t1"])
	assert.Less(t, indexMap["t1"], indexMap["t3"])
}

func TestActivateTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, event.EventCollection, user.Collection))
	}()

	t.Run("DependencyChain", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection, event.EventCollection, user.Collection))
		u := &user.DBUser{
			Id: "user",
		}
		require.NoError(t, u.Insert(t.Context()))
		tasks := []Task{
			{Id: "t0", Requester: evergreen.PatchVersionRequester, Priority: evergreen.DisabledTaskPriority},
			{Id: "t1", Requester: evergreen.PatchVersionRequester, DependsOn: []Dependency{{TaskId: "t0"}}, Activated: false, EstimatedNumActivatedGeneratedTasks: utility.ToIntPtr(100)},
			{Id: "t2", Requester: evergreen.PatchVersionRequester, DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t1"}}, Activated: false, DeactivatedForDependency: true},
			{Id: "t3", Requester: evergreen.PatchVersionRequester, DependsOn: []Dependency{{TaskId: "t0"}}, Activated: false, DeactivatedForDependency: true},
			{Id: "t4", Requester: evergreen.PatchVersionRequester, DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t3"}}, Activated: false, DeactivatedForDependency: true},
			{Id: "t5", Requester: evergreen.PatchVersionRequester, DependsOn: []Dependency{{TaskId: "t0"}}, Activated: true, DeactivatedForDependency: true},
		}
		for _, task := range tasks {
			require.NoError(t, task.Insert(t.Context()))
		}

		updatedIDs := []string{"t0", "t3", "t4"}
		activatedDependencyIDs, err := ActivateTasks(ctx, []Task{tasks[0]}, time.Time{}, true, u.Id)
		assert.NoError(t, err)
		assert.ElementsMatch(t, updatedIDs, activatedDependencyIDs)

		u, err = user.FindOne(ctx, user.ById(u.Id))
		require.NoError(t, err)
		require.NotNil(t, u)
		assert.Len(t, updatedIDs, u.NumScheduledPatchTasks)

		dbTasks, err := FindAll(ctx, All)
		assert.NoError(t, err)
		assert.Len(t, dbTasks, 6)

		for _, task := range dbTasks {
			assert.EqualValues(t, 0, task.Priority)
			if utility.StringSliceContains(updatedIDs, task.Id) {
				assert.True(t, task.Activated)
				events, err := event.FindAllByResourceID(t.Context(), task.Id)
				require.NoError(t, err)
				assert.Len(t, events, 1)
			} else {
				for _, origTask := range tasks {
					if origTask.Id == task.Id {
						assert.Equal(t, origTask.Activated, task.Activated, "task '%s' mismatch", task.Id)
					}
				}
				events, err := event.FindAllByResourceID(t.Context(), task.Id)
				require.NoError(t, err)
				assert.Empty(t, events)
			}
		}

		activatedDependencyIDs, err = ActivateTasks(ctx, []Task{tasks[1]}, time.Time{}, true, u.Id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf("cannot schedule %d tasks, maximum hourly per-user limit is %d", 102, 100))
		assert.Empty(t, activatedDependencyIDs)
	})

	t.Run("NoopActivatedTask", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection, event.EventCollection))
		task := Task{
			Id:            "t0",
			Activated:     true,
			ActivatedTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
			ActivatedBy:   "octocat",
		}
		require.NoError(t, task.Insert(t.Context()))

		activatedDependencyIDs, err := ActivateTasks(ctx, []Task{task}, time.Now(), true, "abyssinian")
		assert.NoError(t, err)
		assert.Empty(t, activatedDependencyIDs)

		events, err := event.FindAllByResourceID(t.Context(), task.Id)
		require.NoError(t, err)
		assert.Empty(t, events)

		dbTask, err := FindOneId(ctx, task.Id)
		require.NoError(t, err)
		require.NotNil(t, dbTask)
		assert.True(t, task.Activated)
		assert.True(t, task.ActivatedTime.Equal(dbTask.ActivatedTime))
		assert.Equal(t, task.ActivatedBy, dbTask.ActivatedBy)
	})
}

func TestDeactivateTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, event.EventCollection))

	tasks := []Task{
		{Id: "t0", DisplayOnly: true, ExecutionTasks: []string{"t6"}, Activated: true},
		{Id: "t1"},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t1"}, {TaskId: "t0"}}, Activated: false},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t1"}}},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t2"}}, Activated: true},
		{Id: "t5", DependsOn: []Dependency{{TaskId: "t4"}}, Activated: true},
		{Id: "t6", Activated: true},
		{Id: "t7", DependsOn: []Dependency{{TaskId: "t6"}}, Activated: true},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert(t.Context()))
	}

	updatedIDs := []string{"t0", "t4", "t5", "t6", "t7"}
	err := DeactivateTasks(ctx, []Task{tasks[0]}, true, "")
	assert.NoError(t, err)

	dbTasks, err := FindAll(ctx, All)
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 8)

	for _, task := range dbTasks {
		if utility.StringSliceContains(updatedIDs, task.Id) {
			assert.False(t, task.Activated)
		} else {
			for _, origTask := range tasks {
				if origTask.Id == task.Id {
					assert.Equal(t, origTask.Activated, task.Activated)
				}
			}
		}
	}
}

func TestMarkAsContainerDispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	getDispatchableContainerTasks := func() []Task {
		return []Task{
			{
				Id:                 "should_not_be_dispatched",
				Activated:          true,
				ActivatedTime:      time.Now(),
				Status:             evergreen.TaskUndispatched,
				ContainerAllocated: true,
				ExecutionPlatform:  ExecutionPlatformContainer,
			},
			{
				Id:                 "should_be_dispatched",
				Activated:          true,
				ActivatedTime:      time.Now(),
				Status:             evergreen.TaskUndispatched,
				ContainerAllocated: true,
				ExecutionPlatform:  ExecutionPlatformContainer,
			},
		}
	}

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	const podID = "pod_id"

	checkTaskDispatched := func(t *testing.T, taskID string) {
		dbTask, err := FindOneId(ctx, taskID)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.Equal(t, evergreen.TaskDispatched, dbTask.Status)
		assert.False(t, utility.IsZeroTime(dbTask.DispatchTime))
		assert.False(t, utility.IsZeroTime(dbTask.LastHeartbeat))
		assert.Equal(t, podID, dbTask.PodID)
		assert.Equal(t, evergreen.AgentVersion, dbTask.AgentVersion)
		output, ok := dbTask.initializeTaskOutputInfo(env)
		require.True(t, ok)
		assert.Equal(t, output, dbTask.TaskOutputInfo)
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, tsks []Task){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, tsks []Task) {
			for _, tsk := range tsks {
				require.NoError(t, tsk.Insert(t.Context()))
			}
			require.NoError(t, tsks[1].MarkAsContainerDispatched(ctx, env, podID, evergreen.AgentVersion))
			checkTaskDispatched(t, tsks[1].Id)
		},
		"FailsWithTaskWithoutContainerAllocated": func(ctx context.Context, t *testing.T, env *mock.Environment, tsks []Task) {
			tsks[1].ContainerAllocated = false
			for _, tsk := range tsks {
				require.NoError(t, tsk.Insert(t.Context()))
			}

			assert.Error(t, tsks[1].MarkAsContainerDispatched(ctx, env, podID, evergreen.AgentVersion))
		},
		"FailsWithDeactivatedTasks": func(ctx context.Context, t *testing.T, env *mock.Environment, tsks []Task) {
			tsks[1].Activated = false
			for _, tsk := range tsks {
				require.NoError(t, tsk.Insert(t.Context()))
			}

			assert.Error(t, tsks[1].MarkAsContainerDispatched(ctx, env, podID, evergreen.AgentVersion))
		},
		"FailsWithDisabledTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsks []Task) {
			tsks[1].Priority = evergreen.DisabledTaskPriority
			for _, tsk := range tsks {
				require.NoError(t, tsk.Insert(t.Context()))
			}

			assert.Error(t, tsks[1].MarkAsContainerDispatched(ctx, env, podID, evergreen.AgentVersion))
		},
		"FailsWithUnmetDependencies": func(ctx context.Context, t *testing.T, env *mock.Environment, tsks []Task) {
			tsks[1].DependsOn = []Dependency{
				{TaskId: "task", Finished: true, Unattainable: true},
			}
			for _, tsk := range tsks {
				require.NoError(t, tsk.Insert(t.Context()))
			}

			assert.Error(t, tsks[1].MarkAsContainerDispatched(ctx, env, podID, evergreen.AgentVersion))
		},
		"FailsWithNonexistentTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsks []Task) {
			require.Error(t, tsks[1].MarkAsContainerDispatched(ctx, env, podID, evergreen.AgentVersion))

			dbTask, err := FindOneId(ctx, tsks[1].Id)
			assert.NoError(t, err)
			assert.Zero(t, dbTask)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			require.NoError(t, db.Clear(Collection))

			tCase(tctx, t, env, getDispatchableContainerTasks())
		})
	}
}

func TestMarkAsContainerAllocated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	checkTaskAllocated := func(t *testing.T, taskID string) {
		dbTask, err := FindOneId(ctx, taskID)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.True(t, dbTask.ContainerAllocated)
		assert.False(t, utility.IsZeroTime(dbTask.ContainerAllocatedTime))
		assert.Zero(t, dbTask.AgentVersion)
		assert.NotZero(t, dbTask.ContainerAllocationAttempts)
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, tsk.MarkAsContainerAllocated(ctx, env))
			checkTaskAllocated(t, tsk.Id)
		},
		"FailsWithAllocatedTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.ContainerAllocated = true
			require.NoError(t, tsk.Insert(t.Context()))

			assert.Error(t, tsk.MarkAsContainerAllocated(ctx, env))
		},
		"FailsWithAllocatedDBTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.ContainerAllocated = true
			require.NoError(t, tsk.Insert(t.Context()))
			tsk.ContainerAllocated = false

			assert.Error(t, tsk.MarkAsContainerAllocated(ctx, env))
		},
		"FailsWithTaskWithNoRemainingAllocationAttempts": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.ContainerAllocationAttempts = maxContainerAllocationAttempts
			require.NoError(t, tsk.Insert(t.Context()))

			assert.Error(t, tsk.MarkAsContainerAllocated(ctx, env))
		},
		"FailsWithDBTaskWithNoRemainingAllocationAttempts": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.ContainerAllocationAttempts = maxContainerAllocationAttempts
			require.NoError(t, tsk.Insert(t.Context()))
			tsk.ContainerAllocationAttempts = 0

			assert.Error(t, tsk.MarkAsContainerAllocated(ctx, env))
		},
		"FailsWithInactiveTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.Activated = false
			require.NoError(t, tsk.Insert(t.Context()))

			assert.Error(t, tsk.MarkAsContainerAllocated(ctx, env))
		},
		"FailsForTaskWithStatusOtherThanUndispatched": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.Status = evergreen.TaskSucceeded
			require.NoError(t, tsk.Insert(t.Context()))

			assert.Error(t, tsk.MarkAsContainerAllocated(ctx, env))
		},
		"FailsForTaskWithUnmetDependencies": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.DependsOn = []Dependency{
				{
					TaskId:   "dependency",
					Finished: false,
				},
			}
			require.NoError(t, tsk.Insert(t.Context()))

			assert.Error(t, tsk.MarkAsContainerAllocated(ctx, env))
		},
		"FailsForHostTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.ExecutionPlatform = ExecutionPlatformHost
			require.NoError(t, tsk.Insert(t.Context()))

			assert.Error(t, tsk.MarkAsContainerAllocated(ctx, env))
		},
		"FailsWithNonexistentTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			require.Error(t, tsk.MarkAsContainerAllocated(ctx, env))

			dbTask, err := FindOneId(ctx, tsk.Id)
			assert.NoError(t, err)
			assert.Zero(t, dbTask)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			require.NoError(t, db.Clear(Collection))
			tsk := Task{
				Id:                utility.RandomString(),
				Activated:         true,
				ActivatedTime:     time.Now(),
				Status:            evergreen.TaskUndispatched,
				ExecutionPlatform: ExecutionPlatformContainer,
			}

			tCase(tctx, t, env, tsk)
		})
	}
}

func TestMarkAsContainerDeallocated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	checkTaskUnallocated := func(t *testing.T, taskID string) {
		dbTask, err := FindOneId(ctx, taskID)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.False(t, dbTask.ContainerAllocated)
		assert.True(t, utility.IsZeroTime(dbTask.ContainerAllocatedTime))
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, tsk.MarkAsContainerDeallocated(ctx, env))
			checkTaskUnallocated(t, tsk.Id)
		},
		"FailsWithUnallocatedTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.ContainerAllocated = false
			require.NoError(t, tsk.Insert(t.Context()))

			assert.Error(t, tsk.MarkAsContainerDeallocated(ctx, env))
		},
		"FailsWithHostTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.ExecutionPlatform = ExecutionPlatformHost
			require.NoError(t, tsk.Insert(t.Context()))

			assert.Error(t, tsk.MarkAsContainerDeallocated(ctx, env))
		},
		"FailsWithUnallocatedDBTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			tsk.ContainerAllocated = false
			require.NoError(t, tsk.Insert(t.Context()))
			tsk.ContainerAllocated = true

			assert.Error(t, tsk.MarkAsContainerDeallocated(ctx, env))
		},
		"FailsWithNonexistentTask": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk Task) {
			require.Error(t, tsk.MarkAsContainerDeallocated(ctx, env))

			dbTask, err := FindOneId(ctx, tsk.Id)
			assert.NoError(t, err)
			assert.Zero(t, dbTask)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			require.NoError(t, db.Clear(Collection))
			tsk := Task{
				Id:                     utility.RandomString(),
				Activated:              true,
				ActivatedTime:          time.Now(),
				Status:                 evergreen.TaskUndispatched,
				ContainerAllocated:     true,
				ContainerAllocatedTime: time.Now(),
				ExecutionPlatform:      ExecutionPlatformContainer,
			}

			tCase(tctx, t, env, tsk)
		})
	}
}

func TestMarkTasksAsContainerDeallocated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	checkTasksUnallocated := func(t *testing.T, taskIDs []string) {
		for _, taskID := range taskIDs {
			dbTask, err := FindOneId(ctx, taskID)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.False(t, dbTask.ContainerAllocated)
			assert.True(t, utility.IsZeroTime(dbTask.ContainerAllocatedTime))
		}
	}

	for tName, tCase := range map[string]func(t *testing.T, tasks []Task){
		"Succeeds": func(t *testing.T, tasks []Task) {
			var taskIDs []string
			for _, tsk := range tasks {
				require.NoError(t, tsk.Insert(t.Context()))
				taskIDs = append(taskIDs, tsk.Id)
			}

			require.NoError(t, MarkTasksAsContainerDeallocated(ctx, taskIDs))
			checkTasksUnallocated(t, taskIDs)
		},
		"NoopsWithHostTask": func(t *testing.T, tasks []Task) {
			tasks[0].ExecutionPlatform = ExecutionPlatformHost
			var taskIDs []string
			for _, tsk := range tasks {
				require.NoError(t, tsk.Insert(t.Context()))
				taskIDs = append(taskIDs, tsk.Id)
			}

			require.NoError(t, MarkTasksAsContainerDeallocated(ctx, taskIDs))
			checkTasksUnallocated(t, taskIDs[1:])
			dbHostTask, err := FindOneId(ctx, tasks[0].Id)
			require.NoError(t, err)
			assert.Equal(t, tasks[0].ContainerAllocated, dbHostTask.ContainerAllocated, "host task should not be updated")
			assert.NotZero(t, dbHostTask.LastHeartbeat, "host task should not be updated")
			assert.NotZero(t, dbHostTask.DispatchTime, "host task should not be updated")
		},
		"UpdatesTaskThatIsAlreadyContainerUnallocated": func(t *testing.T, tasks []Task) {
			tasks[0].ContainerAllocated = false
			var taskIDs []string
			for _, tsk := range tasks {
				require.NoError(t, tsk.Insert(t.Context()))
				taskIDs = append(taskIDs, tsk.Id)
			}

			require.NoError(t, MarkTasksAsContainerDeallocated(ctx, taskIDs))
			checkTasksUnallocated(t, taskIDs)
		},
		"DoesNotUpdateNonexistentTask": func(t *testing.T, tasks []Task) {
			taskIDs := []string{tasks[0].Id}
			for _, tsk := range tasks[1:] {
				require.NoError(t, tsk.Insert(t.Context()))
				taskIDs = append(taskIDs, tsk.Id)
			}

			require.NoError(t, MarkTasksAsContainerDeallocated(ctx, taskIDs))
			checkTasksUnallocated(t, taskIDs[1:])

			dbTask, err := FindOneId(ctx, tasks[0].Id)
			assert.NoError(t, err)
			assert.Zero(t, dbTask)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			ts := utility.BSONTime(time.Now())
			tasks := []Task{
				{
					Id:                     utility.RandomString(),
					Activated:              true,
					ActivatedTime:          time.Now(),
					Status:                 evergreen.TaskUndispatched,
					ContainerAllocated:     true,
					ContainerAllocatedTime: ts,
					DispatchTime:           ts,
					LastHeartbeat:          ts,
					ExecutionPlatform:      ExecutionPlatformContainer,
				},
				{
					Id:                     utility.RandomString(),
					Activated:              true,
					ActivatedTime:          ts,
					Status:                 evergreen.TaskDispatched,
					ContainerAllocated:     true,
					ContainerAllocatedTime: ts,
					DispatchTime:           ts,
					LastHeartbeat:          ts,
					ExecutionPlatform:      ExecutionPlatformContainer,
				},
				{
					Id:                     utility.RandomString(),
					Activated:              true,
					ActivatedTime:          ts,
					Status:                 evergreen.TaskStarted,
					ContainerAllocated:     true,
					ContainerAllocatedTime: ts,
					DispatchTime:           ts,
					StartTime:              ts,
					LastHeartbeat:          ts,
					ExecutionPlatform:      ExecutionPlatformContainer,
				},
			}

			tCase(t, tasks)
		})
	}
}

func TestIsDispatchable(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, tsk Task){
		"ReturnsTrueForHostTask": func(t *testing.T, tsk Task) {
			assert.True(t, tsk.IsDispatchable())
		},
		"ReturnsTrueForTaskWithDefaultedHostExecutionPlatform": func(t *testing.T, tsk Task) {
			tsk.ExecutionPlatform = ""
			assert.True(t, tsk.IsDispatchable())
		},
		"ReturnsTrueForContainerTaskWithoutContainerAllocated": func(t *testing.T, tsk Task) {
			tsk.ExecutionPlatform = ExecutionPlatformContainer
			tsk.ContainerAllocated = false
			assert.True(t, tsk.IsDispatchable())
		},
		"ReturnsTrueForContainerTaskWithContainerAllocated": func(t *testing.T, tsk Task) {
			tsk.ExecutionPlatform = ExecutionPlatformContainer
			tsk.ContainerAllocated = true
			assert.True(t, tsk.IsDispatchable())
		},
		"ReturnsFalseForTaskWithoutUndispatchedStatus": func(t *testing.T, tsk Task) {
			tsk.Status = evergreen.TaskDispatched
			assert.False(t, tsk.IsDispatchable())
		},
		"ReturnsFalseForInactiveTask": func(t *testing.T, tsk Task) {
			tsk.Activated = false
			assert.False(t, tsk.IsDispatchable())
		},
		"ReturnsFalseForDisplayTask": func(t *testing.T, tsk Task) {
			tsk.DisplayOnly = true
			tsk.ExecutionPlatform = ""
			tsk.ExecutionTasks = []string{"exec-task0", "exec-task1"}
			assert.False(t, tsk.IsDispatchable())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			hostDispatchableTask := Task{
				Id:                "task-id",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				ExecutionPlatform: ExecutionPlatformHost,
			}
			tCase(t, hostDispatchableTask)
		})
	}
}

func TestIsHostDispatchable(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, tsk Task){
		"ReturnsTrueForExpectedTask": func(t *testing.T, tsk Task) {
			assert.True(t, tsk.IsHostDispatchable())
		},
		"ReturnsTrueForTaskWithDefaultedHostExecutionPlatform": func(t *testing.T, tsk Task) {
			tsk.ExecutionPlatform = ""
			assert.True(t, tsk.IsHostDispatchable())
		},
		"ReturnsFalseForContainerTask": func(t *testing.T, tsk Task) {
			tsk.ExecutionPlatform = ExecutionPlatformContainer
			assert.False(t, tsk.IsHostDispatchable())
		},
		"ReturnsFalseForTaskWithoutUndispatchedStatus": func(t *testing.T, tsk Task) {
			tsk.Status = evergreen.TaskDispatched
			assert.False(t, tsk.IsHostDispatchable())
		},
		"ReturnsFalseForInactiveTask": func(t *testing.T, tsk Task) {
			tsk.Activated = false
			assert.False(t, tsk.IsHostDispatchable())
		},
		"ReturnsFalseForDisplayTask": func(t *testing.T, tsk Task) {
			tsk.DisplayOnly = true
			tsk.ExecutionPlatform = ""
			tsk.ExecutionTasks = []string{"exec-task0", "exec-task1"}
			assert.False(t, tsk.IsHostDispatchable())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			hostDispatchableTask := Task{
				Id:                "task-id",
				Status:            evergreen.TaskUndispatched,
				Activated:         true,
				ExecutionPlatform: ExecutionPlatformHost,
			}
			tCase(t, hostDispatchableTask)
		})
	}
}

func TestIsContainerDispatchable(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, tsk Task){
		"ReturnsTrueForExpectedTask": func(t *testing.T, tsk Task) {
			assert.True(t, tsk.IsContainerDispatchable())
		},
		"ReturnsFalseForTaskWithDefaultedHostExecutionPlatform": func(t *testing.T, tsk Task) {
			tsk.ExecutionPlatform = ""
			assert.False(t, tsk.IsContainerDispatchable())
		},
		"ReturnsFalseForTaskWithoutContainerAllocated": func(t *testing.T, tsk Task) {
			tsk.ContainerAllocated = false
			assert.False(t, tsk.IsContainerDispatchable())
		},
		"ReturnsFalseForTaskWithUnattainableDependencies": func(t *testing.T, tsk Task) {
			tsk.DependsOn = []Dependency{
				{
					TaskId:       "dependency0",
					Unattainable: true,
					Finished:     true,
				},
			}
			assert.False(t, tsk.IsContainerDispatchable())
		},
		"ReturnsFalseForTaskWithUnfinishedDependencies": func(t *testing.T, tsk Task) {
			tsk.DependsOn = []Dependency{
				{
					TaskId:       "dependency0",
					Unattainable: false,
					Finished:     false,
				},
			}
			assert.False(t, tsk.IsContainerDispatchable())
			assert.False(t, tsk.IsContainerDispatchable())
		},
		"ReturnsFalseForHostTask": func(t *testing.T, tsk Task) {
			tsk.ExecutionPlatform = ExecutionPlatformHost
			assert.False(t, tsk.IsContainerDispatchable())
		},
		"ReturnsFalseForTaskWithoutUndispatchedStatus": func(t *testing.T, tsk Task) {
			tsk.Status = evergreen.TaskDispatched
			assert.False(t, tsk.IsContainerDispatchable())
		},
		"ReturnsFalseForInactiveTask": func(t *testing.T, tsk Task) {
			tsk.Activated = false
			assert.False(t, tsk.IsContainerDispatchable())
		},
		"ReturnsFalseForDisplayTask": func(t *testing.T, tsk Task) {
			tsk.DisplayOnly = true
			tsk.ExecutionPlatform = ""
			tsk.ExecutionTasks = []string{"exec-task0", "exec-task1"}
			assert.False(t, tsk.IsContainerDispatchable())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			containerDispatchableTask := Task{
				Id:                 "task-id",
				Status:             evergreen.TaskUndispatched,
				Activated:          true,
				ContainerAllocated: true,
				ExecutionPlatform:  ExecutionPlatformContainer,
			}
			tCase(t, containerDispatchableTask)
		})
	}
}

func TestShouldAllocateContainer(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, tsk Task){
		"ReturnsTrueForExpectedTask": func(t *testing.T, tsk Task) {
			assert.True(t, tsk.ShouldAllocateContainer())
		},
		"ReturnsFalseForTaskWithDefaultedHostExecutionPlatform": func(t *testing.T, tsk Task) {
			tsk.ExecutionPlatform = ""
			assert.False(t, tsk.ShouldAllocateContainer())
		},
		"ReturnsFalseForTaskAlreadyAllocatedContainer": func(t *testing.T, tsk Task) {
			tsk.ContainerAllocated = true
			assert.False(t, tsk.ShouldAllocateContainer())
		},
		"ReturnsFalseForTaskWithNoRemainingAllocationAttempts": func(t *testing.T, tsk Task) {
			tsk.ContainerAllocationAttempts = maxContainerAllocationAttempts
			assert.False(t, tsk.ShouldAllocateContainer())
		},
		"ReturnsFalseForHostTask": func(t *testing.T, tsk Task) {
			tsk.ExecutionPlatform = ExecutionPlatformHost
			assert.False(t, tsk.ShouldAllocateContainer())
		},
		"ReturnsFalseForTaskWithoutUndispatchedStatus": func(t *testing.T, tsk Task) {
			tsk.Status = evergreen.TaskDispatched
			assert.False(t, tsk.ShouldAllocateContainer())
		},
		"ReturnsFalseForInactiveTask": func(t *testing.T, tsk Task) {
			tsk.Activated = false
			assert.False(t, tsk.ShouldAllocateContainer())
		},
		"ReturnsFalseForTaskWithIncompleteDependencies": func(t *testing.T, tsk Task) {
			tsk.DependsOn = []Dependency{
				{
					TaskId:   "dependency0",
					Finished: false,
				},
			}
			assert.False(t, tsk.ShouldAllocateContainer())
		},
		"ReturnsTrueForTaskWithOverrideDependencies": func(t *testing.T, tsk Task) {
			tsk.DependsOn = []Dependency{
				{
					TaskId:   "dependency0",
					Finished: false,
				},
			}
			tsk.OverrideDependencies = true
			assert.True(t, tsk.ShouldAllocateContainer())
		},
		"ReturnsFalseForDisplayTask": func(t *testing.T, tsk Task) {
			tsk.DisplayOnly = true
			tsk.ExecutionPlatform = ""
			tsk.ExecutionTasks = []string{"exec-task0", "exec-task1"}
			assert.False(t, tsk.ShouldAllocateContainer())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tCase(t, getTaskThatNeedsContainerAllocation())
		})
	}
}

func getTaskThatNeedsContainerAllocation() Task {
	return Task{
		Id:                "task-id",
		Status:            evergreen.TaskUndispatched,
		Activated:         true,
		ExecutionPlatform: ExecutionPlatformContainer,
	}
}

func TestMarkAllForUnattainableDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	// checkTaskAndDB runs identical checks on a task and its corresponding DB document.
	checkTaskAndDB := func(t *testing.T, tsk Task, doCheck func(t *testing.T, taskToCheck Task)) {
		dbTask, err := FindOneId(ctx, tsk.Id)
		require.NoError(t, err)
		require.NotZero(t, dbTask)

		doCheck(t, tsk)
		doCheck(t, *dbTask)
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"BlockedDependency": func(ctx context.Context, t *testing.T) {
			dependentTask := Task{
				Id: "t0",
				DependsOn: []Dependency{
					{
						TaskId:       "t1",
						Unattainable: false,
					},
					{
						TaskId:       "t2",
						Unattainable: false,
					},
				},
			}
			require.NoError(t, dependentTask.Insert(t.Context()))

			updatedDependentTasks, err := MarkAllForUnattainableDependencies(ctx, []Task{dependentTask}, []string{"t1"}, true)
			require.NoError(t, err)
			require.Len(t, updatedDependentTasks, 1)
			dependentTask = updatedDependentTasks[0]

			checkTaskAndDB(t, dependentTask, func(t *testing.T, taskToCheck Task) {
				assert.True(t, dependentTask.Blocked())
				assert.True(t, dependentTask.UnattainableDependency)
				require.Len(t, dependentTask.DependsOn, 2)
				assert.True(t, dependentTask.DependsOn[0].Unattainable)
				assert.False(t, dependentTask.DependsOn[1].Unattainable)
			})
		},
		"BlocksManyTasks": func(ctx context.Context, t *testing.T) {
			dependentTasks := []Task{
				{
					Id: "t0",
					DependsOn: []Dependency{
						{
							TaskId:       "t4",
							Unattainable: false,
						},
						{
							TaskId:       "t2",
							Unattainable: false,
						},
					},
				},
				{
					Id: "t1",
					DependsOn: []Dependency{
						{
							TaskId:       "t4",
							Unattainable: false,
						},
					},
				},
				{
					Id: "t2",
					DependsOn: []Dependency{
						{
							TaskId:       "t5",
							Unattainable: false,
						},
					},
				},
			}
			for _, dependentTask := range dependentTasks {
				require.NoError(t, dependentTask.Insert(t.Context()))
			}

			updatedDependentTasks, err := MarkAllForUnattainableDependencies(ctx, dependentTasks, []string{"t4"}, true)
			assert.NoError(t, err)
			require.Len(t, updatedDependentTasks, len(dependentTasks))

			for _, updatedDependentTask := range updatedDependentTasks {
				switch updatedDependentTask.Id {
				case dependentTasks[0].Id:
					checkTaskAndDB(t, updatedDependentTasks[0], func(t *testing.T, taskToCheck Task) {
						assert.True(t, taskToCheck.Blocked())
						assert.True(t, taskToCheck.UnattainableDependency)
						require.Len(t, taskToCheck.DependsOn, 2)
						assert.True(t, taskToCheck.DependsOn[0].Unattainable)
						assert.False(t, taskToCheck.DependsOn[1].Unattainable)
					})
				case dependentTasks[1].Id:
					checkTaskAndDB(t, updatedDependentTasks[1], func(t *testing.T, taskToCheck Task) {
						assert.True(t, taskToCheck.Blocked())
						assert.True(t, taskToCheck.UnattainableDependency)
						require.Len(t, taskToCheck.DependsOn, 1)
						assert.True(t, taskToCheck.DependsOn[0].Unattainable)
					})
				case dependentTasks[2].Id:
					checkTaskAndDB(t, updatedDependentTasks[2], func(t *testing.T, taskToCheck Task) {
						assert.False(t, taskToCheck.Blocked())
						assert.False(t, taskToCheck.UnattainableDependency)
						require.Len(t, taskToCheck.DependsOn, 1)
						assert.False(t, taskToCheck.DependsOn[0].Unattainable)
					})
				default:
					assert.Fail(t, "unexpected task '%s' in updated tasks", updatedDependentTask.Id)
				}
			}
		},
		"BlocksManyDependenciesForManyTasks": func(ctx context.Context, t *testing.T) {
			dependentTasks := []Task{
				{
					Id: "t0",
					DependsOn: []Dependency{
						{
							TaskId:       "t2",
							Unattainable: false,
						},
						{
							TaskId:       "t4",
							Unattainable: false,
						},
					},
				},
				{
					Id: "t1",
					DependsOn: []Dependency{
						{
							TaskId:       "t4",
							Unattainable: false,
						},
						{
							TaskId:       "t5",
							Unattainable: false,
						},
					},
				},
				{
					Id: "t2",
					DependsOn: []Dependency{
						{
							TaskId:       "t6",
							Unattainable: false,
						},
					},
				},
			}
			for _, dependentTask := range dependentTasks {
				require.NoError(t, dependentTask.Insert(t.Context()))
			}

			updatedDependentTasks, err := MarkAllForUnattainableDependencies(ctx, dependentTasks, []string{"t4", "t5"}, true)
			assert.NoError(t, err)
			require.Len(t, updatedDependentTasks, len(dependentTasks))

			for _, updatedDependentTask := range updatedDependentTasks {
				switch updatedDependentTask.Id {
				case dependentTasks[0].Id:
					checkTaskAndDB(t, updatedDependentTasks[0], func(t *testing.T, taskToCheck Task) {
						assert.True(t, taskToCheck.Blocked())
						assert.True(t, taskToCheck.UnattainableDependency)
						require.Len(t, taskToCheck.DependsOn, 2)
						assert.False(t, taskToCheck.DependsOn[0].Unattainable)
						assert.True(t, taskToCheck.DependsOn[1].Unattainable)
					})
				case dependentTasks[1].Id:
					checkTaskAndDB(t, updatedDependentTasks[1], func(t *testing.T, taskToCheck Task) {
						assert.True(t, taskToCheck.Blocked())
						assert.True(t, taskToCheck.UnattainableDependency)
						require.Len(t, taskToCheck.DependsOn, 2)
						assert.True(t, taskToCheck.DependsOn[0].Unattainable)
						assert.True(t, taskToCheck.DependsOn[1].Unattainable)
					})
				case dependentTasks[2].Id:
					checkTaskAndDB(t, updatedDependentTasks[2], func(t *testing.T, taskToCheck Task) {
						assert.False(t, taskToCheck.Blocked())
						assert.False(t, taskToCheck.UnattainableDependency)
						require.Len(t, taskToCheck.DependsOn, 1)
						assert.False(t, taskToCheck.DependsOn[0].Unattainable)
					})
				default:
					assert.Fail(t, "unexpected task '%s' in updated tasks", updatedDependentTask.Id)
				}
			}
		},
		"NonexistentDependency": func(ctx context.Context, t *testing.T) {
			dependentTask := Task{
				Id: "t0",
				DependsOn: []Dependency{
					{
						TaskId:       "t1",
						Unattainable: false,
					},
					{
						TaskId:       "t2",
						Unattainable: false,
					},
				},
			}
			require.NoError(t, dependentTask.Insert(t.Context()))

			updatedDependentTasks, err := MarkAllForUnattainableDependencies(ctx, []Task{dependentTask}, []string{"t3"}, true)
			require.NoError(t, err)
			require.Len(t, updatedDependentTasks, 1)
			dependentTask = updatedDependentTasks[0]

			checkTaskAndDB(t, dependentTask, func(t *testing.T, taskToCheck Task) {
				assert.False(t, taskToCheck.Blocked())
				assert.False(t, taskToCheck.UnattainableDependency)
				require.Len(t, taskToCheck.DependsOn, 2)
				assert.False(t, taskToCheck.DependsOn[0].Unattainable)
				assert.False(t, taskToCheck.DependsOn[1].Unattainable)
			})
		},
		"OneDependencyUnblocked": func(ctx context.Context, t *testing.T) {
			dependentTask := Task{
				Id: "t0",
				DependsOn: []Dependency{
					{
						TaskId:       "t1",
						Unattainable: true,
					},
					{
						TaskId:       "t2",
						Unattainable: true,
					},
				},
			}
			require.NoError(t, dependentTask.Insert(t.Context()))

			updatedDependentTasks, err := MarkAllForUnattainableDependencies(ctx, []Task{dependentTask}, []string{"t1"}, false)
			require.NoError(t, err)
			require.Len(t, updatedDependentTasks, 1)
			dependentTask = updatedDependentTasks[0]

			checkTaskAndDB(t, dependentTask, func(t *testing.T, taskToCheck Task) {
				assert.True(t, taskToCheck.Blocked())
				assert.True(t, taskToCheck.UnattainableDependency)
				require.Len(t, taskToCheck.DependsOn, 2)
				assert.False(t, taskToCheck.DependsOn[0].Unattainable)
				assert.True(t, taskToCheck.DependsOn[1].Unattainable)
			})
		},
		"AllDependenciesUnblocked": func(ctx context.Context, t *testing.T) {
			dependentTask := Task{
				Id: "t0",
				DependsOn: []Dependency{
					{
						TaskId:       "t1",
						Unattainable: true,
					},
					{
						TaskId:       "t2",
						Unattainable: false,
					},
				},
			}
			require.NoError(t, dependentTask.Insert(t.Context()))

			updatedDependentTasks, err := MarkAllForUnattainableDependencies(ctx, []Task{dependentTask}, []string{"t1"}, false)
			require.NoError(t, err)
			require.Len(t, updatedDependentTasks, 1)
			dependentTask = updatedDependentTasks[0]

			checkTaskAndDB(t, dependentTask, func(t *testing.T, taskToCheck Task) {
				assert.False(t, taskToCheck.Blocked())
				assert.False(t, taskToCheck.UnattainableDependency)
				require.Len(t, taskToCheck.DependsOn, 2)
				assert.False(t, taskToCheck.DependsOn[0].Unattainable)
				assert.False(t, taskToCheck.DependsOn[1].Unattainable)
			})
		},
		"InMemoryTaskOutdated": func(ctx context.Context, t *testing.T) {
			dependentTask := Task{
				Id: "t0",
				DependsOn: []Dependency{
					{
						TaskId:       "t1",
						Unattainable: true,
					},
					{
						TaskId:       "t2",
						Unattainable: false,
					},
				},
			}
			require.NoError(t, dependentTask.Insert(t.Context()))

			dependentTask.DependsOn[1].Unattainable = true

			updatedDependentTasks, err := MarkAllForUnattainableDependencies(ctx, []Task{dependentTask}, []string{"t1"}, false)
			require.NoError(t, err)
			require.Len(t, updatedDependentTasks, 1)
			dependentTask = updatedDependentTasks[0]

			checkTaskAndDB(t, dependentTask, func(t *testing.T, taskToCheck Task) {
				assert.False(t, taskToCheck.Blocked())
				assert.False(t, taskToCheck.UnattainableDependency)
				require.Len(t, taskToCheck.DependsOn, 2)
				assert.False(t, taskToCheck.DependsOn[0].Unattainable)
				assert.False(t, taskToCheck.DependsOn[1].Unattainable)
			})
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(Collection))

			tCase(ctx, t)
		})
	}
}

func TestSetGeneratedTasksToActivate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	task := Task{Id: "t1"}
	assert.NoError(t, task.Insert(t.Context()))

	// add stepback task to variant
	assert.NoError(t, task.SetGeneratedTasksToActivate(ctx, "bv2", "t2"))
	taskFromDb, err := FindOneId(ctx, "t1")
	assert.NoError(t, err)
	assert.NotNil(t, taskFromDb)
	assert.Equal(t, []string{"t2"}, taskFromDb.GeneratedTasksToActivate["bv2"])

	// add different stepback task to variant
	assert.NoError(t, task.SetGeneratedTasksToActivate(ctx, "bv2", "t2.0"))
	taskFromDb, err = FindOneId(ctx, "t1")
	assert.NoError(t, err)
	assert.NotNil(t, taskFromDb)
	assert.Equal(t, []string{"t2", "t2.0"}, taskFromDb.GeneratedTasksToActivate["bv2"])

	// verify duplicate doesn't overwrite
	assert.NoError(t, task.SetGeneratedTasksToActivate(ctx, "bv2", "t2.0"))
	taskFromDb, err = FindOneId(ctx, "t1")
	assert.NoError(t, err)
	assert.NotNil(t, taskFromDb)
	assert.Equal(t, []string{"t2", "t2.0"}, taskFromDb.GeneratedTasksToActivate["bv2"])

	// adding second variant doesn't affect previous
	assert.NoError(t, task.SetGeneratedTasksToActivate(ctx, "bv3", "t3"))
	taskFromDb, err = FindOneId(ctx, "t1")
	assert.NoError(t, err)
	assert.NotNil(t, taskFromDb)
	assert.Equal(t, []string{"t2", "t2.0"}, taskFromDb.GeneratedTasksToActivate["bv2"])
	assert.Equal(t, []string{"t3"}, taskFromDb.GeneratedTasksToActivate["bv3"])
}

func TestSetNextStepbackId(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection))
	task := Task{Id: "t1"}
	require.NoError(t, task.Insert(t.Context()))

	s := StepbackInfo{
		LastFailingStepbackTaskId: "t2",
		LastPassingStepbackTaskId: "t3",
		NextStepbackTaskId:        "t4",
		PreviousStepbackTaskId:    "t5",
	}

	require.NoError(t, SetNextStepbackId(ctx, task.Id, s))
	taskFromDb, err := FindOneId(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, taskFromDb)
	assert.NotEqual("t2", taskFromDb.StepbackInfo.LastFailingStepbackTaskId)
	assert.NotEqual("t3", taskFromDb.StepbackInfo.LastPassingStepbackTaskId)
	assert.Equal("t4", taskFromDb.StepbackInfo.NextStepbackTaskId)
	assert.NotEqual("t5", taskFromDb.StepbackInfo.PreviousStepbackTaskId)
}

func TestSetLastAndPreviousStepbackIds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection))
	task := Task{Id: "t1"}
	require.NoError(t, task.Insert(t.Context()))

	s := StepbackInfo{
		LastFailingStepbackTaskId: "t2",
		LastPassingStepbackTaskId: "t3",
		NextStepbackTaskId:        "t4",
		PreviousStepbackTaskId:    "t5",
	}

	require.NoError(t, SetLastAndPreviousStepbackIds(ctx, task.Id, s))
	taskFromDb, err := FindOneId(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, taskFromDb)
	assert.Equal("t2", taskFromDb.StepbackInfo.LastFailingStepbackTaskId)
	assert.Equal("t3", taskFromDb.StepbackInfo.LastPassingStepbackTaskId)
	assert.NotEqual("t4", taskFromDb.StepbackInfo.NextStepbackTaskId)
	assert.Equal("t5", taskFromDb.StepbackInfo.PreviousStepbackTaskId)
}

func TestGetLatestExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	sample := Task{
		Id:        "task_id_some_other_stuff",
		Execution: 55,
	}
	assert.NoError(t, sample.Insert(t.Context()))
	execution, err := GetLatestExecution(ctx, sample.Id)
	assert.NoError(t, err)
	assert.Equal(t, sample.Execution, execution)
	execution, err = GetLatestExecution(ctx, fmt.Sprintf("%s_3", sample.Id))
	assert.NoError(t, err)
	assert.Equal(t, sample.Execution, execution)
}

func TestArchiveMany(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, OldCollection))
	t1 := Task{
		Id:      "t1",
		Status:  evergreen.TaskFailed,
		Aborted: true,
		Version: "v",
	}
	assert.NoError(t, t1.Insert(t.Context()))
	t2 := Task{
		Id:      "t2",
		Status:  evergreen.TaskFailed,
		Aborted: true,
		Version: "v",
	}
	assert.NoError(t, t2.Insert(t.Context()))
	et := Task{
		Id:      "et",
		Status:  evergreen.TaskSucceeded,
		Version: "v",
	}
	assert.NoError(t, et.Insert(t.Context()))
	dt := Task{
		Id:             "dt",
		Status:         evergreen.TaskSucceeded,
		DisplayOnly:    true,
		ExecutionTasks: []string{et.Id},
		Version:        "v",
	}
	assert.NoError(t, dt.Insert(t.Context()))

	tasks := []Task{t1, t2, dt}
	err := ArchiveMany(ctx, tasks)
	assert.NoError(t, err)
	verifyTasksState := func() {
		currentTasks, err := FindAll(ctx, db.Query(ByVersion("v")))
		assert.NoError(t, err)
		assert.Len(t, currentTasks, 4)
		for _, task := range currentTasks {
			assert.False(t, task.Aborted)
			assert.Equal(t, 1, task.Execution)
		}
		oldTasks, err := FindAllOld(ctx, db.Query(ByVersion("v")))
		assert.NoError(t, err)
		assert.Len(t, oldTasks, 4)
		for _, task := range oldTasks {
			assert.True(t, task.Archived)
			assert.Equal(t, 0, task.Execution)
		}
	}
	verifyTasksState()

	// We shouldn't error if we try archiving again, in case we got stuck part way.
	err = ArchiveMany(ctx, tasks)
	assert.NoError(t, err)
	// Verify that nothing actually changed when re-archiving.
	verifyTasksState()
}

func TestArchiveManyAfterFailedOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, OldCollection))
	et1 := Task{
		Id:                    "et1",
		Status:                evergreen.TaskFailed,
		Execution:             2,
		LatestParentExecution: 2,
		Aborted:               true,
		Version:               "v",
	}
	assert.NoError(t, et1.Insert(t.Context()))
	et2 := Task{
		Id:                    "et2",
		Status:                evergreen.TaskSucceeded,
		Execution:             2,
		LatestParentExecution: 2,
		Aborted:               true,
		Version:               "v",
	}
	assert.NoError(t, et2.Insert(t.Context()))
	t1 := Task{
		Id:             "t1",
		Status:         evergreen.TaskFailed,
		ExecutionTasks: []string{et1.Id, et2.Id},
		Execution:      2,
		Aborted:        true,
		DisplayOnly:    true,
		Version:        "v",
	}
	assert.NoError(t, t1.Insert(t.Context()))
	t2 := Task{
		Id:        "t2",
		Status:    evergreen.TaskSucceeded,
		Execution: 3,
		Aborted:   true,
		Version:   "v",
	}
	assert.NoError(t, t2.Insert(t.Context()))
	et3 := Task{
		Id:        "et3",
		Status:    evergreen.TaskFailed,
		Execution: 0,
		Aborted:   true,
		Version:   "v",
	}
	assert.NoError(t, et3.Insert(t.Context()))
	et4 := Task{
		Id:        "et4",
		Status:    evergreen.TaskSucceeded,
		Execution: 0,
		Aborted:   true,
		Version:   "v",
	}
	assert.NoError(t, et4.Insert(t.Context()))
	et5 := Task{
		Id:        "et5",
		Status:    evergreen.TaskFailed,
		Execution: 0,
		Aborted:   true,
		Version:   "v",
	}
	assert.NoError(t, et5.Insert(t.Context()))
	t3 := Task{
		Id:                      "t3",
		Status:                  evergreen.TaskFailed,
		ExecutionTasks:          []string{et3.Id, et4.Id, et5.Id},
		Execution:               0,
		Aborted:                 true,
		DisplayOnly:             true,
		ResetFailedWhenFinished: true,
		Version:                 "v",
	}
	assert.NoError(t, t3.Insert(t.Context()))
	assert.NoError(t, t3.Archive(ctx)) // Failed only is true
	currentTasks, err := FindAll(ctx, db.Query(ByVersion("v")))
	assert.NoError(t, err)
	for _, task := range currentTasks {
		id := task.Id
		// All execution tasks in the display task we archived
		if id == et3.Id || id == et4.Id || id == et5.Id {
			assert.Equal(t, 1, task.LatestParentExecution)

			// Restarted tasks
			if id == et3.Id || id == et5.Id {
				assert.Equal(t, 1, task.Execution)
			} else {
				assert.Equal(t, 0, task.Execution)
			}
		}
	}

	t4 := Task{
		Id:        "t4",
		Status:    evergreen.TaskSucceeded,
		Execution: 1,
		Aborted:   true,
		Version:   "v",
	}
	assert.NoError(t, t4.Insert(t.Context()))

	// During runtime we do not archive the same task multiple times without resetting in between.
	// For the sake of this test, we manually untoggle CanReset so we can archive the task multiple times in a row.
	err = UpdateOne(
		ctx,
		bson.M{IdKey: t3.Id},
		bson.M{"$set": bson.M{CanResetKey: false}},
	)
	require.NoError(t, err)

	t3Pointer, err := FindByIdExecution(ctx, t3.Id, nil)
	assert.NoError(t, err)
	t3Pointer.ResetFailedWhenFinished = false
	assert.NoError(t, ArchiveMany(ctx, []Task{t1, t2, *t3Pointer, t4}))

	// Before ArchiveMany:
	// t1: et1, et2 (execution 2)
	// t2 (execution 3)
	// t3: et1 (execution 1), et2 (execution 0), et3 (execution 1)
	// t4 (execution 1)

	// After ArchiveMany:
	// t1: et1, et2 (execution 3)
	// t2 (execution 4)
	// t3: t1 (execution 2), t2 (execution 2), t3 (execution 2)
	// t4 (execution 2)
	verifyTasksState := func() {
		currentTasks, err = FindAll(ctx, db.Query(ByVersion("v")))
		assert.NoError(t, err)
		assert.Len(t, currentTasks, 9)

		// Every display task or task should have a '0' LatestParentExecution (it is an execution task only field)
		// For execution tasks, the execution should be their latestparentexecution after archiving all
		for _, task := range currentTasks {
			switch task.Id {
			case t1.Id:
				assert.Equal(t, 3, task.Execution)
				assert.Equal(t, 0, task.LatestParentExecution)
			case et1.Id, et2.Id:
				assert.Equal(t, 3, task.Execution)
				assert.Equal(t, task.LatestParentExecution, task.Execution)
			case t2.Id:
				assert.Equal(t, 4, task.Execution)
				assert.Equal(t, 0, task.LatestParentExecution)
			case t3.Id:
				assert.Equal(t, 2, task.Execution)
				assert.Equal(t, 0, task.LatestParentExecution)
			case et3.Id, et4.Id, et5.Id:
				assert.Equal(t, 2, task.Execution)
				assert.Equal(t, task.LatestParentExecution, task.Execution)
			case t4.Id:
				assert.Equal(t, 2, task.Execution)
				assert.Equal(t, 0, task.LatestParentExecution)
			default:
				assert.Error(t, nil, "A task was not accounted for")
			}
		}
	}
	verifyTasksState()

	// We shouldn't error if we try archiving again, in case we got stuck part way.
	assert.NoError(t, ArchiveMany(ctx, []Task{t1, t2, *t3Pointer, t4}))
	// Verify that nothing actually changed when re-archiving.
	verifyTasksState()
}

func TestAddParentDisplayTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	dt1 := Task{
		Id:             "dt1",
		DisplayOnly:    true,
		ExecutionTasks: []string{"et1", "et2"},
	}
	assert.NoError(t, dt1.Insert(t.Context()))
	dt2 := Task{
		Id:             "dt2",
		DisplayOnly:    true,
		ExecutionTasks: []string{"et3", "et4"},
	}
	assert.NoError(t, dt2.Insert(t.Context()))
	execTasks := []Task{
		{Id: "et1"},
		{Id: "et2"},
		{Id: "et3"},
		{Id: "et4"},
	}
	for _, et := range execTasks {
		assert.NoError(t, et.Insert(t.Context()))
	}
	tasks, err := AddParentDisplayTasks(ctx, execTasks)
	assert.NoError(t, err)
	assert.Equal(t, "et1", tasks[0].Id)
	assert.Equal(t, dt1.Id, tasks[0].DisplayTask.Id)
	assert.Equal(t, "et2", tasks[1].Id)
	assert.Equal(t, dt1.Id, tasks[1].DisplayTask.Id)
	assert.Equal(t, "et3", tasks[2].Id)
	assert.Equal(t, dt2.Id, tasks[2].DisplayTask.Id)
	assert.Equal(t, "et4", tasks[3].Id)
	assert.Equal(t, dt2.Id, tasks[3].DisplayTask.Id)
}

func TestSetCheckRunId(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	t1 := &Task{
		Id: "t1",
	}

	assert.NoError(t, t1.Insert(t.Context()))
	assert.NoError(t, t1.SetCheckRunId(ctx, 12345))

	var err error
	t1, err = FindOneId(ctx, t1.Id)
	require.NotNil(t, t1)
	assert.NoError(t, err)

	assert.Equal(t, int64(12345), utility.FromInt64Ptr(t1.CheckRunId))

}

func TestAddDisplayTaskIdToExecTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	t1 := &Task{
		Id:            "t1",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t2 := &Task{
		Id:            "t2",
		DisplayTaskId: nil,
	}
	t3 := &Task{
		Id:            "t3",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	assert.NoError(t, t1.Insert(t.Context()))
	assert.NoError(t, t2.Insert(t.Context()))
	assert.NoError(t, t3.Insert(t.Context()))

	assert.NoError(t, AddDisplayTaskIdToExecTasks(ctx, "dt", []string{t1.Id, t2.Id}))

	var err error
	t1, err = FindOneId(ctx, t1.Id)
	assert.NoError(t, err)
	assert.Equal(t, "dt", utility.FromStringPtr(t1.DisplayTaskId))

	t2, err = FindOneId(ctx, t2.Id)
	assert.NoError(t, err)
	assert.Equal(t, "dt", utility.FromStringPtr(t2.DisplayTaskId))

	t3, err = FindOneId(ctx, t3.Id)
	assert.NoError(t, err)
	assert.NotEqual(t, "dt", utility.FromStringPtr(t3.DisplayTaskId))
}

func TestAddExecTasksToDisplayTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	dt1 := Task{
		Id:             "dt1",
		DisplayOnly:    true,
		Activated:      false,
		ExecutionTasks: []string{"et1", "et2"},
	}
	assert.NoError(t, dt1.Insert(t.Context()))

	// no tasks to add
	assert.NoError(t, AddExecTasksToDisplayTask(ctx, dt1.Id, []string{}, false))
	dtFromDB, err := FindOneId(ctx, dt1.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dtFromDB)
	assert.Len(t, dtFromDB.ExecutionTasks, 2)
	assert.Contains(t, dtFromDB.ExecutionTasks, "et1")
	assert.Contains(t, dtFromDB.ExecutionTasks, "et2")

	// new and existing tasks to add (existing tasks not duplicated)
	assert.NoError(t, AddExecTasksToDisplayTask(ctx, dt1.Id, []string{"et2", "et3", "et4"}, true))
	dtFromDB, err = FindOneId(ctx, dt1.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dtFromDB)
	assert.Len(t, dtFromDB.ExecutionTasks, 4)
	assert.Contains(t, dtFromDB.ExecutionTasks, "et1")
	assert.Contains(t, dtFromDB.ExecutionTasks, "et2")
	assert.Contains(t, dtFromDB.ExecutionTasks, "et3")
	assert.Contains(t, dtFromDB.ExecutionTasks, "et4")
	assert.True(t, dtFromDB.Activated)
	assert.False(t, utility.IsZeroTime(dtFromDB.ActivatedTime))
}

func TestAbortVersionTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(Collection))
	finishedExecTask := &Task{
		Id:      "et1",
		Version: "v1",
		Status:  evergreen.TaskSucceeded,
	}
	failingExecTask := &Task{
		Id:      "et2",
		Version: "v1",
		Status:  evergreen.TaskFailed,
	}
	otherExecTask := &Task{
		Id:      "et3",
		Version: "v1",
		Status:  evergreen.TaskStarted,
	}
	dt := &Task{
		Id:             "dt",
		Version:        "v1",
		Status:         evergreen.TaskStarted,
		ExecutionTasks: []string{"et1", "et2", "et3"},
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, finishedExecTask, failingExecTask, otherExecTask, dt))

	assert.NoError(t, AbortVersionTasks(ctx, "v1", AbortInfo{TaskID: "et2"}))

	var err error
	dt, err = FindOneId(ctx, "dt")
	assert.NoError(t, err)
	require.NotNil(t, dt)
	assert.False(t, dt.Aborted)
	assert.Empty(t, dt.AbortInfo.TaskID)

	otherExecTask, err = FindOneId(ctx, "et3")
	assert.NoError(t, err)
	require.NotNil(t, otherExecTask)
	assert.True(t, otherExecTask.Aborted)
	assert.NotEmpty(t, otherExecTask.AbortInfo.TaskID)
	assert.Equal(t, evergreen.TaskAborted, otherExecTask.DisplayStatusCache)
}

func TestArchive(t *testing.T) {
	ctx := t.Context()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, OldCollection, event.EventCollection))
	}()
	checkTaskIsArchived := func(t *testing.T, oldTaskID string) {
		dbTask, err := FindOneOldId(ctx, oldTaskID)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.NotZero(t, dbTask.OldTaskId)
		assert.NotEqual(t, dbTask.OldTaskId, dbTask.Id)
		assert.True(t, dbTask.Archived)
		assert.False(t, dbTask.Aborted)
		assert.Zero(t, dbTask.AbortInfo)
	}

	checkEventLogHostTaskExecutions := func(t *testing.T, hostID, oldTaskID string, _ int) {
		dbTask, err := FindOneOldId(ctx, oldTaskID)
		require.NoError(t, err)
		require.NotZero(t, dbTask)

		events, err := event.FindAllByResourceID(t.Context(), hostID)
		require.NoError(t, err)
		assert.NotEmpty(t, events)

		for _, e := range events {
			hostEventData, ok := e.Data.(*event.HostEventData)
			require.True(t, ok)
			require.Equal(t, hostEventData.TaskId, dbTask.OldTaskId)
			require.Equal(t, hostEventData.Execution, strconv.Itoa(dbTask.Execution))
		}
	}
	for tName, tCase := range map[string]func(t *testing.T, tsk Task){
		"ArchivesHostTaskAndUpdatesEventLog": func(t *testing.T, tsk Task) {
			archivedTaskID := MakeOldID(tsk.Id, tsk.Execution)
			archivedExecution := tsk.Execution
			require.NoError(t, tsk.Insert(t.Context()))

			hostID := "hostID"
			event.LogHostRunningTaskSet(ctx, hostID, tsk.Id, 0)
			event.LogHostRunningTaskCleared(ctx, hostID, tsk.Id, 0)

			require.NoError(t, tsk.Archive(ctx))

			checkTaskIsArchived(t, archivedTaskID)
			checkEventLogHostTaskExecutions(t, hostID, archivedTaskID, archivedExecution)
		},
		"ArchivesDisplayTaskAndItsExecutionTasks": func(t *testing.T, dt Task) {
			execTask := Task{
				Id:            "execTask",
				DisplayTaskId: utility.ToStringPtr(dt.Id),
				Status:        evergreen.TaskSucceeded,
			}
			archivedExecTaskID := MakeOldID(execTask.Id, execTask.Execution)
			archivedExecution := execTask.Execution
			require.NoError(t, execTask.Insert(t.Context()))

			hostID := "hostID"
			event.LogHostRunningTaskSet(ctx, hostID, execTask.Id, 0)
			event.LogHostRunningTaskCleared(ctx, hostID, execTask.Id, 0)

			dt.DisplayOnly = true
			dt.ExecutionTasks = []string{execTask.Id}
			archivedDisplayTaskID := MakeOldID(dt.Id, dt.Execution)
			require.NoError(t, dt.Insert(t.Context()))

			require.NoError(t, dt.Archive(ctx))

			checkTaskIsArchived(t, archivedExecTaskID)
			checkTaskIsArchived(t, archivedDisplayTaskID)

			checkEventLogHostTaskExecutions(t, hostID, archivedExecTaskID, archivedExecution)
		},
		"ArchivesContainerTask": func(t *testing.T, tsk Task) {
			archivedTaskID := MakeOldID(tsk.Id, tsk.Execution)
			tsk.ExecutionPlatform = ExecutionPlatformContainer
			require.NoError(t, tsk.Insert(ctx))

			require.NoError(t, tsk.Archive(ctx))
			checkTaskIsArchived(t, archivedTaskID)
		},
		"ArchivingIncompleteTaskIsANoop": func(t *testing.T, tsk Task) {
			tsk.Status = evergreen.TaskUndispatched
			require.NoError(t, tsk.Insert(ctx))

			require.NoError(t, tsk.Archive(ctx))

			dbOldTask, err := FindOneOldByIdAndExecution(ctx, tsk.Id, tsk.Execution)
			require.NoError(t, err)
			assert.Zero(t, dbOldTask, "should not archive an incomplete task")
		},
		"MultipleArchivesOnSameTaskIsIdempotent": func(t *testing.T, tsk Task) {
			originalExecution := tsk.Execution
			require.NoError(t, tsk.Insert(ctx))

			require.NoError(t, tsk.Archive(ctx))

			checkTaskIsArchived(t, MakeOldID(tsk.Id, originalExecution))

			dbTask, err := FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, originalExecution+1, dbTask.Execution, "execution number should be incremented after archiving")

			require.NoError(t, dbTask.Archive(ctx))

			checkTaskIsArchived(t, MakeOldID(tsk.Id, originalExecution))

			dbNextTaskExecution, err := FindOneOldByIdAndExecution(ctx, tsk.Id, originalExecution+1)
			require.NoError(t, err)
			assert.Zero(t, dbNextTaskExecution, "should not archive the task again when it's already been archived and is in the process of restarting")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, OldCollection, event.EventCollection))
			tsk := Task{
				Id:        "taskID",
				Status:    evergreen.TaskSucceeded,
				Execution: 0,
			}
			tCase(t, tsk)
		})
	}
}

func TestArchiveFailedOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, OldCollection, event.EventCollection))
	}()

	assert.NoError(t, db.ClearCollections(Collection, OldCollection, event.EventCollection))
	t1 := Task{
		Id:      "t1",
		Status:  evergreen.TaskFailed,
		Version: "v",
	}
	assert.NoError(t, t1.Insert(t.Context()))
	t2 := Task{
		Id:      "t2",
		Status:  evergreen.TaskSucceeded,
		Version: "v",
	}
	assert.NoError(t, t2.Insert(t.Context()))
	dt := &Task{
		Id:                      "dt",
		DisplayOnly:             true,
		ExecutionTasks:          []string{"t1", "t2"},
		Status:                  evergreen.TaskFailed,
		Version:                 "v",
		ResetFailedWhenFinished: true,
	}
	assert.NoError(t, dt.Insert(t.Context()))

	checkTaskIsArchived := func(t *testing.T, oldTaskID string) {
		dbTask, err := FindOneOldId(ctx, oldTaskID)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.NotZero(t, dbTask.OldTaskId)
		assert.NotEqual(t, dbTask.OldTaskId, dbTask.Id)
		assert.True(t, dbTask.Archived)
		assert.False(t, dbTask.Aborted)
		assert.Zero(t, dbTask.AbortInfo)
	}

	checkTaskIsNotArchived := func(t *testing.T, taskID string, execution int) {
		task, err := FindOneIdAndExecution(ctx, taskID, execution)
		assert.NoError(t, err)
		assert.False(t, task.Archived)

		oldT, err := FindOneOldId(ctx, MakeOldID(taskID, execution))
		assert.NoError(t, err)
		assert.Nil(t, oldT)

		nextExecution, err := FindOneIdAndExecution(ctx, taskID, execution+1)
		assert.NoError(t, err)
		assert.Nil(t, nextExecution)
	}

	checkEventLogHostTaskExecutions := func(t *testing.T, hostID, oldTaskID string, _ int) {
		dbTask, err := FindOneOldId(ctx, oldTaskID)
		require.NoError(t, err)
		require.NotZero(t, dbTask)

		events, err := event.FindAllByResourceID(t.Context(), hostID)
		require.NoError(t, err)
		assert.NotEmpty(t, events)

		for _, e := range events {
			hostEventData, ok := e.Data.(*event.HostEventData)
			require.True(t, ok)
			require.Equal(t, hostEventData.TaskId, dbTask.OldTaskId)
			require.Equal(t, hostEventData.Execution, strconv.Itoa(dbTask.Execution), len(events))
		}
	}

	t.Run("ArchivesOnlyFailedExecutionTasks", func(t *testing.T) {
		dt.ResetFailedWhenFinished = true

		// Gets the future archived tasks information.
		t1, err := FindOneIdAndExecution(ctx, dt.ExecutionTasks[0], dt.Execution)
		require.NoError(t, err)
		archivedT1 := MakeOldID(t1.Id, t1.Execution)
		archivedExecution := t1.Execution

		hostID := "hostID"
		event.LogHostRunningTaskSet(ctx, hostID, t1.Id, 0)
		event.LogHostRunningTaskCleared(ctx, hostID, t1.Id, 0)

		// Verifies the execution before and after calling Archive
		archivedDisplayTaskID := MakeOldID(dt.Id, dt.Execution)
		require.Equal(t, 0, dt.Execution)
		require.NoError(t, dt.Archive(ctx))
		dt, err = FindOneId(ctx, dt.Id)
		require.NoError(t, err)
		require.Equal(t, 1, dt.Execution)

		t1, err = FindOneId(ctx, dt.ExecutionTasks[0])
		require.NoError(t, err)
		require.Equal(t, 1, t1.Execution)
		require.Equal(t, t1.LatestParentExecution, t1.Execution)
		t2, err := FindOneId(ctx, dt.ExecutionTasks[1])
		require.NoError(t, err)
		require.Equal(t, 0, t2.Execution)
		require.Equal(t, 1, t2.LatestParentExecution)

		// Cross checks the collections to ensure that each task was or was not archived
		checkTaskIsArchived(t, archivedT1)
		checkTaskIsNotArchived(t, t2.Id, 0)
		checkTaskIsArchived(t, archivedDisplayTaskID)

		checkEventLogHostTaskExecutions(t, hostID, archivedT1, archivedExecution)
	})

	// This test is for the edge case of archiving with only failed execution tasks, then archiving all execution tasks
	t.Run("ArchivesExecutionTasksAfterFailedOnly", func(t *testing.T) {
		// Manually clear CanReset for the sake of this test.
		err := UpdateOne(
			ctx,
			bson.M{IdKey: dt.Id},
			bson.M{"$set": bson.M{CanResetKey: false}},
		)
		require.NoError(t, err)
		dt.CanReset = false
		dt.ResetFailedWhenFinished = false
		// Verifies the results from the last test as a basis (more on below comment)
		require.Equal(t, 1, dt.Execution)
		t1, err := FindOneId(ctx, dt.ExecutionTasks[0])
		require.NoError(t, err)
		require.Equal(t, 1, t1.Execution, t1.Execution)
		t2, err := FindOneId(ctx, dt.ExecutionTasks[1])
		require.NoError(t, err)
		require.Equal(t, 0, t2.Execution)
		// This ensures that the latest (highest execution) task in the database for each ID is proper.
		// The dt should have 1, as well as the restarted t1. But t2 should have 0
		archivedT1 := MakeOldID(t1.Id, t1.Execution)
		archivedExecutionT1 := t1.Execution
		archivedT2 := MakeOldID(t2.Id, t2.Execution)

		hostID := "hostID2"
		event.LogHostRunningTaskSet(ctx, hostID, t1.Id, 1)
		event.LogHostRunningTaskCleared(ctx, hostID, t1.Id, 1)

		// Verifies the display task is archived a second time after calling archive
		archivedDisplayTaskID := MakeOldID(dt.Id, dt.Execution)
		require.NoError(t, dt.Archive(ctx))
		dt, err = FindOneId(ctx, dt.Id)
		require.NoError(t, err)
		require.Equal(t, 2, dt.Execution)

		t1, err = FindOneId(ctx, dt.ExecutionTasks[0])
		require.NoError(t, err)
		require.Equal(t, 2, t1.Execution)
		require.Equal(t, t1.LatestParentExecution, t1.Execution)
		t2, err = FindOneId(ctx, dt.ExecutionTasks[1])
		require.NoError(t, err)
		require.Equal(t, 2, t2.Execution)
		require.Equal(t, t2.LatestParentExecution, t2.Execution)

		// Cross checks the tasks to ensure they were archived
		checkTaskIsArchived(t, archivedT1)
		checkTaskIsArchived(t, archivedT2)
		checkTaskIsArchived(t, archivedDisplayTaskID)

		checkEventLogHostTaskExecutions(t, hostID, archivedT1, archivedExecutionT1)
	})
}

func TestByExecutionTasksAndMaxExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tasksToFetch := []string{"t1", "t2"}
	t.Run("Fetching latest execution with same executions", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection, OldCollection))
		t1 := Task{
			Id:        "t1",
			Version:   "v1",
			Execution: 1,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(t.Context(), Collection, t1))

		ot1 := t1
		ot1.Execution = 0
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(t.Context(), OldCollection, ot1))

		t2 := Task{
			Id:        "t2",
			Version:   "v1",
			Execution: 1,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(t.Context(), Collection, t2))
		ot2 := t2
		ot2.Execution = 0
		ot2 = *ot2.makeArchivedTask()
		assert.NoError(t, db.Insert(t.Context(), OldCollection, ot2))

		tasks, err := FindByExecutionTasksAndMaxExecution(ctx, tasksToFetch, 1)
		tasks = convertOldTasksIntoTasks(tasks)
		assert.NoError(t, err)
		assert.Len(t, tasks, 2)
		assertTasksAreEqual(t, t1, tasks[0], 1)
		assertTasksAreEqual(t, t2, tasks[1], 1)
	})
	t.Run("Fetching latest execution with mismatching executions", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection, OldCollection))
		t1 := Task{
			Id:        "t1",
			Version:   "v1",
			Execution: 2,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(t.Context(), Collection, t1))

		ot1 := t1
		ot1.Execution = 1
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(t.Context(), OldCollection, ot1))

		ot1.Execution = 1
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(t.Context(), OldCollection, ot1))

		t2 := Task{
			Id:        "t2",
			Version:   "v1",
			Execution: 1,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(t.Context(), Collection, t2))
		ot2 := t2
		ot2.Execution = 0
		ot2 = *ot2.makeArchivedTask()
		assert.NoError(t, db.Insert(t.Context(), OldCollection, ot2))

		tasks, err := FindByExecutionTasksAndMaxExecution(ctx, tasksToFetch, 2)
		tasks = convertOldTasksIntoTasks(tasks)
		assert.NoError(t, err)
		assert.Len(t, tasks, 2)
		assertTasksAreEqual(t, t1, tasks[0], 2)
		assertTasksAreEqual(t, t2, tasks[1], 1)
	})
	t.Run("Fetching old executions when there are even older executions", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection, OldCollection))

		// Both tasks have 2 previous executions.
		t1 := Task{
			Id:        "t1",
			Version:   "v1",
			Execution: 2,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(t.Context(), Collection, t1))

		ot1 := t1
		ot1.Execution = 1
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(t.Context(), OldCollection, ot1))

		ot1 = t1
		ot1.Execution = 0
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(t.Context(), OldCollection, ot1))

		t2 := Task{
			Id:        "t2",
			Version:   "v1",
			Execution: 2,
			Status:    evergreen.TaskFailed,
		}
		assert.NoError(t, db.Insert(t.Context(), Collection, t2))

		ot2 := t2
		ot2.Execution = 1
		ot2 = *ot2.makeArchivedTask()
		assert.NoError(t, db.Insert(t.Context(), OldCollection, ot2))

		ot2 = t2
		ot2.Execution = 0
		ot2 = *ot2.makeArchivedTask()
		assert.NoError(t, db.Insert(t.Context(), OldCollection, ot2))

		tasks, err := FindByExecutionTasksAndMaxExecution(ctx, tasksToFetch, 1)
		tasks = convertOldTasksIntoTasks(tasks)
		assert.NoError(t, err)
		assert.Len(t, tasks, 2)
		assert.Equal(t, 1, tasks[0].Execution)
		assert.Equal(t, 1, tasks[1].Execution)
	})
}

func TestFindTaskOnPreviousCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:                  "t1",
		Version:             "v1",
		Execution:           0,
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildVariant:        "bv",
		DisplayName:         "dn",
		Project:             "p",
	}
	assert.NoError(t, db.Insert(t.Context(), Collection, t1))
	t2 := Task{
		Id:                  "t2",
		Version:             "v2",
		Execution:           0,
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 2,
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildVariant:        "bv",
		DisplayName:         "dn",
		Project:             "p",
	}
	assert.NoError(t, db.Insert(t.Context(), Collection, t2))

	task, err := t2.FindTaskOnPreviousCommit(ctx)
	assert.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, t1.Id, task.Id)
	assert.Equal(t, t1.Version, task.Version)
	t3 := Task{
		Id:                  "t3",
		Version:             "v3",
		Execution:           0,
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 3,
		Requester:           evergreen.TriggerRequester,
		BuildVariant:        "bv",
		DisplayName:         "dn",
		Project:             "p",
	}
	assert.NoError(t, db.Insert(t.Context(), Collection, t3))
	t4 := Task{
		Id:                  "t4",
		Version:             "v4",
		Execution:           0,
		Status:              evergreen.TaskSucceeded,
		RevisionOrderNumber: 4,
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildVariant:        "bv",
		DisplayName:         "dn",
		Project:             "p",
	}
	assert.NoError(t, db.Insert(t.Context(), Collection, t4))

	// Should fetch the latest mainline commit task and should not consider non gitter tasks
	task, err = t4.FindTaskOnPreviousCommit(ctx)
	assert.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, t2.Id, task.Id)
	assert.Equal(t, t2.Version, task.Version)
}

type TaskConnectorFetchByIdSuite struct {
	suite.Suite
}

func TestTaskConnectorFetchByIdSuite(t *testing.T) {
	s := &TaskConnectorFetchByIdSuite{}
	suite.Run(t, s)
}

func (s *TaskConnectorFetchByIdSuite) SetupTest() {
	s.Require().NoError(db.Clear(Collection))
	for i := 0; i < 10; i++ {
		testTask := &Task{
			Id:            fmt.Sprintf("task_%d", i),
			BuildId:       fmt.Sprintf("build_%d", i),
			DisplayTaskId: utility.ToStringPtr(""),
		}
		s.NoError(testTask.Insert(s.T().Context()))
	}
}

func (s *TaskConnectorFetchByIdSuite) TestFindById() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 10; i++ {
		found, err := FindOneId(ctx, fmt.Sprintf("task_%d", i))
		s.NoError(err)
		s.Equal(found.BuildId, fmt.Sprintf("build_%d", i))
	}
}

func (s *TaskConnectorFetchByIdSuite) TestFindByIdAndExecution() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Require().NoError(db.ClearCollections(Collection, OldCollection))
	testTask1 := &Task{
		Id:        "task_1",
		Execution: 0,
		BuildId:   "build_1",
		Status:    evergreen.TaskSucceeded,
	}
	s.NoError(testTask1.Insert(s.T().Context()))
	for i := 0; i < 10; i++ {
		s.NoError(testTask1.Archive(ctx))
		err := UpdateOne(
			ctx,
			bson.M{IdKey: "task_1"},
			bson.M{"$set": bson.M{CanResetKey: false}},
		)
		s.NoError(err)
		testTask1.Execution += 1
	}
	for i := 0; i < 10; i++ {
		task, err := FindOneIdAndExecution(ctx, "task_1", i)
		s.NoError(err)
		s.Equal(task.Id, fmt.Sprintf("task_1_%d", i))
		s.Equal(task.Execution, i)
	}
}

func (s *TaskConnectorFetchByIdSuite) TestFindByVersion() {
	s.Require().NoError(db.ClearCollections(Collection, OldCollection, annotations.Collection))
	taskKnown2 := &Task{
		Id:            "task_known",
		Execution:     2,
		Version:       "version_known",
		Status:        evergreen.TaskSucceeded,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	taskNotKnown := &Task{
		Id:            "task_not_known",
		Execution:     0,
		Version:       "version_not_known",
		Status:        evergreen.TaskFailed,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	taskNoAnnotation := &Task{
		Id:            "task_no_annotation",
		Execution:     0,
		Version:       "version_no_annotation",
		Status:        evergreen.TaskFailed,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	taskWithEmptyIssues := &Task{
		Id:            "task_with_empty_issues",
		Execution:     0,
		Version:       "version_with_empty_issues",
		Status:        evergreen.TaskFailed,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	s.NoError(taskKnown2.Insert(s.T().Context()))
	s.NoError(taskNotKnown.Insert(s.T().Context()))
	s.NoError(taskNoAnnotation.Insert(s.T().Context()))
	s.NoError(taskWithEmptyIssues.Insert(s.T().Context()))

	issue := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "chaya.malik"}}

	annotationExecution0 := annotations.TaskAnnotation{TaskId: "task_known", TaskExecution: 0, SuspectedIssues: []annotations.IssueLink{issue}}
	annotationExecution1 := annotations.TaskAnnotation{TaskId: "task_known", TaskExecution: 1, SuspectedIssues: []annotations.IssueLink{issue}}
	annotationExecution2 := annotations.TaskAnnotation{TaskId: "task_known", TaskExecution: 2, Issues: []annotations.IssueLink{issue}}

	annotationWithSuspectedIssue := annotations.TaskAnnotation{TaskId: "task_not_known", TaskExecution: 0, SuspectedIssues: []annotations.IssueLink{issue}}
	annotationWithEmptyIssues := annotations.TaskAnnotation{TaskId: "task_not_known", TaskExecution: 0, Issues: []annotations.IssueLink{}, SuspectedIssues: []annotations.IssueLink{issue}}

	s.NoError(annotationExecution0.Upsert(s.T().Context()))
	s.NoError(annotationExecution1.Upsert(s.T().Context()))
	s.NoError(annotationExecution2.Upsert(s.T().Context()))
	s.NoError(annotationWithSuspectedIssue.Upsert(s.T().Context()))
	s.NoError(annotationWithEmptyIssues.Upsert(s.T().Context()))

	ctx := context.TODO()
	opts := GetTasksByVersionOptions{}
	t, _, err := GetTasksByVersion(ctx, "version_known", opts)
	s.NoError(err)
	// ignore annotation for successful task
	s.Equal(evergreen.TaskSucceeded, t[0].DisplayStatus)

	// test with empty issues list
	t, _, err = GetTasksByVersion(ctx, "version_not_known", opts)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, t[0].DisplayStatus)

	// test with no annotation document
	t, _, err = GetTasksByVersion(ctx, "version_no_annotation", opts)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, t[0].DisplayStatus)

	// test with empty issues
	t, _, err = GetTasksByVersion(ctx, "version_with_empty_issues", opts)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, t[0].DisplayStatus)
}

func (s *TaskConnectorFetchByIdSuite) TestFindOldTasksByIDWithDisplayTasks() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Require().NoError(db.ClearCollections(Collection, OldCollection))
	testTask1 := &Task{
		Id:            "task_1",
		Execution:     0,
		BuildId:       "build_1",
		Status:        evergreen.TaskSucceeded,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	s.NoError(testTask1.Insert(s.T().Context()))
	testTask2 := &Task{
		Id:            "task_2",
		Execution:     0,
		BuildId:       "build_1",
		DisplayOnly:   true,
		Status:        evergreen.TaskSucceeded,
		DisplayTaskId: utility.ToStringPtr(""),
	}
	s.NoError(testTask2.Insert(s.T().Context()))
	for i := 0; i < 10; i++ {
		s.NoError(testTask1.Archive(ctx))
		testTask1.Execution += 1
		s.NoError(testTask2.Archive(ctx))
		testTask2.Execution += 1
	}
	tasks, err := FindOldWithDisplayTasks(ctx, ByOldTaskID("task_1"))
	s.NoError(err)
	s.Len(tasks, 10)
	for i := range tasks {
		s.Equal(i, tasks[i].Execution)
	}

	tasks, err = FindOldWithDisplayTasks(ctx, ByOldTaskID("task_2"))
	s.NoError(err)
	s.Len(tasks, 10)
	for i := range tasks {
		s.Equal(i, tasks[i].Execution)
	}
}

func (s *TaskConnectorFetchByIdSuite) TestFindByIdFail() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	found, err := FindOneId(ctx, "fake_task")
	s.NoError(err)
	s.Nil(found)
}

func convertOldTasksIntoTasks(tasks []Task) []Task {
	updatedTasks := []Task{}
	for _, t := range tasks {
		if t.OldTaskId != "" {
			t.Id = t.OldTaskId
		}
		updatedTasks = append(updatedTasks, t)
	}
	return updatedTasks
}

func assertTasksAreEqual(t *testing.T, expected, actual Task, exectedExecution int) {
	assert.Equal(t, expected.Id, actual.Id)
	assert.Equal(t, expected.Version, actual.Version)
	assert.Equal(t, expected.Status, actual.Status)
	assert.Equal(t, exectedExecution, actual.Execution)
}

func TestFindAbortingAndResettingDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, tsk Task, depTasks []Task){
		"ReturnsAllMatchingDependencies": func(t *testing.T, tsk Task, depTasks []Task) {
			require.NoError(t, tsk.Insert(t.Context()))

			found, err := tsk.FindAbortingAndResettingDependencies(ctx)
			assert.NoError(t, err)
			require.Len(t, found, 2)
			expected := []string{depTasks[1].Id, depTasks[3].Id}
			for _, foundTask := range found {
				assert.True(t, utility.StringSliceContains(expected, foundTask.Id), "should not have returned task '%s'", foundTask.Id)
			}
		},
		"ReturnsTransitiveMatchingDependencies": func(t *testing.T, tsk Task, depTasks []Task) {
			intermediateDepTask := Task{
				Id: "intermediate_dependency",
				DependsOn: []Dependency{
					{TaskId: depTasks[1].Id},
				},
			}
			require.NoError(t, intermediateDepTask.Insert(t.Context()))
			tsk.DependsOn = []Dependency{{TaskId: intermediateDepTask.Id}}
			require.NoError(t, tsk.Insert(t.Context()))

			found, err := tsk.FindAbortingAndResettingDependencies(ctx)
			assert.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, depTasks[1].Id, found[0].Id)
		},
		"IgnoresNonexistentTasks": func(t *testing.T, tsk Task, depTasks []Task) {
			tsk.DependsOn = append(tsk.DependsOn, Dependency{TaskId: "nonexistent"})
			require.NoError(t, tsk.Insert(t.Context()))

			found, err := tsk.FindAbortingAndResettingDependencies(ctx)
			assert.NoError(t, err)
			require.Len(t, found, 2)
			expected := []string{depTasks[1].Id, depTasks[3].Id}
			for _, foundTask := range found {
				assert.True(t, utility.StringSliceContains(expected, foundTask.Id), "should not have returned task '%s'", foundTask.Id)
			}
		},
		"IgnoresAbortingAndResettingTasksNotInDependencies": func(t *testing.T, tsk Task, depTasks []Task) {
			tsk.DependsOn = []Dependency{tsk.DependsOn[0], tsk.DependsOn[2], tsk.DependsOn[3]}
			require.NoError(t, tsk.Insert(t.Context()))

			found, err := tsk.FindAbortingAndResettingDependencies(ctx)
			assert.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, depTasks[3].Id, found[0].Id)
		},
		"ReturnsNoResultsForNoDependencies": func(t *testing.T, tsk Task, depTasks []Task) {
			tsk.DependsOn = nil
			require.NoError(t, tsk.Insert(t.Context()))

			found, err := tsk.FindAbortingAndResettingDependencies(ctx)
			assert.NoError(t, err)
			assert.Empty(t, found)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))

			tsk := Task{
				Id: "task_id",
			}
			depTasks := []Task{
				{
					Id:      "dep_task0",
					Aborted: true,
				},
				{
					Id:                "dep_task1",
					Aborted:           true,
					ResetWhenFinished: true,
				},
				{
					Id:                "dep_task3",
					ResetWhenFinished: true,
				},
				{
					Id:                      "dep_task4",
					Aborted:                 true,
					ResetFailedWhenFinished: true,
					DependsOn:               []Dependency{},
				},
			}
			for _, depTask := range depTasks {
				require.NoError(t, depTask.Insert(t.Context()))
				tsk.DependsOn = append(tsk.DependsOn, Dependency{TaskId: depTask.Id})
			}

			tCase(t, tsk, depTasks)
		})
	}
}

func TestHasResults(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, OldCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, OldCollection))
	}()

	for _, test := range []struct {
		name              string
		tsk               *Task
		executionTasks    []Task
		oldExecutionTasks []Task
		hasResults        bool
	}{
		{
			name: "RegularTaskNoResults",
			tsk:  &Task{Id: "task"},
		},
		{
			name: "RegularTaskLegacyResultsFlag",
			tsk: &Task{
				Id:             "task",
				HasTestResults: true,
			},
			hasResults: true,
		},
		{
			name: "DisplayTaskNoResults",
			tsk: &Task{
				Id:             "display_task",
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec_task0", "exec_task1"},
			},
			executionTasks: []Task{
				{Id: "exec_task0"},
				{Id: "exec_task1"},
			},
		},
		{
			name: "DisplayTaskLegacyResultsFlag",
			tsk: &Task{
				Id:             "display_task",
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec_task0", "exec_task1", "exec_task2"},
			},
			executionTasks: []Task{
				{Id: "exec_task0", HasTestResults: true},
				{Id: "exec_task1", HasTestResults: true},
				{Id: "exec_task2"},
			},
			hasResults: true,
		},
		{
			name: "ArchivedDisplayTaskLegacyResultsFlag",
			tsk: &Task{
				Id:             "display_task",
				DisplayOnly:    true,
				Execution:      2,
				ExecutionTasks: []string{"exec_task0", "exec_task1", "exec_task2", "exec_task3"},
				Archived:       true,
			},
			executionTasks: []Task{
				{Id: "exec_task0"},
				{Id: "exec_task1"},
				{Id: "exec_task2"},
			},
			oldExecutionTasks: []Task{
				{Id: "exec_task3_0", OldTaskId: "exec_task3", Execution: 0},
				{Id: "exec_task3_1", OldTaskId: "exec_task3", Execution: 1, HasTestResults: true},
			},
			hasResults: true,
		},
		{
			name: "ArchivedDisplayTaskResultsServicePopulated",
			tsk: &Task{
				Id:             "display_task",
				Execution:      2,
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec_task0", "exec_task1", "exec_task2", "exec_task3"},
				Archived:       true,
			},
			executionTasks: []Task{
				{Id: "exec_task0"},
				{Id: "exec_task1"},
				{Id: "exec_task2"},
			},
			oldExecutionTasks: []Task{
				{Id: "exec_task3_0", OldTaskId: "exec_task3", Execution: 0},
				{Id: "exec_task3_1", OldTaskId: "exec_task3", Execution: 1, HasTestResults: true},
			},
			hasResults: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			for _, execTask := range test.executionTasks {
				_, err := db.Replace(t.Context(), Collection, ById(execTask.Id), &execTask)
				require.NoError(t, err)
			}
			for _, execTask := range test.oldExecutionTasks {
				_, err := db.Replace(t.Context(), OldCollection, ById(execTask.Id), &execTask)
				require.NoError(t, err)
			}

			assert.Equal(t, test.hasResults, test.tsk.HasResults(ctx))
		})
	}
}

func TestCreateTestResultsTaskOptions(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, OldCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, OldCollection))
	}()

	for _, test := range []struct {
		name              string
		tsk               *Task
		executionTasks    []Task
		oldExecutionTasks []Task
		expectedOpts      []Task
	}{
		{
			name: "RegularTaskNoResults",
			tsk:  &Task{Id: "task"},
		},
		{
			name: "RegularTaskResultsLegacyResultsFlag",
			tsk: &Task{
				Id:             "task",
				Execution:      1,
				HasTestResults: true,
			},
			expectedOpts: []Task{
				{
					Id:             "task",
					Execution:      1,
					HasTestResults: true,
				},
			},
		},
		{
			name: "RegularTaskResults",
			tsk: &Task{
				Id:             "task",
				Execution:      1,
				HasTestResults: true,
			},
			expectedOpts: []Task{
				{
					Id:             "task",
					Execution:      1,
					HasTestResults: true,
				},
			},
		},
		{

			name: "ArchivedRegularTaskResultsLegacyResultsFlags",
			tsk: &Task{
				Id:             "task_0",
				OldTaskId:      "task",
				Execution:      0,
				HasTestResults: true,
				Archived:       true,
			},
			expectedOpts: []Task{
				{
					Id:             "task",
					OldTaskId:      "task",
					Execution:      0,
					HasTestResults: true,
					Archived:       true,
				},
			},
		},
		{
			name: "ArchivedRegularTaskResults",
			tsk: &Task{
				Id:             "task_0",
				OldTaskId:      "task",
				Execution:      0,
				HasTestResults: true,
				Archived:       true,
			},
			expectedOpts: []Task{
				{
					Id:             "task",
					OldTaskId:      "task",
					Execution:      0,
					HasTestResults: true,
					Archived:       true,
				},
			},
		},
		{
			name: "DisplayTaskNoResults",
			tsk: &Task{
				Id:             "display_task",
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec_task0", "exec_task1"},
			},
			executionTasks: []Task{
				{Id: "exec_task0"},
				{Id: "exec_task1"},
			},
		},
		{
			name: "DisplayTaskLegacyResultsFlag",
			tsk: &Task{
				Id:             "display_task",
				Execution:      1,
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec_task0", "exec_task1", "exec_task2"},
			},
			executionTasks: []Task{
				{Id: "exec_task0", HasTestResults: true},
				{Id: "exec_task1", Execution: 1, HasTestResults: true},
				{Id: "exec_task2"},
			},
			expectedOpts: []Task{
				{Id: "exec_task0", HasTestResults: true},
				{Id: "exec_task1", Execution: 1, HasTestResults: true},
			},
		},
		{
			name: "DisplayTaskResults",
			tsk: &Task{
				Id:             "display_task",
				Execution:      1,
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec_task0", "exec_task1", "exec_task2"},
			},
			executionTasks: []Task{
				{Id: "exec_task0", HasTestResults: true},
				{Id: "exec_task1", Execution: 1, HasTestResults: true},
				{Id: "exec_task2"},
			},
			expectedOpts: []Task{
				{Id: "exec_task0", HasTestResults: true},
				{Id: "exec_task1", Execution: 1, HasTestResults: true},
			},
		},
		{
			name: "ArchivedDisplayTaskLegacyResultsFlag",
			tsk: &Task{
				Id:             "display_task",
				DisplayOnly:    true,
				Execution:      2,
				ExecutionTasks: []string{"exec_task0", "exec_task1", "exec_task2", "exec_task3"},
				Archived:       true,
			},
			executionTasks: []Task{
				{Id: "exec_task0", HasTestResults: true},
				{Id: "exec_task1", Execution: 2, HasTestResults: true},
				{Id: "exec_task2"},
			},
			oldExecutionTasks: []Task{
				{Id: "exec_task1_0", OldTaskId: "exec_task1", HasTestResults: true},
				{Id: "exec_task1_1", OldTaskId: "exec_task1", Execution: 1, HasTestResults: true},
				{Id: "exec_task3_0", OldTaskId: "exec_task3", Execution: 0, HasTestResults: true},
				{Id: "exec_task3_1", OldTaskId: "exec_task3", Execution: 1, HasTestResults: true},
			},
			expectedOpts: []Task{
				{Id: "exec_task0", Execution: 0, HasTestResults: true, DependsOn: []Dependency{}},
				{Id: "exec_task1", Execution: 2, HasTestResults: true, DependsOn: []Dependency{}},
				{Id: "exec_task3", OldTaskId: "exec_task3", Archived: true, Execution: 1, HasTestResults: true, DependsOn: []Dependency{}},
			},
		},
		{
			name: "ArchivedDisplayTaskResults",
			tsk: &Task{
				Id:             "display_task",
				Execution:      2,
				DisplayOnly:    true,
				ExecutionTasks: []string{"exec_task0", "exec_task1", "exec_task2", "exec_task3"},
				Archived:       true,
			},
			executionTasks: []Task{
				{Id: "exec_task0", HasTestResults: true},
				{Id: "exec_task1", Execution: 2, HasTestResults: true},
				{Id: "exec_task2"},
			},
			oldExecutionTasks: []Task{
				{Id: "exec_task1_0", OldTaskId: "exec_task1", HasTestResults: true},
				{Id: "exec_task1_1", OldTaskId: "exec_task1", Execution: 1, HasTestResults: true},
				{Id: "exec_task3_0", OldTaskId: "exec_task3", Execution: 0, HasTestResults: true},
				{Id: "exec_task3_1", OldTaskId: "exec_task3", Execution: 1, HasTestResults: true},
			},
			expectedOpts: []Task{
				{Id: "exec_task0", HasTestResults: true, DependsOn: []Dependency{}},
				{Id: "exec_task1", Execution: 2, HasTestResults: true, DependsOn: []Dependency{}},
				{Id: "exec_task3", OldTaskId: "exec_task3", Execution: 1, Archived: true, HasTestResults: true, DependsOn: []Dependency{}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			for _, execTask := range test.executionTasks {
				_, err := db.Replace(t.Context(), Collection, ById(execTask.Id), &execTask)
				require.NoError(t, err)
			}
			for _, execTask := range test.oldExecutionTasks {
				execTask.Archived = true
				_, err := db.Replace(t.Context(), OldCollection, ById(execTask.Id), &execTask)
				require.NoError(t, err)
			}

			opts, err := test.tsk.GetTestResultsTasks(ctx)
			require.NoError(t, err)
			assert.ElementsMatch(t, test.expectedOpts, opts)
		})
	}
}

func TestWillRun(t *testing.T) {
	t.Run("TaskWillRunIfActivated", func(t *testing.T) {
		tsk := Task{
			Status:    evergreen.TaskUndispatched,
			Activated: true,
		}
		assert.True(t, tsk.WillRun())
	})
	t.Run("TaskWillNotRunIfDeactivated", func(t *testing.T) {
		tsk := Task{
			Status:    evergreen.TaskUndispatched,
			Activated: false,
		}
		assert.False(t, tsk.WillRun())
	})
	t.Run("TaskWillNotRunIfItIsAlreadyInProgress", func(t *testing.T) {
		tsk := Task{
			Status:    evergreen.TaskStarted,
			Activated: true,
		}
		assert.False(t, tsk.WillRun())
	})
	t.Run("TaskWillRunEvenIfDependenciesAreNotYetFinished", func(t *testing.T) {
		tsk := Task{
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			DependsOn: []Dependency{{Finished: false}},
		}
		assert.True(t, tsk.WillRun())
	})
	t.Run("TaskWillRunIfAllDependenciesAreMet", func(t *testing.T) {
		tsk := Task{
			Status:            evergreen.TaskUndispatched,
			Activated:         true,
			ExecutionPlatform: ExecutionPlatformContainer,
			DependsOn:         []Dependency{{Finished: true, Unattainable: false}},
		}
		assert.True(t, tsk.WillRun())
	})
	t.Run("TaskWillNotRunIfDependenciesAreUnattainable", func(t *testing.T) {
		tsk := Task{
			Status:            evergreen.TaskUndispatched,
			Activated:         true,
			ExecutionPlatform: ExecutionPlatformContainer,
			DependsOn:         []Dependency{{Finished: true, Unattainable: true}},
		}
		assert.False(t, tsk.WillRun())
	})
}

func TestIsInProgress(t *testing.T) {
	for _, status := range evergreen.TaskCompletedStatuses {
		t.Run(fmt.Sprintf("Status%sIsNotInProgress", cases.Title(language.AmericanEnglish).String(status)), func(t *testing.T) {
			tsk := Task{
				Status: status,
			}
			assert.False(t, tsk.IsInProgress())
		})
	}
	for _, status := range evergreen.TaskInProgressStatuses {
		t.Run(fmt.Sprintf("Status%sIsInProgress", cases.Title(language.AmericanEnglish).String(status)), func(t *testing.T) {
			tsk := Task{
				Status: status,
			}
			assert.True(t, tsk.IsInProgress())
		})
	}
}

func TestReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		require.NoError(t, db.Clear(Collection))
	}()

	t.Run("NoDependencies", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		t0 := Task{
			Id:       "t0",
			Status:   evergreen.TaskSucceeded,
			CanReset: true,
		}
		assert.NoError(t, t0.Insert(t.Context()))

		assert.NoError(t, t0.Reset(ctx, "user"))
		dbTask, err := FindOneId(ctx, t0.Id)
		assert.NoError(t, err)
		assert.False(t, dbTask.UnattainableDependency)
		assert.Equal(t, "user", dbTask.ActivatedBy)
	})

	t.Run("UnattainableDependency", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		t0 := Task{
			Id:                     "t0",
			UnattainableDependency: true,
			DependsOn: []Dependency{
				{TaskId: "t1", Unattainable: true},
				{TaskId: "t2", Unattainable: false},
			},
			Status:   evergreen.TaskSucceeded,
			CanReset: true,
		}
		assert.NoError(t, t0.Insert(t.Context()))

		assert.NoError(t, t0.Reset(ctx, ""))
		dbTask, err := FindOneId(ctx, t0.Id)
		assert.NoError(t, err)
		assert.True(t, dbTask.UnattainableDependency)
	})

	t.Run("AttainableDependencies", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		t0 := Task{
			Id: "t0",
			DependsOn: []Dependency{
				{TaskId: "t1", Unattainable: false},
				{TaskId: "t2", Unattainable: false},
			},
			Status:   evergreen.TaskSucceeded,
			CanReset: true,
		}
		assert.NoError(t, t0.Insert(t.Context()))

		assert.NoError(t, t0.Reset(ctx, ""))
		dbTask, err := FindOneId(ctx, t0.Id)
		assert.NoError(t, err)
		assert.False(t, dbTask.UnattainableDependency)
	})

	t.Run("UnsetsExpectedFields", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		t0 := Task{
			Id:                      "t0",
			Status:                  evergreen.TaskSucceeded,
			Details:                 apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded},
			TaskOutputInfo:          &TaskOutput{TaskLogs: TaskLogOutput{Version: 1}},
			ResultsFailed:           true,
			HasTestResults:          true,
			ResetWhenFinished:       true,
			IsAutomaticRestart:      true,
			ResetFailedWhenFinished: true,
			OverrideDependencies:    true,
			CanReset:                true,
			HasAnnotations:          true,
			AgentVersion:            "a1",
			HostId:                  "h",
			PodID:                   "p",
			HostCreateDetails:       []HostCreateDetail{{HostId: "h"}},
			NumNextTaskDispatches:   3,
		}
		assert.NoError(t, t0.Insert(t.Context()))

		assert.NoError(t, t0.Reset(ctx, ""))
		dbTask, err := FindOneId(ctx, t0.Id)
		assert.NoError(t, err)
		assert.False(t, dbTask.ResultsFailed)
		assert.False(t, dbTask.HasTestResults)
		assert.False(t, dbTask.ResetWhenFinished)
		assert.False(t, dbTask.IsAutomaticRestart)
		assert.False(t, dbTask.ResetFailedWhenFinished)
		assert.False(t, dbTask.OverrideDependencies)
		assert.False(t, dbTask.HasAnnotations)
		assert.False(t, dbTask.CanReset)
		assert.Equal(t, "", dbTask.AgentVersion)
		assert.Equal(t, "", dbTask.HostId)
		assert.Equal(t, "", dbTask.PodID)
		assert.Empty(t, dbTask.HostCreateDetails)
		assert.Empty(t, dbTask.TaskOutputInfo)
		assert.Empty(t, dbTask.Details)
		assert.Zero(t, dbTask.NumNextTaskDispatches)

	})

}

func TestResetTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		require.NoError(t, db.Clear(Collection))
	}()

	t.Run("NoDependencies", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		t0 := Task{
			Id:       "t0",
			Status:   evergreen.TaskSucceeded,
			CanReset: true,
		}
		assert.NoError(t, t0.Insert(t.Context()))

		assert.NoError(t, ResetTasks(ctx, []Task{t0}, "user"))
		dbTask, err := FindOneId(ctx, t0.Id)
		assert.NoError(t, err)
		assert.False(t, dbTask.UnattainableDependency)
		assert.Equal(t, "user", dbTask.ActivatedBy)
	})

	t.Run("UnattainableDependency", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		t0 := Task{
			Id:                     "t0",
			UnattainableDependency: true,
			DependsOn: []Dependency{
				{TaskId: "t1", Unattainable: true},
				{TaskId: "t2", Unattainable: false},
			},
			Status:   evergreen.TaskSucceeded,
			CanReset: true,
		}
		assert.NoError(t, t0.Insert(t.Context()))

		assert.NoError(t, ResetTasks(ctx, []Task{t0}, ""))
		dbTask, err := FindOneId(ctx, t0.Id)
		assert.NoError(t, err)
		assert.True(t, dbTask.UnattainableDependency)
	})

	t.Run("AttainableDependencies", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		t0 := Task{
			Id: "t0",
			DependsOn: []Dependency{
				{TaskId: "t1", Unattainable: false},
				{TaskId: "t2", Unattainable: false},
			},
			Status:   evergreen.TaskSucceeded,
			CanReset: true,
		}
		assert.NoError(t, t0.Insert(t.Context()))

		assert.NoError(t, ResetTasks(ctx, []Task{t0}, ""))
		dbTask, err := FindOneId(ctx, t0.Id)
		assert.NoError(t, err)
		assert.False(t, dbTask.UnattainableDependency)
	})
}

func TestGenerateNotRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, tsk *Task){
		"ReturnsTaskThatNeedsGeneration": func(t *testing.T, tsk *Task) {
			require.NoError(t, tsk.Insert(t.Context()))

			tasks, err := GenerateNotRun(ctx)
			require.NoError(t, err)
			require.Len(t, tasks, 1)
			assert.Equal(t, tsk.Id, tasks[0].Id)
		},
		"IgnoresFinishedTasks": func(t *testing.T, tsk *Task) {
			tsk.Status = evergreen.TaskFailed
			require.NoError(t, tsk.Insert(t.Context()))

			tasks, err := GenerateNotRun(ctx)
			require.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"IgnoresTasksThatAlreadyFinishedGenerating": func(t *testing.T, tsk *Task) {
			tsk.GeneratedTasks = true
			require.NoError(t, tsk.Insert(t.Context()))

			tasks, err := GenerateNotRun(ctx)
			require.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"IgnoresTasksThatHaveNothingToGenerate": func(t *testing.T, tsk *Task) {
			tsk.GeneratedJSONAsString = nil
			require.NoError(t, tsk.Insert(t.Context()))

			tasks, err := GenerateNotRun(ctx)
			require.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"IgnoresTasksWhoseGenerationRequestIsStale": func(t *testing.T, tsk *Task) {
			tsk.StartTime = time.Now().Add(-100000 * time.Hour)
			require.NoError(t, tsk.Insert(t.Context()))

			tasks, err := GenerateNotRun(ctx)
			require.NoError(t, err)
			assert.Empty(t, tasks)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))

			tCase(t, &Task{
				Id:                    "task_id",
				Status:                evergreen.TaskStarted,
				StartTime:             time.Now(),
				GeneratedTasks:        false,
				GeneratedJSONAsString: []string{"some_generated_json"},
			})
		})
	}
}

func TestSetGeneratedJSON(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, tsk *Task){
		"Succeeds": func(t *testing.T, tsk *Task) {
			files := GeneratedJSONFiles{"generated_json"}
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, tsk.SetGeneratedJSON(ctx, files))

			dbTask, err := FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, files, dbTask.GeneratedJSONAsString)
		},
		"NoopsForAlreadySetGeneratedJSON": func(t *testing.T, tsk *Task) {
			originalFiles := GeneratedJSONFiles{"generated_files"}
			tsk.GeneratedJSONAsString = originalFiles
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, tsk.SetGeneratedJSON(ctx, []string{"new_generated_json"}))

			dbTask, err := FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.EqualValues(t, originalFiles, dbTask.GeneratedJSONAsString)
			assert.Empty(t, dbTask.GeneratedJSONStorageMethod)
		},
		"NoopsForAlreadySetGeneratedJSONDBStorage": func(t *testing.T, tsk *Task) {
			originalFiles := GeneratedJSONFiles{"generated_json"}
			tsk.GeneratedJSONAsString = originalFiles
			tsk.GeneratedJSONStorageMethod = evergreen.ProjectStorageMethodDB
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, tsk.SetGeneratedJSON(ctx, GeneratedJSONFiles{"new_generated_json"}))

			dbTask, err := FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, originalFiles, dbTask.GeneratedJSONAsString)
			assert.Equal(t, evergreen.ProjectStorageMethodDB, dbTask.GeneratedJSONStorageMethod)
		},
		"NoopsForAlreadySetGeneratedJSONS3Storage": func(t *testing.T, tsk *Task) {
			tsk.GeneratedJSONStorageMethod = evergreen.ProjectStorageMethodS3
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, tsk.SetGeneratedJSON(ctx, GeneratedJSONFiles{"new_generated_json"}))

			dbTask, err := FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, evergreen.ProjectStorageMethodS3, dbTask.GeneratedJSONStorageMethod)
			assert.Empty(t, dbTask.GeneratedJSONAsString)
		},
		"FailsForNonexistentTask": func(t *testing.T, tsk *Task) {
			assert.Error(t, tsk.SetGeneratedJSON(ctx, GeneratedJSONFiles{"generated_json"}))
			assert.Empty(t, tsk.GeneratedJSONAsString)
			assert.Empty(t, tsk.GeneratedJSONStorageMethod)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))

			tCase(t, &Task{
				Id:        "task_id",
				Status:    evergreen.TaskStarted,
				StartTime: time.Now(),
			})
		})
	}
}

func TestSetGeneratedJSONStorageMethod(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, tsk *Task){
		"Succeeds": func(t *testing.T, tsk *Task) {
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, tsk.SetGeneratedJSONStorageMethod(ctx, evergreen.ProjectStorageMethodS3))

			dbTask, err := FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, evergreen.ProjectStorageMethodS3, dbTask.GeneratedJSONStorageMethod)
		},
		"NoopsForAlreadySetGeneratedJSONStorageMethod": func(t *testing.T, tsk *Task) {
			tsk.GeneratedJSONStorageMethod = evergreen.ProjectStorageMethodDB
			require.NoError(t, tsk.Insert(t.Context()))

			require.NoError(t, tsk.SetGeneratedJSONStorageMethod(ctx, evergreen.ProjectStorageMethodS3))

			dbTask, err := FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, evergreen.ProjectStorageMethodDB, dbTask.GeneratedJSONStorageMethod)
		},
		"FailsForNonexistentTask": func(t *testing.T, tsk *Task) {
			assert.Error(t, tsk.SetGeneratedJSONStorageMethod(ctx, evergreen.ProjectStorageMethodDB))
			assert.Empty(t, tsk.GeneratedJSONStorageMethod)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))

			tCase(t, &Task{
				Id:        "task_id",
				Status:    evergreen.TaskStarted,
				StartTime: time.Now(),
			})
		})
	}
}

func TestCalculateOnDemandCost(t *testing.T) {
	runtimeSeconds := 3600.0
	distroCost := distro.CostData{
		OnDemandRate: 0.1,
	}
	financeConfig := evergreen.CostConfig{
		OnDemandDiscount: 0.2,
	}
	runtimeHours := runtimeSeconds / 3600.0
	predictedCost := runtimeHours * distroCost.OnDemandRate * (1 - financeConfig.OnDemandDiscount)
	actualCost := CalculateOnDemandCost(runtimeSeconds, distroCost, financeConfig)
	assert.Equal(t, float32(predictedCost), float32(actualCost))
}

func TestCalculateAdjustedTaskCost(t *testing.T) {
	runtimeSeconds := 3600.0
	distroCost := distro.CostData{
		OnDemandRate:    0.20,
		SavingsPlanRate: 0.10,
	}
	financeConfig := evergreen.CostConfig{
		FinanceFormula:      0.6,
		SavingsPlanDiscount: 0.5,
		OnDemandDiscount:    0.4,
	}
	runtimeHours := runtimeSeconds / 3600.0
	savingsPlanPortion := financeConfig.FinanceFormula * distroCost.SavingsPlanRate * financeConfig.SavingsPlanDiscount
	onDemandPortion := (1 - financeConfig.FinanceFormula) * distroCost.OnDemandRate * (1 - financeConfig.OnDemandDiscount)
	predictedCost := (savingsPlanPortion + onDemandPortion) * runtimeHours
	actualCost := CalculateAdjustedTaskCost(runtimeSeconds, distroCost, financeConfig)
	assert.InDelta(t, predictedCost, actualCost, 0.001)
}

func TestCalculateTaskCost(t *testing.T) {
	runtimeSeconds := 3600.0
	distroCost := distro.CostData{
		OnDemandRate:    0.20,
		SavingsPlanRate: 0.10,
	}
	financeConfig := evergreen.CostConfig{
		FinanceFormula:      0.6,
		SavingsPlanDiscount: 0.5,
		OnDemandDiscount:    0.04,
	}
	taskCost := CalculateTaskCost(runtimeSeconds, distroCost, financeConfig)
	expectedOnDemand := CalculateOnDemandCost(runtimeSeconds, distroCost, financeConfig)
	expectedAdjusted := CalculateAdjustedTaskCost(runtimeSeconds, distroCost, financeConfig)
	assert.Equal(t, expectedOnDemand, taskCost.OnDemandEC2Cost)
	assert.Equal(t, expectedAdjusted, taskCost.AdjustedEC2Cost)
	assert.False(t, taskCost.IsZero())
}

func TestTaskCostIsZero(t *testing.T) {
	zeroTaskCost := cost.Cost{}
	assert.True(t, zeroTaskCost.IsZero())
	nonZeroOnDemand := cost.Cost{OnDemandEC2Cost: 0.1}
	assert.False(t, nonZeroOnDemand.IsZero())
	nonZeroAdjusted := cost.Cost{AdjustedEC2Cost: 0.1}
	assert.False(t, nonZeroAdjusted.IsZero())
	nonZeroBoth := cost.Cost{OnDemandEC2Cost: 0.1, AdjustedEC2Cost: 0.2}
	assert.False(t, nonZeroBoth.IsZero())
}

func TestHasValidDistro(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(Collection, distro.Collection))

	validDistro := distro.Distro{
		Id: "valid-distro",
	}
	require.NoError(t, validDistro.Insert(ctx))

	t.Run("TaskWithValidPrimaryDistro", func(t *testing.T) {
		task := &Task{
			Id:       "task-with-valid-distro",
			DistroId: validDistro.Id,
		}
		assert.Equal(t, true, task.HasValidDistro(ctx))
	})

	t.Run("TaskWithInvalidPrimaryDistroButValidSecondaryDistro", func(t *testing.T) {
		task := &Task{
			Id:               "task-with-secondary",
			DistroId:         "nonexistent-distro",
			SecondaryDistros: []string{"nonexistent-distro-2", validDistro.Id},
		}
		assert.Equal(t, true, task.HasValidDistro(ctx))
	})

	t.Run("TaskWithNoValidDistros", func(t *testing.T) {
		task := &Task{
			Id:               "task-no-valid-distro",
			DistroId:         "nonexistent-distro",
			SecondaryDistros: []string{"nonexistent-distro-2", "nonexistent-distro-3"},
		}
		assert.Equal(t, false, task.HasValidDistro(ctx))
	})
}
