package testresult

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/sometimes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendResults(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, task.ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, task.ClearLocal(ctx, env))
	}()
	svc := task.NewLocalService(env)
	task0 := task.Task{
		Id:             "task0",
		Execution:      0,
		ResultsService: task.TestResultsServiceLocal,
	}
	savedResults0 := make([]TestResult, 10)
	for i := 0; i < len(savedResults0); i++ {
		result := generateTestResult()
		result.TaskID = task0.Id
		result.Execution = task0.Execution
		if i%2 != 0 {
			result.Status = evergreen.TestFailedStatus
		}
		savedResults0[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults0))

	task1 := task.Task{
		Id:             "task1",
		Execution:      0,
		ResultsService: task.TestResultsServiceLocal,
	}
	savedResults1 := make([]TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := generateTestResult()
		result.TaskID = task1.Id
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults1))

	task2 := task.Task{
		Id:             "task2",
		Execution:      1,
		ResultsService: task.TestResultsServiceLocal,
	}
	savedResults2 := make([]TestResult, 10)
	for i := 0; i < len(savedResults2); i++ {
		result := generateTestResult()
		result.TaskID = task2.Id
		result.Execution = task2.Execution
		savedResults2[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults2))

	dbResults, err := svc.Get(ctx, []task.Task{task0, task1, task2})
	require.NoError(t, err)
	require.Len(t, dbResults, 3)
	for i, result := range dbResults {
		assert.Equal(t, result.Stats.TotalCount, 10)
		assert.Len(t, result.Results, 10)
		if i == 0 {
			assert.Equal(t, result.Stats.FailedCount, 5)
			failedResults := 0
			for _, r := range result.Results {
				if r.Status == evergreen.TestFailedStatus {
					failedResults++
				}
			}
			assert.Equal(t, result.Stats.FailedCount, failedResults)
		}
	}

}

func generateTestResult() TestResult {
	result := TestResult{
		TestName:      utility.RandomString(),
		Status:        evergreen.TestSucceededStatus,
		TestStartTime: time.Now().Add(-30 * time.Hour).UTC().Round(time.Millisecond),
		TestEndTime:   time.Now().UTC().Round(time.Millisecond),
	}
	if sometimes.Half() {
		result.DisplayTestName = utility.RandomString()
		result.GroupID = utility.RandomString()
		result.LogTestName = utility.RandomString()
		result.LogURL = utility.RandomString()
		result.RawLogURL = utility.RandomString()
		result.LineNum = rand.Intn(1000)
	}
	return result
}
