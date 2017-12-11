package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest"
)

// DBTestConnector is a struct that implements the Test related methods
// from the Connector through interactions with the backing database.
type DBTestConnector struct{}

func (tc *DBTestConnector) FindTestsByTaskId(taskId, filename, status string, limit,
	sort, execution int) ([]testresult.TestResult, error) {

	pipeline := testresult.TestResultsPipeline(taskId, filename, status, limit, sort, execution)
	res := []testresult.TestResult{}

	err := testresult.Aggregate(pipeline, &res)
	if err != nil {
		return []testresult.TestResult{}, err
	}
	if len(res) == 0 {
		var message string
		if status != "" {
			message = fmt.Sprintf("tests for task with taskId '%s' and status '%s' not found", taskId, status)
		} else {
			message = fmt.Sprintf("tests for task with taskId '%s' not found", taskId)
		}
		return []testresult.TestResult{}, &rest.APIError{
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

func (mtc *MockTestConnector) FindTestsByTaskId(taskId, testFilename, status string, limit,
	sort, execution int) ([]testresult.TestResult, error) {
	if mtc.StoredError != nil {
		return []testresult.TestResult{}, mtc.StoredError
	}

	// loop until the filename is found
	for ix, t := range mtc.CachedTests {
		if t.TestFile == testFilename {
			// We've found the test
			var testsToReturn []testresult.TestResult
			if sort < 0 {
				if ix-limit > 0 {
					testsToReturn = mtc.CachedTests[ix-(limit) : ix]
				} else {
					testsToReturn = mtc.CachedTests[:ix]
				}
			} else {
				if ix+limit > len(mtc.CachedTests) {
					testsToReturn = mtc.CachedTests[ix:]
				} else {
					testsToReturn = mtc.CachedTests[ix : ix+limit]
				}
			}
			return testsToReturn, nil
		}
	}
	return nil, nil
}
