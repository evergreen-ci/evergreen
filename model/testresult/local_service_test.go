package testresult

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	svc := newLocalService(env)
	require.NoError(t, ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, ClearLocal(ctx, env))
	}()

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
	require.NoError(t, InsertLocal(ctx, env, savedResults0...))

	task1 := TaskOptions{TaskID: "task1", Execution: 0}
	savedResults1 := make([]TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	require.NoError(t, InsertLocal(ctx, env, savedResults1...))

	task2 := TaskOptions{TaskID: "task2", Execution: 1}
	savedResults2 := make([]TestResult, 10)
	for i := 0; i < len(savedResults2); i++ {
		result := getTestResult()
		result.TaskID = task2.TaskID
		result.Execution = task2.Execution
		savedResults2[i] = result
	}
	require.NoError(t, InsertLocal(ctx, env, savedResults2...))

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
	require.NoError(t, InsertLocal(ctx, env, savedResults3...))

	task4 := TaskOptions{TaskID: "task4", Execution: 1}
	savedResults4 := make([]TestResult, maxSampleSize)
	for i := 0; i < len(savedResults3); i++ {
		result := getTestResult()
		result.TaskID = task4.TaskID
		result.Execution = task4.Execution
		result.Status = evergreen.TestFailedStatus
		savedResults4[i] = result
	}
	require.NoError(t, InsertLocal(ctx, env, savedResults4...))

	emptyTask := TaskOptions{TaskID: "DNE", Execution: 0}

	t.Run("GetMergedTaskTestResults", func(t *testing.T) {
		t.Run("WithoutFilterAndSortOpts", func(t *testing.T) {
			taskOpts := []TaskOptions{task1, task2, task0, emptyTask}
			taskResults, err := svc.GetMergedTaskTestResults(ctx, taskOpts, nil)
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
			taskResults, err := svc.GetMergedTaskTestResults(ctx, taskOpts, filterOpts)
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
		stats, err := svc.GetMergedTaskTestResultsStats(ctx, taskOpts)
		require.NoError(t, err)

		assert.Equal(t, len(savedResults0)+len(savedResults1)+len(savedResults2), stats.TotalCount)
		assert.Equal(t, len(savedResults0)/2, stats.FailedCount)
		assert.Nil(t, stats.FilteredCount)
	})
	t.Run("GetMergedFailedTestSample", func(t *testing.T) {
		t.Run("FailingTests", func(t *testing.T) {
			taskOpts := []TaskOptions{task3, task4, emptyTask}
			sample, err := svc.GetMergedFailedTestSample(ctx, taskOpts)
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
			sample, err := svc.GetMergedFailedTestSample(ctx, taskOpts)
			require.NoError(t, err)
			assert.Nil(t, sample)
		})
	})
	t.Run("GetFailedTestSamples", func(t *testing.T) {
		t.Run("WithoutRegexFilters", func(t *testing.T) {
			taskOpts := []TaskOptions{task3, task4, emptyTask}
			samples, err := svc.GetFailedTestSamples(ctx, taskOpts, nil)
			require.NoError(t, err)

			expectedSamples := []TaskTestResultsFailedSample{
				{
					TaskID:    task3.TaskID,
					Execution: task3.Execution,
					MatchingFailedTestNames: func() []string {
						sample := make([]string, len(savedResults3)/2)
						for i := 0; i < len(savedResults3)/2; i++ {
							sample[i] = savedResults3[2*i].GetDisplayTestName()
						}

						return sample
					}(),
					TotalFailedNames: len(savedResults3) / 2,
				},
				{
					TaskID:    task4.TaskID,
					Execution: task4.Execution,
					MatchingFailedTestNames: func() []string {
						sample := make([]string, len(savedResults4))
						for i := 0; i < len(savedResults4); i++ {
							sample[i] = savedResults4[i].GetDisplayTestName()
						}

						return sample
					}(),
					TotalFailedNames: len(savedResults4),
				},
			}
			assert.ElementsMatch(t, expectedSamples, samples)
		})
		t.Run("WithRegexFiltersMatch", func(t *testing.T) {
			taskOpts := []TaskOptions{task3, task4}
			regexFilters := []string{savedResults4[0].GetDisplayTestName(), savedResults4[1].GetDisplayTestName()}
			samples, err := svc.GetFailedTestSamples(ctx, taskOpts, regexFilters)
			require.NoError(t, err)

			expectedSamples := []TaskTestResultsFailedSample{
				{
					TaskID:                  task3.TaskID,
					Execution:               task3.Execution,
					MatchingFailedTestNames: nil,
					TotalFailedNames:        len(savedResults3) / 2,
				},
				{
					TaskID:                  task4.TaskID,
					Execution:               task4.Execution,
					MatchingFailedTestNames: regexFilters,
					TotalFailedNames:        len(savedResults4),
				},
			}
			assert.ElementsMatch(t, expectedSamples, samples)
		})
		t.Run("WithRegexFiltersNoMatch", func(t *testing.T) {
			taskOpts := []TaskOptions{task3, task4}
			regexFilters := []string{"DNE"}
			samples, err := svc.GetFailedTestSamples(ctx, taskOpts, regexFilters)
			require.NoError(t, err)

			expectedSamples := []TaskTestResultsFailedSample{
				{
					TaskID:                  task3.TaskID,
					Execution:               task3.Execution,
					MatchingFailedTestNames: nil,
					TotalFailedNames:        len(savedResults3) / 2,
				},
				{
					TaskID:                  task4.TaskID,
					Execution:               task4.Execution,
					MatchingFailedTestNames: nil,
					TotalFailedNames:        len(savedResults4),
				},
			}
			assert.ElementsMatch(t, expectedSamples, samples)
		})
	})
}

func TestLocalFilterAndSortTestResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	svc := newLocalService(env)
	defer func() {
		assert.NoError(t, ClearLocal(ctx, env))
	}()

	getResults := func() []TestResult {
		return []TestResult{
			{
				TestName:      "A test",
				Status:        "Pass",
				TestStartTime: time.Date(1996, time.August, 31, 12, 5, 10, 1, time.UTC),
				TestEndTime:   time.Date(1996, time.August, 31, 12, 5, 12, 0, time.UTC),
			},
			{
				TestName:        "B test",
				DisplayTestName: "Display",
				Status:          "Fail",
				TestStartTime:   time.Date(1996, time.August, 31, 12, 5, 10, 3, time.UTC),
				TestEndTime:     time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
			},
			{
				TestName:      "C test",
				Status:        "Fail",
				TestStartTime: time.Date(1996, time.August, 31, 12, 5, 10, 2, time.UTC),
				TestEndTime:   time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
			},
			{
				TestName:      "D test",
				Status:        "Pass",
				TestStartTime: time.Date(1996, time.August, 31, 12, 5, 10, 4, time.UTC),
				TestEndTime:   time.Date(1996, time.August, 31, 12, 5, 11, 0, time.UTC),
				GroupID:       "llama",
			},
		}
	}
	results := getResults()

	baseTaskID := "base_task"
	baseResults := []TestResult{
		{
			TaskID:   baseTaskID,
			TestName: "A test",
			Status:   "Pass",
		},
		{
			TaskID:          baseTaskID,
			TestName:        "B test",
			DisplayTestName: "Display",
			Status:          "Fail",
		},
		{
			TaskID:   baseTaskID,
			TestName: "C test",
			Status:   "Pass",
		},
		{
			TaskID:   baseTaskID,
			TestName: "D test",
			Status:   "Fail",
		},
	}
	require.NoError(t, InsertLocal(ctx, env, baseResults...))
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
			actualResults, count, err := svc.filterAndSortTestResults(ctx, getResults(), test.opts)
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
