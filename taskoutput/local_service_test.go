package taskoutput

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/sometimes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const MaxSampleSize = 10

var output = TestResultOutput{
	Version: 1,
}

var outputCedar = TestResultOutput{
	Version: 0,
}

func TestLocalService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	svc := testresult.NewLocalService(env)
	require.NoError(t, testresult.ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, testresult.ClearLocal(ctx, env))
	}()

	task0 := TaskOptions{TaskID: "task0", Execution: 0, ResultsService: testresult.TestResultsServiceLocal}
	savedResults0 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults0); i++ {
		result := getTestResult()
		result.TaskID = task0.TaskID
		result.Execution = task0.Execution
		if i%2 != 0 {
			result.Status = evergreen.TestFailedStatus
		}
		savedResults0[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults0))

	task1 := TaskOptions{TaskID: "task1", Execution: 0, ResultsService: testresult.TestResultsServiceLocal}
	savedResults1 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults1))
	task2 := TaskOptions{TaskID: "task2", Execution: 1, ResultsService: testresult.TestResultsServiceLocal}
	savedResults2 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults2); i++ {
		result := getTestResult()
		result.TaskID = task2.TaskID
		result.Execution = task2.Execution
		savedResults2[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults2))
	task3 := TaskOptions{TaskID: "task3", Execution: 0, ResultsService: testresult.TestResultsServiceLocal}
	savedResults3 := make([]testresult.TestResult, MaxSampleSize)
	for i := 0; i < len(savedResults3); i++ {
		result := getTestResult()
		result.TaskID = task3.TaskID
		result.Execution = task3.Execution
		if i%2 == 0 {
			result.Status = evergreen.TestFailedStatus
		}
		savedResults3[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults3))
	task4 := TaskOptions{TaskID: "task4", Execution: 1, ResultsService: testresult.TestResultsServiceLocal}
	savedResults4 := make([]testresult.TestResult, MaxSampleSize)
	for i := 0; i < len(savedResults3); i++ {
		result := getTestResult()
		result.TaskID = task4.TaskID
		result.Execution = task4.Execution
		result.Status = evergreen.TestFailedStatus
		savedResults4[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults4))
	emptyTask := TaskOptions{TaskID: "DNE", Execution: 0, ResultsService: testresult.TestResultsServiceLocal}

	t.Run("GetMergedTaskTestResults", func(t *testing.T) {
		t.Run("WithoutFilterAndSortOpts", func(t *testing.T) {
			taskOpts := []TaskOptions{task1, task2, task0, emptyTask}
			taskResults, err := output.GetMergedTaskTestResults(ctx, env, taskOpts, nil)
			require.NoError(t, err)

			assert.Equal(t, len(taskResults.Results), taskResults.Stats.TotalCount)
			assert.Equal(t, len(savedResults0)/2, taskResults.Stats.FailedCount)
			assert.Equal(t, len(taskResults.Results), utility.FromIntPtr(taskResults.Stats.FilteredCount))
			expectedResults := append(append(append([]testresult.TestResult{}, savedResults0...), savedResults1...), savedResults2...)
			assert.Equal(t, expectedResults, taskResults.Results)
		})
		t.Run("WithFilterAndSortOpts", func(t *testing.T) {
			taskOpts := []TaskOptions{task0}
			filterOpts := &testresult.FilterOptions{Statuses: []string{evergreen.TestSucceededStatus}}
			taskResults, err := output.GetMergedTaskTestResults(ctx, env, taskOpts, filterOpts)
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
	t.Run("GetTaskTestResultsStats", func(t *testing.T) {
		taskOpts := []TaskOptions{task1, task2, task0, emptyTask}
		stats, err := output.GetTaskTestResultsStats(ctx, env, taskOpts)
		require.NoError(t, err)

		assert.Equal(t, len(savedResults0)+len(savedResults1)+len(savedResults2), stats.TotalCount)
		assert.Equal(t, len(savedResults0)/2, stats.FailedCount)
		assert.Nil(t, stats.FilteredCount)
	})
	t.Run("GetFailedTestSamples", func(t *testing.T) {
		t.Run("WithoutRegexFilters", func(t *testing.T) {
			taskOpts := []TaskOptions{task3, task4, emptyTask}
			samples, err := GetFailedTestSamples(ctx, env, taskOpts, nil)
			require.NoError(t, err)

			expectedSamples := []testresult.TaskTestResultsFailedSample{
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
			samples, err := GetFailedTestSamples(ctx, env, taskOpts, regexFilters)
			require.NoError(t, err)

			expectedSamples := []testresult.TaskTestResultsFailedSample{
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
			samples, err := GetFailedTestSamples(ctx, env, taskOpts, regexFilters)
			require.NoError(t, err)

			expectedSamples := []testresult.TaskTestResultsFailedSample{
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
	svc := testresult.NewLocalService(env)
	defer func() {
		assert.NoError(t, testresult.ClearLocal(ctx, env))
	}()

	getResults := func() []testresult.TestResult {
		return []testresult.TestResult{
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
				TestName:        "C test",
				DisplayTestName: "B",
				Status:          "Fail",
				TestStartTime:   time.Date(1996, time.August, 31, 12, 5, 10, 2, time.UTC),
				TestEndTime:     time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
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
	baseResults := []testresult.TestResult{
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
			TaskID:          baseTaskID,
			TestName:        "C test",
			DisplayTestName: "B",
			Status:          "Pass",
		},
		{
			TaskID:   baseTaskID,
			TestName: "D test",
			Status:   "Fail",
		},
	}
	require.NoError(t, svc.AppendTestResults(ctx, baseResults))
	resultsWithBaseStatus := getResults()
	require.Len(t, resultsWithBaseStatus, len(baseResults))
	for i := range resultsWithBaseStatus {
		resultsWithBaseStatus[i].BaseStatus = baseResults[i].Status
	}

	for _, test := range []struct {
		name            string
		opts            *testresult.FilterOptions
		expectedResults []testresult.TestResult
		expectedCount   int
		hasErr          bool
	}{
		{
			name: "InvalidSortByKey",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{
					{Key: testresult.SortByTestNameKey},
					{Key: "invalid"},
				},
			},
			hasErr: true,
		},
		{
			name: "DuplicateSortByKey",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{
					{Key: testresult.SortByTestNameKey},
					{Key: testresult.SortByStatusKey},
					{Key: testresult.SortByTestNameKey},
				},
			},
			hasErr: true,
		},
		{
			name: "SortByBaseStatusWithoutBaseTasksFindOptions",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{{Key: testresult.SortByBaseStatusKey}},
			},
			hasErr: true,
		},
		{
			name:   "NegativeLimit",
			opts:   &testresult.FilterOptions{Limit: -1},
			hasErr: true,
		},
		{

			name: "NegativePage",
			opts: &testresult.FilterOptions{
				Limit: 1,
				Page:  -1,
			},
			hasErr: true,
		},
		{
			name:   "PageWithoutLimit",
			opts:   &testresult.FilterOptions{Page: 1},
			hasErr: true,
		},
		{
			name:   "InvalidTestNameRegex",
			opts:   &testresult.FilterOptions{TestName: "*"},
			hasErr: true,
		},
		{
			name:            "EmptyOptions",
			expectedResults: results,
			expectedCount:   4,
		},
		{
			name: "TestNameRegexFilter",
			opts: &testresult.FilterOptions{TestName: "A|B"},
			expectedResults: []testresult.TestResult{
				results[0],
				results[2],
			},
			expectedCount: 2,
		},
		{
			name: "TestNameExcludeDisplayNamesFilter",
			opts: &testresult.FilterOptions{
				TestName:            "B",
				ExcludeDisplayNames: true,
			},
			expectedResults: results[1:2],
			expectedCount:   1,
		},
		{
			name:            "DisplayTestNameFilter",
			opts:            &testresult.FilterOptions{TestName: "Display"},
			expectedResults: results[1:2],
			expectedCount:   1,
		},
		{
			name:            "StatusFilter",
			opts:            &testresult.FilterOptions{Statuses: []string{"Fail"}},
			expectedResults: results[1:3],
			expectedCount:   2,
		},
		{
			name:            "GroupIDFilter",
			opts:            &testresult.FilterOptions{GroupID: "llama"},
			expectedResults: results[3:4],
			expectedCount:   1,
		},
		{
			name: "SortByDurationASC",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{{Key: testresult.SortByDurationKey}},
			},
			expectedResults: []testresult.TestResult{
				results[3],
				results[0],
				results[2],
				results[1],
			},
			expectedCount: 4,
		},
		{
			name: "SortByDurationDSC",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{
					{
						Key:      testresult.SortByDurationKey,
						OrderDSC: true,
					},
				},
			},
			expectedResults: []testresult.TestResult{
				results[1],
				results[2],
				results[0],
				results[3],
			},
			expectedCount: 4,
		},
		{
			name: "SortByTestNameASC",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{{Key: testresult.SortByTestNameKey}},
			},
			expectedResults: []testresult.TestResult{
				results[0],
				results[2],
				results[3],
				results[1],
			},
			expectedCount: 4,
		},
		{
			name: "SortByTestNameDCS",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{
					{
						Key:      testresult.SortByTestNameKey,
						OrderDSC: true,
					},
				},
			},
			expectedResults: []testresult.TestResult{
				results[1],
				results[3],
				results[2],
				results[0],
			},
			expectedCount: 4,
		},
		{
			name: "SortByStatusASC",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{{Key: testresult.SortByStatusKey}},
			},
			expectedResults: []testresult.TestResult{
				results[1],
				results[2],
				results[0],
				results[3],
			},
			expectedCount: 4,
		},
		{
			name: "SortByStatusDSC",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{
					{
						Key:      testresult.SortByStatusKey,
						OrderDSC: true,
					},
				},
			},
			expectedResults: []testresult.TestResult{
				results[0],
				results[3],
				results[1],
				results[2],
			},
			expectedCount: 4,
		},
		{
			name: "SortByStartTimeASC",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{{Key: testresult.SortByStartKey}},
			},
			expectedResults: []testresult.TestResult{
				results[0],
				results[2],
				results[1],
				results[3],
			},
			expectedCount: 4,
		},
		{
			name: "SortByStartTimeDSC",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{
					{
						Key:      testresult.SortByStartKey,
						OrderDSC: true,
					},
				},
			},
			expectedResults: []testresult.TestResult{
				results[3],
				results[1],
				results[2],
				results[0],
			},
			expectedCount: 4,
		},
		{
			name: "SortByBaseStatusASC",
			opts: &testresult.FilterOptions{
				Sort:      []testresult.SortBy{{Key: testresult.SortByBaseStatusKey}},
				BaseTasks: []testresult.TaskOptions{{TaskID: baseTaskID}},
			},
			expectedResults: []testresult.TestResult{
				resultsWithBaseStatus[1],
				resultsWithBaseStatus[3],
				resultsWithBaseStatus[0],
				resultsWithBaseStatus[2],
			},
			expectedCount: 4,
		},
		{
			name: "SortByBaseStatusDSC",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{
					{
						Key:      testresult.SortByBaseStatusKey,
						OrderDSC: true,
					},
				},
				BaseTasks: []testresult.TaskOptions{{TaskID: baseTaskID}},
			},
			expectedResults: []testresult.TestResult{
				resultsWithBaseStatus[0],
				resultsWithBaseStatus[2],
				resultsWithBaseStatus[1],
				resultsWithBaseStatus[3],
			},
			expectedCount: 4,
		},
		{
			name: "MultiSort",
			opts: &testresult.FilterOptions{
				Sort: []testresult.SortBy{
					{
						Key: testresult.SortByStatusKey,
					},
					{
						Key:      testresult.SortByTestNameKey,
						OrderDSC: true,
					},
				},
			},
			expectedResults: []testresult.TestResult{
				results[1],
				results[2],
				results[3],
				results[0],
			},
			expectedCount: 4,
		},
		{
			name: "BaseStatus",
			opts: &testresult.FilterOptions{BaseTasks: []testresult.TaskOptions{{TaskID: baseTaskID}}},
			expectedResults: []testresult.TestResult{
				resultsWithBaseStatus[0],
				resultsWithBaseStatus[1],
				resultsWithBaseStatus[2],
				resultsWithBaseStatus[3],
			},
			expectedCount: 4,
		},
		{
			name:            "Limit",
			opts:            &testresult.FilterOptions{Limit: 3},
			expectedResults: results[0:3],
			expectedCount:   4,
		},
		{
			name: "LimitAndPage",
			opts: &testresult.FilterOptions{
				Limit: 3,
				Page:  1,
			},
			expectedResults: results[3:],
			expectedCount:   4,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			actualResults, count, err := output.filterAndSortTestResults(ctx, env, getResults(), test.opts)
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

func getTestResult() testresult.TestResult {
	result := testresult.TestResult{
		TestName:      utility.RandomString(),
		Status:        evergreen.TestSucceededStatus,
		TestStartTime: time.Now().Add(-30 * time.Hour).UTC().Round(time.Millisecond),
		TestEndTime:   time.Now().UTC().Round(time.Millisecond),
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
