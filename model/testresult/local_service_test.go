package testresult

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/sometimes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemService(t *testing.T) {
	svc := newInMemService(newInMemStore())

	task0 := TaskOptions{TaskID: "task0", Execution: 0}
	savedResults0 := make([]TestResult, 10)
	for i := 0; i < len(savedResults0); i++ {
		result := getTestResult()
		result.TaskID = task0.TaskID
		result.Execution = task0.Execution
		if i%2 != 0 {
			result.Status = evergreen.TestFailedStatus
		}
		savedResults0[i] = result
	}
	svc.store.appendResults(task0.TaskID, task0.Execution, savedResults0)

	task1 := TaskOptions{TaskID: "task1", Execution: 0}
	savedResults1 := make([]TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	svc.store.appendResults(task1.TaskID, task1.Execution, savedResults1)

	task2 := TaskOptions{TaskID: "task2", Execution: 1}
	savedResults2 := make([]TestResult, 10)
	for i := 0; i < len(savedResults2); i++ {
		result := getTestResult()
		result.TaskID = task2.TaskID
		result.Execution = task2.Execution
		savedResults2[i] = result
	}
	svc.store.appendResults(task2.TaskID, task2.Execution, savedResults2)

	emptyTask := TaskOptions{TaskID: "DNE", Execution: 0}

	t.Run("GetMergedTaskTestResults", func(t *testing.T) {
		t.Run("WithoutFilterAndSortOpts", func(t *testing.T) {
			taskOpts := []TaskOptions{task1, task2, task0, emptyTask}
			taskResults, err := svc.GetMergedTaskTestResults(context.Background(), taskOpts, nil)
			require.NoError(t, err)

			assert.Equal(t, len(taskResults.Results), taskResults.Stats.TotalCount)
			assert.Equal(t, len(savedResults0)/2, taskResults.Stats.FailedCount)
			assert.Equal(t, len(taskResults.Results), utility.FromIntPtr(taskResults.Stats.FilteredCount))
			expectedResults := append(append(append([]TestResult{}, savedResults0...), savedResults1...), savedResults2...)
			assert.Equal(t, expectedResults, taskResults.Results)
		})
		t.Run("WithFilterAndSortOpts", func(t *testing.T) {
			taskOpts := []TaskOptions{task0}
			filterOpts := &FilterOptions{Statuses: []string{evergreen.TestSucceededStatus}}
			taskResults, err := svc.GetMergedTaskTestResults(context.Background(), taskOpts, filterOpts)
			require.NoError(t, err)

			assert.Equal(t, len(savedResults0), taskResults.Stats.TotalCount)
			assert.Equal(t, len(savedResults0)/2, taskResults.Stats.FailedCount)
			assert.Equal(t, len(savedResults0)/2, utility.FromIntPtr(taskResults.Stats.FilteredCount))
			require.Len(t, taskResults.Results, len(savedResults0)/2)
			for i, result := range taskResults.Results {
				require.Equal(t, savedResults0[2*i], result)
			}
		})
	})
	t.Run("GetMergedTaskTestResultsStats", func(t *testing.T) {
		taskOpts := []TaskOptions{task1, task2, task0, emptyTask}
		stats, err := svc.GetMergedTaskTestResultsStats(context.Background(), taskOpts)
		require.NoError(t, err)

		assert.Equal(t, len(savedResults0)+len(savedResults1)+len(savedResults2), stats.TotalCount)
		assert.Equal(t, len(savedResults0)/2, stats.FailedCount)
		assert.Nil(t, stats.FilteredCount)
	})
	t.Run("GetMergedFailedTestSample", func(t *testing.T) {
		task3 := TaskOptions{TaskID: "task3", Execution: 0}
		savedResults3 := make([]TestResult, maxSampleSize)
		for i := 0; i < len(savedResults3); i++ {
			result := getTestResult()
			result.TaskID = task3.TaskID
			result.Execution = task3.Execution
			if i%2 == 0 {
				result.Status = evergreen.TestFailedStatus
			}
			savedResults3[i] = result
		}
		svc.store.appendResults(task3.TaskID, task3.Execution, savedResults3)

		task4 := TaskOptions{TaskID: "task4", Execution: 1}
		savedResults4 := make([]TestResult, maxSampleSize)
		for i := 0; i < len(savedResults3); i++ {
			result := getTestResult()
			result.TaskID = task4.TaskID
			result.Execution = task4.Execution
			result.Status = evergreen.TestFailedStatus
			savedResults4[i] = result
		}
		svc.store.appendResults(task4.TaskID, task4.Execution, savedResults4)

		t.Run("FailingTests", func(t *testing.T) {
			taskOpts := []TaskOptions{task3, task4, emptyTask}
			sample, err := svc.GetMergedFailedTestSample(context.Background(), taskOpts)
			require.NoError(t, err)

			require.Len(t, sample, maxSampleSize)
			for i := 0; i < maxSampleSize/2; i++ {
				require.Equal(t, savedResults3[2*i].GetDisplayTestName(), sample[i])
			}
			for i := maxSampleSize / 2; i < maxSampleSize; i++ {
				require.Equal(t, savedResults4[i-maxSampleSize/2].GetDisplayTestName(), sample[i])
			}
		})
		t.Run("NoFailingTests", func(t *testing.T) {
			taskOpts := []TaskOptions{task1, task2}
			sample, err := svc.GetMergedFailedTestSample(context.Background(), taskOpts)
			require.NoError(t, err)
			assert.Nil(t, sample)
		})
	})
}

func TestInMemFilterAndSortTestResults(t *testing.T) {
	svc := newInMemService(newInMemStore())

	getResults := func() []TestResult {
		return []TestResult{
			{
				TestName: "A test",
				Status:   "Pass",
				Start:    time.Date(1996, time.August, 31, 12, 5, 10, 1, time.UTC),
				End:      time.Date(1996, time.August, 31, 12, 5, 12, 0, time.UTC),
			},
			{
				TestName:        "B test",
				DisplayTestName: "Display",
				Status:          "Fail",
				Start:           time.Date(1996, time.August, 31, 12, 5, 10, 3, time.UTC),
				End:             time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
			},
			{
				TestName: "C test",
				Status:   "Fail",
				Start:    time.Date(1996, time.August, 31, 12, 5, 10, 2, time.UTC),
				End:      time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
			},
			{
				TestName: "D test",
				Status:   "Pass",
				Start:    time.Date(1996, time.August, 31, 12, 5, 10, 4, time.UTC),
				End:      time.Date(1996, time.August, 31, 12, 5, 11, 0, time.UTC),
				GroupID:  "llama",
			},
		}
	}
	results := getResults()

	baseTaskID := "base_task"
	baseResults := []TestResult{
		{
			TestName: "A test",
			Status:   "Pass",
		},
		{
			TestName:        "B test",
			DisplayTestName: "Display",
			Status:          "Fail",
		},
		{
			TestName: "C test",
			Status:   "Pass",
		},
		{
			TestName: "D test",
			Status:   "Fail",
		},
	}
	svc.store.appendResults(baseTaskID, 0, baseResults)
	resultsWithBaseStatus := getResults()
	require.Len(t, resultsWithBaseStatus, len(baseResults))
	for i := range resultsWithBaseStatus {
		resultsWithBaseStatus[i].BaseStatus = baseResults[i].Status
	}

	for _, test := range []struct {
		name            string
		opts            *FilterOptions
		expectedResults []TestResult
		expectedCount   int
		hasErr          bool
	}{
		{
			name:   "InvalidSortBy",
			opts:   &FilterOptions{SortBy: "invalid"},
			hasErr: true,
		},
		{
			name:   "NegativeLimit",
			opts:   &FilterOptions{Limit: -1},
			hasErr: true,
		},
		{

			name: "NegativePage",
			opts: &FilterOptions{
				Limit: 1,
				Page:  -1,
			},
			hasErr: true,
		},
		{
			name:   "PageWithoutLimit",
			opts:   &FilterOptions{Page: 1},
			hasErr: true,
		},
		{
			name:   "InvalidTestNameRegex",
			opts:   &FilterOptions{TestName: "*"},
			hasErr: true,
		},
		{
			name:            "EmptyOptions",
			expectedResults: results,
			expectedCount:   4,
		},
		{
			name:            "TestNameExactMatchFilter",
			opts:            &FilterOptions{TestName: "A test"},
			expectedResults: results[0:1],
			expectedCount:   1,
		},
		{
			name: "TestNameRegexFilter",
			opts: &FilterOptions{TestName: "A|C"},
			expectedResults: []TestResult{
				results[0],
				results[2],
			},
			expectedCount: 2,
		},
		{
			name:            "DisplayTestNameFilter",
			opts:            &FilterOptions{TestName: "Display"},
			expectedResults: results[1:2],
			expectedCount:   1,
		},
		{
			name:            "StatusFilter",
			opts:            &FilterOptions{Statuses: []string{"Fail"}},
			expectedResults: results[1:3],
			expectedCount:   2,
		},
		{
			name:            "GroupIDFilter",
			opts:            &FilterOptions{GroupID: "llama"},
			expectedResults: results[3:4],
			expectedCount:   1,
		},
		{
			name: "SortByDurationASC",
			opts: &FilterOptions{SortBy: SortByDuration},
			expectedResults: []TestResult{
				results[3],
				results[0],
				results[2],
				results[1],
			},
			expectedCount: 4,
		},
		{
			name: "SortByDurationDSC",
			opts: &FilterOptions{
				SortBy:       SortByDuration,
				SortOrderDSC: true,
			},
			expectedResults: []TestResult{
				results[1],
				results[2],
				results[0],
				results[3],
			},
			expectedCount: 4,
		},
		{
			name: "SortByTestNameASC",
			opts: &FilterOptions{SortBy: SortByTestName},
			expectedResults: []TestResult{
				results[0],
				results[2],
				results[3],
				results[1],
			},
			expectedCount: 4,
		},
		{
			name: "SortByTestNameDCS",
			opts: &FilterOptions{
				SortBy:       SortByTestName,
				SortOrderDSC: true,
			},
			expectedResults: []TestResult{
				results[1],
				results[3],
				results[2],
				results[0],
			},
			expectedCount: 4,
		},
		{
			name: "SortByStatusASC",
			opts: &FilterOptions{SortBy: SortByStatus},
			expectedResults: []TestResult{
				results[1],
				results[2],
				results[0],
				results[3],
			},
			expectedCount: 4,
		},
		{
			name: "SortByStatusDSC",
			opts: &FilterOptions{
				SortBy:       SortByStatus,
				SortOrderDSC: true,
			},
			expectedResults: []TestResult{
				results[0],
				results[3],
				results[1],
				results[2],
			},
			expectedCount: 4,
		},
		{
			name: "SortByStartTimeASC",
			opts: &FilterOptions{SortBy: SortByStart},
			expectedResults: []TestResult{
				results[0],
				results[2],
				results[1],
				results[3],
			},
			expectedCount: 4,
		},
		{
			name: "SortByStartTimeDCS",
			opts: &FilterOptions{
				SortBy:       SortByStart,
				SortOrderDSC: true,
			},
			expectedResults: []TestResult{
				results[3],
				results[1],
				results[2],
				results[0],
			},
			expectedCount: 4,
		},
		{
			name: "SortByBaseStatusASC",
			opts: &FilterOptions{
				SortBy:    SortByBaseStatus,
				BaseTasks: []TaskOptions{{TaskID: baseTaskID}},
			},
			expectedResults: []TestResult{
				resultsWithBaseStatus[1],
				resultsWithBaseStatus[3],
				resultsWithBaseStatus[0],
				resultsWithBaseStatus[2],
			},
			expectedCount: 4,
		},
		{
			name: "SortByBaseStatusDSC",
			opts: &FilterOptions{
				SortBy:       SortByBaseStatus,
				SortOrderDSC: true,
				BaseTasks:    []TaskOptions{{TaskID: baseTaskID}},
			},
			expectedResults: []TestResult{
				resultsWithBaseStatus[0],
				resultsWithBaseStatus[2],
				resultsWithBaseStatus[1],
				resultsWithBaseStatus[3],
			},
			expectedCount: 4,
		},
		{
			name: "BaseStatus",
			opts: &FilterOptions{BaseTasks: []TaskOptions{{TaskID: baseTaskID}}},
			expectedResults: []TestResult{
				resultsWithBaseStatus[0],
				resultsWithBaseStatus[1],
				resultsWithBaseStatus[2],
				resultsWithBaseStatus[3],
			},
			expectedCount: 4,
		},
		{
			name:            "Limit",
			opts:            &FilterOptions{Limit: 3},
			expectedResults: results[0:3],
			expectedCount:   4,
		},
		{
			name: "LimitAndPage",
			opts: &FilterOptions{
				Limit: 3,
				Page:  1,
			},
			expectedResults: results[3:],
			expectedCount:   4,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			actualResults, count, err := svc.filterAndSortTestResults(getResults(), test.opts)
			if test.hasErr {
				assert.Nil(t, actualResults)
				assert.Zero(t, count)
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectedResults, actualResults)
				assert.Equal(t, test.expectedCount, count)
			}
		})
	}
}

func getTestResult() TestResult {
	result := TestResult{
		TestName: utility.RandomString(),
		Status:   evergreen.TestSucceededStatus,
		Start:    time.Now().Add(-30 * time.Hour).UTC().Round(time.Millisecond),
		End:      time.Now().UTC().Round(time.Millisecond),
	}
	// Optional fields, we should test that we handle them properly when
	// they are populated and when they do not.
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
