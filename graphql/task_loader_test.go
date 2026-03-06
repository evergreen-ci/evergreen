package graphql

import (
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTask(t *testing.T) {
	require.NoError(t, db.Clear(task.Collection))

	testTasks := []task.Task{
		{
			Id:           "task1",
			DisplayName:  "Task One",
			Status:       "success",
			Project:      "test-project",
			Version:      "version1",
			BuildVariant: "ubuntu",
			CreateTime:   time.Now(),
		},
		{
			Id:           "task2",
			DisplayName:  "Task Two",
			Status:       "failed",
			Project:      "test-project",
			Version:      "version2",
			BuildVariant: "ubuntu",
			CreateTime:   time.Now(),
		},
		{
			Id:           "task3",
			DisplayName:  "Task Three",
			Status:       "started",
			Project:      "test-project",
			Version:      "version3",
			BuildVariant: "ubuntu",
			CreateTime:   time.Now(),
		},
	}
	for _, tsk := range testTasks {
		require.NoError(t, tsk.Insert(t.Context()))
	}

	t.Run("SingleTaskLookup", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		result, err := GetTask(ctx, "task1")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "task1", utility.FromStringPtr(result.Id))
		assert.Equal(t, "Task One", utility.FromStringPtr(result.DisplayName))
		assert.Equal(t, "success", utility.FromStringPtr(result.Status))
	})

	t.Run("TaskNotFound", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		result, err := GetTask(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Nil(t, result, "should return nil for non-existent task")
	})

	t.Run("BatchedLookups", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		// Simulate concurrent requests that should be batched.
		var wg sync.WaitGroup
		type getTaskResult struct {
			taskID string
			found  bool
			err    error
		}
		resultsChan := make(chan getTaskResult, 3)

		taskIDs := []string{"task1", "task2", "task3"}
		for _, id := range taskIDs {
			wg.Add(1)
			go func(taskID string) {
				defer wg.Done()
				result, err := GetTask(ctx, taskID)
				resultsChan <- getTaskResult{taskID: taskID, found: result != nil, err: err}
			}(id)
		}

		wg.Wait()
		close(resultsChan)

		results := make(map[string]bool)
		for result := range resultsChan {
			require.NoError(t, result.err, "lookup should not error")
			results[result.taskID] = result.found
		}
		assert.Len(t, results, 3)
		for _, id := range taskIDs {
			assert.True(t, results[id], "expected to find task '%s'", id)
		}
	})

	t.Run("MixedExistingAndNonExisting", func(t *testing.T) {
		ctx := setupLoaderContext(t.Context())

		var wg sync.WaitGroup
		type getTaskResult struct {
			taskID string
			found  bool
			err    error
		}
		resultsChan := make(chan getTaskResult, 3)

		taskIDs := []string{"task1", "nonexistent", "task3"}
		for _, id := range taskIDs {
			wg.Add(1)
			go func(taskID string) {
				defer wg.Done()
				result, err := GetTask(ctx, taskID)
				resultsChan <- getTaskResult{taskID: taskID, found: result != nil, err: err}
			}(id)
		}

		wg.Wait()
		close(resultsChan)

		results := make(map[string]bool)
		for result := range resultsChan {
			require.NoError(t, result.err, "lookup should not error")
			results[result.taskID] = result.found
		}
		assert.Len(t, results, 3)
		assert.True(t, results["task1"], "task1 should be found")
		assert.False(t, results["nonexistent"], "nonexistent should not be found")
		assert.True(t, results["task3"], "task3 should be found")
	})
}
