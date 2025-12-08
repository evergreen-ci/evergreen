package task

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestGetExpectedCostsForWindow(t *testing.T) {
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
			TaskCost:     TaskCost{OnDemandCost: onDemand, AdjustedCost: adjusted},
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

		results, err := getExpectedCostsForWindow(ctx, displayName, project, bv, now.Add(-5*time.Hour), now)
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
		results, err := getExpectedCostsForWindow(ctx, displayName, project, bv, now.Add(-5*time.Hour), now)
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

		results, err := getExpectedCostsForWindow(ctx, "task-a", project, bv, now.Add(-5*time.Hour), now)
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

		results, err := getExpectedCostsForWindow(ctx, displayName, project, bv, now.Add(-5*time.Hour), now)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.InDelta(t, 1.0, results[0].AvgOnDemandCost, 0.01)
	})

	t.Run("ExcludeTasksWithoutCost", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection))

		zeroCost := makeTask("t1", displayName, 1, 0, 0)
		require.NoError(t, zeroCost.Insert(ctx))

		results, err := getExpectedCostsForWindow(ctx, displayName, project, bv, now.Add(-5*time.Hour), now)
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})
}

func TestComputePredictedCostForWeek(t *testing.T) {
	ctx := context.Background()

	t.Run("ReturnsZero", func(t *testing.T) {
		task := &Task{
			Id:           "test-task",
			DisplayName:  "test",
			BuildVariant: "bv",
			Project:      "proj",
		}

		result, err := task.ComputePredictedCostForWeek(ctx)
		require.NoError(t, err)
		assert.True(t, result.PredictedCost.IsZero())
		assert.True(t, result.PredictedCostStdDev.IsZero())
	})
}

func TestHasCostPrediction(t *testing.T) {
	t.Run("NoPrediction", func(t *testing.T) {
		task := &Task{
			PredictedTaskCost: TaskCost{},
		}
		assert.False(t, task.HasCostPrediction())
	})

	t.Run("WithPrediction", func(t *testing.T) {
		task := &Task{
			PredictedTaskCost: TaskCost{
				OnDemandCost: 1.0,
				AdjustedCost: 0.8,
			},
		}
		assert.True(t, task.HasCostPrediction())
	})
}

func TestGetDisplayCost(t *testing.T) {
	for _, tc := range []struct {
		name     string
		task     Task
		wantCost TaskCost
	}{
		{
			name: "PrioritizesActualCost",
			task: Task{
				TaskCost:          TaskCost{OnDemandCost: 5.0, AdjustedCost: 4.0},
				PredictedTaskCost: TaskCost{OnDemandCost: 3.0, AdjustedCost: 2.4},
			},
			wantCost: TaskCost{OnDemandCost: 5.0, AdjustedCost: 4.0},
		},
		{
			name: "FallsBackToPredicted",
			task: Task{
				PredictedTaskCost: TaskCost{OnDemandCost: 3.0, AdjustedCost: 2.4},
			},
			wantCost: TaskCost{OnDemandCost: 3.0, AdjustedCost: 2.4},
		},
		{
			name:     "ReturnsZeroIfNoCosts",
			task:     Task{},
			wantCost: TaskCost{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost := tc.task.GetDisplayCost()
			assert.Equal(t, tc.wantCost, cost)
		})
	}
}

func TestTaskCostStdDevIsZero(t *testing.T) {
	t.Run("ZeroValues", func(t *testing.T) {
		stdDev := TaskCostStdDev{}
		assert.True(t, stdDev.IsZero())
	})

	t.Run("NonZeroOnDemand", func(t *testing.T) {
		stdDev := TaskCostStdDev{OnDemandCost: 1.0}
		assert.False(t, stdDev.IsZero())
	})

	t.Run("NonZeroAdjusted", func(t *testing.T) {
		stdDev := TaskCostStdDev{AdjustedCost: 1.0}
		assert.False(t, stdDev.IsZero())
	})

	t.Run("NonZeroBoth", func(t *testing.T) {
		stdDev := TaskCostStdDev{OnDemandCost: 1.0, AdjustedCost: 0.8}
		assert.False(t, stdDev.IsZero())
	})
}
