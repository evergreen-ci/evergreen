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
	"github.com/stretchr/testify/require"
	mgobson "gopkg.in/mgo.v2/bson"
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

	sc := &DBConnector{}
	t.Run("Success", func(t *testing.T) {
		results, err := sc.FindTestById(tests[0].ID.Hex())
		assert.NoError(t, err)
		require.NotEmpty(t, results)
		assert.Equal(t, tests[0], results[0])
	})
	t.Run("InvalidID", func(t *testing.T) {
		results, err := sc.FindTestById("invalid")
		assert.Error(t, err)
		assert.Empty(t, results)
	})
}

func TestFindTestsByTaskId(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	serviceContext := &DBConnector{}

	emptyTask := &task.Task{
		Id: "empty_task",
	}
	assert.NoError(emptyTask.Insert())
	foundTests, err := serviceContext.FindTestsByTaskId("empty_task", "", "", "", 0, 0)
	assert.NoError(err, "missing tests should not return a 404")
	assert.Len(foundTests, 0)

	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))

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
		foundTests, err = serviceContext.FindTestsByTaskId(taskId, "", "", "", 0, 0)
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
			foundTests, err = serviceContext.FindTestsByTaskId(fmt.Sprintf("task_%d", i), "", "", status, 0, 0)
			assert.NoError(err)
			assert.Equal(numTests/2, len(foundTests))
			for _, t := range foundTests {
				assert.Equal(status, t.Status)
			}
		}
	}

	foundTests, err = serviceContext.FindTestsByTaskId("fake_task", "", "", "", 0, 0)
	assert.Error(err)
	assert.Len(foundTests, 0)
	apiErr, ok := err.(gimlet.ErrorResponse)
	assert.True(ok)
	assert.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func TestGetTestCountByTaskIdAndFilter(t *testing.T) {
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
				EndTime:   float64(j),
				StartTime: 0,
			}
		}
		assert.NoError(testTask.Insert())
		for _, test := range tests {
			assert.NoError(test.Insert())
		}
	}

	for i := 0; i < numTasks; i++ {
		taskId := fmt.Sprintf("task_%d", i)
		count, err := serviceContext.GetTestCountByTaskIdAndFilters(taskId, "", []string{}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "", []string{"pass"}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests/2)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "", []string{"fail"}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests/2)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "", []string{"pass", "fail"}, 0)
		assert.NoError(err)
		assert.Equal(count, 10)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestNum1", []string{}, 0)
		assert.NoError(err)
		assert.Equal(count, 1)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestNum2", []string{}, 0)
		assert.NoError(err)
		assert.Equal(count, 1)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestN", []string{}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestN", []string{"pass", "fail"}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestN", []string{"pass"}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests/2)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "", []string{"pa"}, 0)
		assert.NoError(err)
		assert.Equal(count, 0)

		count, err = serviceContext.GetTestCountByTaskIdAndFilters(taskId, "", []string{"not_a_real_status"}, 0)
		assert.NoError(err)
		assert.Equal(count, 0)
	}
	count, err := serviceContext.GetTestCountByTaskIdAndFilters("fake_task", "", []string{}, 0)
	assert.Error(err)
	assert.Equal(count, 0)
}

func TestFindTestsByTaskIdFilterSortPaginate(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	serviceContext := &DBConnector{}
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

	for i := 0; i < numTasks; i++ {
		taskId := fmt.Sprintf("task_%d", i)
		foundTests, err := serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "test_file", "", 1, 0, 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, numTests)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{"pass"}, "test_file", "", 1, 0, 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, numTests/2)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{"fail"}, "test_file", "", 1, 0, 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, numTests/2)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "TestSuite/TestNum1", []string{}, "test_file", "", 1, 0, 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, 1)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "TestSuite/TestNum2", []string{}, "test_file", "", 1, 0, 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, 1)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "test_file", "", 1, 0, 5, 0)
		assert.NoError(err)
		assert.Len(foundTests, 5)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "test_file", "", 1, 1, 5, 0)
		assert.NoError(err)
		assert.Len(foundTests, 5)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "test_file", "", 1, 2, 5, 0)
		assert.NoError(err)
		assert.Len(foundTests, 0)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "test_file", "", -1, 0, 0, 0)
		assert.NoError(err)

		for i := range foundTests {
			assert.True(foundTests[i].TestFile == testObjects[last-i])
		}
		assert.Len(foundTests, 10)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "duration", "", -1, 0, 0, 0)
		assert.NoError(err)
		for i, test := range foundTests {
			assert.True(test.EndTime == float64(last-i))
		}

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "duration", "", 1, 0, 0, 0)
		assert.NoError(err)
		for i, test := range foundTests {
			assert.True(test.EndTime == float64(i))
		}

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{"pa"}, "duration", "", 1, 0, 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, 0)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{"not_a_real_status"}, "duration", "", 1, 0, 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, 0)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "fail", []string{"pass"}, "duration", "", 1, 0, 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, 0)

		foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{"pass", "fail"}, "duration", "", 1, 0, 0, 0)
		assert.NoError(err)
		assert.Len(foundTests, 10)
	}
	foundTests, err := serviceContext.FindTestsByTaskIdFilterSortPaginate("fake_task", "", []string{}, "duration", "", 1, 0, 0, 0)
	assert.Error(err)
	assert.Len(foundTests, 0)
	apiErr, ok := err.(gimlet.ErrorResponse)
	assert.True(ok)
	assert.Equal(http.StatusNotFound, apiErr.StatusCode)

	foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate("task_0", "", []string{"pass", "fail"}, "duration", "group_0", 1, 0, 0, 0)
	assert.NoError(err)
	assert.Len(foundTests, 10)

	foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate("task_0", "", []string{"pass", "fail"}, "duration", "unreal-group-id", 1, 0, 0, 0)
	assert.NoError(err)
	assert.Len(foundTests, 0)
}
func TestFindTestsByTaskIdFilterSortPaginatePaginationOrderDependsOnObjectId(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	serviceContext := &DBConnector{}

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

	foundTests, err := serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "status", "", 1, 0, 1, 0)
	assert.NoError(err)
	assert.Len(foundTests, 1)
	assert.True(foundTests[0].ID == idThree)

	foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "status", "", 1, 1, 1, 0)
	assert.NoError(err)
	assert.Len(foundTests, 1)
	assert.True(foundTests[0].ID == idTwo)

	foundTests, err = serviceContext.FindTestsByTaskIdFilterSortPaginate(taskId, "", []string{}, "status", "", 1, 2, 1, 0)
	assert.NoError(err)
	assert.Len(foundTests, 1)
	assert.True(foundTests[0].ID == idOne)
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
