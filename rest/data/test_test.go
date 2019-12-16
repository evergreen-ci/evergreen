package data

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
)

func TestFindTestsByTaskId(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	serviceContext := &DBConnector{}
	numTests := 10
	numTasks := 2
	testObjects := make([]string, numTests)
	for ix := range testObjects {
		testObjects[ix] = fmt.Sprintf("TestSuite/TestNum%d", ix)
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

	for i := 0; i < numTasks; i++ {
		taskId := fmt.Sprintf("task_%d", i)
		foundTests, err := serviceContext.FindTestsByTaskId(taskId, "", "", "", 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, numTests)

		foundTests, err = serviceContext.FindTestsByTaskId(taskId, "", "", "", i+1, 0)
		assert.NoError(err)
		assert.Len(foundTests, i+1)

		for _, testName := range testObjects {
			foundTests, err = serviceContext.FindTestsByTaskId(taskId, "", testName, "", 0, 0)
			assert.NoError(err)
			assert.Len(foundTests, 1)
		}

		for _, status := range []string{"pass", "fail"} {
			foundTests, err := serviceContext.FindTestsByTaskId(fmt.Sprintf("task_%d", i), "", "", status, 0, 0)
			assert.NoError(err)
			assert.Equal(numTests/2, len(foundTests))
			for _, t := range foundTests {
				assert.Equal(status, t.Status)
			}
		}
	}

	foundTests, err := serviceContext.FindTestsByTaskId("fake_task", "", "", "", 0, 0)
	assert.Error(err)
	assert.Len(foundTests, 0)
	apiErr, ok := err.(gimlet.ErrorResponse)
	assert.True(ok)
	assert.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func TestFindTestsByDisplayTaskId(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	serviceContext := &DBConnector{}
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
		Id:             "no_tasks",
		DisplayOnly:    true,
		ExecutionTasks: []string{},
	}
	assert.NoError(displayTaskWithoutTasks.Insert())
	foundTests, err := serviceContext.FindTestsByTaskId("with_tasks", "", "", "", 0, 0)
	assert.NoError(err)
	assert.Len(foundTests, 20)
	foundTests, err = serviceContext.FindTestsByTaskId("without_tasks", "", "", "", 0, 0)
	assert.Error(err)
	assert.Len(foundTests, 0)
}
