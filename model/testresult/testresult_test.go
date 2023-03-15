package testresult

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/sometimes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMergedTaskTestResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, ClearLocal(ctx, env))
	}()
	srv, handler := newMockCedarServer(env)
	defer srv.Close()

	task0 := TaskOptions{
		TaskID:         "task0",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
	}
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

	task1 := TaskOptions{
		TaskID:         "task1",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
	}
	savedResults1 := make([]TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	require.NoError(t, InsertLocal(ctx, env, savedResults1...))

	task2 := TaskOptions{
		TaskID:         "task2",
		Execution:      1,
		ResultsService: TestResultsServiceLocal,
	}
	savedResults2 := make([]TestResult, 10)
	for i := 0; i < len(savedResults2); i++ {
		result := getTestResult()
		result.TaskID = task2.TaskID
		result.Execution = task2.Execution
		savedResults2[i] = result
	}
	require.NoError(t, InsertLocal(ctx, env, savedResults2...))

	externalServiceTask := TaskOptions{
		TaskID:         "external_service_task",
		Execution:      1,
		ResultsService: TestResultsServiceCedar,
	}
	externalServiceResults := make([]TestResult, 10)
	for i := 0; i < len(externalServiceResults); i++ {
		result := getTestResult()
		result.TaskID = externalServiceTask.TaskID
		result.Execution = externalServiceTask.Execution
		externalServiceResults[i] = result
	}

	for _, test := range []struct {
		name                string
		setup               func(t *testing.T)
		taskOpts            []TaskOptions
		filterOpts          *FilterOptions
		expectedTaskResults TaskTestResults
		hasErr              bool
	}{
		{
			name:   "NilTaskOptions",
			hasErr: true,
		},
		{
			name:     "NilTaskOptions",
			taskOpts: []TaskOptions{},
			hasErr:   true,
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts: []TaskOptions{externalServiceTask},
			hasErr:   true,
		},
		{
			name: "UnsupportedService",
			taskOpts: []TaskOptions{
				{
					TaskID:         "task",
					Execution:      0,
					ResultsService: "DNE",
				},
			},
			hasErr: true,
		},
		{
			name:     "WithoutFilterOptions",
			taskOpts: []TaskOptions{task1, task2, task0},
			expectedTaskResults: TaskTestResults{
				Stats: TaskTestResultsStats{
					TotalCount:    len(savedResults0) + len(savedResults1) + len(savedResults2),
					FailedCount:   len(savedResults0) / 2,
					FilteredCount: utility.ToIntPtr(len(savedResults0) + len(savedResults1) + len(savedResults2)),
				},
				Results: append(append(append([]TestResult{}, savedResults0...), savedResults1...), savedResults2...),
			},
		},
		{
			name:       "WithFilterOptions",
			taskOpts:   []TaskOptions{task0},
			filterOpts: &FilterOptions{TestName: savedResults0[0].GetDisplayTestName()},
			expectedTaskResults: TaskTestResults{
				Stats: TaskTestResultsStats{
					TotalCount:    len(savedResults0),
					FailedCount:   len(savedResults0) / 2,
					FilteredCount: utility.ToIntPtr(1),
				},
				Results: savedResults0[0:1],
			},
		},
		{
			name: "DifferentServices",
			setup: func(t *testing.T) {
				data, err := json.Marshal(&TaskTestResults{
					Stats: TaskTestResultsStats{
						TotalCount:    len(externalServiceResults),
						FilteredCount: utility.ToIntPtr(len(externalServiceResults)),
					},
					Results: externalServiceResults,
				})
				require.NoError(t, err)

				handler.status = http.StatusOK
				handler.data = data
			},
			taskOpts: []TaskOptions{externalServiceTask, task0},
			expectedTaskResults: TaskTestResults{
				Stats: TaskTestResultsStats{
					TotalCount:    len(externalServiceResults),
					FilteredCount: utility.ToIntPtr(len(externalServiceResults)),
				},
				Results: externalServiceResults,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			taskResults, err := GetMergedTaskTestResults(ctx, env, test.taskOpts, test.filterOpts)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.expectedTaskResults, taskResults)
		})
	}
}

func TestGetMergedTaskTestResultsStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, ClearLocal(ctx, env))
	}()
	srv, handler := newMockCedarServer(env)
	defer srv.Close()

	task0 := TaskOptions{
		TaskID:         "task0",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
	}
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

	task1 := TaskOptions{
		TaskID:         "task1",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
	}
	savedResults1 := make([]TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	require.NoError(t, InsertLocal(ctx, env, savedResults1...))

	externalServiceTask := TaskOptions{
		TaskID:         "external_service_task",
		Execution:      0,
		ResultsService: TestResultsServiceCedar,
	}
	externalServiceStats := TaskTestResultsStats{
		TotalCount:  5,
		FailedCount: 2,
	}

	for _, test := range []struct {
		name          string
		setup         func(t *testing.T)
		taskOpts      []TaskOptions
		expectedStats TaskTestResultsStats
		hasErr        bool
	}{
		{
			name:   "NilTaskOptions",
			hasErr: true,
		},
		{
			name:     "NilTaskOptions",
			taskOpts: []TaskOptions{},
			hasErr:   true,
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts: []TaskOptions{externalServiceTask},
			hasErr:   true,
		},
		{
			name: "UnsupportedService",
			taskOpts: []TaskOptions{
				{
					TaskID:         "task",
					Execution:      0,
					ResultsService: "DNE",
				},
			},
			hasErr: true,
		},
		{
			name:     "SameService",
			taskOpts: []TaskOptions{task0, task1},
			expectedStats: TaskTestResultsStats{
				TotalCount:  len(savedResults0) + len(savedResults1),
				FailedCount: len(savedResults0) / 2,
			},
		},
		{
			name: "DifferentServices",
			setup: func(t *testing.T) {
				data, err := json.Marshal(&externalServiceStats)
				require.NoError(t, err)

				handler.status = http.StatusOK
				handler.data = data
			},
			taskOpts: []TaskOptions{externalServiceTask, task0},
			expectedStats: TaskTestResultsStats{
				TotalCount:  len(savedResults0) + externalServiceStats.TotalCount,
				FailedCount: len(savedResults0)/2 + externalServiceStats.FailedCount,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			stats, err := GetMergedTaskTestResultsStats(ctx, env, test.taskOpts)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.expectedStats, stats)
		})
	}
}

func TestGetMergedFailedTestSample(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, ClearLocal(ctx, env))
	}()
	srv, handler := newMockCedarServer(env)
	defer srv.Close()

	task0 := TaskOptions{
		TaskID:         "task0",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
	}
	sample0 := make([]string, 2)
	for i := 0; i < len(sample0); i++ {
		result := getTestResult()
		result.TaskID = task0.TaskID
		result.Execution = task0.Execution
		result.Status = evergreen.TestFailedStatus
		sample0[i] = result.GetDisplayTestName()
		require.NoError(t, InsertLocal(ctx, env, result))
	}

	task1 := TaskOptions{
		TaskID:         "task1",
		Execution:      1,
		ResultsService: TestResultsServiceLocal,
	}
	sample1 := make([]string, 2)
	for i := 0; i < len(sample1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		result.Status = evergreen.TestFailedStatus
		sample1[i] = result.GetDisplayTestName()
		require.NoError(t, InsertLocal(ctx, env, result))
	}

	externalServiceTask := TaskOptions{
		TaskID:         "external_service_task",
		Execution:      0,
		ResultsService: TestResultsServiceCedar,
	}
	externalServiceSample := []string{"test0", "test1"}

	for _, test := range []struct {
		name           string
		setup          func(t *testing.T)
		taskOpts       []TaskOptions
		expectedSample []string
		hasErr         bool
	}{
		{
			name:   "NilTaskOptions",
			hasErr: true,
		},
		{
			name:     "NilTaskOptions",
			taskOpts: []TaskOptions{},
			hasErr:   true,
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts: []TaskOptions{externalServiceTask},
			hasErr:   true,
		},
		{
			name: "UnsupportedService",
			taskOpts: []TaskOptions{
				{
					TaskID:         "task",
					Execution:      0,
					ResultsService: "DNE",
				},
			},
			hasErr: true,
		},
		{
			name:           "SameService",
			taskOpts:       []TaskOptions{task0, task1},
			expectedSample: append(append([]string{}, sample0...), sample1...),
		},
		{
			name: "DifferentServices",
			setup: func(t *testing.T) {
				data, err := json.Marshal(&externalServiceSample)
				require.NoError(t, err)

				handler.status = http.StatusOK
				handler.data = data
			},
			taskOpts:       []TaskOptions{externalServiceTask, task0},
			expectedSample: append(append([]string{}, externalServiceSample...), sample0...),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			sample, err := GetMergedFailedTestSample(ctx, env, test.taskOpts)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.ElementsMatch(t, test.expectedSample, sample)
		})
	}
}

func TestGetFailedTestSamples(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, ClearLocal(ctx, env))
	}()
	srv, handler := newMockCedarServer(env)
	defer srv.Close()

	task0 := TaskOptions{
		TaskID:         "task0",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
	}
	sample0 := make([]string, 2)
	for i := 0; i < len(sample0); i++ {
		result := getTestResult()
		result.TaskID = task0.TaskID
		result.Execution = task0.Execution
		result.Status = evergreen.TestFailedStatus
		sample0[i] = result.GetDisplayTestName()
		require.NoError(t, InsertLocal(ctx, env, result))
	}

	task1 := TaskOptions{
		TaskID:         "task1",
		Execution:      1,
		ResultsService: TestResultsServiceLocal,
	}
	sample1 := make([]string, 2)
	for i := 0; i < len(sample1); i++ {
		result := getTestResult()
		result.TaskID = task1.TaskID
		result.Execution = task1.Execution
		result.Status = evergreen.TestFailedStatus
		sample1[i] = result.GetDisplayTestName()
		require.NoError(t, InsertLocal(ctx, env, result))
	}

	externalServiceTask := TaskOptions{
		TaskID:         "external_service_task",
		Execution:      0,
		ResultsService: TestResultsServiceCedar,
	}
	externalServiceSample := []string{"test0", "test1"}

	for _, test := range []struct {
		name            string
		setup           func(t *testing.T)
		taskOpts        []TaskOptions
		regexFilters    []string
		expectedSamples []TaskTestResultsFailedSample
		hasErr          bool
	}{
		{
			name:   "NilTaskOptions",
			hasErr: true,
		},
		{
			name:     "NilTaskOptions",
			taskOpts: []TaskOptions{},
			hasErr:   true,
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts: []TaskOptions{externalServiceTask},
			hasErr:   true,
		},
		{
			name: "UnsupportedService",
			taskOpts: []TaskOptions{
				{
					TaskID:         "task",
					Execution:      0,
					ResultsService: "DNE",
				},
			},
			hasErr: true,
		},
		{
			name:     "SameService",
			taskOpts: []TaskOptions{task0, task1},
			expectedSamples: []TaskTestResultsFailedSample{
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
			name: "DifferentServices",
			setup: func(t *testing.T) {
				data, err := json.Marshal(&[]TaskTestResultsFailedSample{
					{
						TaskID:                  externalServiceTask.TaskID,
						Execution:               externalServiceTask.Execution,
						MatchingFailedTestNames: externalServiceSample,
						TotalFailedNames:        len(externalServiceSample),
					},
				})
				require.NoError(t, err)

				handler.status = http.StatusOK
				handler.data = data
			},
			taskOpts: []TaskOptions{externalServiceTask, task0},
			expectedSamples: []TaskTestResultsFailedSample{
				{
					TaskID:                  task0.TaskID,
					Execution:               task0.Execution,
					MatchingFailedTestNames: sample0,
					TotalFailedNames:        len(sample0),
				},
				{
					TaskID:                  externalServiceTask.TaskID,
					Execution:               externalServiceTask.Execution,
					MatchingFailedTestNames: externalServiceSample,
					TotalFailedNames:        len(externalServiceSample),
				},
			},
		},
		{
			name: "WithRegexFilter",
			setup: func(t *testing.T) {
				data, err := json.Marshal(&[]TaskTestResultsFailedSample{
					{
						TaskID:                  externalServiceTask.TaskID,
						Execution:               externalServiceTask.Execution,
						MatchingFailedTestNames: externalServiceSample,
						TotalFailedNames:        len(externalServiceSample),
					},
				})
				require.NoError(t, err)

				handler.status = http.StatusOK
				handler.data = data
			},
			taskOpts:     []TaskOptions{externalServiceTask, task0},
			regexFilters: []string{"test"},
			expectedSamples: []TaskTestResultsFailedSample{
				{
					TaskID:           task0.TaskID,
					Execution:        task0.Execution,
					TotalFailedNames: len(sample0),
				},
				{
					TaskID:                  externalServiceTask.TaskID,
					Execution:               externalServiceTask.Execution,
					MatchingFailedTestNames: externalServiceSample,
					TotalFailedNames:        len(externalServiceSample),
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

func getTestResult() TestResult {
	result := TestResult{
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
