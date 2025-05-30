package taskoutput

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskTestResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, testresult.ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, testresult.ClearLocal(ctx, env))
	}()
	srv, handler := newMockCedarServer(env)
	defer srv.Close()
	svc := testresult.NewLocalService(env)
	task0 := testresult.TaskOptions{
		TaskID:         "task0",
		Execution:      0,
		ResultsService: testresult.TestResultsServiceLocal,
	}
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

	task1 := testresult.TaskOptions{
		TaskID:         "task1",
		Execution:      0,
		ResultsService: testresult.TestResultsServiceLocal,
	}
	savedResults1 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults1))

	task2 := testresult.TaskOptions{
		TaskID:         "task2",
		Execution:      1,
		ResultsService: testresult.TestResultsServiceLocal,
	}
	savedResults2 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults2); i++ {
		result := getTestResult()
		result.TaskID = task2.TaskID
		result.Execution = task2.Execution
		savedResults2[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults2))

	externalServiceTask := testresult.TaskOptions{
		TaskID:         "external_service_task",
		Execution:      1,
		ResultsService: testresult.TestResultsServiceCedar,
	}
	externalServiceResults := make([]testresult.TestResult, 10)
	for i := 0; i < len(externalServiceResults); i++ {
		result := getTestResult()
		result.TaskID = externalServiceTask.TaskID
		result.Execution = externalServiceTask.Execution
		externalServiceResults[i] = result
	}

	for _, test := range []struct {
		name                string
		setup               func(t *testing.T)
		taskOpts            []testresult.TaskOptions
		filterOpts          *testresult.FilterOptions
		expectedTaskResults testresult.TaskTestResults
		output              TestResultOutput
		hasErr              bool
	}{
		{
			name:   "NilTaskOptions",
			output: output,
			expectedTaskResults: testresult.TaskTestResults{
				Stats: testresult.TaskTestResultsStats{
					TotalCount:    0,
					FailedCount:   0,
					FilteredCount: utility.ToIntPtr(0),
				},
				Results: nil,
			},
		},
		{
			name:     "NilTaskOptions",
			output:   output,
			taskOpts: []testresult.TaskOptions{},
			expectedTaskResults: testresult.TaskTestResults{
				Stats: testresult.TaskTestResultsStats{
					TotalCount:    0,
					FailedCount:   0,
					FilteredCount: utility.ToIntPtr(0),
				},
				Results: nil,
			},
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			output:   outputCedar,
			taskOpts: []testresult.TaskOptions{externalServiceTask},
			hasErr:   true,
		},
		{
			name:     "WithoutFilterOptions",
			output:   output,
			taskOpts: []testresult.TaskOptions{task1, task2, task0},
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
			taskOpts:   []testresult.TaskOptions{task0},
			filterOpts: &testresult.FilterOptions{TestName: savedResults0[0].GetDisplayTestName()},
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
			taskResults, err := test.output.GetMergedTaskTestResults(ctx, env, createTaskoutputTaskOpts(test.taskOpts), test.filterOpts)
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

	require.NoError(t, testresult.ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, testresult.ClearLocal(ctx, env))
	}()
	srv, handler := newMockCedarServer(env)
	defer srv.Close()
	svc := testresult.NewLocalService(env)
	task0 := testresult.TaskOptions{
		TaskID:         "task0",
		Execution:      0,
		ResultsService: testresult.TestResultsServiceLocal,
	}
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

	task1 := testresult.TaskOptions{
		TaskID:         "task1",
		Execution:      0,
		ResultsService: testresult.TestResultsServiceLocal,
	}
	savedResults1 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, savedResults1))

	externalServiceTask := testresult.TaskOptions{
		TaskID:         "external_service_task",
		Execution:      0,
		ResultsService: testresult.TestResultsServiceCedar,
	}

	for _, test := range []struct {
		name          string
		setup         func(t *testing.T)
		taskOpts      []testresult.TaskOptions
		expectedStats testresult.TaskTestResultsStats
		output        TestResultOutput
		hasErr        bool
	}{
		{
			name:   "NilTaskOptions",
			output: output,
		},
		{
			name:     "NilTaskOptions",
			taskOpts: []testresult.TaskOptions{},
			output:   output,
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts: []testresult.TaskOptions{externalServiceTask},
			hasErr:   true,
			output:   outputCedar,
		},
		{
			name:     "SameService",
			taskOpts: []testresult.TaskOptions{task0, task1},
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

			stats, err := test.output.GetTaskTestResultsStats(ctx, env, createTaskoutputTaskOpts(test.taskOpts))
			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.expectedStats, stats)
		})
	}
}

type mockHandler struct {
	status int
	data   []byte
}

func (h *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.status > 0 {
		w.WriteHeader(h.status)
	}
	if h.data != nil {
		_, _ = w.Write(h.data)
	}
}

func newMockCedarServer(env evergreen.Environment) (*httptest.Server, *mockHandler) {
	handler := &mockHandler{}
	srv := httptest.NewServer(handler)
	env.Settings().Cedar = evergreen.CedarConfig{
		BaseURL:  strings.TrimPrefix(srv.URL, "http://"),
		Insecure: true,
	}

	return srv, handler
}

func TestGetFailedTestSamples(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, testresult.ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, testresult.ClearLocal(ctx, env))
	}()
	srv, handler := newMockCedarServer(env)
	defer srv.Close()
	svc := testresult.NewLocalService(env)
	task0 := testresult.TaskOptions{
		TaskID:         "task0",
		Execution:      0,
		ResultsService: testresult.TestResultsServiceLocal,
	}
	sample0 := make([]string, 2)
	for i := 0; i < len(sample0); i++ {
		result := getTestResult()
		result.TaskID = task0.TaskID
		result.Execution = task0.Execution
		result.Status = evergreen.TestFailedStatus
		sample0[i] = result.GetDisplayTestName()
		require.NoError(t, svc.AppendTestResults(ctx, []testresult.TestResult{result}))
	}

	task1 := testresult.TaskOptions{
		TaskID:         "task1",
		Execution:      1,
		ResultsService: testresult.TestResultsServiceLocal,
	}
	sample1 := make([]string, 2)
	for i := 0; i < len(sample1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		result.Status = evergreen.TestFailedStatus
		sample1[i] = result.GetDisplayTestName()
		require.NoError(t, svc.AppendTestResults(ctx, []testresult.TestResult{result}))
	}

	externalServiceTask := testresult.TaskOptions{
		TaskID:         "external_service_task",
		Execution:      0,
		ResultsService: testresult.TestResultsServiceCedar,
	}

	for _, test := range []struct {
		name            string
		setup           func(t *testing.T)
		taskOpts        []testresult.TaskOptions
		regexFilters    []string
		expectedSamples []testresult.TaskTestResultsFailedSample
		hasErr          bool
	}{
		{
			name:   "NilTaskOptions",
			hasErr: true,
		},
		{
			name:     "NilTaskOptions",
			taskOpts: []testresult.TaskOptions{},
			hasErr:   true,
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts: []testresult.TaskOptions{externalServiceTask},
			hasErr:   true,
		},
		{
			name:     "SameService",
			taskOpts: []testresult.TaskOptions{task0, task1},
			expectedSamples: []testresult.TaskTestResultsFailedSample{
				{
					TaskID:                  task0.TaskID,
					Execution:               task0.Execution,
					MatchingFailedTestNames: sample0,
					TotalFailedNames:        len(sample0),
				},
				{
					TaskID:                  task1.TaskID,
					Execution:               task1.Execution,
					MatchingFailedTestNames: sample1,
					TotalFailedNames:        len(sample1),
				},
			},
		},
		{
			name:         "WithRegexFilter",
			taskOpts:     []testresult.TaskOptions{task0},
			regexFilters: []string{"test"},
			expectedSamples: []testresult.TaskTestResultsFailedSample{
				{
					TaskID:           task0.TaskID,
					Execution:        task0.Execution,
					TotalFailedNames: len(sample0),
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			samples, err := GetFailedTestSamples(ctx, env, createTaskoutputTaskOpts(test.taskOpts), test.regexFilters)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.ElementsMatch(t, test.expectedSamples, samples)
		})
	}
}
