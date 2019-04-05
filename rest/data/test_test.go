package data

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
)

func TestFindTestsByTaskId(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindTestsByTaskId")
	
	assert := assert.New(t)
	assert.NoError(db.Clear(task.Collection))

	serviceContext := &DBConnector{}
	numTests := 10
	numTasks := 2
	testObjects := make([]string, numTests)
	for ix := range testObjects {
		testObjects[ix] = fmt.Sprintf("object_id_%d_", ix)
	}
	sort.StringSlice(testObjects).Sort()

	testutil.HandleTestingErr(db.Clear(task.Collection), t, "Error clearing"+
		" '%v' collection", task.Collection)
	testutil.HandleTestingErr(db.Clear(testresult.Collection), t, "Error clearing"+
		" '%v' collection", testresult.Collection)
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

	for i := 0; i < numTasks; i++ {
		foundTests, err := serviceContext.FindTestsByTaskId(fmt.Sprintf("task_%d", i), "", "", 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, numTests)
	}
	for _, status := range []string{"pass", "fail"} {
		for i := 0; i < numTasks; i++ {
			foundTests, err := serviceContext.FindTestsByTaskId(fmt.Sprintf("task_%d", i), "", status, 0, 0)
			assert.NoError(err)
			assert.Equal(numTests/2, len(foundTests))
			for _, t := range foundTests {
				assert.Equal(status, t.Status)
			}
		}
	}

	taskId := "task_1"
	for i := 0; i < numTests; i++ {
		foundTests, err := serviceContext.FindTestsByTaskId(taskId, "", "", 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, numTests)
	}
	taskname := "task_0"
	limit := 2
	for i := 0; i < numTests/limit; i++ {
		foundTests, err := serviceContext.FindTestsByTaskId(taskname, "", "", limit, 0)
		assert.NoError(err)
		assert.Len(foundTests, limit)
	}

	foundTests, err := serviceContext.FindTestsByTaskId("fake_task", "", "", 0, 0)
	assert.Error(err)
	assert.Len(foundTests, 0)
	apiErr, ok := err.(gimlet.ErrorResponse)
	assert.True(ok)
	assert.Equal(http.StatusNotFound, apiErr.StatusCode)

	taskname = "task_0"
	foundTests, err = serviceContext.FindTestsByTaskId(taskname, "", "", 1, 0)
	assert.NoError(err)
	assert.Len(foundTests, 1)
	test1 := foundTests[0]
	assert.Equal(testObjects[0], test1.TestFile)
}

func TestFindTestsByDisplayTaskId(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindTestsByTaskId")
	
	assert := assert.New(t)
	assert.NoError(db.Clear(task.Collection))

	serviceContext := &DBConnector{}
	numTests := 10
	numTasks := 2
	testObjects := make([]string, numTests)
	for ix := range testObjects {
		testObjects[ix] = fmt.Sprintf("object_id_%d_", ix)
	}
	sort.StringSlice(testObjects).Sort()

	testutil.HandleTestingErr(db.Clear(task.Collection), t, "Error clearing"+
		" '%v' collection", task.Collection)
	testutil.HandleTestingErr(db.Clear(testresult.Collection), t, "Error clearing"+
		" '%v' collection", testresult.Collection)
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
		Id:             "no_tasks",
		DisplayOnly:    true,
		ExecutionTasks: []string{},
	}
	assert.NoError(displayTaskWithoutTasks.Insert())
	foundTests, err := serviceContext.FindTestsByTaskId("with_tasks", "", "", 0, 0)
	assert.NoError(err)
	assert.Len(foundTests, 20)
	foundTests, err = serviceContext.FindTestsByTaskId("without_tasks", "", "", 0, 0)
	assert.Error(err)
	assert.Len(foundTests, 0)
}
