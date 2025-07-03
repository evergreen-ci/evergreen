package task

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/testresult"
	resultTestutil "github.com/evergreen-ci/evergreen/model/testresult/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	goparquet "github.com/fraugster/parquet-go"
	"github.com/fraugster/parquet-go/floor"
	"github.com/mongodb/grip/sometimes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { testutil.Setup() }

var output = TaskOutput{
	TestResults: TestResultOutput{
		Version: TestResultServiceEvergreen,
		BucketConfig: evergreen.BucketConfig{
			Type:              evergreen.BucketTypeLocal,
			TestResultsPrefix: "test-results",
		},
	},
}

func TestEvergreenService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	svc := NewTestResultService(env)
	require.NoError(t, ClearTestResults(ctx, env))
	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, ClearTestResults(ctx, env))
		assert.NoError(t, db.Clear(Collection))
	}()

	output.TestResults.BucketConfig.Name = t.TempDir()
	testBucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: output.TestResults.BucketConfig.Name})
	require.NoError(t, err)

	task0 := Task{Id: "task0", Execution: 0, TaskOutputInfo: &output}
	task1 := Task{Id: "task1", Execution: 0, TaskOutputInfo: &output}
	task2 := Task{Id: "task2", Execution: 1, TaskOutputInfo: &output}
	task3 := Task{Id: "task3", Execution: 0, TaskOutputInfo: &output}
	task4 := Task{Id: "task4", Execution: 1, TaskOutputInfo: &output}
	emptyTask := Task{Id: "DNE", Execution: 0, TaskOutputInfo: &output}
	require.NoError(t, db.InsertMany(t.Context(), Collection, task0, task1, task2, task3, task4))

	savedResults0 := saveTestResults(t, ctx, testBucket, svc, &task0, 10)
	savedResults1 := saveTestResults(t, ctx, testBucket, svc, &task1, 10)
	savedResults2 := saveTestResults(t, ctx, testBucket, svc, &task2, 10)
	savedResults3 := saveTestResults(t, ctx, testBucket, svc, &task3, 10)
	savedResults4 := saveTestResults(t, ctx, testBucket, svc, &task4, 10)

	t.Run("GetMergedTaskTestResults", func(t *testing.T) {
		t.Run("WithoutFilterAndSortOpts", func(t *testing.T) {
			taskOpts := []Task{task1, task2, task0, emptyTask}
			taskResults, err := getMergedTaskTestResults(ctx, env, taskOpts, nil)
			require.NoError(t, err)

			assert.Equal(t, len(taskResults.Results), taskResults.Stats.TotalCount)
			assert.Equal(t, len(savedResults0)/2, taskResults.Stats.FailedCount)
			assert.Equal(t, len(taskResults.Results), utility.FromIntPtr(taskResults.Stats.FilteredCount))
			expectedResults := append(append(append([]testresult.TestResult{}, savedResults0...), savedResults1...), savedResults2...)
			assert.Equal(t, expectedResults, taskResults.Results)
		})
		t.Run("WithFilterAndSortOpts", func(t *testing.T) {
			taskOpts := []Task{task0}
			filterOpts := &FilterOptions{Statuses: []string{evergreen.TestSucceededStatus}}
			taskResults, err := getMergedTaskTestResults(ctx, env, taskOpts, filterOpts)
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
		taskOpts := []Task{task1, task2, task0, emptyTask}
		stats, err := getTaskTestResultsStats(ctx, env, taskOpts)
		require.NoError(t, err)

		assert.Equal(t, len(savedResults0)+len(savedResults1)+len(savedResults2), stats.TotalCount)
		assert.Equal(t, len(savedResults0)/2, stats.FailedCount)
		assert.Nil(t, stats.FilteredCount)
	})
	t.Run("GetFailedTestSamples", func(t *testing.T) {
		t.Run("WithoutRegexFilters", func(t *testing.T) {
			taskOpts := []Task{task3, task4, emptyTask}
			samples, err := GetFailedTestSamples(ctx, env, taskOpts, nil)
			require.NoError(t, err)

			expectedSamples := []testresult.TaskTestResultsFailedSample{
				{
					TaskID:    task3.Id,
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
					TaskID:    task4.Id,
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
			taskOpts := []Task{task3, task4}
			regexFilters := []string{savedResults4[0].GetDisplayTestName(), savedResults4[1].GetDisplayTestName()}
			samples, err := GetFailedTestSamples(ctx, env, taskOpts, regexFilters)
			require.NoError(t, err)

			expectedSamples := []testresult.TaskTestResultsFailedSample{
				{
					TaskID:                  task3.Id,
					Execution:               task3.Execution,
					MatchingFailedTestNames: nil,
					TotalFailedNames:        len(savedResults3) / 2,
				},
				{
					TaskID:                  task4.Id,
					Execution:               task4.Execution,
					MatchingFailedTestNames: regexFilters,
					TotalFailedNames:        len(savedResults4),
				},
			}
			assert.ElementsMatch(t, expectedSamples, samples)
		})
		t.Run("WithRegexFiltersNoMatch", func(t *testing.T) {
			taskOpts := []Task{task3, task4}
			regexFilters := []string{"DNE"}
			samples, err := GetFailedTestSamples(ctx, env, taskOpts, regexFilters)
			require.NoError(t, err)

			expectedSamples := []testresult.TaskTestResultsFailedSample{
				{
					TaskID:                  task3.Id,
					Execution:               task3.Execution,
					MatchingFailedTestNames: nil,
					TotalFailedNames:        len(savedResults3) / 2,
				},
				{
					TaskID:                  task4.Id,
					Execution:               task4.Execution,
					MatchingFailedTestNames: nil,
					TotalFailedNames:        len(savedResults4),
				},
			}
			assert.ElementsMatch(t, expectedSamples, samples)
		})
	})
}

func TestEvergreenFilterAndSortTestResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	svc := NewTestResultService(env)
	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, ClearTestResults(ctx, env))
		require.NoError(t, db.Clear(Collection))
	}()

	getResults := func() []testresult.TestResult {
		return []testresult.TestResult{
			{
				TaskID:        "task1",
				TestName:      "A test",
				Status:        "Pass",
				TestStartTime: time.Date(1996, time.August, 31, 12, 5, 10, 1, time.UTC),
				TestEndTime:   time.Date(1996, time.August, 31, 12, 5, 12, 0, time.UTC),
			},
			{
				TaskID:          "task1",
				TestName:        "B test",
				DisplayTestName: "Display",
				Status:          "Fail",
				TestStartTime:   time.Date(1996, time.August, 31, 12, 5, 10, 3, time.UTC),
				TestEndTime:     time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
			},
			{
				TaskID:          "task1",
				TestName:        "C test",
				DisplayTestName: "B",
				Status:          "Fail",
				TestStartTime:   time.Date(1996, time.August, 31, 12, 5, 10, 2, time.UTC),
				TestEndTime:     time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
			},
			{
				TaskID:        "task1",
				TestName:      "D test",
				Status:        "Pass",
				TestStartTime: time.Date(1996, time.August, 31, 12, 5, 10, 4, time.UTC),
				TestEndTime:   time.Date(1996, time.August, 31, 12, 5, 11, 0, time.UTC),
				GroupID:       "llama",
			},
		}
	}

	output.TestResults.BucketConfig.Name = t.TempDir()
	baseTask := Task{
		Id:             "base_task",
		TaskOutputInfo: &output,
	}
	task1 := Task{
		Id:             "task1",
		TaskOutputInfo: &output,
	}
	require.NoError(t, baseTask.Insert(ctx))
	require.NoError(t, task1.Insert(ctx))

	results := getResults()
	tr := getTestResults()
	tr.Info.TaskID = baseTask.Id
	tr.Info.Execution = baseTask.Execution

	testBucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: output.TestResults.BucketConfig.Name})
	require.NoError(t, err)
	w, err := testBucket.Writer(ctx, fmt.Sprintf("%s/%s", output.TestResults.BucketConfig.TestResultsPrefix, testresult.PartitionKey(tr.CreatedAt, tr.Info.Project, tr.ID)))
	require.NoError(t, err)
	defer func() { assert.NoError(t, w.Close()) }()
	pw := floor.NewWriter(goparquet.NewFileWriter(w, goparquet.WithSchemaDefinition(ParquetTestResultsSchemaDef)))

	baseResults := []testresult.TestResult{
		{
			TaskID:   baseTask.Id,
			TestName: "A test",
			Status:   "Pass",
		},
		{
			TaskID:          baseTask.Id,
			TestName:        "B test",
			DisplayTestName: "Display",
			Status:          "Fail",
		},
		{
			TaskID:          baseTask.Id,
			TestName:        "C test",
			DisplayTestName: "B",
			Status:          "Pass",
		},
		{
			TaskID:   baseTask.Id,
			TestName: "D test",
			Status:   "Fail",
		},
	}

	savedParquet := testresult.ParquetTestResults{
		Version:   tr.Info.Version,
		Variant:   tr.Info.Variant,
		TaskName:  tr.Info.TaskName,
		TaskID:    tr.Info.TaskID,
		Execution: int32(tr.Info.Execution),
		Requester: tr.Info.Requester,
		CreatedAt: tr.CreatedAt.UTC(),
		Results:   make([]testresult.ParquetTestResult, 4),
	}

	for i := 0; i < len(baseResults); i++ {
		savedParquet.Results[i] = testresult.ParquetTestResult{
			TestName:       baseResults[i].TestName,
			GroupID:        utility.ToStringPtr(baseResults[i].GroupID),
			Status:         baseResults[i].Status,
			LogInfo:        baseResults[i].LogInfo,
			TaskCreateTime: baseResults[i].TaskCreateTime.UTC(),
			TestStartTime:  baseResults[i].TestStartTime.UTC(),
			TestEndTime:    baseResults[i].TestEndTime.UTC(),
		}
		if baseResults[i].DisplayTestName != "" {
			savedParquet.Results[i].DisplayTestName = utility.ToStringPtr(baseResults[i].DisplayTestName)
			savedParquet.Results[i].GroupID = utility.ToStringPtr(baseResults[i].GroupID)
			savedParquet.Results[i].LogTestName = utility.ToStringPtr(baseResults[i].LogTestName)
			savedParquet.Results[i].LogURL = utility.ToStringPtr(baseResults[i].LogURL)
			savedParquet.Results[i].RawLogURL = utility.ToStringPtr(baseResults[i].RawLogURL)
			savedParquet.Results[i].LineNum = utility.ToInt32Ptr(int32(baseResults[i].LineNum))
		}
	}

	require.NoError(t, pw.Write(savedParquet))
	require.NoError(t, pw.Close())
	require.NoError(t, db.Insert(ctx, testresult.Collection, tr))
	require.NoError(t, svc.AppendTestResultMetadata(resultTestutil.MakeAppendTestResultMetadataReq(ctx, baseResults, tr.ID)))

	resultsWithBaseStatus := getResults()
	require.Len(t, resultsWithBaseStatus, len(baseResults))
	for i := range resultsWithBaseStatus {
		resultsWithBaseStatus[i].BaseStatus = baseResults[i].Status
	}

	for _, test := range []struct {
		name            string
		opts            *FilterOptions
		expectedResults []testresult.TestResult
		expectedCount   int
		hasErr          bool
	}{
		{
			name: "InvalidSortByKey",
			opts: &FilterOptions{
				Sort: []testresult.SortBy{
					{Key: testresult.SortByTestNameKey},
					{Key: "invalid"},
				},
			},
			hasErr: true,
		},
		{
			name: "DuplicateSortByKey",
			opts: &FilterOptions{
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
			opts: &FilterOptions{
				Sort: []testresult.SortBy{{Key: testresult.SortByBaseStatusKey}},
			},
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
			name: "TestNameRegexFilter",
			opts: &FilterOptions{TestName: "A|B"},
			expectedResults: []testresult.TestResult{
				results[0],
				results[2],
			},
			expectedCount: 2,
		},
		{
			name: "TestNameExcludeDisplayNamesFilter",
			opts: &FilterOptions{
				TestName:            "B",
				ExcludeDisplayNames: true,
			},
			expectedResults: results[1:2],
			expectedCount:   1,
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
			opts: &FilterOptions{
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
			opts: &FilterOptions{
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
			opts: &FilterOptions{
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
			opts: &FilterOptions{
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
			opts: &FilterOptions{
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
			opts: &FilterOptions{
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
			opts: &FilterOptions{
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
			opts: &FilterOptions{
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
			opts: &FilterOptions{
				Sort:      []testresult.SortBy{{Key: testresult.SortByBaseStatusKey}},
				BaseTasks: []Task{{Id: baseTask.Id, TaskOutputInfo: &output}},
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
			opts: &FilterOptions{
				Sort: []testresult.SortBy{
					{
						Key:      testresult.SortByBaseStatusKey,
						OrderDSC: true,
					},
				},
				BaseTasks: []Task{{Id: baseTask.Id, TaskOutputInfo: &output}},
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
			opts: &FilterOptions{
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
			opts: &FilterOptions{BaseTasks: []Task{{Id: baseTask.Id, TaskOutputInfo: &output}}},
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
			actualResults, count, err := filterAndSortTestResults(ctx, env, getResults(), test.opts)
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
		TestName:       utility.RandomString(),
		Status:         evergreen.TestSucceededStatus,
		TestStartTime:  time.Now().Add(-30 * time.Hour).UTC().Round(time.Millisecond),
		TaskCreateTime: time.Now().Add(-30 * time.Hour).UTC().Round(time.Millisecond),
		TestEndTime:    time.Now().UTC().Round(time.Millisecond),
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

func getTestResults() *testresult.DbTaskTestResults {
	info := testresult.TestResultsInfo{
		Project:   utility.RandomString(),
		Version:   utility.RandomString(),
		Variant:   utility.RandomString(),
		TaskName:  utility.RandomString(),
		TaskID:    utility.RandomString(),
		Execution: rand.Intn(5),
		Requester: utility.RandomString(),
	}
	// Optional fields, we should test that we handle them properly when
	// they are populated and when they do not.
	if sometimes.Half() {
		info.DisplayTaskName = utility.RandomString()
		info.DisplayTaskID = utility.RandomString()
	}

	return &testresult.DbTaskTestResults{
		ID:          info.ID(),
		Info:        info,
		CreatedAt:   time.Now().Add(-time.Hour).UTC().Round(time.Millisecond),
		CompletedAt: time.Now().UTC().Round(time.Millisecond),
	}
}

func saveTestResults(t *testing.T, ctx context.Context, testBucket pail.Bucket, svc TestResultsService, tsk *Task, length int) []testresult.TestResult {
	tr := getTestResults()
	tr.Info.TaskID = tsk.Id
	tr.Info.Execution = tsk.Execution
	savedResults := make([]testresult.TestResult, length)

	savedParquet := testresult.ParquetTestResults{
		Version:   tr.Info.Version,
		Variant:   tr.Info.Variant,
		TaskName:  tr.Info.TaskName,
		TaskID:    tr.Info.TaskID,
		Execution: int32(tr.Info.Execution),
		Requester: tr.Info.Requester,
		CreatedAt: tr.CreatedAt.UTC(),
		Results:   make([]testresult.ParquetTestResult, length),
	}

	for i := 0; i < len(savedResults); i++ {
		result := getTestResult()
		result.TaskID = tr.Info.TaskID
		result.Execution = tr.Info.Execution
		if tsk.Id == "task0" && i%2 != 0 {
			result.Status = evergreen.TestFailedStatus
		} else if tsk.Id == "task3" && i%2 == 0 {
			result.Status = evergreen.TestFailedStatus
		} else if tsk.Id == "task4" || tsk.Id == "task5" {
			result.Status = evergreen.TestFailedStatus
		}
		savedResults[i] = result
		savedParquet.Results[i] = testresult.ParquetTestResult{
			TestName:       result.TestName,
			GroupID:        utility.ToStringPtr(result.GroupID),
			Status:         result.Status,
			LogInfo:        result.LogInfo,
			TaskCreateTime: result.TaskCreateTime.UTC(),
			TestStartTime:  result.TestStartTime.UTC(),
			TestEndTime:    result.TestEndTime.UTC(),
		}
		if result.DisplayTestName != "" {
			savedParquet.Results[i].DisplayTestName = utility.ToStringPtr(result.DisplayTestName)
			savedParquet.Results[i].GroupID = utility.ToStringPtr(result.GroupID)
			savedParquet.Results[i].LogTestName = utility.ToStringPtr(result.LogTestName)
			savedParquet.Results[i].LogURL = utility.ToStringPtr(result.LogURL)
			savedParquet.Results[i].RawLogURL = utility.ToStringPtr(result.RawLogURL)
			savedParquet.Results[i].LineNum = utility.ToInt32Ptr(int32(result.LineNum))
		}
	}

	w, err := testBucket.Writer(ctx, fmt.Sprintf("%s/%s", output.TestResults.BucketConfig.TestResultsPrefix, testresult.PartitionKey(tr.CreatedAt, tr.Info.Project, tr.ID)))
	require.NoError(t, err)
	defer func() { assert.NoError(t, w.Close()) }()

	pw := floor.NewWriter(goparquet.NewFileWriter(w, goparquet.WithSchemaDefinition(ParquetTestResultsSchemaDef)))
	require.NoError(t, pw.Write(savedParquet))
	require.NoError(t, pw.Close())
	require.NoError(t, db.Insert(ctx, testresult.Collection, tr))
	require.NoError(t, svc.AppendTestResultMetadata(resultTestutil.MakeAppendTestResultMetadataReq(ctx, savedResults, tr.ID)))
	return savedResults
}
