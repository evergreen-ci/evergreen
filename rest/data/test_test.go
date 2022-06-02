package data

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestFindTestById(t *testing.T) {
	tests := []testresult.TestResult{
		testresult.TestResult{
			ID:        mgobson.ObjectIdHex("507f191e810c19729de860ea"),
			TaskID:    "task",
			Execution: 0,
			Status:    "pass",
		}, testresult.TestResult{
			ID:        mgobson.ObjectIdHex("407f191e810c19729de860ea"),
			TaskID:    "task",
			Execution: 0,
			Status:    "pass",
		}, testresult.TestResult{
			ID:        mgobson.ObjectIdHex("307f191e810c19729de860ea"),
			TaskID:    "task",
			Execution: 0,
			Status:    "pass",
		},
	}
	for _, test := range tests {
		require.NoError(t, test.Insert())
	}

	t.Run("Success", func(t *testing.T) {
		results, err := FindTestById(tests[0].ID.Hex())
		assert.NoError(t, err)
		require.NotEmpty(t, results)
		assert.Equal(t, tests[0], results[0])
	})
	t.Run("InvalidID", func(t *testing.T) {
		results, err := FindTestById("invalid")
		assert.Error(t, err)
		assert.Empty(t, results)
	})
}

func TestFindTestsByTaskId(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))
	assert.NoError(db.EnsureIndex(testresult.Collection, mongo.IndexModel{
		Keys: testresult.TestResultsIndex}))
	serviceContext := &DBTestConnector{}

	emptyTask := &task.Task{
		Id: "empty_task",
	}
	assert.NoError(emptyTask.Insert())
	foundTests, err := serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: "empty_task"})
	assert.NoError(err, "missing tests should not return a 404")
	assert.Len(foundTests, 0)

	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	numTests := 10
	numTasks := 2
	testObjects := make([]string, numTests)

	for ix := range testObjects {
		testObjects[ix] = fmt.Sprintf("TestSuite/TestNum%d", ix)
	}
	last := len(testObjects) - 1
	sort.StringSlice(testObjects).Sort()
	for i := 0; i < numTasks; i++ {
		id := fmt.Sprintf("task_%d", i)
		testTask := &task.Task{
			Id: id,
		}
		tests := make([]testresult.TestResult, numTests)
		for j := 0; j < numTests; j++ {
			status := "pass"
			if j%2 == 0 {
				status = "fail"
			}
			tests[j] = testresult.TestResult{
				TaskID:    id,
				Execution: 0,
				Status:    status,
				TestFile:  testObjects[j],
				EndTime:   float64(j),
				StartTime: 0,
				GroupID:   fmt.Sprintf("group_%d", i),
			}
		}
		assert.NoError(testTask.Insert())
		for _, test := range tests {
			assert.NoError(test.Insert())
		}
	}
	task := &task.Task{
		Id: "task_2",
	}
	assert.NoError(task.Insert())
	testID := mgobson.ObjectIdHex("5949645c9acd9604fdd202d9")
	testResult := testresult.TestResult{ID: testID, TaskID: "task_2"}
	assert.NoError(testResult.Insert())

	for i := 0; i < numTasks; i++ {
		taskId := fmt.Sprintf("task_%d", i)
		foundTests, err := serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "test_file", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, numTests)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{"pass"}, SortBy: "test_file", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, numTests/2)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{"fail"}, SortBy: "test_file", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, numTests/2)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "TestSuite/TestNum1", Statuses: []string{}, SortBy: "test_file", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, 1)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "TestSuite/TestNum2", Statuses: []string{}, SortBy: "test_file", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, 1)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "test_file", GroupID: "", SortDir: 1, Page: 0, Limit: 5, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, 5)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "test_file", GroupID: "", SortDir: 1, Page: 1, Limit: 5, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, 5)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "test_file", GroupID: "", SortDir: 1, Page: 2, Limit: 5, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, 0)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "test_file", GroupID: "", SortDir: -1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)

		for i := range foundTests {
			assert.True(foundTests[i].TestFile == testObjects[last-i])
		}
		assert.Len(foundTests, 10)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "duration", GroupID: "", SortDir: -1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		for i, test := range foundTests {
			assert.True(test.EndTime == float64(last-i))
		}

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "duration", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		for i, test := range foundTests {
			assert.True(test.EndTime == float64(i))
		}

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{"pa"}, SortBy: "duration", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, 0)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{"not_a_real_status"}, SortBy: "duration", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, 0)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "fail", Statuses: []string{"pass"}, SortBy: "duration", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, 0)

		foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{"pass", "fail"}, SortBy: "duration", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
		assert.NoError(err)
		assert.Len(foundTests, 10)
	}
	foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TestID: string(testID), TaskID: "task_2"})
	assert.NoError(err)
	assert.Len(foundTests, 1)

	foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: "fake_task", TestName: "", Statuses: []string{}, SortBy: "duration", GroupID: "", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
	assert.NoError(err)
	assert.Len(foundTests, 0)

	foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: "task_0", TestName: "", Statuses: []string{"pass", "fail"}, SortBy: "duration", GroupID: "group_0", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
	assert.NoError(err)
	assert.Len(foundTests, 10)

	foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: "task_0", TestName: "", Statuses: []string{"pass", "fail"}, SortBy: "duration", GroupID: "unreal-group-id", SortDir: 1, Page: 0, Limit: 0, Execution: 0})
	assert.NoError(err)
	assert.Len(foundTests, 0)
}

func TestFindTestsByTaskIdPaginationOrderDependsOnObjectId(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	serviceContext := &DBTestConnector{}

	taskId := "TaskOne"
	Task := &task.Task{
		Id: taskId,
	}
	idOne := mgobson.ObjectIdHex("507f191e810c19729de860ea")
	idTwo := mgobson.ObjectIdHex("407f191e810c19729de860ea")
	idThree := mgobson.ObjectIdHex("307f191e810c19729de860ea")
	tests := []testresult.TestResult{
		testresult.TestResult{
			ID:        idOne,
			TaskID:    taskId,
			Execution: 0,
			Status:    "pass",
		}, testresult.TestResult{
			ID:        idTwo,
			TaskID:    taskId,
			Execution: 0,
			Status:    "pass",
		}, testresult.TestResult{
			ID:        idThree,
			TaskID:    taskId,
			Execution: 0,
			Status:    "pass",
		},
	}

	assert.NoError(Task.Insert())
	for _, test := range tests {
		assert.NoError(test.Insert())
	}

	foundTests, err := serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "status", GroupID: "", SortDir: 1, Page: 0, Limit: 1, Execution: 0})
	assert.NoError(err)
	assert.Len(foundTests, 1)
	assert.True(foundTests[0].ID == idThree)

	foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "status", GroupID: "", SortDir: 1, Page: 1, Limit: 1, Execution: 0})
	assert.NoError(err)
	assert.Len(foundTests, 1)
	assert.True(foundTests[0].ID == idTwo)

	foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: taskId, TestName: "", Statuses: []string{}, SortBy: "status", GroupID: "", SortDir: 1, Page: 2, Limit: 1, Execution: 0})
	assert.NoError(err)
	assert.Len(foundTests, 1)
	assert.True(foundTests[0].ID == idOne)
}

func TestFindTestsByDisplayTaskId(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	serviceContext := &DBTestConnector{}
	numTests := 10
	numTasks := 2
	testObjects := make([]string, numTests)
	for ix := range testObjects {
		testObjects[ix] = fmt.Sprintf("object_id_%d_", ix)
	}
	sort.StringSlice(testObjects).Sort()

	for i := 0; i < numTasks; i++ {
		id := fmt.Sprintf("task_%d", i)
		testTask := &task.Task{
			Id: id,
		}
		tests := make([]testresult.TestResult, numTests)
		for j := 0; j < numTests; j++ {
			status := "pass"
			if j%2 == 0 {
				status = "fail"
			}
			tests[j] = testresult.TestResult{
				TaskID:    id,
				Execution: 0,
				Status:    status,
				TestFile:  testObjects[j],
			}
		}
		assert.NoError(testTask.Insert())
		for _, test := range tests {
			assert.NoError(test.Insert())
		}
	}

	displayTaskWithTasks := &task.Task{
		Id:             "with_tasks",
		DisplayOnly:    true,
		ExecutionTasks: []string{"task_0", "task_1", "does_not_exist"},
	}
	assert.NoError(displayTaskWithTasks.Insert())
	displayTaskWithoutTasks := &task.Task{
		Id:             "without_tasks",
		DisplayOnly:    true,
		ExecutionTasks: []string{},
	}
	assert.NoError(displayTaskWithoutTasks.Insert())
	foundTests, err := serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: "with_tasks", ExecutionTasks: displayTaskWithTasks.ExecutionTasks})
	assert.NoError(err)
	assert.Len(foundTests, 20)

	foundTests, err = serviceContext.FindTestsByTaskId(FindTestsByTaskIdOpts{TaskID: "without_tasks", ExecutionTasks: displayTaskWithoutTasks.ExecutionTasks})
	assert.NoError(err)
	assert.Len(foundTests, 0)
}

func TestCountTestsByTaskID(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, testresult.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, testresult.Collection))
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("CedarTestResults", func(t *testing.T) {
		regularTask := &task.Task{
			Id:              "cedar",
			HasCedarResults: true,
		}
		require.NoError(t, regularTask.Insert())
		displayTask := &task.Task{
			Id:              "cedar_display",
			Execution:       1,
			HasCedarResults: true,
			DisplayOnly:     true,
		}
		require.NoError(t, displayTask.Insert())
		handler := &mockCedarHandler{}
		srv := httptest.NewServer(handler)
		defer srv.Close()
		evergreen.GetEnvironment().Settings().Cedar.BaseURL = srv.URL

		for _, test := range []struct {
			name       string
			task       *task.Task
			execution  int
			response   apimodels.CedarTestResultsStats
			statusCode int
			hasErr     bool
		}{
			{
				name: "TaskDNE",
				task: &task.Task{
					Id:              "DNE",
					HasCedarResults: true,
				},
				statusCode: http.StatusOK,
				hasErr:     true,
			},
			{
				name:       "TaskExecutionDNE",
				task:       regularTask,
				execution:  100,
				statusCode: http.StatusOK,
				hasErr:     true,
			},
			{
				name:       "CedarError",
				task:       regularTask,
				execution:  regularTask.Execution,
				statusCode: http.StatusInternalServerError,
				hasErr:     true,
			},
			{
				name:       "TaskWithoutResults",
				task:       regularTask,
				execution:  regularTask.Execution,
				statusCode: http.StatusNotFound,
			},
			{
				name:       "TaskWithResults",
				task:       regularTask,
				execution:  regularTask.Execution,
				response:   apimodels.CedarTestResultsStats{TotalCount: 100},
				statusCode: http.StatusOK,
			},
			{
				name:       "DisplayTask",
				task:       displayTask,
				execution:  displayTask.Execution,
				response:   apimodels.CedarTestResultsStats{TotalCount: 100},
				statusCode: http.StatusOK,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				responseData, err := json.Marshal(&test.response)
				require.NoError(t, err)
				handler.Response = responseData
				handler.StatusCode = test.statusCode

				count, err := CountTestsByTaskID(ctx, test.task.Id, test.execution)
				if test.hasErr {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)

					vals := handler.LastRequest.URL.Query()
					var displayStr string
					if test.task.DisplayOnly {
						displayStr = "true"
					}
					assert.Equal(t, strconv.Itoa(test.task.Execution), vals.Get("execution"))
					assert.Equal(t, displayStr, vals.Get("display_task"))
					assert.Equal(t, test.response.TotalCount, count)
				}
			})
		}
	})
	t.Run("DBTestResults", func(t *testing.T) {
		t0 := &task.Task{Id: "t0"}
		require.NoError(t, t0.Insert())
		t1 := &task.Task{Id: "t1"}
		require.NoError(t, t1.Insert())
		t2 := &task.Task{
			Id:             "t2",
			DisplayOnly:    true,
			ExecutionTasks: []string{t0.Id, t1.Id},
		}
		require.NoError(t, t2.Insert())

		numTests := 10
		for i := 0; i < numTests; i++ {
			tr := &testresult.TestResult{TaskID: t0.Id}
			require.NoError(t, tr.Insert())
			tr = &testresult.TestResult{TaskID: t1.Id}
			require.NoError(t, tr.Insert())
		}

		t.Run("ExecutionTask", func(t *testing.T) {
			count, err := CountTestsByTaskID(ctx, t0.Id, 0)
			require.NoError(t, err)
			assert.Equal(t, numTests, count)
		})
		t.Run("DisplayTask", func(t *testing.T) {
			count, err := CountTestsByTaskID(ctx, t2.Id, 0)
			require.NoError(t, err)
			assert.Equal(t, len(t2.ExecutionTasks)*numTests, count)
		})
	})
}

type mockCedarHandler struct {
	Response    []byte
	StatusCode  int
	LastRequest *http.Request
}

func (h *mockCedarHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.LastRequest = r
	w.WriteHeader(h.StatusCode)
	_, _ = w.Write(h.Response)
}
