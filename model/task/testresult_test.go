package task

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/sometimes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskTestResults(t *testing.T) {
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
	task0 := Task{
		Id:             "task0",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
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
	require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: savedResults0}))

	task1 := Task{
		Id:             "task1",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
		TaskOutputInfo: &output,
	}
	savedResults1 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := getTestResult()
		result.TaskID = task1.Id
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: savedResults1}))

	task2 := Task{
		Id:             "task2",
		Execution:      1,
		ResultsService: TestResultsServiceLocal,
		TaskOutputInfo: &output,
	}
	savedResults2 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults2); i++ {
		result := getTestResult()
		result.TaskID = task2.Id
		result.Execution = task2.Execution
		savedResults2[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: savedResults2}))

	externalServiceTask := Task{
		Id:             "external_service_task",
		Execution:      1,
		ResultsService: TestResultsServiceCedar,
		TaskOutputInfo: &outputCedar,
	}
	externalServiceResults := make([]testresult.TestResult, 10)
	for i := 0; i < len(externalServiceResults); i++ {
		result := getTestResult()
		result.TaskID = externalServiceTask.Id
		result.Execution = externalServiceTask.Execution
		externalServiceResults[i] = result
	}

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
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts: []Task{externalServiceTask},
			hasErr:   true,
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

	require.NoError(t, ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, ClearLocal(ctx, env))
	}()
	srv, handler := newMockCedarServer(env)
	defer srv.Close()
	svc := NewLocalService(env)
	task0 := Task{
		Id:             "task0",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
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
	require.NoError(t, db.Insert(ctx, testresult.Collection, testresult.DbTaskTestResults{
		ID:   info.ID(),
		Info: info,
	}))
	require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: savedResults0}))

	task1 := Task{
		Id:             "task1",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
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
	require.NoError(t, db.Insert(ctx, testresult.Collection, testresult.DbTaskTestResults{
		ID:   info.ID(),
		Info: info,
	}))
	require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: savedResults1}))

	externalServiceTask := Task{
		Id:             "external_service_task",
		Execution:      0,
		ResultsService: TestResultsServiceCedar,
		TaskOutputInfo: &outputCedar,
	}

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
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts: []Task{externalServiceTask},
			hasErr:   true,
			output:   outputCedar,
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
	env.Settings().Cedar.BaseURL = strings.TrimPrefix(srv.URL, "http://")
	env.Settings().Cedar.Insecure = true

	return srv, handler
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
	task0 := Task{
		Id:             "task0",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
	}
	sample0 := make([]string, 2)
	for i := 0; i < len(sample0); i++ {
		result := getTestResult()
		result.TaskID = task0.Id
		result.Execution = task0.Execution
		result.Status = evergreen.TestFailedStatus
		sample0[i] = result.GetDisplayTestName()
		require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: []testresult.TestResult{result}}))
	}

	task1 := Task{
		Id:             "task1",
		Execution:      1,
		ResultsService: TestResultsServiceLocal,
	}
	sample1 := make([]string, 2)
	for i := 0; i < len(sample1); i++ {
		result := getTestResult()
		result.TaskID = task1.Id
		result.Execution = task1.Execution
		result.Status = evergreen.TestFailedStatus
		sample1[i] = result.GetDisplayTestName()
		require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: []testresult.TestResult{result}}))
	}

	externalServiceTask := Task{
		Id:             "external_service_task",
		Execution:      0,
		ResultsService: TestResultsServiceCedar,
		TaskOutputInfo: &outputCedar,
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
			name: "ServiceError",
			setup: func(_ *testing.T) {
				handler.status = http.StatusInternalServerError
				handler.data = nil
			},
			taskOpts: []Task{externalServiceTask},
			hasErr:   true,
		},
		{
			name:     "SameService",
			taskOpts: []Task{task0, task1},
			expectedSamples: []testresult.TaskTestResultsFailedSample{
				{
					TaskID:                  task0.Id,
					Execution:               task0.Execution,
					MatchingFailedTestNames: sample0,
					TotalFailedNames:        len(sample0),
				},
				{
					TaskID:                  task1.Id,
					Execution:               task1.Execution,
					MatchingFailedTestNames: sample1,
					TotalFailedNames:        len(sample1),
				},
			},
		},
		{
			name:         "WithRegexFilter",
			taskOpts:     []Task{task0},
			regexFilters: []string{"test"},
			expectedSamples: []testresult.TaskTestResultsFailedSample{
				{
					TaskID:           task0.Id,
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

	require.NoError(t, ClearLocal(ctx, env))
	defer func() {
		assert.NoError(t, ClearLocal(ctx, env))
	}()
	svc := NewLocalService(env)
	task0 := Task{
		Id:             "task0",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
	}
	savedResults0 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults0); i++ {
		result := generateTestResult()
		result.TaskID = task0.Id
		result.Execution = task0.Execution
		if i%2 != 0 {
			result.Status = evergreen.TestFailedStatus
		}
		savedResults0[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: savedResults0}))

	task1 := Task{
		Id:             "task1",
		Execution:      0,
		ResultsService: TestResultsServiceLocal,
	}
	savedResults1 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults1); i++ {
		result := generateTestResult()
		result.TaskID = task1.Id
		result.Execution = task1.Execution
		savedResults1[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: savedResults1}))

	task2 := Task{
		Id:             "task2",
		Execution:      1,
		ResultsService: TestResultsServiceLocal,
	}
	savedResults2 := make([]testresult.TestResult, 10)
	for i := 0; i < len(savedResults2); i++ {
		result := generateTestResult()
		result.TaskID = task2.Id
		result.Execution = task2.Execution
		savedResults2[i] = result
	}
	require.NoError(t, svc.AppendTestResults(ctx, testresult.DbTaskTestResults{Results: savedResults2}))

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

func generateTestResult() testresult.TestResult {
	result := testresult.TestResult{
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
