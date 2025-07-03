package task

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/testresult"
	resultTestutil "github.com/evergreen-ci/evergreen/model/testresult/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskTestResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, ClearTestResults(ctx, env))
	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, ClearTestResults(ctx, env))
		require.NoError(t, db.Clear(Collection))
	}()
	svc := NewTestResultService(env)

	output.TestResults.BucketConfig.Name = t.TempDir()
	testBucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: output.TestResults.BucketConfig.Name})
	require.NoError(t, err)

	task0 := Task{
		Id:        "task0",
		Execution: 0,

		TaskOutputInfo: &output,
	}
	task1 := Task{
		Id:        "task1",
		Execution: 0,

		TaskOutputInfo: &output,
	}
	task2 := Task{
		Id:        "task2",
		Execution: 1,

		TaskOutputInfo: &output,
	}
	require.NoError(t, db.InsertMany(t.Context(), Collection, task0, task1, task2))

	savedResults0 := saveTestResults(t, ctx, testBucket, svc, &task0, 10)
	savedResults1 := saveTestResults(t, ctx, testBucket, svc, &task1, 10)
	savedResults2 := saveTestResults(t, ctx, testBucket, svc, &task2, 10)

	for _, test := range []struct {
		name                string
		setup               func(t *testing.T)
		taskOpts            []Task
		filterOpts          *FilterOptions
		expectedTaskResults testresult.TaskTestResults
		output              TaskOutput
		hasErr              bool
	}{
		{
			name:   "Niltask.Task",
			output: output,
			expectedTaskResults: testresult.TaskTestResults{
				Stats: testresult.TaskTestResultsStats{
					TotalCount:    0,
					FailedCount:   0,
					FilteredCount: nil,
				},
				Results: nil,
			},
		},
		{
			name:     "Niltask.Task",
			output:   output,
			taskOpts: []Task{},
			expectedTaskResults: testresult.TaskTestResults{
				Stats: testresult.TaskTestResultsStats{
					TotalCount:    0,
					FailedCount:   0,
					FilteredCount: nil,
				},
				Results: nil,
			},
		},
		{
			name:     "WithoutFilterOptions",
			taskOpts: []Task{task1, task2, task0},
			expectedTaskResults: testresult.TaskTestResults{
				Stats: testresult.TaskTestResultsStats{
					TotalCount:    len(savedResults0) + len(savedResults1) + len(savedResults2),
					FailedCount:   len(savedResults0) / 2,
					FilteredCount: utility.ToIntPtr(len(savedResults0) + len(savedResults1) + len(savedResults2)),
				},
				Results: append(append(append([]testresult.TestResult{}, savedResults0...), savedResults1...), savedResults2...),
			},
		},
		{
			name:       "WithFilterOptions",
			output:     output,
			taskOpts:   []Task{task0},
			filterOpts: &FilterOptions{TestName: savedResults0[0].GetDisplayTestName()},
			expectedTaskResults: testresult.TaskTestResults{
				Stats: testresult.TaskTestResultsStats{
					TotalCount:    len(savedResults0),
					FailedCount:   len(savedResults0) / 2,
					FilteredCount: utility.ToIntPtr(1),
				},
				Results: savedResults0[0:1],
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}
			taskResults, err := getMergedTaskTestResults(ctx, env, test.taskOpts, test.filterOpts)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.expectedTaskResults, taskResults)
		})
	}
}

func TestGetTaskTestResultsStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, ClearTestResults(ctx, env))
	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, ClearTestResults(ctx, env))
		require.NoError(t, db.Clear(Collection))
	}()
	svc := NewTestResultService(env)
	task0 := Task{
		Id:        "task0",
		Execution: 0,

		TaskOutputInfo: &output,
	}
	savedResults0 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults0); i++ {
		result := getTestResult()
		result.TaskID = task0.Id
		result.Execution = task0.Execution
		if i%2 != 0 {
			result.Status = evergreen.TestFailedStatus
		}
		savedResults0[i] = result
	}
	info := testresult.TestResultsInfo{
		TaskID:    task0.Id,
		Execution: task0.Execution,
	}
	tr := testresult.DbTaskTestResults{
		ID:          info.ID(),
		Info:        info,
		CreatedAt:   time.Now().Add(-time.Hour).UTC().Round(time.Millisecond),
		CompletedAt: time.Now().UTC().Round(time.Millisecond),
	}
	require.NoError(t, db.Insert(ctx, testresult.Collection, tr))
	require.NoError(t, svc.AppendTestResultMetadata(resultTestutil.MakeAppendTestResultMetadataReq(ctx, savedResults0, tr.ID)))

	task1 := Task{
		Id:        "task1",
		Execution: 0,

		TaskOutputInfo: &output,
	}
	savedResults1 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := getTestResult()
		result.TaskID = task1.Id
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	info = testresult.TestResultsInfo{
		TaskID:    task1.Id,
		Execution: task1.Execution,
	}
	tr = testresult.DbTaskTestResults{
		ID:          info.ID(),
		Info:        info,
		CreatedAt:   time.Now().Add(-time.Hour).UTC().Round(time.Millisecond),
		CompletedAt: time.Now().UTC().Round(time.Millisecond),
	}
	require.NoError(t, db.Insert(ctx, testresult.Collection, tr))
	require.NoError(t, svc.AppendTestResultMetadata(resultTestutil.MakeAppendTestResultMetadataReq(ctx, savedResults1, tr.ID)))

	for _, test := range []struct {
		name          string
		setup         func(t *testing.T)
		taskOpts      []Task
		expectedStats testresult.TaskTestResultsStats
		output        TaskOutput
		hasErr        bool
	}{
		{
			name:   "Niltask.Task",
			output: output,
		},
		{
			name:     "Niltask.Task",
			taskOpts: []Task{},
			output:   output,
		},
		{
			name:     "SameService",
			taskOpts: []Task{task0, task1},
			expectedStats: testresult.TaskTestResultsStats{
				TotalCount:  len(savedResults0) + len(savedResults1),
				FailedCount: len(savedResults0) / 2,
			},
			output: output,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			stats, err := getTaskTestResultsStats(ctx, env, test.taskOpts)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.expectedStats, stats)
		})
	}
}

func TestGetFailedTestSamples(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, ClearTestResults(ctx, env))
	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, ClearTestResults(ctx, env))
		require.NoError(t, db.Clear(Collection))
	}()
	svc := NewTestResultService(env)
	task5 := Task{
		Id:        "task5",
		Execution: 0,

		TaskOutputInfo: &output,
	}
	task4 := Task{
		Id:        "task4",
		Execution: 1,

		TaskOutputInfo: &output,
	}

	output.TestResults.BucketConfig.Name = t.TempDir()
	testBucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: output.TestResults.BucketConfig.Name})
	require.NoError(t, err)

	require.NoError(t, db.InsertMany(t.Context(), Collection, task5, task4))

	sample0 := make([]string, 2)
	sample1 := make([]string, 2)

	savedResults0 := saveTestResults(t, ctx, testBucket, svc, &task5, 2)
	for i := 0; i < len(savedResults0); i++ {
		sample0[i] = savedResults0[i].GetDisplayTestName()
	}
	savedResults1 := saveTestResults(t, ctx, testBucket, svc, &task4, 2)
	for i := 0; i < len(savedResults1); i++ {
		sample1[i] = savedResults1[i].GetDisplayTestName()
	}
	for _, test := range []struct {
		name            string
		setup           func(t *testing.T)
		taskOpts        []Task
		regexFilters    []string
		expectedSamples []testresult.TaskTestResultsFailedSample
		hasErr          bool
	}{
		{
			name:   "Niltask.Task",
			hasErr: true,
		},
		{
			name:     "Niltask.Task",
			taskOpts: []Task{},
			hasErr:   true,
		},
		{
			name:     "SameService",
			taskOpts: []Task{task5, task4},
			expectedSamples: []testresult.TaskTestResultsFailedSample{
				{
					TaskID:                  task5.Id,
					Execution:               task5.Execution,
					MatchingFailedTestNames: sample0,
					TotalFailedNames:        len(sample0),
				},
				{
					TaskID:                  task4.Id,
					Execution:               task4.Execution,
					MatchingFailedTestNames: sample1,
					TotalFailedNames:        len(sample1),
				},
			},
		},
		{
			name:         "WithRegexFilter",
			taskOpts:     []Task{task5},
			regexFilters: []string{"test"},
			expectedSamples: []testresult.TaskTestResultsFailedSample{
				{
					TaskID:           task5.Id,
					Execution:        task5.Execution,
					TotalFailedNames: len(sample0),
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			samples, err := GetFailedTestSamples(ctx, env, test.taskOpts, test.regexFilters)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.ElementsMatch(t, test.expectedSamples, samples)
		})
	}
}

func TestAppendResults(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, ClearTestResults(ctx, env))
	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, ClearTestResults(ctx, env))
		require.NoError(t, db.Clear(Collection))
	}()
	svc := NewTestResultService(env)

	output.TestResults.BucketConfig.Name = t.TempDir()
	testBucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: output.TestResults.BucketConfig.Name})
	require.NoError(t, err)

	task0 := Task{
		Id:        "task0",
		Execution: 0,

		TaskOutputInfo: &output,
	}
	task1 := Task{
		Id:        "task1",
		Execution: 0,

		TaskOutputInfo: &output,
	}
	task2 := Task{
		Id:        "task2",
		Execution: 1,

		TaskOutputInfo: &output,
	}
	require.NoError(t, db.InsertMany(t.Context(), Collection, task0, task1, task2))

	_ = saveTestResults(t, ctx, testBucket, svc, &task0, 10)
	_ = saveTestResults(t, ctx, testBucket, svc, &task1, 10)
	_ = saveTestResults(t, ctx, testBucket, svc, &task2, 10)

	dbResults, err := svc.Get(ctx, []Task{task0, task1, task2})
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
