package task

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestGenerateTasksEstimationsOnlyIncludesSucceeded(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := evergreen.GetEnvironment().DB().Collection(Collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: TaskHistoricalDataIndex})
	assert.NoError(err)

	bv := "bv"
	project := "proj"
	displayName := "display_name"

	assert.NoError((&Task{
		Id:                         "t1",
		DisplayName:                displayName,
		BuildVariant:               bv,
		Project:                    project,
		Status:                     evergreen.TaskFailed,
		NumGeneratedTasks:          1,
		NumActivatedGeneratedTasks: 11,
		StartTime:                  time.Now().Add(-10 * time.Hour),
		FinishTime:                 time.Now().Add(-9 * time.Hour),
		GeneratedTasks:             true,
	}).Insert(t.Context()))

	assert.NoError((&Task{
		Id:                         "t2",
		DisplayName:                displayName,
		BuildVariant:               bv,
		Project:                    project,
		Status:                     evergreen.TaskSucceeded,
		NumGeneratedTasks:          2,
		NumActivatedGeneratedTasks: 20,
		StartTime:                  time.Now().Add(-20 * time.Hour),
		FinishTime:                 time.Now().Add(-19 * time.Hour),
		GeneratedTasks:             true,
	}).Insert(t.Context()))

	results, err := GetBatchedGenerateTasksEstimations(ctx, project, bv, []string{displayName})
	require.NoError(err)
	require.Contains(results, displayName)
	assert.Equal(2, results[displayName].EstimatedNumGeneratedTasks)
	assert.Equal(20, results[displayName].EstimatedNumActivatedGeneratedTasks)
}

func TestGenerateTasksEstimationsNoPreviousTasks(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := evergreen.GetEnvironment().DB().Collection(Collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: TaskHistoricalDataIndex})
	assert.NoError(err)

	results, err := GetBatchedGenerateTasksEstimations(ctx, "project", "bv", []string{"new task with no history"})
	assert.NoError(err)
	assert.NotContains(results, "new task with no history")

	task := Task{
		DisplayName:  "new task with no history",
		GenerateTask: true,
	}
	task.SetGenerateTasksEstimationsFromMap(results)
	assert.Equal(0, utility.FromIntPtr(task.EstimatedNumGeneratedTasks))
	assert.Equal(0, utility.FromIntPtr(task.EstimatedNumActivatedGeneratedTasks))
}

func TestGetBatchedGenerateTasksEstimations(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := evergreen.GetEnvironment().DB().Collection(Collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: TaskHistoricalDataIndex})
	assert.NoError(err)

	bv := "bv"
	project := "proj"

	assert.NoError((&Task{
		Id:                         "h1",
		DisplayName:                "gen_a",
		BuildVariant:               bv,
		Project:                    project,
		Status:                     evergreen.TaskSucceeded,
		NumGeneratedTasks:          10,
		NumActivatedGeneratedTasks: 8,
		StartTime:                  time.Now().Add(-20 * time.Hour),
		FinishTime:                 time.Now().Add(-19 * time.Hour),
		GeneratedTasks:             true,
	}).Insert(t.Context()))
	assert.NoError((&Task{
		Id:                         "h2",
		DisplayName:                "gen_a",
		BuildVariant:               bv,
		Project:                    project,
		Status:                     evergreen.TaskSucceeded,
		NumGeneratedTasks:          20,
		NumActivatedGeneratedTasks: 16,
		StartTime:                  time.Now().Add(-10 * time.Hour),
		FinishTime:                 time.Now().Add(-9 * time.Hour),
		GeneratedTasks:             true,
	}).Insert(t.Context()))

	assert.NoError((&Task{
		Id:                         "h3",
		DisplayName:                "gen_b",
		BuildVariant:               bv,
		Project:                    project,
		Status:                     evergreen.TaskSucceeded,
		NumGeneratedTasks:          6,
		NumActivatedGeneratedTasks: 4,
		StartTime:                  time.Now().Add(-5 * time.Hour),
		FinishTime:                 time.Now().Add(-4 * time.Hour),
		GeneratedTasks:             true,
	}).Insert(t.Context()))

	assert.NoError((&Task{
		Id:                         "h4",
		DisplayName:                "gen_b",
		BuildVariant:               bv,
		Project:                    project,
		Status:                     evergreen.TaskFailed,
		NumGeneratedTasks:          100,
		NumActivatedGeneratedTasks: 100,
		StartTime:                  time.Now().Add(-3 * time.Hour),
		FinishTime:                 time.Now().Add(-2 * time.Hour),
		GeneratedTasks:             true,
	}).Insert(t.Context()))

	results, err := GetBatchedGenerateTasksEstimations(ctx, project, bv, []string{"gen_a", "gen_b", "gen_c"})
	require.NoError(err)

	require.Contains(results, "gen_a")
	assert.Equal(15, results["gen_a"].EstimatedNumGeneratedTasks)
	assert.Equal(12, results["gen_a"].EstimatedNumActivatedGeneratedTasks)

	require.Contains(results, "gen_b")
	assert.Equal(6, results["gen_b"].EstimatedNumGeneratedTasks)
	assert.Equal(4, results["gen_b"].EstimatedNumActivatedGeneratedTasks)

	assert.NotContains(results, "gen_c")
}

func TestGetBatchedGenerateTasksEstimationsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results, err := GetBatchedGenerateTasksEstimations(ctx, "proj", "bv", []string{})
	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestSetGenerateTasksEstimationsFromMap(t *testing.T) {
	estimations := map[string]generateTasksEstimation{
		"gen_a": {
			EstimatedNumGeneratedTasks:          15,
			EstimatedNumActivatedGeneratedTasks: 12,
		},
	}

	t.Run("GeneratorTaskWithMatchingEntry", func(t *testing.T) {
		task := Task{
			DisplayName:  "gen_a",
			GenerateTask: true,
		}
		task.SetGenerateTasksEstimationsFromMap(estimations)
		assert.Equal(t, 15, utility.FromIntPtr(task.EstimatedNumGeneratedTasks))
		assert.Equal(t, 12, utility.FromIntPtr(task.EstimatedNumActivatedGeneratedTasks))
	})

	t.Run("GeneratorTaskWithNoEntry", func(t *testing.T) {
		task := Task{
			DisplayName:  "gen_unknown",
			GenerateTask: true,
		}
		task.SetGenerateTasksEstimationsFromMap(estimations)
		assert.Equal(t, 0, utility.FromIntPtr(task.EstimatedNumGeneratedTasks))
		assert.Equal(t, 0, utility.FromIntPtr(task.EstimatedNumActivatedGeneratedTasks))
	})
}
