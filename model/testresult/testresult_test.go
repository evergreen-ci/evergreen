package testresult

import (
	"context"
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
	svc := NewLocalService(env)
	cedarSvc := NewCedarService(env)

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
	require.NoError(t, svc.AppendTestResults(ctx, savedResults0))

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
	require.NoError(t, svc.AppendTestResults(ctx, savedResults1))

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
	require.NoError(t, svc.AppendTestResults(ctx, savedResults2))

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
		resultService       TestResultsService
		hasErr              bool
	}{
		{
			name:          "NilTaskOptions",
			resultService: svc,
			expectedTaskResults: TaskTestResults{
				Stats: TaskTestResultsStats{
					TotalCount:    0,
					FailedCount:   0,
					FilteredCount: utility.ToIntPtr(0),
				},
				Results: nil,
			},
		},
		{
			name:          "NilTaskOptions",
			resultService: svc,
			taskOpts:      []TaskOptions{},
			expectedTaskResults: TaskTestResults{
				Stats: TaskTestResultsStats{
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
			resultService: cedarSvc,
			taskOpts:      []TaskOptions{externalServiceTask},
			hasErr:        true,
		},
		{
			name:          "WithoutFilterOptions",
			resultService: svc,
			taskOpts:      []TaskOptions{task1, task2, task0},
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
			name:          "WithFilterOptions",
			resultService: svc,
			taskOpts:      []TaskOptions{task0},
			filterOpts:    &FilterOptions{TestName: savedResults0[0].GetDisplayTestName()},
			expectedTaskResults: TaskTestResults{
				Stats: TaskTestResultsStats{
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

			taskResults, err := test.resultService.GetMergedTaskTestResults(ctx, test.taskOpts, test.filterOpts)
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
	svc := NewLocalService(env)
	cedarSvc := NewCedarService(env)

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
	require.NoError(t, svc.AppendTestResults(ctx, savedResults0))

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
	require.NoError(t, svc.AppendTestResults(ctx, savedResults1))

	externalServiceTask := TaskOptions{
		TaskID:         "external_service_task",
		Execution:      0,
		ResultsService: TestResultsServiceCedar,
	}

	for _, test := range []struct {
		name          string
		setup         func(t *testing.T)
		taskOpts      []TaskOptions
		expectedStats TaskTestResultsStats
		resultService TestResultsService
		hasErr        bool
	}{
		{
			name:          "NilTaskOptions",
			resultService: svc,
		},
		{
			name:          "NilTaskOptions",
			taskOpts:      []TaskOptions{},
			resultService: svc,
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts:      []TaskOptions{externalServiceTask},
			hasErr:        true,
			resultService: cedarSvc,
		},
		{
			name:     "SameService",
			taskOpts: []TaskOptions{task0, task1},
			expectedStats: TaskTestResultsStats{
				TotalCount:  len(savedResults0) + len(savedResults1),
				FailedCount: len(savedResults0) / 2,
			},
			resultService: svc,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			stats, err := test.resultService.GetMergedTaskTestResultsStats(ctx, test.taskOpts)
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
	svc := NewLocalService(env)
	cedarSvc := NewCedarService(env)
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
		require.NoError(t, svc.AppendTestResults(ctx, []TestResult{result}))
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
		require.NoError(t, svc.AppendTestResults(ctx, []TestResult{result}))
	}

	externalServiceTask := TaskOptions{
		TaskID:         "external_service_task",
		Execution:      0,
		ResultsService: TestResultsServiceCedar,
	}

	for _, test := range []struct {
		name           string
		setup          func(t *testing.T)
		taskOpts       []TaskOptions
		expectedSample []string
		hasErr         bool
		resultService  TestResultsService
	}{
		{
			name:          "NilTaskOptions",
			resultService: svc,
		},
		{
			name:          "NilTaskOptions",
			taskOpts:      []TaskOptions{},
			resultService: svc,
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts:      []TaskOptions{externalServiceTask},
			hasErr:        true,
			resultService: cedarSvc,
		},
		{
			name:           "SameService",
			taskOpts:       []TaskOptions{task0, task1},
			expectedSample: append(append([]string{}, sample0...), sample1...),
			resultService:  svc,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			sample, err := test.resultService.GetMergedFailedTestSample(ctx, test.taskOpts)
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
	svc := NewLocalService(env)
	cedarSvc := NewCedarService(env)
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
		require.NoError(t, svc.AppendTestResults(ctx, []TestResult{result}))
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
		require.NoError(t, svc.AppendTestResults(ctx, []TestResult{result}))
	}

	externalServiceTask := TaskOptions{
		TaskID:         "external_service_task",
		Execution:      0,
		ResultsService: TestResultsServiceCedar,
	}

	for _, test := range []struct {
		name            string
		setup           func(t *testing.T)
		taskOpts        []TaskOptions
		regexFilters    []string
		expectedSamples []TaskTestResultsFailedSample
		hasErr          bool
		resultService   TestResultsService
	}{
		{
			name:          "NilTaskOptions",
			resultService: svc,
		},
		{
			name:          "NilTaskOptions",
			taskOpts:      []TaskOptions{},
			resultService: svc,
		},
		{
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts:      []TaskOptions{externalServiceTask},
			hasErr:        true,
			resultService: cedarSvc,
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
			resultService: svc,
		},
		{
			name:         "WithRegexFilter",
			taskOpts:     []TaskOptions{task0},
			regexFilters: []string{"test"},
			expectedSamples: []TaskTestResultsFailedSample{
				{
					TaskID:           task0.TaskID,
					Execution:        task0.Execution,
					TotalFailedNames: len(sample0),
				},
			},
			resultService: svc,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			samples, err := test.resultService.GetFailedTestSamples(ctx, test.taskOpts, test.regexFilters)
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
