package task

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpectedDuration(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: TaskHistoricalDataIndex})
	assert.NoError(err)
	bv := "bv"
	project := "proj"
	now := time.Now()

	t1 := Task{
		Id:           "t1",
		BuildVariant: bv,
		Project:      project,
		Status:       evergreen.TaskSucceeded,
		FinishTime:   now,
		StartTime:    now.Add(-10 * time.Minute),
		TimeTaken:    10 * time.Minute,
	}
	assert.NoError(t1.Insert(t.Context()))
	t2 := Task{
		Id:           "t2",
		BuildVariant: bv,
		Project:      project,
		Status:       evergreen.TaskSucceeded,
		FinishTime:   now,
		StartTime:    now.Add(-30 * time.Minute),
		TimeTaken:    30 * time.Minute,
	}
	assert.NoError(t2.Insert(t.Context()))
	t3 := Task{
		Id:           "t3",
		BuildVariant: bv,
		Project:      project,
		Status:       evergreen.TaskSucceeded,
		FinishTime:   now,
		StartTime:    now.Add(-35 * time.Minute),
		TimeTaken:    35 * time.Minute,
	}
	assert.NoError(t3.Insert(t.Context()))
	t4 := Task{
		Id:           "t4",
		BuildVariant: bv,
		Project:      project,
		Status:       evergreen.TaskSucceeded,
		FinishTime:   now,
		StartTime:    now.Add(-25 * time.Minute),
		TimeTaken:    25 * time.Minute,
	}
	assert.NoError(t4.Insert(t.Context()))

	results, err := getExpectedDurationsForWindow(t.Context(), "", project, bv, now.Add(-1*time.Hour), now)
	assert.NoError(err)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.EqualValues(25*time.Minute, results[0].ExpectedDuration)
	assert.InDelta(9.35*float64(time.Minute), results[0].StdDev, 0.01*float64(time.Minute))
}

// clearTasksAndEstimateCaches resets the task collection and the package-level caches, which otherwise leak between tests.
func clearTasksAndEstimateCaches(t *testing.T) {
	reset := func() {
		require.NoError(t, db.ClearCollections(Collection))
		expectedDurationCache.Purge()
		generateTasksEstimationCache.Purge()
		noGenerateTasksHistoryCache.Purge()
	}
	reset()
	t.Cleanup(reset)
}

func TestFetchExpectedDurationSharesEstimateBetweenSiblingTasks(t *testing.T) {
	const (
		project      = "proj"
		buildVariant = "bv"
		displayName  = "compile"
	)

	clearTasksAndEstimateCaches(t)
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: TaskHistoricalDataIndex})
	require.NoError(t, err)

	now := time.Now()
	insertHistory := func(id string, timeTaken time.Duration) {
		history := Task{
			Id:           id,
			DisplayName:  displayName,
			BuildVariant: buildVariant,
			Project:      project,
			Status:       evergreen.TaskSucceeded,
			StartTime:    now.Add(-timeTaken),
			FinishTime:   now,
			TimeTaken:    timeTaken,
		}
		require.NoError(t, history.Insert(t.Context()))
	}

	newTask := func(id string) *Task {
		tsk := &Task{Id: id, DisplayName: displayName, BuildVariant: buildVariant, Project: project}
		require.NoError(t, tsk.Insert(t.Context()))
		return tsk
	}

	insertHistory("history", 20*time.Minute)
	assert.Equal(t, 20*time.Minute, newTask("first").FetchExpectedDuration(t.Context()).Average)

	// A different history proves the sibling came from the cache, and that the next task after a reset sees the new value.
	require.NoError(t, db.ClearCollections(Collection))
	insertHistory("newHistory", 30*time.Minute)
	assert.Equal(t, 20*time.Minute, newTask("sibling").FetchExpectedDuration(t.Context()).Average)

	expectedDurationCache.Purge()
	assert.Equal(t, 30*time.Minute, newTask("afterExpiry").FetchExpectedDuration(t.Context()).Average)
}
