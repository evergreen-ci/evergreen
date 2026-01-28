package task

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/cost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestGetPredictedCostsForWindow(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, db.ClearCollections(Collection))

	_, err := evergreen.GetEnvironment().DB().Collection(Collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: TaskHistoricalDataIndex})
	require.NoError(t, err)

	const (
		bv          = "test-bv"
		project     = "test-proj"
		displayName = "test-task"
	)
	now := time.Now()

	makeTask := func(id, name string, hoursAgo int, onDemand, adjusted float64) Task {
		return Task{
			Id:           id,
			DisplayName:  name,
			BuildVariant: bv,
			Project:      project,
			Status:       evergreen.TaskSucceeded,
			FinishTime:   now.Add(time.Duration(-hoursAgo) * time.Hour),
			StartTime:    now.Add(time.Duration(-hoursAgo-1) * time.Hour),
			TaskCost:     cost.Cost{OnDemandEC2Cost: onDemand, AdjustedEC2Cost: adjusted},
		}
	}

	t.Run("WithHistoricalData", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection))

		for _, task := range []Task{
			makeTask("t1", displayName, 1, 1.0, 0.8),
			makeTask("t2", displayName, 2, 2.0, 1.6),
			makeTask("t3", displayName, 3, 3.0, 2.4),
		} {
			require.NoError(t, task.Insert(ctx))
		}

		results, err := getPredictedCostsForWindow(ctx, displayName, project, bv, now.Add(-5*time.Hour), now)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, displayName, results[0].DisplayName)
		assert.InDelta(t, 2.0, results[0].AvgOnDemandCost, 0.01)
		assert.InDelta(t, 1.6, results[0].AvgAdjustedCost, 0.01)
		assert.Greater(t, results[0].StdDevOnDemandCost, 0.0)
		assert.Greater(t, results[0].StdDevAdjustedCost, 0.0)
	})

	t.Run("NoHistoricalData", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection))
		results, err := getPredictedCostsForWindow(ctx, displayName, project, bv, now.Add(-5*time.Hour), now)
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})

	t.Run("FilterByDisplayName", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection))

		for _, task := range []Task{
			makeTask("t1", "task-a", 1, 1.0, 0.8),
			makeTask("t2", "task-b", 1, 2.0, 1.6),
		} {
			require.NoError(t, task.Insert(ctx))
		}

		results, err := getPredictedCostsForWindow(ctx, "task-a", project, bv, now.Add(-5*time.Hour), now)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "task-a", results[0].DisplayName)
		assert.InDelta(t, 1.0, results[0].AvgOnDemandCost, 0.01)
	})

	t.Run("ExcludeTimedOutTasks", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection))

		timedOut := makeTask("t1", displayName, 1, 100.0, 80.0)
		timedOut.Status = evergreen.TaskFailed
		timedOut.Details = apimodels.TaskEndDetail{TimedOut: true}
		require.NoError(t, timedOut.Insert(ctx))

		normal := makeTask("t2", displayName, 1, 1.0, 0.8)
		require.NoError(t, normal.Insert(ctx))

		results, err := getPredictedCostsForWindow(ctx, displayName, project, bv, now.Add(-5*time.Hour), now)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.InDelta(t, 1.0, results[0].AvgOnDemandCost, 0.01)
	})

	t.Run("ExcludeTasksWithoutCost", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection))

		zeroCost := makeTask("t1", displayName, 1, 0, 0)
		require.NoError(t, zeroCost.Insert(ctx))

		results, err := getPredictedCostsForWindow(ctx, displayName, project, bv, now.Add(-5*time.Hour), now)
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})
}

func TestComputePredictedCostForWeek(t *testing.T) {
	task := &Task{Id: "test", DisplayName: "test", BuildVariant: "bv", Project: "proj"}
	result, err := task.ComputePredictedCostForWeek(context.Background())
	require.NoError(t, err)
	assert.True(t, result.PredictedCost.IsZero())
}

func TestHasCostPrediction(t *testing.T) {
	assert.False(t, (&Task{}).HasCostPrediction())
	assert.True(t, (&Task{PredictedTaskCost: cost.Cost{OnDemandEC2Cost: 1.0}}).HasCostPrediction())
}

func TestGetDisplayCost(t *testing.T) {
	tests := []struct {
		task Task
		want cost.Cost
	}{
		{Task{TaskCost: cost.Cost{OnDemandEC2Cost: 5.0, AdjustedEC2Cost: 4.0}, PredictedTaskCost: cost.Cost{OnDemandEC2Cost: 3.0}}, cost.Cost{OnDemandEC2Cost: 5.0, AdjustedEC2Cost: 4.0}},
		{Task{PredictedTaskCost: cost.Cost{OnDemandEC2Cost: 3.0, AdjustedEC2Cost: 2.4}}, cost.Cost{OnDemandEC2Cost: 3.0, AdjustedEC2Cost: 2.4}},
		{Task{}, cost.Cost{}},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.task.GetDisplayCost())
	}
}

func TestComputeCostPredictionsInParallel(t *testing.T) {
	ctx := context.Background()

	predictions, err := computeCostPredictionsInParallel(ctx, []Task{})
	require.NoError(t, err)
	assert.Empty(t, predictions)

	task := Task{Id: "task1", DisplayName: "test", BuildVariant: "bv", Project: "proj"}
	predictions, err = computeCostPredictionsInParallel(ctx, []Task{task})
	require.NoError(t, err)
	assert.Len(t, predictions, 1)
	assert.Contains(t, predictions, "task1")

	tasks := []Task{
		{Id: "task1", DisplayName: "test", BuildVariant: "bv", Project: "proj"},
		{Id: "task2", DisplayName: "test2", BuildVariant: "bv", Project: "proj"},
	}
	predictions, err = computeCostPredictionsInParallel(ctx, tasks)
	require.NoError(t, err)
	assert.Len(t, predictions, 2)
	assert.Contains(t, predictions, "task1")
	assert.Contains(t, predictions, "task2")
}

func TestComputePredictedCostsForTasks(t *testing.T) {
	ctx := context.Background()

	predictions, err := ComputePredictedCostsForTasks(ctx, Tasks{})
	require.NoError(t, err)
	assert.Empty(t, predictions)

	task := &Task{Id: "task1", Activated: false}
	predictions, err = ComputePredictedCostsForTasks(ctx, Tasks{task})
	require.NoError(t, err)
	assert.Empty(t, predictions)

	task = &Task{Id: "task1", DisplayName: "test", BuildVariant: "bv", Project: "proj", Activated: true}
	predictions, err = ComputePredictedCostsForTasks(ctx, Tasks{task})
	require.NoError(t, err)
	assert.Contains(t, predictions, "task1")
	assert.True(t, predictions["task1"].IsZero()) // No historical data
}

func TestPredictedTaskCostSetCorrectly(t *testing.T) {
	predicted := cost.Cost{OnDemandEC2Cost: 5.0, AdjustedEC2Cost: 4.0}
	task := &Task{PredictedTaskCost: predicted}
	assert.Equal(t, predicted, task.PredictedTaskCost)

	task = &Task{}
	assert.True(t, task.PredictedTaskCost.IsZero())

	task = &Task{Id: "task1", DisplayName: "test", BuildVariant: "bv", Project: "proj"}
	result, err := task.ComputePredictedCostForWeek(context.Background())
	require.NoError(t, err)
	assert.True(t, result.PredictedCost.IsZero())
}
