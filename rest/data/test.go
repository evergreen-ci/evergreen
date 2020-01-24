package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/gimlet"
)

// DBTestConnector is a struct that implements the Test related methods
// from the Connector through interactions with the backing database.
type DBTestConnector struct{}

func (tc *DBTestConnector) FindTestsByTaskId(taskId, testId, testName, status string, limit, execution int) ([]testresult.TestResult, error) {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", err.Error()),
		}
	}
	if t == nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", taskId),
		}
	}
	var taskIds []string
	if t.DisplayOnly {
		taskIds = t.ExecutionTasks
	} else {
		taskIds = []string{taskId}
	}
	q := testresult.TestResultsQuery(taskIds, testId, testName, status, limit, execution)
	res, err := testresult.Find(q)
	if err != nil {
		return []testresult.TestResult{}, err
	}
	if len(res) == 0 {
		var message string
		testMsg := "tests"
		if testName != "" {
			testMsg = fmt.Sprintf("test '%s'", testName)
		}
		if status != "" {
			message = fmt.Sprintf("%s for task with taskId '%s', execution %d, and status '%s' not found", testMsg, taskId, execution, status)
		} else {
			message = fmt.Sprintf("%s for task with taskId '%s' and execution %d not found", testMsg, taskId, execution)
		}
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    message,
		}
	}

	return res, nil
}

func (tc *DBTestConnector) FindTestsByTaskIdSortAndPaginate(taskId, filter, sortBy string, sortDir, page, limit, execution int) ([]testresult.TestResult, error) {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", err.Error()),
		}
	}
	if t == nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", taskId),
		}
	}
	var taskIds []string
	if t.DisplayOnly {
		taskIds = t.ExecutionTasks
	} else {
		taskIds = []string{taskId}
	}

	res, err := testresult.TestResultsAggregationSortAndPaginate(taskIds, filter, sortBy, sortDir, page, limit, execution)
	if err != nil {
		return []testresult.TestResult{}, err
	}
	if len(res) == 0 {
		var message string
		message = fmt.Sprintf("tests for task with taskId '%s' and execution %d not found on page %d with limit %d	", taskId, execution, page, limit)
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    message,
		}
	}

	return res, nil
}

// MockTaskConnector stores a cached set of tests that are queried against by the
// implementations of the Connector interface's Test related functions.
type MockTestConnector struct {
	CachedTests []testresult.TestResult
	StoredError error
}

func (mtc *MockTestConnector) FindTestsByTaskId(taskId, testId, testName, status string, limit, execution int) ([]testresult.TestResult, error) {
	if mtc.StoredError != nil {
		return []testresult.TestResult{}, mtc.StoredError
	}

	// loop until the testId is found
	for ix, t := range mtc.CachedTests {
		if string(t.ID) == testId { // We've found the test to start from
			if testName == "" {
				return mtc.findAllTestsFromIx(ix, limit), nil
			}
			return mtc.findTestsByNameFromIx(testName, ix, limit), nil
		}
	}
	return nil, nil
}

func (tc *MockTestConnector) FindTestsByTaskIdSortAndPaginate(taskId, filter, sortBy string, sortDir, page, limit, execution int) ([]testresult.TestResult, error) {
	//todo
	return nil, nil
}

func (mtc *MockTestConnector) findAllTestsFromIx(ix, limit int) []testresult.TestResult {
	if ix+limit > len(mtc.CachedTests) {
		return mtc.CachedTests[ix:]
	}
	return mtc.CachedTests[ix : ix+limit]
}

func (mtc *MockTestConnector) findTestsByNameFromIx(name string, ix, limit int) []testresult.TestResult {
	possibleTests := mtc.CachedTests[ix:]
	testResults := []testresult.TestResult{}
	for jx, t := range possibleTests {
		if t.TestFile == name {
			testResults = append(testResults, possibleTests[jx])
		}
		if len(testResults) == limit {
			return testResults
		}
	}
	return testResults
}
