package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/gimlet"
)

// DBTestConnector is a struct that implements the Test related methods
// from the Connector through interactions with the backing database.
type DBTestConnector struct{}

func (tc *DBTestConnector) FindTestsByTaskId(taskId, testId, status string, limit, execution int) ([]testresult.TestResult, error) {

	q := testresult.TestResultsQuery(taskId, testId, status, limit, execution)
	res, err := testresult.Find(q)
	if err != nil {
		return []testresult.TestResult{}, err
	}
	if len(res) == 0 {
		var message string
		if status != "" {
			message = fmt.Sprintf("tests for task with taskId '%s', execution %d, and status '%s' not found", taskId, execution, status)
		} else {
			message = fmt.Sprintf("tests for task with taskId '%s' and execution %d not found", taskId, execution)
		}
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

func (mtc *MockTestConnector) FindTestsByTaskId(taskId, testId, status string, limit, execution int) ([]testresult.TestResult, error) {
	if mtc.StoredError != nil {
		return []testresult.TestResult{}, mtc.StoredError
	}

	// loop until the testId is found
	for ix, t := range mtc.CachedTests {
		if string(t.ID) == testId {
			// We've found the test
			var testsToReturn []testresult.TestResult
			if ix+limit > len(mtc.CachedTests) {
				testsToReturn = mtc.CachedTests[ix:]
			} else {
				testsToReturn = mtc.CachedTests[ix : ix+limit]
			}
			return testsToReturn, nil
		}
	}
	return nil, nil
}
